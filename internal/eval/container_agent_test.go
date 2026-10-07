package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
	"gopkg.in/yaml.v3"
)

// captureStdout runs fn and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	func() {
		defer func() {
			os.Stdout = orig
			_ = w.Close()
		}()
		fn()
	}()
	return <-done
}

// fakeExecer is an agentExecer standing in for a container. For the helper
// path it behaves like the real container would: it decodes the JSON request
// from stdin and runs the real backend through inference.ServeExec.
type fakeExecer struct {
	copies []string // "dest<-src"
	cmds   [][]string
	stdins []string

	// kiro-cli scripting.
	kiroStdout string
	kiroStderr string
	kiroExit   int

	// execErr, when set, is returned as a transport failure.
	execErr error
	// helperStdout, when set, replaces the helper's output.
	helperStdout *string
	helperStderr string
	helperExit   int
	// block makes ExecWithStdin wait for ctx cancellation.
	block bool
}

func (f *fakeExecer) CopyTo(_ context.Context, dest, src string) error {
	f.copies = append(f.copies, dest+"<-"+src)
	return nil
}

func (f *fakeExecer) ExecWithStdin(ctx context.Context, cmd []string, stdin io.Reader) (sandbox.ExecResult, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	f.cmds = append(f.cmds, cmd)
	f.stdins = append(f.stdins, string(data))

	if f.block {
		<-ctx.Done()
		return sandbox.ExecResult{}, ctx.Err()
	}
	if f.execErr != nil {
		return sandbox.ExecResult{}, f.execErr
	}

	if cmd[0] == "kiro-cli" {
		return sandbox.ExecResult{Stdout: f.kiroStdout, Stderr: f.kiroStderr, ExitCode: f.kiroExit}, nil
	}

	if f.helperStdout != nil {
		return sandbox.ExecResult{Stdout: *f.helperStdout, Stderr: f.helperStderr, ExitCode: f.helperExit}, nil
	}
	var out bytes.Buffer
	if err := inference.ServeExec(ctx, cmd[len(cmd)-1], strings.NewReader(string(data)), &out); err != nil {
		return sandbox.ExecResult{Stderr: err.Error(), ExitCode: 1}, nil
	}
	return sandbox.ExecResult{Stdout: out.String(), Stderr: f.helperStderr, ExitCode: f.helperExit}, nil
}

func testContainerConfig() *ContainerConfig {
	return &ContainerConfig{
		Platform:     "linux/arm64",
		WorkspaceDir: "/workspace",
		ResourceLimits: sandbox.ResourceLimits{
			Memory:  1024 * 1024 * 1024,
			Timeout: time.Minute,
		},
	}
}

// useFakeLinuxBinary replaces the helper-binary resolver and returns the path
// it hands out.
func useFakeLinuxBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kairon-linux")
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := resolveLinuxBinary
	resolveLinuxBinary = func(string) (string, error) { return bin, nil }
	t.Cleanup(func() { resolveLinuxBinary = orig })
	return bin
}

// useFakeContainer routes invokeAgent's container branch through x.
func useFakeContainer(t *testing.T, x agentExecer) {
	t.Helper()
	orig := invokeInContainer
	invokeInContainer = func(req inference.Request, c *ContainerConfig) (inference.Response, *ErrorContext, error) {
		return runAgentInContainer(context.Background(), x, req, c)
	}
	t.Cleanup(func() { invokeInContainer = orig })
}

func pinAgentModel(agent, model string) {
	cfg.pins = &runPins{Judge: "j", Agents: map[string]agentPin{agent: {Model: model}}}
}

// 1 MiB prompt with quotes, newlines and command substitutions.
func nastyPrompt() string {
	const unit = "PROMPT-MARKER \"double\" 'single' `tick`\n$(echo marker) ${HOME} \\n ; false && true || true | cat #\n"
	return strings.Repeat(unit, (1<<20)/len(unit)+1)[:1<<20]
}

func assertNoPromptInArgv(t *testing.T, cmds [][]string) {
	t.Helper()
	for _, cmd := range cmds {
		for _, arg := range cmd {
			if strings.Contains(arg, "PROMPT-MARKER") || strings.Contains(arg, "$(") || len(arg) > 1024 {
				t.Errorf("prompt leaked into argv element (len %d): %.80q", len(arg), arg)
			}
		}
	}
}

