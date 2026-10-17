package eval

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jbrinkman/kairon/internal/checks"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

const (
	// workspaceRootEnv relocates per-case workspaces. A user's TMPDIR may not
	// be a path the container runtime can bind-mount (Docker Desktop, Podman
	// machine), so it can be pointed at a shared location.
	workspaceRootEnv = "KAIRON_EVAL_WORKSPACE_ROOT"

	workspaceDirPrefix = "kairon-eval-ws-"
	evalOutputDirName  = ".eval"
	kiroDirName        = ".kiro"
	binDirName         = "bin"

	// Files in .eval/ the fake gh reads (see sandbox.RenderGHIssue).
	ghIssueJSONName = "gh-issue.json"
	ghIssueTextName = "gh-issue.txt"

	// fixedGitDate keeps the fixture commit reproducible.
	fixedGitDate = "2000-01-01T00:00:00Z"
)

// workspaceNameRe is the allowed shape of a case's workspace fixture name.
var workspaceNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// mockCommandRe is the allowed shape of a mocked command name: a bare file
// name that cannot contain a path separator or start with '.' or '-'.
var mockCommandRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// reservedMockCommands names commands that may not be mocked, each with the
// reason reported to the eval author. "gh" shadows the harness-owned fake gh and "kairon" the
// harness-mounted kairon binary (the mock dir is first on PATH).
// The remaining names are POSIX helpers the mock shim itself invokes via PATH
// (see fixtures/mock-cli.sh); because the staged mock directory is first on
// PATH, mocking one of them would make the shim re-enter itself (recursion or
// hang) and would also be hit by any sibling mock in the same case.
var reservedMockCommands = map[string]string{
	"gh":       "reserved for the harness's fake gh",
	"kairon":   "reserved for the harness's kairon binary",
	"basename": "used internally by the mock shim and cannot be mocked",
	"dirname":  "used internally by the mock shim and cannot be mocked",
	"cat":      "used internally by the mock shim and cannot be mocked",
	"head":     "used internally by the mock shim and cannot be mocked",
	"tr":       "used internally by the mock shim and cannot be mocked",
	"sed":      "used internally by the mock shim and cannot be mocked",
}

// caseWorkspace is the host-side workspace of one test case: a git repo whose
// single commit is the fixture, plus harness-owned .eval/ (outputs) and
// .kiro/ (staged agent and skill configuration).
type caseWorkspace struct {
	Dir     string // absolute, symlink-resolved workspace root (a git repo)
	EvalDir string // Dir/.eval
	KiroDir string // Dir/.kiro
	BinDir  string // <root>/bin: holds the fake gh and case mocks, mounted read-only at /opt/kairon/bin

	// BaseCommit is the sha of the single fixture commit, set by gitInit.
	BaseCommit string

	// root is the private (0700) parent that holds Dir. It keeps other local
	// users from traversing into the world-writable workspace.
	root string
}

// validateWorkspaceName checks a case's workspace fixture name. The empty name
// is valid and selects the default empty workspace.
func validateWorkspaceName(name string) error {
	if name == "" {
		return nil
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) || !workspaceNameRe.MatchString(name) {
		return fmt.Errorf("invalid workspace name %q: must match %s and not be '.' or '..'", name, workspaceNameRe)
	}
	return nil
}

// workspaceFixtureDir is the fixture directory for a validated workspace name.
func workspaceFixtureDir(name string) string {
	return evalsPath("fixtures", "workspaces", name)
}

