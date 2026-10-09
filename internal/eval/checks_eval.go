package eval

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// checkCommandTimeout bounds one `command` check. It is a variable so tests
// can shorten it; it is not a schema field.
var checkCommandTimeout = 5 * time.Minute

const (
	// maxCheckFileBytes caps what file_contains / file_not_contains read.
	maxCheckFileBytes = 10 << 20
	// maxCheckDetail caps CheckResult.Detail.
	maxCheckDetail = 2048
)

// errPathSymlink marks a path that traverses (or ends in) a symlink.
var errPathSymlink = errors.New("path traverses a symlink")

// EvaluateChecks evaluates checks against in and returns one result per check
// in list order (Index is 1-based). Non-command checks run first, against the
// workspace exactly as the agent left it; command checks then run in listed
// order. It looks only at CheckInput: it never touches cfg, a case workspace
// or a rubric. An invalid check yields a failed result, never a panic.
func EvaluateChecks(checks []Check, in CheckInput) []CheckResult {
	results := make([]CheckResult, len(checks))
	for _, commands := range []bool{false, true} {
		for i := range checks {
			if (checks[i].Type == CheckCommand) != commands {
				continue
			}
			results[i] = evaluateOne(i+1, checks[i], in)
		}
	}
	return results
}

// evaluateOne validates (and compiles) a copy of c and runs it.
func evaluateOne(index int, c Check, in CheckInput) (res CheckResult) {
	res = CheckResult{Index: index, Type: c.Type, Label: checkLabel(index, c)}
	defer func() {
		if r := recover(); r != nil { // defensive: never crash a run on a bad check
			res.Passed = false
			res.Detail = truncateDetail(fmt.Sprintf("internal error evaluating check: %v", r))
		}
	}()

	// Loader-compiled checks are re-validated too, so a hand-built check and a
	// loaded one take the same path. The evaluator does not care about the
	// criterion, so a missing one is not an error here.
	cp := c
	if cp.Criterion == "" {
		cp.Criterion = "-"
	}
	if err := validateCheck(&cp, cp.injectDir, nil); err != nil {
		res.Detail = truncateDetail("invalid check: " + err.Error())
		return res
	}

	passed, detail := runCheck(cp, in)
	res.Passed = passed
	res.Detail = truncateDetail(detail)
	return res
}

func runCheck(c Check, in CheckInput) (bool, string) {
	switch c.Type {
	case CheckFileExists, CheckFileAbsent, CheckFileContains, CheckFileNotContains, CheckChangedFiles, CheckCommand:
		if err := checkDir(in.Dir); err != nil {
			return false, err.Error()
		}
	}
	switch c.Type {
	case CheckFileExists:
		return evalFileExists(c, in.Dir, true)
	case CheckFileAbsent:
		return evalFileExists(c, in.Dir, false)
	case CheckFileContains:
		return evalFileContains(c, in.Dir, true)
	case CheckFileNotContains:
		return evalFileContains(c, in.Dir, false)
	case CheckOutputContains, CheckOutputNotContains:
		ev, detail := turnEvidence(c, in)
		if detail != "" {
			return false, detail
		}
		return evalText(c, ev.Output, "output", c.Type == CheckOutputContains)
	case CheckGHLogContains, CheckGHLogNotContains:
		ev, detail := turnEvidence(c, in)
		if detail != "" {
			return false, detail
		}
		if ev.GHLogOversized {
			return false, "gh log exceeds 10 MiB; cannot score reliably"
		}
		return evalText(c, ev.GHLog, "gh log", c.Type == CheckGHLogContains)
	case CheckChangedFiles:
		return evalChangedFiles(c, in)
	case CheckCommand:
		return evalCommand(c, in.Dir)
	}
	return false, fmt.Sprintf("unknown check type %q", c.Type)
}

// turnEvidence picks the output / gh log a text check reads. Without per-turn
// evidence (a classic case) it is the top-level Output / GHLog. With it, a
// check reads its own turn, or the last turn when it names none. A non-empty
// detail means the check cannot be evaluated (turn out of range).
func turnEvidence(c Check, in CheckInput) (TurnEvidence, string) {
	if len(in.Turns) == 0 {
		return TurnEvidence{Output: in.Output, GHLog: in.GHLog, GHLogOversized: in.GHLogOversized}, ""
	}
	if c.Turn == nil {
		return in.Turns[len(in.Turns)-1], ""
	}
	if *c.Turn < 1 || *c.Turn > len(in.Turns) {
		return TurnEvidence{}, fmt.Sprintf("turn %d is out of range: the case has %d turn(s)", *c.Turn, len(in.Turns))
	}
	return in.Turns[*c.Turn-1], ""
}

// checkDir requires dir to be an existing directory.
func checkDir(dir string) error {
	if dir == "" {
		return errors.New("no workspace directory given")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("workspace directory unusable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace directory %q is not a directory", dir)
	}
	return nil
}