func TestContainerKiroCLIPromptOnStdinNotArgv(t *testing.T) {
	chdirTemp(t)
	pinAgentModel("builder", "m1")
	prompt := nastyPrompt()
	x := &fakeExecer{kiroStdout: "\x1b[1mhello\x1b[0m"}
	useFakeContainer(t, x)

	out, cost, rec, ec, err := invokeAgent("builder", prompt, testContainerConfig(), nil)
	if err != nil {
		t.Fatalf("invokeAgent: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q, want ANSI stripped", out)
	}
	if len(x.stdins) != 1 || x.stdins[0] != prompt {
		t.Fatalf("stdin did not carry the exact prompt (got %d bytes)", len(x.stdins[0]))
	}
	assertNoPromptInArgv(t, x.cmds)
	if len(x.copies) != 0 {
		t.Errorf("kiro-cli path must not copy a helper: %v", x.copies)
	}
	if ec != nil {
		t.Errorf("ec = %+v", ec)
	}
	if cost.UsageSource != "estimated" || cost.Model != "m1" || cost.TokensIn != len(prompt)/4 {
		t.Errorf("cost = %+v", cost)
	}
	if rec.Model != "m1" || !rec.Estimated || rec.Role != "agent" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestContainerHelperPromptOnStdinNotArgv(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	bin := useFakeLinuxBinary(t)
	prompt := nastyPrompt()
	x := &fakeExecer{}
	useFakeContainer(t, x)

	stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "scripted"}}}
	out, _, _, _, err := invokeAgent("selftest", prompt, testContainerConfig(), stub)
	if err != nil {
		t.Fatalf("invokeAgent: %v", err)
	}
	if out != "scripted" {
		t.Errorf("output = %q", out)
	}
	assertNoPromptInArgv(t, x.cmds)
	if len(x.stdins) != 1 || !strings.Contains(x.stdins[0], "PROMPT-MARKER") {
		t.Errorf("the JSON request on stdin must carry the prompt")
	}
	wantCmd := "/tmp/kairon inference-exec --backend stub"
	if got := strings.Join(x.cmds[0], " "); got != wantCmd {
		t.Errorf("cmd = %q, want %q", got, wantCmd)
	}
	if len(x.copies) != 1 || x.copies[0] != "/tmp/kairon<-"+bin {
		t.Errorf("copies = %v", x.copies)
	}
}

func TestContainerKiroCLIArgvMatchesNative(t *testing.T) {
	for _, model := range []string{"", "claude-sonnet-4.5"} {
		t.Run("model="+model, func(t *testing.T) {
			chdirTemp(t)
			_, calls := installFakeKiroCLI(t, "cat >/dev/null\necho ok")
			if model != "" {
				pinAgentModel("builder", model)
			}

			if _, _, _, _, err := invokeAgent("builder", "p", nil, nil); err != nil {
				t.Fatal(err)
			}
			native := readCalls(t, calls)
			if len(native) != 1 {
				t.Fatalf("native calls = %q", native)
			}

			x := &fakeExecer{kiroStdout: "ok"}
			useFakeContainer(t, x)
			if _, _, _, _, err := invokeAgent("builder", "p", testContainerConfig(), nil); err != nil {
				t.Fatal(err)
			}
			cmd := x.cmds[0]
			if cmd[0] != "kiro-cli" {
				t.Fatalf("program = %q", cmd[0])
			}
			if got := strings.Join(cmd[1:], " "); got != native[0] {
				t.Errorf("container argv %q != native %q", got, native[0])
			}
			hasModel := false
			for _, a := range cmd {
				hasModel = hasModel || a == "--model"
			}
			if model == "" && hasModel {
				t.Errorf("--model sent for an empty model: %v", cmd)
			}
			if model != "" && (len(cmd) < 2 || cmd[len(cmd)-2] != "--model" || cmd[len(cmd)-1] != model) {
				t.Errorf("argv does not end with --model %s: %v", model, cmd)
			}
		})
	}
}

// selftestEvalsAbs is resolved at package init (cwd = package dir) so tests
// that chdir to a temp dir can still read the checked-in fixtures.
var selftestEvalsAbs, _ = filepath.Abs(selftestEvalsDir)