// validateCaseFields validates the workspace and timeout fields of a loaded
// case. file is the case file name, used in error messages.
func validateCaseFields(tc TestCase, file string) error {
	if err := validateTurnFields(tc, file); err != nil {
		return err
	}
	if err := validateWorkspaceName(tc.Workspace); err != nil {
		return fmt.Errorf("case %q (%s): %w", tc.Name, file, err)
	}
	if tc.Workspace != "" {
		dir := workspaceFixtureDir(tc.Workspace)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("case %q (%s): workspace fixture %q not found at %s", tc.Name, file, tc.Workspace, dir)
		}
		// A fixture may override .kiro content (skills, *-conventions), but NOT
		// the case's own agent config: provenance (prompt_sha256, resources) is
		// computed from <evals-dir>/agents or project .kiro, never the fixture,
		// so a fixture-provided agent config would run one prompt and record
		// another. Refuse it at load; put agent config in <evals-dir>/agents/.
		if tc.Agent != "" {
			agentCfg := filepath.Join(dir, kiroDirName, "agents", tc.Agent+".json")
			if _, err := os.Stat(agentCfg); err == nil {
				return fmt.Errorf("case %q (%s): workspace fixture %q provides %s, which would not match the pinned provenance; put the agent config in <evals-dir>/agents/ instead",
					tc.Name, file, tc.Workspace, filepath.Join(kiroDirName, "agents", tc.Agent+".json"))
			}
		}
	}
	if err := validateSandboxFields(tc, file); err != nil {
		return err
	}
	if tc.Stub != nil && tc.Stub.Judge != nil {
		if len(tc.Stub.Judge) == 0 {
			return fmt.Errorf("%w: case %q (%s): stub.judge must not be empty", errMalformedCase, tc.Name, file)
		}
		for i, a := range tc.Stub.Judge {
			if strings.TrimSpace(a) == "" {
				return fmt.Errorf("%w: case %q (%s): stub.judge entry %d must not be blank", errMalformedCase, tc.Name, file, i+1)
			}
		}
	}
	if tc.Timeout != "" {
		d, err := time.ParseDuration(tc.Timeout)
		if err != nil {
			return fmt.Errorf("case %q (%s): invalid timeout %q: %w", tc.Name, file, tc.Timeout, err)
		}
		if d <= 0 {
			return fmt.Errorf("case %q (%s): invalid timeout %q: must be a positive duration", tc.Name, file, tc.Timeout)
		}
	}
	return nil
}

// validateTurnFields validates the multi-turn shape of a case. Without
// simulated_user: turns and input are mutually exclusive, a turns list is
// non-empty with no blank entries, a stub scripts exactly one entry per turn,
// and every check turn is within the number of turns (a classic input case
// has one turn). Classic cases keep their existing stub behaviour (only entry
// 0 is used), so nothing about them is rejected here beyond an out-of-range
// check turn. A simulated_user case is validated by validateSimulatedUser.
func validateTurnFields(tc TestCase, file string) error {
	if tc.SimulatedUser != nil {
		return validateSimulatedUser(tc, file)
	}
	if tc.Turns != nil {
		if strings.TrimSpace(tc.Input) != "" {
			return fmt.Errorf("case %q (%s): turns and input are mutually exclusive; use turns for a multi-turn case", tc.Name, file)
		}
		if err := validateScriptedTurns(tc, file); err != nil {
			return err
		}
		if tc.Stub != nil && len(tc.Stub.Turns) != len(tc.Turns) {
			return fmt.Errorf("case %q (%s): stub.turns has %d entries but the case has %d turns; script exactly one stub turn per turn",
				tc.Name, file, len(tc.Stub.Turns), len(tc.Turns))
		}
	}
	if tc.Stub != nil && len(tc.Stub.Routes) > 0 {
		return fmt.Errorf("case %q (%s): stub.routes requires simulated_user", tc.Name, file)
	}
	n := len(tc.userTurns())
	for i, c := range tc.Checks {
		if isFactCheck(c.Type) {
			return fmt.Errorf("case %q (%s): check %d (%s) requires simulated_user", tc.Name, file, i+1, c.Type)
		}
		if c.Turn != nil && *c.Turn > n {
			return fmt.Errorf("case %q (%s): check %d (%s): turn %d is beyond the case's %d turn(s)", tc.Name, file, i+1, c.Type, *c.Turn, n)
		}
	}
	return nil
}

// validateScriptedTurns checks that a present turns list is non-empty with no
// blank entries.
func validateScriptedTurns(tc TestCase, file string) error {
	if len(tc.Turns) == 0 {
		return fmt.Errorf("case %q (%s): turns must list at least one user message", tc.Name, file)
	}
	for i, turn := range tc.Turns {
		if strings.TrimSpace(turn) == "" {
			return fmt.Errorf("case %q (%s): turns[%d] is empty", tc.Name, file, i)
		}
	}
	return nil
}

// isFactCheck reports whether t is one of the simulated-user fact checks.
func isFactCheck(t CheckType) bool {
	return t == CheckFactAsked || t == CheckFactNotAsked
}

