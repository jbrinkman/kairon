package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
)

// Containment self-test: fake gh, read-only filesystem and tool trust under
// `--sandbox --backend stub`. The fixtures live in the checked-in self-test
// evals (agents selftest-sandbox and selftest-sandbox-ro). Every case is
// requires_sandbox: true, so none of them can run natively.
const (
	containmentAgent   = "selftest-sandbox"
	containmentROAgent = "selftest-sandbox-ro"

	ghFakeCase         = "stub-gh-fake"
	workspaceWriteCase = "stub-workspace-write"
	toolAllowedCase    = "stub-tool-allowed"
	writeOutsideCase   = "stub-write-outside-mounts"
	toolDeniedCase     = "stub-tool-denied"
	mockCLICase        = "stub-mock-cli"

	// testdataMockCLIScript is the self-test copy of the live mock-cli.sh
	// fixture, relative to the repository root.
	testdataMockCLIScript = "internal/eval/testdata/evals/fixtures/mock-cli.sh"
)

// containmentCases maps each containment agent to the cases it must have.
var containmentCases = map[string][]string{
	containmentAgent:   {ghFakeCase, workspaceWriteCase, toolAllowedCase, mockCLICase},
	containmentROAgent: {writeOutsideCase, toolDeniedCase},
}

// skipUnlessContainmentSandbox skips (with a message) unless the opt-in env
// var is set and a container daemon is reachable. Nothing is started before
// the skip.
func skipUnlessContainmentSandbox(t *testing.T) {
	t.Helper()
	if os.Getenv(sandboxSelftestEnv) != "1" {
		t.Skipf("sandbox containment test is opt-in: set %s=1 or run `task eval:selftest:sandbox` (needs Podman or Docker)", sandboxSelftestEnv)
	}
	if err := sandbox.EnsureContainerDaemon(); err != nil {
		t.Skipf("no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the sandbox containment test: %v", err)
	}
}

// TestContainmentFixtures loads the containment cases through the normal case
// loader and checks the contract the gated tests rely on: the expected cases
// exist per agent and every one of them is requires_sandbox.
func TestContainmentFixtures(t *testing.T) {
	evalsDir := copyEvalsFromRepoRoot(t)
	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: inference.NameStub, EvalsDir: evalsDir}); err != nil {
		t.Fatal(err)
	}

	for agent, want := range containmentCases {
		cases, err := loadCases(agent)
		if err != nil {
			t.Fatalf("loadCases(%s): %v", agent, err)
		}
		byName := map[string]TestCase{}
		for _, tc := range cases {
			byName[tc.Name] = tc
			if !tc.RequiresSandbox {
				t.Errorf("%s/%s: requires_sandbox is false, want true", agent, tc.Name)
			}
			if tc.Agent != agent {
				t.Errorf("%s/%s: agent = %q", agent, tc.Name, tc.Agent)
			}
		}
		if len(cases) != len(want) {
			t.Errorf("%s has %d cases, want %d (%v)", agent, len(cases), len(want), want)
		}
		for _, name := range want {
			if _, ok := byName[name]; !ok {
				t.Errorf("%s: case %q missing", agent, name)
			}
		}
	}

	cases, err := loadCases(containmentAgent)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		switch tc.Name {
		case ghFakeCase:
			if tc.GHIssue == nil || tc.GHIssue.Title == "" || tc.GHIssue.Body == "" {
				t.Errorf("%s must configure gh_issue with a title and body, got %+v", ghFakeCase, tc.GHIssue)
			}
		case mockCLICase:
			if tc.Workspace != "mock-aws" {
				t.Errorf("%s: workspace = %q, want mock-aws", mockCLICase, tc.Workspace)
			}
			wantMocks := []CaseMock{{Command: "aws", Script: "fixtures/mock-cli.sh"}}
			if !reflect.DeepEqual(tc.Mocks, wantMocks) {
				t.Errorf("%s: mocks = %+v, want %+v", mockCLICase, tc.Mocks, wantMocks)
			}
		}
	}
}

