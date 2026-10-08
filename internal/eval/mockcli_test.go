package eval

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mockCLIScript is the live reusable fake-CLI fixture, relative to the repo
// root.
const mockCLIScript = ".kairon/evals/fixtures/mock-cli.sh"

// mockCLIShells returns the POSIX shells to run the fixture under: sh always
// (skipping when absent), plus dash and busybox sh when installed.
func mockCLIShells(t *testing.T) map[string][]string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh fixture")
	}
	shells := map[string][]string{}
	if p, err := exec.LookPath("sh"); err == nil {
		shells["sh"] = []string{p}
	}
	if p, err := exec.LookPath("dash"); err == nil {
		shells["dash"] = []string{p}
	}
	if p, err := exec.LookPath("busybox"); err == nil {
		shells["busybox"] = []string{p, "sh"}
	}
	if len(shells) == 0 {
		t.Skip("no POSIX shell available")
	}
	return shells
}

// mockCLIEnv is one hermetic mock installation: a workspace holding .eval/
// and .mocks/aws/, and a bin dir with the script installed as "aws".
type mockCLIEnv struct {
	t         *testing.T
	shell     []string
	workspace string
	evalDir   string
	dataDir   string
	script    string
	extraPath string
	env       []string
}

func newMockCLIEnv(t *testing.T, shell []string) *mockCLIEnv {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(mockCLIScript)))
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	e := &mockCLIEnv{
		t:         t,
		shell:     shell,
		workspace: ws,
		evalDir:   filepath.Join(ws, ".eval"),
		dataDir:   filepath.Join(ws, ".mocks", "aws"),
		script:    filepath.Join(t.TempDir(), "aws"),
	}
	for _, d := range []string{e.evalDir, e.dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(e.script, src, 0o755); err != nil {
		t.Fatal(err)
	}
	e.env = []string{"KAIRON_EVAL_DIR=" + e.evalDir}
	return e
}

// data writes a canned reply file into the default data dir.
func (e *mockCLIEnv) data(name, content string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.dataDir, name), []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// run executes the script as "aws" with args and returns stdout, stderr and
// the exit status.
func (e *mockCLIEnv) run(args ...string) (stdout, stderr string, code int) {
	e.t.Helper()
	argv := append(append([]string{}, e.shell[1:]...), e.script)
	argv = append(argv, args...)
	c := exec.Command(e.shell[0], argv...)
	pathEnv := "PATH=" + os.Getenv("PATH")
	if e.extraPath != "" {
		pathEnv = "PATH=" + e.extraPath + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	c.Env = append([]string{pathEnv, "HOME=" + e.workspace}, e.env...)
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	err := c.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		e.t.Fatalf("run %v: %v", argv, err)
	}
	return so.String(), se.String(), code
}

func (e *mockCLIEnv) log() string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.evalDir, "mock-aws.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		e.t.Fatal(err)
	}
	return string(b)
}

// forEachMockShell runs fn once per available shell as a subtest.
func forEachMockShell(t *testing.T, fn func(t *testing.T, e *mockCLIEnv)) {
	t.Helper()
	for name, sh := range mockCLIShells(t) {
		t.Run(name, func(t *testing.T) {
			fn(t, newMockCLIEnv(t, sh))
		})
	}
}

func TestMockCLISyntaxAndHeader(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(mockCLIScript)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.HasPrefix(text, "#!/bin/sh\n") {
		t.Error("script must start with #!/bin/sh")
	}
	header, code := text, ""
	if i := strings.Index(text, "\nLC_ALL="); i > 0 {
		header, code = text[:i], text[i:]
	}
	for _, want := range []string{
		"INSTALL AS <cmd>", "DATA LAYOUT", "LOOKUP ORDER", ".rc",
		"mock-<cmd>.log", "LIMITS", "KAIRON_MOCK_DATA", "default.out",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("header comment does not document %q", want)
		}
	}
	for _, bad := range []string{"[[", "local ", "#!/bin/bash", "declare ", "<<<", "$'"} {
		if strings.Contains(code, bad) {
			t.Errorf("script uses non-POSIX construct %q", bad)
		}
	}
	for name, sh := range mockCLIShells(t) {
		argv := append(append([]string{}, sh[1:]...), "-n", filepath.Join(repoRoot(t), filepath.FromSlash(mockCLIScript)))
		if out, err := exec.Command(sh[0], argv...).CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s", name, err, out)
		}
	}
}

func TestMockCLITemplateParity(t *testing.T) {
	root := repoRoot(t)
	live, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(mockCLIScript)))
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := os.ReadFile(filepath.Join(root, "cmd", "kairon", "templates", "kairon", "evals", "fixtures", "mock-cli.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(live, tmpl) {
		t.Error("template mock-cli.sh differs from the live fixture; re-run the sync commands")
	}
}