// validateSimulatedUser validates a case with simulated_user: the opening
// input, the optional scripted turns that follow the simulated phase, the fact
// bank, default_reply, stop_when, max_turns, the stub script and the fact and
// turn rules of the checks. It also compiles stop_when into the case.
func validateSimulatedUser(tc TestCase, file string) error {
	su := tc.SimulatedUser
	fail := func(format string, args ...any) error {
		return fmt.Errorf("case %q (%s): "+format, append([]any{tc.Name, file}, args...)...)
	}
	if strings.TrimSpace(tc.Input) == "" {
		return fail("simulated_user requires input (the opening user message)")
	}
	if tc.Turns != nil {
		if err := validateScriptedTurns(tc, file); err != nil {
			return err
		}
	}
	if len(su.Facts) == 0 {
		return fail("simulated_user.facts must list at least one fact")
	}
	ids := map[string]bool{}
	for i, f := range su.Facts {
		switch {
		case strings.TrimSpace(f.ID) == "":
			return fail("simulated_user.facts[%d].id must not be blank", i)
		case len(strings.Fields(f.ID)) != 1 || f.ID != strings.TrimSpace(f.ID):
			return fail("simulated_user.facts[%d].id %q must not contain whitespace", i, f.ID)
		case strings.EqualFold(f.ID, noFact):
			return fail("simulated_user.facts[%d].id %q is reserved (%q means no fact matched)", i, f.ID, noFact)
		case ids[f.ID]:
			return fail("simulated_user.facts[%d]: duplicate id %q", i, f.ID)
		case strings.TrimSpace(f.Answer) == "":
			return fail("simulated_user.facts[%d].answer must not be blank", i)
		case f.Discoverable == nil:
			return fail("simulated_user.facts[%d] (%s): discoverable is required (true or false)", i, f.ID)
		}
		ids[f.ID] = true
	}
	if strings.TrimSpace(su.DefaultReply) == "" {
		return fail("simulated_user.default_reply must not be blank")
	}
	if su.StopWhen != "" {
		re, err := regexp.Compile(su.StopWhen)
		if err != nil {
			return fail("simulated_user.stop_when is not a valid regular expression: %v", err)
		}
		su.stopRE = re
	}
	if su.MaxTurns != nil && *su.MaxTurns < 1 {
		return fail("simulated_user.max_turns must be at least 1, got %d", *su.MaxTurns)
	}
	maxTurns, scripted := tc.maxTurns(), len(tc.Turns)
	if tc.Stub != nil {
		lo, hi := 1+scripted, maxTurns+scripted
		if n := len(tc.Stub.Turns); n < lo || n > hi {
			return fail("stub.turns has %d entries but a simulated case with max_turns %d and %d scripted turn(s) runs between %d and %d agent turns; script between %d and %d stub turns",
				n, maxTurns, scripted, lo, hi, lo, hi)
		}
		if n := len(tc.Stub.Routes); n > maxTurns-1 {
			return fail("stub.routes has %d entries but max_turns %d answers at most %d times (the last simulated turn is never answered)", n, maxTurns, maxTurns-1)
		}
		for i, r := range tc.Stub.Routes {
			if !ids[r] && !strings.EqualFold(r, noFact) {
				return fail("stub.routes[%d] %q is neither a fact id nor %q", i, r, noFact)
			}
		}
	}
	n := maxTurns + scripted
	for i, c := range tc.Checks {
		if isFactCheck(c.Type) && c.Fact != "" && !ids[c.Fact] {
			return fail("check %d (%s): fact %q is not defined in simulated_user.facts", i+1, c.Type, c.Fact)
		}
		if c.Turn != nil && *c.Turn > n {
			return fail("check %d (%s): turn %d is beyond the case's %d turn(s) (max_turns %d + %d scripted)", i+1, c.Type, *c.Turn, n, maxTurns, scripted)
		}
	}
	return nil
}

// validateSandboxFields validates gh_issue, mocks and requires_sandbox. A gh_issue is
// only meaningful to the fake gh, which exists only under --sandbox: a native
// run would call the developer's real gh, so the case must be sandbox-only.
// gh_issue.number defaults to 1 when unset.
func validateSandboxFields(tc TestCase, file string) error {
	if tc.GHIssue != nil {
		if strings.TrimSpace(tc.GHIssue.Title) == "" {
			return fmt.Errorf("case %q (%s): gh_issue.title is required", tc.Name, file)
		}
		if tc.GHIssue.Number < 0 {
			return fmt.Errorf("case %q (%s): gh_issue.number must be positive", tc.Name, file)
		}
		if !tc.RequiresSandbox {
			return fmt.Errorf("case %q (%s): gh_issue requires requires_sandbox: true (a native run would call the real gh)", tc.Name, file)
		}
	}
	return validateMocks(tc, file)
}

