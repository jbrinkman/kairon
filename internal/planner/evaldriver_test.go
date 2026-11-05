package planner

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	interactiveFilter = "name==interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split,interactive-document-handoff,interactive-path-only-message,interactive-decline-implementation"
	automatedFilter   = "name==automated-best-effort-fill,automated-bug-classification,automated-punch-out,automated-dependencies-line"
)

// requireBash skips the test when bash is not available.
func requireBash(t *testing.T) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	return bash
}

// treeSnapshot returns "path size" lines for every file and directory under dir,
// sorted, so two snapshots can be compared for equality.
func treeSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		lines = append(lines, fmt.Sprintf("%s %d %d", rel, info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestEvalDriverDryRun(t *testing.T) {
	bash := requireBash(t)
	root := repoRoot(t)
	evalsDir := filepath.Join(root, ".kairon", "evals")
	before := treeSnapshot(t, evalsDir)

	cmd := exec.Command(bash, filepath.Join("scripts", "eval-planner.sh"), "--dry-run")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dry run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()

	var evalLines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "kairon eval ") {
			evalLines = append(evalLines, strings.TrimSpace(l))
		}
	}
	if len(evalLines) != 2 {
		t.Fatalf("want exactly 2 'kairon eval' command lines, got %d:\n%s", len(evalLines), out)
	}
	interactive, automated := evalLines[0], evalLines[1]

	for name, line := range map[string]string{"interactive": interactive, "automated": automated} {
		if !strings.Contains(line, " --sandbox ") {
			t.Errorf("%s command lacks --sandbox: %s", name, line)
		}
		if !strings.Contains(line, " planner ") {
			t.Errorf("%s command does not target the planner suite: %s", name, line)
		}
	}
	if !strings.Contains(interactive, "--filter '"+interactiveFilter+"'") {
		t.Errorf("interactive command lacks exact filter %q: %s", interactiveFilter, interactive)
	}
	if strings.Contains(interactive, "--evals-dir") {
		t.Errorf("interactive half must use the default evals dir: %s", interactive)
	}
	if !strings.Contains(automated, "--filter '"+automatedFilter+"'") {
		t.Errorf("automated command lacks exact filter %q: %s", automatedFilter, automated)
	}
	if !strings.Contains(automated, "--evals-dir ") {
		t.Errorf("automated command lacks --evals-dir overlay: %s", automated)
	}
	if !strings.Contains(out, "planner-automated.json") || !strings.Contains(out, `"name": "planner"`) {
		t.Errorf("dry run does not describe the overlay manifest rename:\n%s", out)
	}
	if !strings.Contains(out, "planner-automated-prompt.md") {
		t.Errorf("dry run does not describe the overlay prompt:\n%s", out)
	}

	if after := treeSnapshot(t, evalsDir); after != before {
		t.Errorf("dry run modified .kairon/evals:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// fakeKairon writes a stand-in for the kairon binary. It records its arguments
// (one line per invocation) to $FAKE_LOG, snapshots the overlay manifest and
// prompt, and creates one new result directory in the evals dir it was given
// (or the default one). It exits with $FAKE_FAIL_INTERACTIVE or
// $FAKE_FAIL_AUTOMATED when the matching half is running.
func fakeKairon(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "kairon-fake")
	script := `#!/usr/bin/env bash
set -u
echo "$*" >> "$FAKE_LOG"
evals="$FAKE_DEFAULT_EVALS"
half=interactive
prev=""
for a in "$@"; do
  if [ "$prev" = "--evals-dir" ]; then evals="$a"; fi
  case "$a" in *automated-best-effort-fill*) half=automated ;; esac
  prev="$a"
done
if [ "$half" = automated ]; then
  mkdir -p "$FAKE_CAPTURE"
  cp "$evals/agents/planner.json" "$FAKE_CAPTURE/planner.json"
  cp "$evals/agents/planner-automated-prompt.md" "$FAKE_CAPTURE/prompt.md"
  find "$evals" -path "*/skills/*" > "$FAKE_CAPTURE/skills.txt" 2>/dev/null || true
  [ -d "$evals/results" ] && ls "$evals/results" > "$FAKE_CAPTURE/preexisting-results.txt"
fi
mkdir -p "$evals/results/new-$half"
echo "$half" > "$evals/results/new-$half/summary.json"
if [ "$half" = interactive ] && [ -n "${FAKE_FAIL_INTERACTIVE:-}" ]; then exit 3; fi
if [ "$half" = automated ] && [ -n "${FAKE_FAIL_AUTOMATED:-}" ]; then exit 4; fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kairon: %v", err)
	}
	return path
}

type driverRun struct {
	err      error
	out      string
	log      string
	capture  string
	evalsDir string
}

func runDriver(t *testing.T, failInteractive, failAutomated bool) driverRun {
	t.Helper()
	bash := requireBash(t)
	root := repoRoot(t)
	tmp := t.TempDir()

	evalsDir := filepath.Join(tmp, "evals")
	for _, d := range []string{"cases/planner", "rubrics", "fixtures", "results/old-run"} {
		if err := os.MkdirAll(filepath.Join(evalsDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(evalsDir, "results", "old-run", "summary.json"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalsDir, "rubrics", "planner.yaml"), []byte("name: planner\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tmp, "calls.log")
	capture := filepath.Join(tmp, "capture")
	cmd := exec.Command(bash, filepath.Join("scripts", "eval-planner.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"KAIRON_BIN="+fakeKairon(t, tmp),
		"KAIRON_EVALS_DIR="+evalsDir,
		"TMPDIR="+tmp,
		"FAKE_LOG="+logPath,
		"FAKE_CAPTURE="+capture,
		"FAKE_DEFAULT_EVALS="+evalsDir,
	)
	if failInteractive {
		cmd.Env = append(cmd.Env, "FAKE_FAIL_INTERACTIVE=1")
	}
	if failAutomated {
		cmd.Env = append(cmd.Env, "FAKE_FAIL_AUTOMATED=1")
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	logData, _ := os.ReadFile(logPath)
	return driverRun{err: err, out: out.String(), log: string(logData), capture: capture, evalsDir: evalsDir}
}

func TestEvalDriverOverlayAndResults(t *testing.T) {
	r := runDriver(t, false, false)
	if r.err != nil {
		t.Fatalf("driver failed: %v\n%s", r.err, r.out)
	}

	calls := strings.Split(strings.TrimSpace(r.log), "\n")
	if len(calls) != 2 {
		t.Fatalf("want 2 kairon invocations, got %d:\n%s", len(calls), r.log)
	}
	if !strings.Contains(calls[0], interactiveFilter) || !strings.Contains(calls[1], automatedFilter) {
		t.Errorf("unexpected filters in calls:\n%s", r.log)
	}

	// The overlay manifest is planner-automated.json with only name changed.
	root := repoRoot(t)
	orig := string(readRepoFile(t, ".kiro/agents/planner-automated.json"))
	got, err := os.ReadFile(filepath.Join(r.capture, "planner.json"))
	if err != nil {
		t.Fatalf("overlay manifest not captured: %v", err)
	}
	want := strings.Replace(orig, `"name": "planner-automated"`, `"name": "planner"`, 1)
	if string(got) != want {
		t.Errorf("overlay planner.json is not planner-automated.json with only name changed:\n--- got\n%s\n--- want\n%s", got, want)
	}
	prompt, err := os.ReadFile(filepath.Join(r.capture, "prompt.md"))
	if err != nil {
		t.Fatalf("overlay prompt not captured: %v", err)
	}
	if string(prompt) != string(readRepoFile(t, ".kiro/agents/planner-automated-prompt.md")) {
		t.Errorf("overlay prompt differs from .kiro/agents/planner-automated-prompt.md")
	}

	// The shared skill must not be copied into the overlay.
	if skills, _ := os.ReadFile(filepath.Join(r.capture, "skills.txt")); strings.TrimSpace(string(skills)) != "" {
		t.Errorf("overlay must not contain skills (the tracked planner-shared is used), got:\n%s", skills)
	}
	// Existing results are not part of the overlay (it copies without results/).
	if pre, _ := os.ReadFile(filepath.Join(r.capture, "preexisting-results.txt")); strings.Contains(string(pre), "old-run") {
		t.Errorf("overlay must not carry over existing results, got: %s", pre)
	}

	// New result directories from both halves are under the evals dir results.
	for _, d := range []string{"new-interactive", "new-automated", "old-run"} {
		if _, err := os.Stat(filepath.Join(r.evalsDir, "results", d)); err != nil {
			t.Errorf("results/%s missing after run: %v", d, err)
		}
	}
	_ = root
}

func TestEvalDriverExitsNonZeroWhenEitherHalfFails(t *testing.T) {
	for name, tc := range map[string]struct{ interactive, automated bool }{
		"interactive fails": {true, false},
		"automated fails":   {false, true},
		"both fail":         {true, true},
	} {
		t.Run(name, func(t *testing.T) {
			r := runDriver(t, tc.interactive, tc.automated)
			if r.err == nil {
				t.Fatalf("driver exited 0 although a half failed:\n%s", r.out)
			}
			// Both halves still run, and results are copied back.
			if n := len(strings.Split(strings.TrimSpace(r.log), "\n")); n != 2 {
				t.Errorf("want both halves to run, got %d invocations:\n%s", n, r.log)
			}
			if _, err := os.Stat(filepath.Join(r.evalsDir, "results", "new-automated")); err != nil {
				t.Errorf("automated results not copied back on failure: %v", err)
			}
		})
	}
}