func TestMockCLILogsOneLinePerCall(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("default.out", "ok\n")
		if _, _, code := e.run("s3", "cp", "a.txt", "s3://b/k"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		if got, want := e.log(), "aws s3 cp a.txt s3://b/k\n"; got != want {
			t.Fatalf("log = %q, want %q", got, want)
		}
		e.run("sts", "get-caller-identity")
		if got, want := e.log(), "aws s3 cp a.txt s3://b/k\naws sts get-caller-identity\n"; got != want {
			t.Fatalf("log = %q, want %q", got, want)
		}
		e.run()
		if got := strings.Count(e.log(), "\n"); got != 3 {
			t.Fatalf("log has %d lines, want 3: %q", got, e.log())
		}
		entries, _ := os.ReadDir(e.evalDir)
		if len(entries) != 1 || entries[0].Name() != "mock-aws.log" {
			t.Fatalf("eval dir should only hold mock-aws.log, got %v", entries)
		}
	})
}

func TestMockCLILogQuoting(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.run("echo", "hello world", "it's", "", "line1\nline2", "a;b", "$(x)")
		want := "aws echo 'hello world' 'it'\\''s' '' 'line1 line2' 'a;b' '$(x)'\n"
		if got := e.log(); got != want {
			t.Fatalf("log = %q, want %q", got, want)
		}
	})
}

func TestMockCLIReplyLookupOrder(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("default.out", "default\n")
		out, _, code := e.run("s3", "cp")
		if out != "default\n" || code != 0 {
			t.Fatalf("default: out=%q code=%d", out, code)
		}
		e.data("s3.out", "a1\n")
		if out, _, _ = e.run("s3", "cp"); out != "a1\n" {
			t.Fatalf("a1.out should beat default.out, got %q", out)
		}
		e.data("s3-cp.out", "a1-a2\n")
		if out, _, _ = e.run("s3", "cp"); out != "a1-a2\n" {
			t.Fatalf("a1-a2.out should beat a1.out, got %q", out)
		}
		// Another a2 falls back to a1.out, another a1 to default.out.
		if out, _, _ = e.run("s3", "ls"); out != "a1\n" {
			t.Fatalf("s3 ls should use s3.out, got %q", out)
		}
		if out, _, _ = e.run("ec2", "describe"); out != "default\n" {
			t.Fatalf("ec2 describe should use default.out, got %q", out)
		}
	})
}

func TestMockCLISkipsLeadingFlags(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("s3-cp.out", "copied\n")
		e.data("default.out", "default\n")
		out, _, _ := e.run("--no-cli-pager", "--debug", "s3", "cp", "--recursive", "x", "y")
		if out != "copied\n" {
			t.Fatalf("flags must be skipped when choosing a1/a2, got %q", out)
		}
		// Flag values are not skipped: "x" becomes a1.
		if out, _, _ = e.run("--profile", "x", "s3", "cp"); out != "default\n" {
			t.Fatalf("a flag value is a1, expected default.out, got %q", out)
		}
	})
}

func TestMockCLISanitizesArguments(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		secret := filepath.Join(e.workspace, "secret")
		if err := os.WriteFile(secret+".out", []byte("LEAK\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		e.data("default.out", "default\n")
		e.data("s3-cp.out", "s3-cp\n")
		// Characters outside [A-Za-z0-9_] are dropped: "s.3" and "c-p" select s3-cp.
		if out, _, _ := e.run("s.3", "c-p"); out != "s3-cp\n" {
			t.Fatalf("sanitised lookup: got %q", out)
		}
		// Traversal cannot reach ../../secret.out (relative to the data dir).
		for _, args := range [][]string{
			{"../../secret"},
			{"..", "..", "secret"},
			{"../../../secret", "x"},
			{secret},
			{"/", "etc/passwd"},
		} {
			out, _, _ := e.run(args...)
			if strings.Contains(out, "LEAK") {
				t.Fatalf("args %q escaped the data dir: %q", args, out)
			}
		}
		// Data under the sanitised name inside the data dir is still found.
		e.data("secret.out", "inside\n")
		if out, _, _ := e.run("../../secret"); out != "inside\n" {
			t.Fatalf("expected sanitised name to resolve inside the data dir, got %q", out)
		}
	})
}