// validateMocks validates the case's mocks. A mock is only placed on PATH by
// the sandbox, so a native run would silently skip it and call the real tool:
// the case must be sandbox-only.
func validateMocks(tc TestCase, file string) error {
	if len(tc.Mocks) == 0 {
		return nil
	}
	if !tc.RequiresSandbox {
		return fmt.Errorf("case %q (%s): mocks requires requires_sandbox: true (a native run would call the real tool)", tc.Name, file)
	}
	seen := map[string]bool{}
	for i, m := range tc.Mocks {
		if err := validateMockCommand(m.Command); err != nil {
			return fmt.Errorf("case %q (%s): mocks[%d]: %w", tc.Name, file, i, err)
		}
		if seen[m.Command] {
			return fmt.Errorf("case %q (%s): mocks[%d]: duplicate command %q", tc.Name, file, i, m.Command)
		}
		seen[m.Command] = true
		if err := validateMockScript(m.Script); err != nil {
			return fmt.Errorf("case %q (%s): mocks[%d] (%s): %w", tc.Name, file, i, m.Command, err)
		}
	}
	return nil
}

// validateMockCommand checks that command is a bare command name that cannot
// escape the bin directory and is not reserved (the fake gh, or a helper the
// mock shim invokes via PATH).
func validateMockCommand(command string) error {
	if command == "" {
		return fmt.Errorf("command is required")
	}
	if !mockCommandRe.MatchString(command) {
		return fmt.Errorf("invalid command %q: must match %s", command, mockCommandRe)
	}
	if reason, ok := reservedMockCommands[command]; ok {
		return fmt.Errorf("command %q is %s", command, reason)
	}
	return nil
}

// validateMockScript checks that script is a relative path inside the evals
// directory naming an existing regular file. Symlinks are rejected: os.Stat
// and os.ReadFile follow them, so a symlinked fixture pointing outside the
// evals directory would otherwise be staged, defeating the restriction.
func validateMockScript(script string) error {
	if script == "" {
		return fmt.Errorf("script is required")
	}
	if !filepath.IsLocal(script) {
		return fmt.Errorf("invalid script %q: must be a relative path inside the evals directory (no absolute path or '..')", script)
	}
	p := evalsPath(script)
	info, err := os.Lstat(p)
	if err != nil {
		return fmt.Errorf("script %q not found at %s", script, p)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("script %q at %s is a symlink; mock scripts must be regular files inside the evals directory", script, p)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("script %q at %s is not a regular file", script, p)
	}
	return nil
}

// stageMocks writes each mock script to BinDir/<command> with mode 0755. The
// directory is bind-mounted read-only first on the container PATH, so the
// agent cannot alter a mock. An existing file (the fake gh) is never replaced.
// The inputs are re-checked because newCaseWorkspace may be handed a case that
// did not come through loadCases.
func (w *caseWorkspace) stageMocks(mocks []CaseMock) error {
	for i, m := range mocks {
		if err := validateMockCommand(m.Command); err != nil {
			return fmt.Errorf("mocks[%d]: %w", i, err)
		}
		if !filepath.IsLocal(m.Script) {
			return fmt.Errorf("mocks[%d] (%s): invalid script %q", i, m.Command, m.Script)
		}
		src := evalsPath(m.Script)
		if info, lerr := os.Lstat(src); lerr != nil {
			return fmt.Errorf("mocks[%d] (%s): script %q: %w", i, m.Command, m.Script, lerr)
		} else if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("mocks[%d] (%s): script %q is a symlink; mock scripts must be regular files inside the evals directory", i, m.Command, m.Script)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("mocks[%d] (%s): reading script: %w", i, m.Command, err)
		}
		dst := filepath.Join(w.BinDir, m.Command)
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return fmt.Errorf("mocks[%d] (%s): staging %s: %w", i, m.Command, dst, err)
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return fmt.Errorf("mocks[%d] (%s): writing %s: %w", i, m.Command, dst, werr)
		}
		if err := os.Chmod(dst, 0o755); err != nil { // not subject to umask
			return fmt.Errorf("mocks[%d] (%s): %w", i, m.Command, err)
		}
	}
	return nil
}