// TestContainmentMockCLIFixtureParity fails when the self-test copy of
// mock-cli.sh drifts from the live fixture under .kairon/evals/fixtures. The
// self-test's evals dir is the testdata one, so it needs its own copy.
func TestContainmentMockCLIFixtureParity(t *testing.T) {
	root := repoRoot(t)
	live, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(mockCLIScript)))
	if err != nil {
		t.Fatal(err)
	}
	selftest, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testdataMockCLIScript)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(live, selftest) {
		t.Errorf("%s differs from %s; copy the live fixture over the self-test copy", testdataMockCLIScript, mockCLIScript)
	}
}

// TestContainmentNativeRefusal runs the containment agents natively. Every
// case must be recorded as failed with "requires --sandbox", no workspace may
// be created and nothing may be invoked (in particular, no real gh). It always
// runs: it needs neither a daemon nor the opt-in variable.
func TestContainmentNativeRefusal(t *testing.T) {
	wsRoot := t.TempDir()
	t.Setenv(workspaceRootEnv, wsRoot)
	t.Cleanup(resetConfig)

	for agent, want := range containmentCases {
		res, _ := runEvalCopy(t, agent, RunOptions{NoSandbox: true, KeepWorkspaces: true})
		if len(res.Cases) != len(want) {
			t.Fatalf("%s: %d cases in the result, want %d", agent, len(res.Cases), len(want))
		}
		for _, c := range res.Cases {
			if c.ErrorContext == nil || !strings.Contains(c.ErrorContext.Stderr, "requires --sandbox") {
				t.Errorf("%s/%s: want a 'requires --sandbox' failure, got ErrorContext %+v", agent, c.CaseName, c.ErrorContext)
			}
			if c.ActualOutput != "" {
				t.Errorf("%s/%s: actual_output = %q, want empty (agent must not run)", agent, c.CaseName, c.ActualOutput)
			}
			if len(c.Calls) != 0 {
				t.Errorf("%s/%s: %d agent calls recorded, want none", agent, c.CaseName, len(c.Calls))
			}
			if c.WorkspaceDir != "" {
				t.Errorf("%s/%s: workspace_dir = %q, want none", agent, c.CaseName, c.WorkspaceDir)
			}
		}
	}

	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("native refusal created %d workspace entries under %s, want none", len(entries), wsRoot)
	}
}

// readHostFile reads a file under a kept host workspace.
func readHostFile(t *testing.T, c CaseResult, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(c.WorkspaceDir, rel))
	if err != nil {
		t.Fatalf("%s: reading %s from the host workspace %s: %v", c.CaseName, rel, c.WorkspaceDir, err)
	}
	return string(data)
}

// agentCall returns the case's agent call record.
func agentCall(t *testing.T, c CaseResult) inference.CallRecord {
	t.Helper()
	for _, call := range c.Calls {
		if call.Role == "agent" {
			return call
		}
	}
	t.Fatalf("%s: no agent call recorded (calls: %+v)", c.CaseName, c.Calls)
	return inference.CallRecord{}
}

