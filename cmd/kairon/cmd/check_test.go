package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/jbrinkman/kairon/internal/inference"
)

// checkFixtures returns the absolute path of the kairon check fixtures.
func checkFixtures(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "internal", "eval", "testdata", "check-cli"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// runCheck executes `kairon check <args...>` through the root command, so the
// test also proves the command is registered. It returns stdout and the error
// Execute returned, and restores the flag state afterwards.
func runCheck(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cc, _, ferr := rootCmd.Find([]string{"check"})
	if ferr != nil || cc == nil || cc.Name() != "check" {
		t.Fatalf("kairon check command is not registered on the root command (Find: %v)", ferr)
	}

	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(append([]string{"check"}, args...))
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		cc.SilenceUsage = false
		cc.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
	err := rootCmd.Execute()
	return stdout.String(), err
}

// inDir chdirs into dir for the test.
func inDir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// writeEvalsConfig writes ./.kairon/config.yaml in the current directory.
func writeEvalsConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".kairon", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".kairon", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func outputLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestCheckCommandRegistered(t *testing.T) {
	cc, _, err := rootCmd.Find([]string{"check"})
	if err != nil || cc == nil || cc.Name() != "check" {
		t.Fatalf("kairon check command is not registered on the root command (Find: %v)", err)
	}
}

func TestCheckFlagsRegistered(t *testing.T) {
	cc, _, err := rootCmd.Find([]string{"check"})
	if err != nil || cc.Name() != "check" {
		t.Fatalf("kairon check command is not registered (Find: %v)", err)
	}
	for _, name := range []string{"checks", "dir", "output", "gh-log", "hidden-dir", "backend"} {
		if cc.Flags().Lookup(name) == nil {
			t.Errorf("--%s flag not registered on check", name)
		}
	}
	for _, name := range []string{"checks", "dir"} {
		f := cc.Flags().Lookup(name)
		if f == nil {
			continue
		}
		if got := f.Annotations[cobraRequiredAnnotation]; len(got) != 1 || got[0] != "true" {
			t.Errorf("--%s must be required, annotations = %v", name, f.Annotations)
		}
	}
	backend := cc.Flags().Lookup("backend")
	if backend == nil {
		return
	}
	if backend.DefValue != inference.NameKiroCLI {
		t.Errorf("--backend default = %q, want %q", backend.DefValue, inference.NameKiroCLI)
	}
	for _, name := range inference.Names() {
		if !strings.Contains(backend.Usage, name) {
			t.Errorf("--backend usage %q does not list backend %q", backend.Usage, name)
		}
	}
}

// cobraRequiredAnnotation is the annotation cobra sets with MarkFlagRequired.
const cobraRequiredAnnotation = "cobra_annotation_bash_completion_one_required_flag"

func TestCheckPassFixture(t *testing.T) {
	fx := checkFixtures(t)
	out, err := runCheck(t, "--checks", filepath.Join(fx, "pass.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err != nil {
		t.Fatalf("pass.yaml: unexpected error %v\nstdout:\n%s", err, out)
	}
	lines := outputLines(out)
	if len(lines) != 4 {
		t.Fatalf("want one line per check (4), got %d:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "PASS ") {
			t.Errorf("line %q is not a PASS line", l)
		}
	}
}

func TestCheckFailFixture(t *testing.T) {
	fx := checkFixtures(t)
	out, err := runCheck(t, "--checks", filepath.Join(fx, "fail.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err == nil {
		t.Fatalf("fail.yaml: expected an error, got none\nstdout:\n%s", out)
	}
	if !strings.Contains(err.Error(), "1 of 2 checks failed") {
		t.Errorf("error = %q, want it to say '1 of 2 checks failed'", err)
	}
	lines := outputLines(out)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "PASS #1 file_exists path=hello.txt") {
		t.Errorf("line 1 = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "FAIL #2 file_exists path=missing.txt") {
		t.Errorf("line 2 = %q, want a FAIL line naming '#2 file_exists path=missing.txt'", lines[1])
	}
}

func TestCheckInvalidFixtureRunsNothing(t *testing.T) {
	fx := checkFixtures(t)
	out, err := runCheck(t, "--checks", filepath.Join(fx, "invalid.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err == nil {
		t.Fatalf("invalid.yaml: expected an error\nstdout:\n%s", out)
	}
	for _, want := range []string{"check #2", "file_contains"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if out != "" {
		t.Errorf("no result line may be printed for an invalid checks file, got:\n%s", out)
	}
}

func TestCheckInvalidDoesNotRunEarlierCommand(t *testing.T) {
	dir := t.TempDir()
	checksFile := filepath.Join(t.TempDir(), "checks.yaml")
	body := "- type: command\n  run: touch ran\n- type: file_contains\n  path: ran\n"
	if err := os.WriteFile(checksFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runCheck(t, "--checks", checksFile, "--dir", dir)
	if err == nil || !strings.Contains(err.Error(), "check #2") || !strings.Contains(err.Error(), "file_contains") {
		t.Fatalf("err = %v, want an invalid-checks error naming check #2 (file_contains)", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "ran")); statErr == nil {
		t.Fatal("command check #1 ran although check #2 is invalid: validation must finish before any check runs")
	}
}

func TestCheckJudgeStubBackendNoModelCall(t *testing.T) {
	fx := checkFixtures(t)
	inDir(t, t.TempDir())
	writeEvalsConfig(t, "evals:\n  judge_model: claude-sonnet-5.5\n")

	// The real stub backend is used (inference.New): it never calls a model, and
	// without a script it would answer "case has no stub.judge", so a pass proves
	// the command supplied the built-in yes script.
	out, err := runCheck(t, "--backend", "stub",
		"--checks", filepath.Join(fx, "judge.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err != nil {
		t.Fatalf("judge.yaml --backend stub: %v\nstdout:\n%s", err, out)
	}
	lines := outputLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS #1 judge") {
		t.Fatalf("want a single 'PASS #1 judge' line, got:\n%s", out)
	}
}

// recordingBackend answers every request "yes" and records the requests.
type recordingBackend struct {
	reqs []inference.Request
}

func (r *recordingBackend) Name() string                { return "recording" }
func (r *recordingBackend) Available() error            { return nil }
func (r *recordingBackend) StartupProbe() time.Duration { return 0 }
func (r *recordingBackend) Invoke(_ context.Context, req inference.Request) (inference.Response, error) {
	r.reqs = append(r.reqs, req)
	return inference.Response{
		Text: `===JSON_START==={"reasoning": "greets", "answer": "yes"}===JSON_END===`,
	}, nil
}

func TestCheckJudgeRequestShape(t *testing.T) {
	fx := checkFixtures(t)
	inDir(t, t.TempDir())
	writeEvalsConfig(t, "evals:\n  judge_model: claude-haiku-4.5\n")

	rec := &recordingBackend{}
	orig := checkNewBackend
	t.Cleanup(func() { checkNewBackend = orig })
	checkNewBackend = func(string) (inference.Backend, error) { return rec, nil }

	out, err := runCheck(t, "--checks", filepath.Join(fx, "judge.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err != nil {
		t.Fatalf("judge.yaml: %v\nstdout:\n%s", err, out)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("backend got %d requests, want 1", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.Role != inference.RoleJudge {
		t.Errorf("Role = %v, want RoleJudge", req.Role)
	}
	if !req.YesNo {
		t.Error("YesNo = false, want true")
	}
	if req.Model != "claude-haiku-4.5" {
		t.Errorf("Model = %q, want the configured evals.judge_model claude-haiku-4.5", req.Model)
	}
	if req.Stub != nil {
		t.Errorf("Stub = %+v, want nil for a non-stub backend", req.Stub)
	}
	if lines := outputLines(out); len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS #1 judge") || !strings.Contains(lines[0], "greets") {
		t.Errorf("want one PASS judge line carrying the reasoning, got:\n%s", out)
	}
}

func TestCheckJudgeModelNotAllowedRefusedBeforeAnyCheck(t *testing.T) {
	work := t.TempDir()
	inDir(t, t.TempDir())
	writeEvalsConfig(t, "evals:\n  judge_model: not-an-allowed-model\n")

	rec := &recordingBackend{}
	orig := checkNewBackend
	t.Cleanup(func() { checkNewBackend = orig })
	checkNewBackend = func(string) (inference.Backend, error) { return rec, nil }

	checksFile := filepath.Join(t.TempDir(), "checks.yaml")
	body := "- type: command\n  run: touch ran\n- type: judge\n  question: Is it fine?\n  files: [ran]\n"
	if err := os.WriteFile(checksFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCheck(t, "--checks", checksFile, "--dir", work)
	if err == nil || !strings.Contains(err.Error(), "evals.judge_model") {
		t.Fatalf("err = %v, want a refusal mentioning evals.judge_model", err)
	}
	if !strings.Contains(err.Error(), "not-an-allowed-model") {
		t.Errorf("err = %q does not name the refused model", err)
	}
	if out != "" {
		t.Errorf("no result line may be printed, got:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(work, "ran")); statErr == nil {
		t.Error("a check ran although the judge model was refused")
	}
	if len(rec.reqs) != 0 {
		t.Errorf("backend was called %d times, want 0", len(rec.reqs))
	}
}

func TestCheckWithoutJudgeNeverReadsEvalsConfig(t *testing.T) {
	fx := checkFixtures(t)
	inDir(t, t.TempDir())
	// Malformed YAML and a refused judge model: both would fail LoadEvals/CheckModel.
	writeEvalsConfig(t, "evals: [this is: not valid\n")

	out, err := runCheck(t, "--checks", filepath.Join(fx, "pass.yaml"), "--dir", filepath.Join(fx, "dir"))
	if err != nil {
		t.Fatalf("pass.yaml with a broken evals config: %v\nstdout:\n%s", err, out)
	}
}

func TestCheckEvidenceFlags(t *testing.T) {
	fx := checkFixtures(t)
	checksFile := filepath.Join(fx, "evidence.yaml")
	dir := filepath.Join(fx, "dir")

	t.Run("all flags pass", func(t *testing.T) {
		out, err := runCheck(t, "--checks", checksFile, "--dir", dir,
			"--output", filepath.Join(fx, "output.txt"),
			"--gh-log", filepath.Join(fx, "gh.log"),
			"--hidden-dir", filepath.Join(fx, "hidden"))
		if err != nil {
			t.Fatalf("evidence.yaml with all flags: %v\nstdout:\n%s", err, out)
		}
		if lines := outputLines(out); len(lines) != 3 {
			t.Errorf("want 3 PASS lines, got:\n%s", out)
		}
	})

	t.Run("without hidden-dir the inject check is invalid", func(t *testing.T) {
		out, err := runCheck(t, "--checks", checksFile, "--dir", dir,
			"--output", filepath.Join(fx, "output.txt"),
			"--gh-log", filepath.Join(fx, "gh.log"))
		if err == nil || !strings.Contains(err.Error(), "check #3") {
			t.Fatalf("err = %v, want an invalid-checks error naming check #3", err)
		}
		if out != "" {
			t.Errorf("nothing may run, got:\n%s", out)
		}
	})

	t.Run("without output and gh-log the evidence checks fail", func(t *testing.T) {
		out, err := runCheck(t, "--checks", checksFile, "--dir", dir,
			"--hidden-dir", filepath.Join(fx, "hidden"))
		if err == nil {
			t.Fatalf("expected failure without --output/--gh-log\nstdout:\n%s", out)
		}
		if !strings.Contains(out, "FAIL #1 output_contains") || !strings.Contains(out, "FAIL #2 gh_log_contains") {
			t.Errorf("want FAIL lines for #1 output_contains and #2 gh_log_contains, got:\n%s", out)
		}
	})
}

func TestCheckRejectsBadArgumentsBeforeAnyCheck(t *testing.T) {
	fx := checkFixtures(t)
	work := t.TempDir()
	checksFile := filepath.Join(t.TempDir(), "checks.yaml")
	if err := os.WriteFile(checksFile, []byte("- type: command\n  run: touch ran\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("unknown backend", func(t *testing.T) {
		out, err := runCheck(t, "--backend", "nope", "--checks", checksFile, "--dir", work)
		if err == nil {
			t.Fatal("expected an error for an unknown backend")
		}
		for _, want := range []string{"nope", inference.NameKiroCLI, inference.NameStub} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		if _, statErr := os.Stat(filepath.Join(work, "ran")); statErr == nil {
			t.Error("a check ran despite the unknown backend")
		}
	})

	t.Run("dir is not a directory", func(t *testing.T) {
		notDir := filepath.Join(fx, "pass.yaml")
		_, err := runCheck(t, "--checks", checksFile, "--dir", notDir)
		if err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("err = %v, want 'not a directory'", err)
		}
	})

	t.Run("dir does not exist", func(t *testing.T) {
		_, err := runCheck(t, "--checks", checksFile, "--dir", filepath.Join(work, "missing"))
		if err == nil {
			t.Fatal("expected an error for a missing --dir")
		}
	})

	t.Run("unreadable output file", func(t *testing.T) {
		_, err := runCheck(t, "--checks", checksFile, "--dir", work, "--output", filepath.Join(work, "nope.txt"))
		if err == nil {
			t.Fatal("expected an error for an unreadable --output")
		}
		if _, statErr := os.Stat(filepath.Join(work, "ran")); statErr == nil {
			t.Error("a check ran despite the unreadable --output")
		}
	})

	t.Run("missing required flags", func(t *testing.T) {
		_, err := runCheck(t)
		if err == nil || !strings.Contains(err.Error(), "required flag") {
			t.Fatalf("err = %v, want a required-flag error", err)
		}
	})
}

func TestCheckGHLogOversizedFailsGHLogChecks(t *testing.T) {
	work := t.TempDir()
	ghLog := filepath.Join(t.TempDir(), "gh.log")
	// "issue edit" is present in the first bytes, but the file is over the cap.
	big := append([]byte("issue edit\n"), bytes.Repeat([]byte("x"), 10<<20+1)...)
	if err := os.WriteFile(ghLog, big, 0o644); err != nil {
		t.Fatal(err)
	}
	checksFile := filepath.Join(t.TempDir(), "checks.yaml")
	if err := os.WriteFile(checksFile, []byte("- type: gh_log_contains\n  pattern: issue edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCheck(t, "--checks", checksFile, "--dir", work, "--gh-log", ghLog)
	if err == nil || !strings.HasPrefix(strings.TrimSpace(out), "FAIL #1 gh_log_contains") {
		t.Fatalf("an oversized gh log must fail gh_log checks explicitly; err=%v out=%q", err, out)
	}
}