// newCaseWorkspace builds the host-side workspace for tc. On error nothing is
// left behind.
func newCaseWorkspace(tc TestCase) (ws *caseWorkspace, err error) {
	if err := validateWorkspaceName(tc.Workspace); err != nil {
		return nil, fmt.Errorf("case %q: %w", tc.Name, err)
	}
	var fixture string
	if tc.Workspace != "" {
		fixture = workspaceFixtureDir(tc.Workspace)
		info, serr := os.Stat(fixture)
		if serr != nil || !info.IsDir() {
			return nil, fmt.Errorf("case %q: workspace fixture %q not found at %s", tc.Name, tc.Workspace, fixture)
		}
	}

	base := os.Getenv(workspaceRootEnv)
	if base == "" {
		base = os.TempDir()
	}
	if base, err = filepath.Abs(base); err != nil {
		return nil, fmt.Errorf("resolving workspace root: %w", err)
	}
	if err = os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspace root %s: %w", base, err)
	}
	// Docker Desktop and Podman machine only share real paths (macOS /var is
	// a symlink to /private/var).
	if base, err = filepath.EvalSymlinks(base); err != nil {
		return nil, fmt.Errorf("resolving workspace root: %w", err)
	}
	root, err := os.MkdirTemp(base, workspaceDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("creating workspace: %w", err)
	}
	w := &caseWorkspace{root: root}
	defer func() {
		if err != nil {
			_ = w.Remove()
			ws = nil
		}
	}()

	w.Dir = filepath.Join(root, "ws")
	w.EvalDir = filepath.Join(w.Dir, evalOutputDirName)
	w.KiroDir = filepath.Join(w.Dir, kiroDirName)
	w.BinDir = filepath.Join(root, binDirName)
	if err = os.Mkdir(w.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspace: %w", err)
	}
	// The fake gh lives outside the workspace so the agent cannot modify it;
	// the directory is bind-mounted read-only.
	if err = sandbox.WriteFakeGH(w.BinDir); err != nil {
		return nil, err
	}
	// Author-supplied mocks go in the same read-only directory.
	if err = w.stageMocks(tc.Mocks); err != nil {
		return nil, fmt.Errorf("case %q: %w", tc.Name, err)
	}

	// Files the fixture provides, relative to the workspace, so that staging
	// can give them precedence.
	fixtureFiles := map[string]bool{}
	if fixture != "" {
		if err = copyTree(fixture, w.Dir, func(rel string) bool { return rel == ".git" }, func(rel string) { fixtureFiles[rel] = true }); err != nil {
			return nil, fmt.Errorf("copying workspace fixture %s: %w", fixture, err)
		}
	}

	if err = w.gitInit(); err != nil {
		return nil, err
	}

	// Harness-owned paths stay out of 'git status'. info/exclude (not a
	// tracked file) leaves the fixture tree untouched, and files the fixture
	// already tracks under .kiro stay tracked.
	exclude := filepath.Join(w.Dir, ".git", "info", "exclude")
	if err = os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return nil, err
	}
	f, oerr := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if oerr != nil {
		return nil, oerr
	}
	_, werr := f.WriteString("\n" + evalOutputDirName + "/\n" + kiroDirName + "/\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return nil, werr
	}

	if err = w.stageKiro(fixtureFiles); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(w.EvalDir, 0o755); err != nil {
		return nil, err
	}
	if err = w.writeGHIssue(tc.GHIssue); err != nil {
		return nil, err
	}
	if err = w.setPermissions(); err != nil {
		return nil, err
	}
	return w, nil
}

// writeGHIssue renders the case's gh_issue into .eval/ for the fake gh. It
// writes nothing when the case defines no gh_issue.
func (w *caseWorkspace) writeGHIssue(issue *sandbox.GHIssue) error {
	if issue == nil {
		return nil
	}
	doc, text, err := sandbox.RenderGHIssue(*issue)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(w.EvalDir, ghIssueJSONName), doc, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", ghIssueJSONName, err)
	}
	if err := os.WriteFile(filepath.Join(w.EvalDir, ghIssueTextName), []byte(text), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", ghIssueTextName, err)
	}
	return nil
}