func loadSelftestStub(t *testing.T, name string) *inference.StubScript {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(selftestEvalsAbs, "cases", "selftest", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var tc TestCase
	if err := yaml.Unmarshal(data, &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Stub == nil {
		t.Fatalf("%s has no stub", name)
	}
	return tc.Stub
}

func TestContainerStubMatchesNativeStub(t *testing.T) {
	for _, name := range []string{"stub-basic", "stub-usage"} {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			useStubBackend(t)
			useFakeLinuxBinary(t)
			pinAgentModel("selftest", "claude-sonnet-5.5")
			stub := loadSelftestStub(t, name)
			prompt := "prompt for " + name

			nOut, nCost, nRec, nEC, nErr := invokeAgent("selftest", prompt, nil, stub)
			if nErr != nil {
				t.Fatal(nErr)
			}

			useFakeContainer(t, &fakeExecer{})
			cOut, cCost, cRec, cEC, cErr := invokeAgent("selftest", prompt, testContainerConfig(), stub)
			if cErr != nil {
				t.Fatal(cErr)
			}

			if cOut != stub.Turns[0].Response || cOut != nOut {
				t.Errorf("output %q, native %q, scripted %q", cOut, nOut, stub.Turns[0].Response)
			}
			if cCost != nCost {
				t.Errorf("cost %+v != native %+v", cCost, nCost)
			}
			// Wall-clock duration legitimately differs; everything else must match.
			cRec.DurationMS, nRec.DurationMS = 0, 0
			if cRec != nRec {
				t.Errorf("record %+v != native %+v", cRec, nRec)
			}
			if cEC != nil || nEC != nil {
				t.Errorf("error contexts: container %+v native %+v", cEC, nEC)
			}
			if cCost.UsageSource == "" || cCost.Model != nCost.Model {
				t.Errorf("cost lost model/usage source: %+v", cCost)
			}
		})
	}
}