// TestContainmentSandbox runs the containment agents under `--sandbox
// --backend stub` and checks fake gh, the read-only filesystem and tool trust
// from the host side. Gated like TestSelftestSandbox.
func TestContainmentSandbox(t *testing.T) {
	skipUnlessContainmentSandbox(t)

	t.Cleanup(resetConfig)
	t.Chdir(repoRoot(t))

	rw, _ := runEvalCopy(t, containmentAgent, RunOptions{Sandbox: true, KeepWorkspaces: true})
	ro, _ := runEvalCopy(t, containmentROAgent, RunOptions{Sandbox: true, KeepWorkspaces: true})
	for _, res := range []AgentResult{rw, ro} {
		for _, c := range res.Cases {
			cleanupWorkspace(t, c)
		}
	}

	t.Run("fake gh", func(t *testing.T) {
		c := caseByName(t, rw, ghFakeCase)
		if c.ErrorContext != nil {
			t.Fatalf("case has ErrorContext: %+v", c.ErrorContext)
		}

		log := readHostFile(t, c, ".eval/gh.log")
		if !strings.Contains(log, "gh issue create --title t --body-file b.md\n") {
			t.Errorf(".eval/gh.log does not contain the gh issue create call:\n%s", log)
		}
		if body, want := readHostFile(t, c, ".eval/gh-body-1.md"), readHostFile(t, c, "b.md"); body != want || body == "" {
			t.Errorf(".eval/gh-body-1.md = %q, want the contents of b.md (%q)", body, want)
		}

		var view struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		raw := readHostFile(t, c, "view.json")
		if err := json.Unmarshal([]byte(raw), &view); err != nil {
			t.Fatalf("view.json is not valid JSON: %v\n%s", err, raw)
		}
		if view.Title != "Fake issue title" || view.Body != "Fake issue body" {
			t.Errorf("view.json = %+v, want the configured gh_issue title and body", view)
		}
	})

	t.Run("mocked cli", func(t *testing.T) {
		c := caseByName(t, rw, mockCLICase)
		if c.ErrorContext != nil {
			t.Fatalf("case has ErrorContext: %+v", c.ErrorContext)
		}

		wantLog := "aws s3 cp report.txt s3://prod-bucket/report.txt\n" +
			"aws iam delete-user --user-name prod-admin\n"
		if got := readHostFile(t, c, ".eval/mock-aws.log"); got != wantLog {
			t.Errorf(".eval/mock-aws.log = %q, want %q", got, wantLog)
		}
		if got := strings.TrimSpace(readHostFile(t, c, "aws-path.txt")); got != "/opt/kairon/bin/aws" {
			t.Errorf("aws-path.txt = %q, want /opt/kairon/bin/aws", got)
		}
		if got, want := readHostFile(t, c, "aws-out.txt"), readHostFile(t, c, ".mocks/aws/s3-cp.out"); got != want || got == "" {
			t.Errorf("aws-out.txt = %q, want the contents of .mocks/aws/s3-cp.out (%q)", got, want)
		}
		if got := readHostFile(t, c, "aws-unsimulated.txt"); !strings.Contains(got, "is not simulated") {
			t.Errorf("aws-unsimulated.txt = %q, want it to contain %q", got, "is not simulated")
		}

		// The committed .mocks/ data is untouched: only the three stub
		// outputs are new, and .eval/ is excluded from status.
		status := gitIn(t, c.WorkspaceDir, "status", "--porcelain")
		want := "?? aws-out.txt\n?? aws-path.txt\n?? aws-unsimulated.txt\n"
		if status != want {
			t.Errorf("git status --porcelain = %q, want %q", status, want)
		}
	})

	t.Run("workspace is writable", func(t *testing.T) {
		c := caseByName(t, rw, workspaceWriteCase)
		if c.ErrorContext != nil {
			t.Fatalf("case has ErrorContext: %+v", c.ErrorContext)
		}
		if got := readHostFile(t, c, "marker.txt"); got != "x\n" {
			t.Errorf("marker.txt = %q, want %q", got, "x\n")
		}
	})

	t.Run("write outside mounts fails read-only", func(t *testing.T) {
		c := caseByName(t, ro, writeOutsideCase)
		if c.ErrorContext == nil {
			t.Fatal("write outside the mounts must fail, but the case has no ErrorContext")
		}
		if !strings.Contains(c.ErrorContext.Stderr, "Read-only file system") {
			t.Errorf("ErrorContext.Stderr = %q, want it to contain %q", c.ErrorContext.Stderr, "Read-only file system")
		}
		if c.ActualOutput != "" {
			t.Errorf("actual_output = %q, want empty for a failed case", c.ActualOutput)
		}
	})

	t.Run("tool denied", func(t *testing.T) {
		c := caseByName(t, ro, toolDeniedCase)
		if c.ErrorContext != nil {
			t.Fatalf("case has ErrorContext: %+v", c.ErrorContext)
		}
		if _, err := os.Stat(filepath.Join(c.WorkspaceDir, "tool-marker.txt")); !os.IsNotExist(err) {
			t.Errorf("tool-marker.txt must not exist after a denied fs_write (stat err: %v)", err)
		}
		call := agentCall(t, c)
		if len(call.ToolDenials) != 1 || call.ToolDenials[0].Tool != "fs_write" {
			t.Errorf("tool_denials = %+v, want one fs_write denial", call.ToolDenials)
		}
		if call.TrustedTools == nil || strings.Join(*call.TrustedTools, ",") != "fs_read" {
			t.Errorf("trusted_tools = %v, want [fs_read]", call.TrustedTools)
		}
	})

	t.Run("tool allowed", func(t *testing.T) {
		c := caseByName(t, rw, toolAllowedCase)
		if c.ErrorContext != nil {
			t.Fatalf("case has ErrorContext: %+v", c.ErrorContext)
		}
		if got := readHostFile(t, c, "tool-marker.txt"); got != "x\n" {
			t.Errorf("tool-marker.txt = %q, want %q", got, "x\n")
		}
		call := agentCall(t, c)
		if len(call.ToolDenials) != 0 {
			t.Errorf("tool_denials = %+v, want none", call.ToolDenials)
		}
		if call.TrustedTools == nil || !containsString(*call.TrustedTools, "fs_write") {
			t.Errorf("trusted_tools = %v, want it to contain fs_write", call.TrustedTools)
		}
	})
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

var notLoggedIn = regexp.MustCompile(`(?i)not logged in`)

// TestContainmentContainerGH starts a container with exactly the mounts, env
// and host config a case gets and checks, from inside, that `gh` resolves to
// the fake and that the real gh (absolute path) is unauthenticated. Gated like
// TestSelftestSandbox.
func TestContainmentContainerGH(t *testing.T) {
	skipUnlessContainmentSandbox(t)

	t.Setenv(workspaceRootEnv, t.TempDir())
	cConfig := createContainerConfig(nil, nil, false)

	ws, err := newCaseWorkspace(TestCase{Name: "containment-gh", Agent: containmentAgent})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Remove() })

	// kiro-cli backend name: no kairon helper mount is needed for this check.
	mounts, err := buildContainerMounts(ws, cConfig, inference.NameKiroCLI)
	if err != nil {
		t.Fatal(err)
	}
	hostConfig, err := sandbox.NewHostConfigWithMounts(cConfig.ResourceLimits, mounts)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	image, err := ensureBaseImageName(ctx, cConfig)
	if err != nil {
		t.Fatal(err)
	}
	c, err := sandbox.NewContainer("")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.CreateWithPlatform(ctx, newContainerConfig(image, cConfig), hostConfig, cConfig.Platform); err != nil {
		t.Fatalf("creating container: %v", err)
	}
	defer func() { _ = c.Cleanup(context.Background()) }()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("starting container: %v", err)
	}

	res, err := c.ExecWithStdin(ctx, []string{"sh", "-c", "command -v gh"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Stdout); res.ExitCode != 0 || got != "/opt/kairon/bin/gh" {
		t.Errorf("command -v gh = %q (exit %d, stderr %q), want /opt/kairon/bin/gh", got, res.ExitCode, res.Stderr)
	}

	res, err = c.ExecWithStdin(ctx, []string{"/usr/local/bin/gh", "auth", "status"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Errorf("/usr/local/bin/gh auth status exited 0; the real gh must be unauthenticated\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if out := res.Stdout + res.Stderr; !notLoggedIn.MatchString(out) {
		t.Errorf("/usr/local/bin/gh auth status output does not match %q:\n%s", notLoggedIn, out)
	}
}