// gitInit initializes the repo with one hermetic commit holding the fixture.
func (w *caseWorkspace) gitInit() error {
	steps := [][]string{
		{"init", "--quiet", "--template=", "-b", "main"},
		{"add", "--all", "--force"},
		{"-c", "user.name=kairon-eval", "-c", "user.email=eval@kairon.invalid",
			"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
			"commit", "--quiet", "--allow-empty", "--no-verify", "-m", "eval workspace fixture"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = w.Dir
		cmd.Env = hermeticGitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s in %s: %w: %s", args[0], w.Dir, err, strings.TrimSpace(string(out)))
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = w.Dir
	cmd.Env = hermeticGitEnv()
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD in %s: %w", w.Dir, err)
	}
	w.BaseCommit = strings.TrimSpace(string(out))
	return nil
}

// hermeticGitEnv returns the process environment with user and system git
// configuration disabled and repository-selecting variables removed, and the
// fixed commit dates so workspace git commits stay reproducible.
func hermeticGitEnv() []string {
	return append(checks.HermeticGitEnv(),
		"GIT_AUTHOR_DATE="+fixedGitDate,
		"GIT_COMMITTER_DATE="+fixedGitDate,
	)
}

// stageKiro builds Dir/.kiro on the host with precedence
// fixture > <evals-dir>/agents > project .kiro. The project's own .kiro is
// only read. A candidate prompt (cfg.candidate) is written last, over all of
// them, and is the one exception to the fixture precedence.
func (w *caseWorkspace) stageKiro(fixtureFiles map[string]bool) error {
	// Anything already present under .kiro came from the fixture.
	fixtureKiro := func(rel string) bool { return fixtureFiles[filepath.Join(kiroDirName, rel)] }
	return stageKiroInto(w.KiroDir, fixtureKiro, false, cfg.candidate)
}

// stageKiroInto builds a staged .kiro directory at kiroDir. It is the single
// staging routine: a case workspace uses it, and so can anything that needs
// the .kiro a case would get (for example baseline provenance, which passes a
// nil fixtureKiro).
//
// Layers are written in precedence order, lowest first:
//  1. the project's .kiro, limited to git-tracked files (see
//     projectKiroFilter) and without skills/*-conventions override hooks;
//  2. the <evals-dir>/agents overlay, unless ignoreOverlay is set;
//  3. the candidate prompt, if any, over every lower layer.
//
// fixtureKiro reports whether a path relative to .kiro was supplied by the
// case fixture (already copied into kiroDir); a lower layer never overwrites
// such a path. It may be nil.
func stageKiroInto(kiroDir string, fixtureKiro func(rel string) bool, ignoreOverlay bool, cand *candidatePrompt) error {
	if err := os.MkdirAll(kiroDir, 0o755); err != nil {
		return err
	}
	isFixture := func(rel string) bool { return fixtureKiro != nil && fixtureKiro(rel) }

	skipProject, err := projectKiroFilter()
	if err != nil {
		return fmt.Errorf("staging project %s: %w", kiroDirName, err)
	}
	if err := copyTree(kiroDirName, kiroDir, func(rel string) bool {
		return isFixture(rel) || skipProject(rel)
	}, nil); err != nil {
		return fmt.Errorf("staging project %s: %w", kiroDirName, err)
	}
	if !ignoreOverlay {
		agents := filepath.Join(kiroDir, "agents")
		if err := copyTree(evalsPath("agents"), agents, func(rel string) bool {
			return isFixture(filepath.Join("agents", rel))
		}, nil); err != nil {
			return fmt.Errorf("staging eval agents: %w", err)
		}
	}
	// A --prompt-file candidate replaces the agent's prompt in this staged
	// copy only, over every lower layer and any fixture file.
	return stageCandidatePromptInto(kiroDir, cand, ignoreOverlay)
}

// isConventionsPath reports whether rel (relative to .kiro) is, or lies
// under, a skills/<name>-conventions override hook directory.
func isConventionsPath(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return len(parts) >= 2 && parts[0] == "skills" && strings.HasSuffix(parts[1], auditConventionsSuffix)
}

// projectKiroFilter returns the skip predicate for the project layer: paths
// are relative to .kiro. It skips skills/*-conventions subtrees and every file
// git does not track, so untracked local state (such as .kiro/settings/) never
// reaches a case. Tracked files are still read from the working tree, so
// uncommitted edits to them are staged.
//
// A working directory outside any git repository has no untracked files by
// definition, so only the conventions filter applies. Any other git failure is
// returned rather than silently disabling the filter.
func projectKiroFilter() (func(rel string) bool, error) {
	files, err := trackedKiroFiles()
	if err != nil {
		return nil, err
	}
	if files == nil {
		return isConventionsPath, nil
	}
	// Directories on the way to a tracked file must be walked.
	dirs := map[string]bool{}
	for f := range files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	return func(rel string) bool {
		if isConventionsPath(rel) {
			return true
		}
		r := filepath.ToSlash(rel)
		return !files[r] && !dirs[r]
	}, nil
}

// trackedKiroFiles returns the set of git-tracked files under .kiro, as
// slash-separated paths relative to .kiro. It returns (nil, nil) when the
// working directory is not inside a git repository.
func trackedKiroFiles() (map[string]bool, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--", kiroDirName)
	cmd.Env = append(hermeticGitEnv(), "LC_ALL=C", "LANGUAGE=C")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 && strings.Contains(stderr.String(), "not a git repository") {
			return nil, nil
		}
		return nil, fmt.Errorf("git ls-files %s: %w: %s", kiroDirName, err, strings.TrimSpace(stderr.String()))
	}
	files := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if rel, ok := strings.CutPrefix(p, kiroDirName+"/"); ok && rel != "" {
			files[rel] = true
		}
	}
	return files, nil
}