func TestContainerDebugLine(t *testing.T) {
	chdirTemp(t)
	pinAgentModel("builder", "m1")
	x := &fakeExecer{kiroStdout: "ok"}
	cc := testContainerConfig()

	cc.Debug = true
	out := captureStdout(t, func() {
		if _, _, err := runAgentInContainer(context.Background(), x, newAgentRequest("builder", "p", nil), cc); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "backend=kiro-cli") || !strings.Contains(out, "agent=builder") || !strings.Contains(out, "model=m1") {
		t.Errorf("debug line missing: %q", out)
	}

	cc.Debug = false
	out = captureStdout(t, func() {
		if _, _, err := runAgentInContainer(context.Background(), x, newAgentRequest("builder", "p", nil), cc); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "Debug") {
		t.Errorf("debug line printed without Debug: %q", out)
	}
}

func TestContainerNoEstimateCostOrGuessedRecord(t *testing.T) {
	// The container record is built by the shared completion, so it is only
	// "estimated" when the usage source says so.
	chdirTemp(t)
	useStubBackend(t)
	useFakeLinuxBinary(t)
	useFakeContainer(t, &fakeExecer{})
	_, _, rec, _, err := invokeAgent("selftest", "p", testContainerConfig(), loadSelftestStub(t, "stub-usage"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Estimated {
		t.Errorf("reported usage recorded as estimated: %+v", rec)
	}
	if rec.InputTokens != 123 || rec.OutputTokens != 45 || rec.Model != "stub-model" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestContainerTimeoutMapping(t *testing.T) {
	chdirTemp(t)
	cc := testContainerConfig()
	cc.ResourceLimits.Timeout = 30 * time.Millisecond
	x := &fakeExecer{block: true}
	useFakeContainer(t, x)

	_, _, rec, ec, err := invokeAgent("builder", "p", cc, nil)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !errors.Is(err, inference.ErrTimeout) {
		t.Errorf("err does not satisfy ErrTimeout: %v", err)
	}
	if !strings.Contains(err.Error(), "Container execution timeout after 30ms") || !strings.Contains(err.Error(), "--resource-limit timeout=") {
		t.Errorf("message = %q", err.Error())
	}
	if ec == nil || !strings.HasPrefix(ec.Stderr, "timeout after 30ms") {
		t.Errorf("ec = %+v", ec)
	}
	if ec != nil && (ec.Platform != "linux/arm64" || ec.WorkingDir != "/workspace") {
		t.Errorf("container context missing: %+v", ec)
	}
	if rec.Error == "" {
		t.Errorf("failed call not recorded: %+v", rec)
	}
}

func TestContainerHelperReportedTimeout(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	useFakeLinuxBinary(t)
	env := `{"response":{},"error":"stub timeout","timeout":true}`
	x := &fakeExecer{helperStdout: &env}
	useFakeContainer(t, x)

	_, _, _, _, err := invokeAgent("selftest", "p", testContainerConfig(), nil)
	if !errors.Is(err, inference.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestContainerErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		x    *fakeExecer
		want string
	}{
		{"oom exit", &fakeExecer{kiroExit: 137, kiroStderr: "Killed: OOMKilled"}, "Container ran out of memory (1024MB limit)"},
		{"oom transport", &fakeExecer{execErr: errors.New("container out of memory")}, "Container ran out of memory (1024MB limit)"},
		{"pull", &fakeExecer{execErr: errors.New("no such image: foo")}, "Failed to pull image"},
		{"timeout text", &fakeExecer{execErr: errors.New("exec timeout")}, "Container execution timeout after 1m0s"},
		{"other transport", &fakeExecer{execErr: errors.New("boom")}, "container execution failed: boom"},
		{"kiro exit", &fakeExecer{kiroExit: 2, kiroStderr: "bad things"}, "kiro-cli invocation failed: exit status 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirTemp(t)
			useFakeContainer(t, tt.x)
			_, _, rec, ec, err := invokeAgent("builder", "p", testContainerConfig(), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
			if rec.Error == "" {
				t.Errorf("failure not recorded: %+v", rec)
			}
			if ec == nil {
				t.Fatal("expected an error context")
			}
		})
	}

	t.Run("exit code and stderr reach the context", func(t *testing.T) {
		chdirTemp(t)
		useFakeContainer(t, &fakeExecer{kiroExit: 2, kiroStderr: "bad things"})
		_, _, _, ec, _ := invokeAgent("builder", "p", testContainerConfig(), nil)
		if ec == nil || ec.ExitCode != 2 || !strings.Contains(ec.Stderr, "bad things") || !strings.HasPrefix(ec.Command, "kiro-cli chat") {
			t.Errorf("ec = %+v", ec)
		}
	})
}

func TestContainerHelperFailures(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	useFakeLinuxBinary(t)

	t.Run("backend error", func(t *testing.T) {
		useFakeContainer(t, &fakeExecer{})
		_, _, rec, _, err := invokeAgent("selftest", "p", testContainerConfig(), nil) // no stub script
		if err == nil || !strings.Contains(err.Error(), "no stub.turns[0].response") || errors.Is(err, inference.ErrTimeout) {
			t.Fatalf("err = %v", err)
		}
		if rec.Error == "" {
			t.Error("not recorded")
		}
	})

	t.Run("unreadable output", func(t *testing.T) {
		garbage := "not json"
		useFakeContainer(t, &fakeExecer{helperStdout: &garbage, helperStderr: "panic: x"})
		_, _, _, ec, err := invokeAgent("selftest", "p", testContainerConfig(), nil)
		if err == nil || !strings.Contains(err.Error(), "unreadable result") {
			t.Fatalf("err = %v", err)
		}
		if ec == nil || !strings.Contains(ec.Stderr, "panic: x") {
			t.Errorf("ec = %+v", ec)
		}
	})

	t.Run("helper exit", func(t *testing.T) {
		empty := ""
		useFakeContainer(t, &fakeExecer{helperStdout: &empty, helperExit: 1, helperStderr: "unknown backend"})
		_, _, _, _, err := invokeAgent("selftest", "p", testContainerConfig(), nil)
		if err == nil || !strings.Contains(err.Error(), "stub helper failed: exit status 1: unknown backend") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("no linux binary", func(t *testing.T) {
		orig := resolveLinuxBinary
		resolveLinuxBinary = func(string) (string, error) { return "", fmt.Errorf("set KAIRON_SANDBOX_BINARY") }
		t.Cleanup(func() { resolveLinuxBinary = orig })
		x := &fakeExecer{}
		useFakeContainer(t, x)
		_, _, _, _, err := invokeAgent("selftest", "p", testContainerConfig(), nil)
		if err == nil || !strings.Contains(err.Error(), "KAIRON_SANDBOX_BINARY") {
			t.Fatalf("err = %v", err)
		}
		if len(x.cmds) != 0 {
			t.Error("exec ran without a helper binary")
		}
	})
}

func TestSharedRequestBuilder(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	pinAgentModel("builder", "m1")
	t.Setenv("KAIRON_EVAL_TIMEOUT", "7s")
	stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "r"}}}
	req := newAgentRequest("builder", "prompt", stub)
	if req.Role != inference.RoleAgent || req.Agent != "builder" || req.Prompt != "prompt" ||
		req.Model != "m1" || req.Timeout != 7*time.Second || req.Stub != stub {
		t.Errorf("req = %+v", req)
	}

	// The container exec is bounded by the sandbox timeout, which also reaches
	// the in-container backend through the request.
	useFakeLinuxBinary(t)
	x := &fakeExecer{}
	useFakeContainer(t, x)
	cc := testContainerConfig()
	cc.ResourceLimits.Timeout = 3 * time.Minute
	if _, _, _, _, err := invokeAgent("builder", "p", cc, stub); err != nil {
		t.Fatal(err)
	}
	var sent inference.Request
	if err := json.Unmarshal([]byte(x.stdins[0]), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Timeout != 3*time.Minute || sent.Model != "m1" || sent.AgentConfigDir != "" || sent.Stub == nil {
		t.Errorf("request on the wire = %+v", sent)
	}
}