// resolveInDir resolves the local path rel below dir by walking its
// components with Lstat. An existing intermediate component that is a symlink
// yields errPathSymlink. A missing path (including one below a regular file)
// returns info == nil and a nil error. The final component is returned as
// Lstat saw it, so it may itself be a symlink; callers decide what to do.
// dir itself is not inspected, so a symlinked workspace root is fine.
func resolveInDir(dir, rel string) (string, os.FileInfo, error) {
	if !filepath.IsLocal(rel) {
		return "", nil, fmt.Errorf("path %q must be relative and stay inside the workspace", rel)
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
	cur := dir
	for i, p := range parts {
		cur = filepath.Join(cur, p)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
				return cur, nil, nil
			}
			return cur, nil, err
		}
		if i == len(parts)-1 {
			return cur, info, nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cur, nil, fmt.Errorf("%w: %s", errPathSymlink, filepath.Join(parts[:i+1]...))
		}
		if !info.IsDir() {
			return cur, nil, nil
		}
	}
	return cur, nil, nil
}

func evalFileExists(c Check, dir string, wantExists bool) (bool, string) {
	_, info, err := resolveInDir(dir, c.Path)
	if err != nil {
		return false, err.Error()
	}
	exists := info != nil
	switch {
	case wantExists && !exists:
		return false, "file does not exist"
	case !wantExists && exists:
		return false, "path exists"
	}
	return true, ""
}

func evalFileContains(c Check, dir string, wantMatch bool) (bool, string) {
	data, detail := readCheckFile(dir, c.Path)
	if detail != "" {
		return false, detail
	}
	return matchDetail(c, string(data), "file", wantMatch)
}

// readCheckFile reads a regular, non-symlink file below dir, at most
// maxCheckFileBytes. A non-empty detail means the read was refused.
func readCheckFile(dir, rel string) ([]byte, string) {
	abs, info, err := resolveInDir(dir, rel)
	switch {
	case err != nil:
		return nil, err.Error()
	case info == nil:
		return nil, "file does not exist"
	case info.Mode()&os.ModeSymlink != 0:
		return nil, "path is a symlink; refusing to follow it"
	case !info.Mode().IsRegular():
		return nil, "path is not a regular file"
	case info.Size() > maxCheckFileBytes:
		return nil, fmt.Sprintf("file is larger than the 10 MiB limit (%d bytes)", info.Size())
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, "cannot open file: " + err.Error()
	}
	defer f.Close()
	// The file must still be the one we inspected (not swapped for a link).
	if fi, err := f.Stat(); err != nil || !os.SameFile(info, fi) {
		return nil, "file changed while being read; refusing"
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCheckFileBytes+1))
	if err != nil {
		return nil, "cannot read file: " + err.Error()
	}
	if len(data) > maxCheckFileBytes {
		return nil, "file is larger than the 10 MiB limit"
	}
	return data, ""
}

func evalText(c Check, text, what string, wantMatch bool) (bool, string) {
	return matchDetail(c, text, what, wantMatch)
}

// matchDetail applies the check's compiled pattern to text.
func matchDetail(c Check, text, what string, wantMatch bool) (bool, string) {
	loc := c.re.FindStringIndex(text)
	switch {
	case wantMatch && loc == nil:
		return false, fmt.Sprintf("pattern /%s/ not found in %s", shorten(c.Pattern, 80), what)
	case !wantMatch && loc != nil:
		return false, fmt.Sprintf("pattern /%s/ matched in %s: %q", shorten(c.Pattern, 80), what, shorten(text[loc[0]:loc[1]], 80))
	}
	return true, ""
}

// shorten truncates s to at most n bytes (on a rune boundary), adding "…".
func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func truncateDetail(s string) string {
	s = strings.ToValidUTF8(s, "?")
	if len(s) <= maxCheckDetail {
		return s
	}
	return shorten(s, maxCheckDetail)
}

// checkLabel names a check for results and reasoning: its 1-based position,
// type and key fields, e.g. `#2 file_exists path=missing.txt`.
func checkLabel(index int, c Check) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s", index, c.Type)
	switch c.Type {
	case CheckCommand:
		fmt.Fprintf(&b, " %q", shorten(c.Run, 80))
	case CheckChangedFiles:
		fmt.Fprintf(&b, " allow=[%s]", shorten(strings.Join(c.Allow, " "), 80))
	default:
		if c.Path != "" {
			fmt.Fprintf(&b, " path=%s", shorten(c.Path, 80))
		}
		if c.Pattern != "" {
			fmt.Fprintf(&b, " pattern=/%s/", shorten(c.Pattern, 80))
		}
	}
	if c.Turn != nil {
		fmt.Fprintf(&b, " turn=%d", *c.Turn)
	}
	return b.String()
}

// absDir returns dir as an absolute path.
func absDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("workspace directory unusable: %w", err)
	}
	return abs, nil
}