// setPermissions makes the workspace usable by an unprivileged container user
// regardless of host ownership: a+rwX everywhere (including .git; git tracks
// no mode bits besides x, so there is no status noise) and a+rX, no
// group/other write, on .kiro, which is mounted read-only.
func (w *caseWorkspace) setPermissions() error {
	if err := filepath.WalkDir(w.Dir, permWalkFunc(w.Dir, w.KiroDir, 0o666)); err != nil {
		return fmt.Errorf("setting workspace permissions: %w", err)
	}
	if err := filepath.WalkDir(w.KiroDir, permWalkFunc(w.KiroDir, "", 0o444)); err != nil {
		return fmt.Errorf("setting %s permissions: %w", kiroDirName, err)
	}
	return nil
}

// permWalkFunc returns a WalkDir callback that applies chmodOpen(add) to every
// entry under root, skipping skipDir's subtree (pass "" for none). Entries that
// vanish mid-walk (fs.ErrNotExist, other than root itself) are skipped; every
// other error is returned.
func permWalkFunc(root, skipDir string, add fs.FileMode) fs.WalkDirFunc {
	return func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An entry listed by ReadDir can be removed before it is visited
			// (e.g. git's transient maintenance.lock). The root is never
			// skippable: its absence is a real setup bug. For a root lstat
			// failure d is nil, so return before touching it.
			if p != root && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if skipDir != "" && p == skipDir {
			return filepath.SkipDir
		}
		return chmodOpen(p, d, add)
	}
}

// chmodOpen adds the bits in add (applied for all of user/group/other) to p,
// plus execute for directories and already-executable files. For read-only
// additions (0o444) group/other write is cleared. Symlinks are left alone.
func chmodOpen(p string, d fs.DirEntry, add fs.FileMode) error {
	if d.Type()&fs.ModeSymlink != 0 {
		return nil
	}
	info, err := d.Info()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // vanished before we could stat it
		}
		return err
	}
	mode := info.Mode().Perm() | add
	if info.IsDir() || info.Mode().Perm()&0o111 != 0 {
		mode |= 0o111
	}
	if add == 0o444 {
		mode &^= 0o022
	}
	if mode == info.Mode().Perm() {
		return nil
	}
	if err := os.Chmod(p, mode); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Remove deletes the workspace. It is best effort: failure (for example files
// left owned by a foreign uid) is reported as a warning and never fails a run,
// so it always returns nil.
func (w *caseWorkspace) Remove() error {
	if w == nil || w.root == "" {
		return nil
	}
	// Directories made unreadable or read-only would block removal.
	_ = filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	if err := os.RemoveAll(w.root); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove eval workspace %s: %v\n", w.root, err)
	}
	return nil
}

// copyTree copies regular files and directories from src into dst, preserving
// the execute bit. Symlinks and other special files are skipped. A missing src
// copies nothing. skip, when non-nil, is called with each path relative to src
// and prevents that entry (and a skipped directory's subtree) from being
// copied. onFile, when non-nil, is called with the src-relative path of every
// regular file copied.
func copyTree(src, dst string, skip func(rel string) bool, onFile func(rel string)) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			if err := copyFile(p, target); err != nil {
				return err
			}
			if onFile != nil {
				onFile(rel)
			}
		}
		return nil
	})
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info.Mode().Perm()&0o111 != 0 {
		mode = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode) // not subject to umask
}