func TestMockCLIExitStatusFromRC(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("iam-delete.out", "AccessDenied\n")
		e.data("iam-delete.rc", "255\n")
		out, _, code := e.run("iam", "delete")
		if out != "AccessDenied\n" || code != 255 {
			t.Fatalf("out=%q code=%d, want AccessDenied and 255", out, code)
		}
		e.data("ok.out", "fine\n")
		if _, _, code = e.run("ok"); code != 0 {
			t.Fatalf("no .rc should exit 0, got %d", code)
		}
		e.data("zero.out", "z\n")
		e.data("zero.rc", "0\n")
		if _, _, code = e.run("zero"); code != 0 {
			t.Fatalf(".rc 0 should exit 0, got %d", code)
		}
		e.data("bad.out", "x\n")
		e.data("bad.rc", "banana\n")
		if _, stderr, code := e.run("bad"); code != 1 || !strings.Contains(stderr, "invalid exit status") {
			t.Fatalf("invalid .rc: code=%d stderr=%q", code, stderr)
		}
		// An .rc without an .out is not a match.
		e.data("orphan.rc", "7\n")
		if _, stderr, code := e.run("orphan"); code != 1 || !strings.Contains(stderr, "is not simulated") {
			t.Fatalf("orphan .rc: code=%d stderr=%q", code, stderr)
		}
	})
}

func TestMockCLIUnsimulatedCall(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		out, stderr, code := e.run("iam", "delete-user", "--user-name", "prod admin")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if out != "" {
			t.Fatalf("stdout = %q, want empty", out)
		}
		if !strings.Contains(stderr, "is not simulated (call logged only)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if want := "aws iam delete-user --user-name 'prod admin'\n"; e.log() != want {
			t.Fatalf("log = %q, want %q", e.log(), want)
		}
	})
}

func TestMockCLIEvalDirErrors(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.env = nil
		_, stderr, code := e.run("s3", "ls")
		if code != 1 || !strings.Contains(stderr, "KAIRON_EVAL_DIR is not set") {
			t.Fatalf("unset: code=%d stderr=%q", code, stderr)
		}
		e.env = []string{"KAIRON_EVAL_DIR=" + filepath.Join(e.workspace, "missing")}
		_, stderr, code = e.run("s3", "ls")
		if code != 1 || !strings.Contains(stderr, "KAIRON_EVAL_DIR is not a directory") {
			t.Fatalf("invalid: code=%d stderr=%q", code, stderr)
		}
		file := filepath.Join(e.workspace, "afile")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		e.env = []string{"KAIRON_EVAL_DIR=" + file}
		if _, stderr, code = e.run("s3", "ls"); code != 1 || !strings.Contains(stderr, "not a directory") {
			t.Fatalf("file: code=%d stderr=%q", code, stderr)
		}
	})
}

func TestMockCLIMockDataOverride(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("default.out", "workspace-data\n")
		other := t.TempDir()
		if err := os.WriteFile(filepath.Join(other, "default.out"), []byte("override\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		e.env = append(e.env, "KAIRON_MOCK_DATA="+other)
		if out, _, code := e.run("s3", "ls"); out != "override\n" || code != 0 {
			t.Fatalf("out=%q code=%d, want override", out, code)
		}
		// An override dir without replies means not simulated, even though the
		// workspace .mocks dir has default.out.
		e.env[len(e.env)-1] = "KAIRON_MOCK_DATA=" + t.TempDir()
		if _, stderr, code := e.run("s3", "ls"); code != 1 || !strings.Contains(stderr, "is not simulated") {
			t.Fatalf("empty override: code=%d stderr=%q", code, stderr)
		}
	})
}

func TestMockCLIDefaultDataDirIsWorkspaceMocks(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		e.data("s3-ls.out", "from .mocks/aws\n")
		if out, _, _ := e.run("s3", "ls"); out != "from .mocks/aws\n" {
			t.Fatalf("out = %q", out)
		}
	})
}

// TestMockCLINeverRunsRealCommand puts a canary named like the mocked command
// on PATH and asserts it is never executed, for matched and unmatched calls.
func TestMockCLINeverRunsRealCommand(t *testing.T) {
	forEachMockShell(t, func(t *testing.T, e *mockCLIEnv) {
		canaryDir := t.TempDir()
		marker := filepath.Join(canaryDir, "canary-ran")
		canary := "#!/bin/sh\n: > '" + marker + "'\nexit 0\n"
		if err := os.WriteFile(filepath.Join(canaryDir, "aws"), []byte(canary), 0o755); err != nil {
			t.Fatal(err)
		}
		e.extraPath = canaryDir
		e.data("s3-cp.out", "mocked\n")
		e.run("s3", "cp", "a", "b")
		e.run("iam", "delete-user")
		e.run()
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("the canary aws on PATH was executed")
		}
	})
}
