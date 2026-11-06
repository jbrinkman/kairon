package eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// Tests for the run context (issue #490): RunOptions.Context reaches every
// Backend.Invoke in this package, and an interrupted run stops without saving
// the cut-off case.

type runCtxKey struct{}

// ctxBackend records the context and request of every Invoke and answers via
// fn, which receives the 0-based call index.
type ctxBackend struct {
	mu   sync.Mutex
	ctxs []context.Context
	reqs []inference.Request
	fn   func(n int, req inference.Request) (inference.Response, error)
}

func (b *ctxBackend) Name() string                { return "ctx-recording" }
func (b *ctxBackend) Available() error            { return nil }
func (b *ctxBackend) StartupProbe() time.Duration { return 0 }

func (b *ctxBackend) Invoke(ctx context.Context, req inference.Request) (inference.Response, error) {
	b.mu.Lock()
	n := len(b.reqs)
	b.ctxs = append(b.ctxs, ctx)
	b.reqs = append(b.reqs, req)
	b.mu.Unlock()
	return b.fn(n, req)
}

// calls returns how many Invokes had the given role.
func (b *ctxBackend) calls(role inference.Role) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, r := range b.reqs {
		if r.Role == role {
			n++
		}
	}
	return n
}

// total returns the number of Invokes of any role.
func (b *ctxBackend) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.reqs)
}

// cancelOnFirstAgentCall returns a ctxBackend whose first agent call cancels
// the run and fails, as a killed kiro-cli does. Every other call is answered
// by the stub backend.
func cancelOnFirstAgentCall(t *testing.T, cancel context.CancelFunc) *ctxBackend {
	t.Helper()
	stub, err := inference.New(inference.NameStub)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	b := &ctxBackend{}
	b.fn = func(_ int, req inference.Request) (inference.Response, error) {
		if req.Role == inference.RoleAgent {
			fired := false
			once.Do(func() { fired = true })
			if fired {
				cancel()
				return inference.Response{}, errors.New("kiro-cli killed")
			}
		}
		return stub.Invoke(context.Background(), req)
	}
	return b
}

func TestInvokeCallersUseRunContext(t *testing.T) {
	sim := &SimulatedUser{Facts: []Fact{{ID: "f1", Answer: "a"}}}
	callers := []struct {
		name string
		call func()
	}{
		{"agent", func() {
			invokeAgent("selftest", "prompt", nil, callOpts{})
		}},
		{"judge", func() {
			scoreLLMJudge(Criterion{Name: "clarity", Description: "d", Scoring: "1-5"}, TestCase{Name: "c", Input: "in"}, "out")
		}},
		{"router", func() {
			var cr CaseResult
			route := newRouter(TestCase{Name: "c", SimulatedUser: sim}, &cr)
			_, _ = route(0, "agent output")
		}},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			t.Cleanup(resetConfig)
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), runCtxKey{}, "run-value"))
			defer cancel()
			if err := configure(RunOptions{Backend: inference.NameStub, Context: ctx}); err != nil {
				t.Fatal(err)
			}
			b := &ctxBackend{fn: func(int, inference.Request) (inference.Response, error) {
				return inference.Response{}, errors.New("not scored")
			}}
			cfg.backend = b

			c.call()

			if b.total() != 1 {
				t.Fatalf("backend got %d calls, want 1", b.total())
			}
			got := b.ctxs[0]
			if v := got.Value(runCtxKey{}); v != "run-value" {
				t.Errorf("backend context value = %v, want the run context's %q", v, "run-value")
			}
			if err := got.Err(); err != nil {
				t.Fatalf("backend context already done before cancel: %v", err)
			}
			cancel()
			if !errors.Is(got.Err(), context.Canceled) {
				t.Errorf("backend context Err after cancelling the run context = %v, want context.Canceled", got.Err())
			}
		})
	}
}

func TestCallContext_NilIsBackground(t *testing.T) {
	if got := (runConfig{}).callContext(); got != context.Background() {
		t.Errorf("callContext with a nil ctx = %v, want context.Background()", got)
	}

	ctx := context.WithValue(context.Background(), runCtxKey{}, "v")
	if got := (runConfig{ctx: ctx}).callContext(); got != ctx {
		t.Errorf("callContext with ctx set = %v, want that ctx", got)
	}

	// configure carries RunOptions.Context into cfg, and a run without a
	// Context leaves cfg.ctx nil.
	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: inference.NameStub, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	if cfg.callContext() != ctx {
		t.Error("configure did not install RunOptions.Context")
	}
	if err := configure(RunOptions{Backend: inference.NameStub}); err != nil {
		t.Fatal(err)
	}
	if cfg.ctx != nil || cfg.callContext() != context.Background() {
		t.Errorf("configure without a Context left ctx = %v, want nil (Background)", cfg.ctx)
	}
}

func TestRunInterrupted(t *testing.T) {
	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: inference.NameStub}); err != nil {
		t.Fatal(err)
	}
	if err := runInterrupted(); err != nil {
		t.Fatalf("nil run context: runInterrupted = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := configure(RunOptions{Backend: inference.NameStub, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	if err := runInterrupted(); err != nil {
		t.Fatalf("live context: runInterrupted = %v, want nil", err)
	}
	cancel()
	err := runInterrupted()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: runInterrupted = %v, want an error wrapping context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("error %q lacks the text %q", err, "interrupted")
	}
}

func TestExecuteCaseRepeats_StopsOnInterrupt(t *testing.T) {
	execEnv(t, false)
	t.Cleanup(resetConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := configure(RunOptions{Backend: inference.NameStub, EvalsDir: "evals", Repeat: 3, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	b := cancelOnFirstAgentCall(t, cancel)
	cfg.backend = b

	var out strings.Builder
	crs := executeCaseRepeats(execRubric(), stubCase("c", "", "", "done"), nil, &out)

	if got := b.calls(inference.RoleAgent); got != 1 {
		t.Errorf("agent calls = %d, want 1: no repeat starts after the run is cancelled", got)
	}
	if len(crs) > 1 {
		t.Errorf("executeCaseRepeats returned %d results, want at most 1", len(crs))
	}
}

func TestRunSingleTestCase_InterruptWritesNoResult(t *testing.T) {
	env := newCandidateE2EEnv(t)
	t.Cleanup(resetConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := configure(RunOptions{Backend: inference.NameStub, EvalsDir: env.evalsDir, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	b := cancelOnFirstAgentCall(t, cancel)
	cfg.backend = b

	var err error
	captureStdout(t, func() {
		err = runSingleTestCase("selftest-checks", "check-one-criterion", nil)
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("runSingleTestCase error = %v, want one wrapping context.Canceled", err)
	}
	files, _ := filepath.Glob(filepath.Join(env.evalsDir, "results", "*", "*.json"))
	if len(files) != 0 {
		t.Errorf("an interrupted single-case run wrote result files: %v", files)
	}
}

func TestRunProgressive_InterruptStopsAndKeepsResumable(t *testing.T) {
	env := newCandidateE2EEnv(t)
	t.Cleanup(resetConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := configure(RunOptions{Backend: inference.NameStub, EvalsDir: env.evalsDir, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	b := cancelOnFirstAgentCall(t, cancel)
	cfg.backend = b

	// selftest-checks has two cases and no judge criterion that needs a call.
	var err error
	captureStdout(t, func() {
		err = Run("selftest-checks", nil)
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want one wrapping context.Canceled", err)
	}
	if got := b.total(); got != 1 {
		t.Errorf("backend calls = %d, want 1: the run must not start the second case", got)
	}

	dirs, _ := filepath.Glob(filepath.Join(env.evalsDir, "results", "*"))
	if len(dirs) != 1 {
		t.Fatalf("result dirs = %v, want exactly 1", dirs)
	}
	if _, statErr := os.Stat(filepath.Join(dirs[0], ".progress")); statErr != nil {
		t.Errorf(".progress is gone, so --resume could not continue the run: %v", statErr)
	}
	// The cut-off case must not be saved: a saved case is skipped by --resume.
	data, readErr := os.ReadFile(filepath.Join(dirs[0], "selftest-checks.json"))
	if readErr == nil {
		var saved AgentResult
		if jerr := json.Unmarshal(data, &saved); jerr != nil {
			t.Fatal(jerr)
		}
		if len(saved.Cases) != 0 {
			names := make([]string, len(saved.Cases))
			for i, c := range saved.Cases {
				names[i] = c.CaseName
			}
			t.Errorf("saved agent JSON holds cases %v, want none (the interrupted case must not be saved)", names)
		}
	} else if !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
}

// TestCheckJudge_UsesRunContext: the judge built by newCheckJudge hands the
// backend a context derived from RunOptions.Context.
func TestCheckJudge_UsesRunContext(t *testing.T) {
	t.Cleanup(resetConfig)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), runCtxKey{}, "run-value"))
	defer cancel()
	if err := configure(RunOptions{Backend: inference.NameStub, Context: ctx}); err != nil {
		t.Fatal(err)
	}
	b := &ctxBackend{fn: func(int, inference.Request) (inference.Response, error) {
		return inference.Response{}, errors.New("not scored")
	}}
	cfg.backend = b

	var cr CaseResult
	_, _ = newCheckJudge(TestCase{Name: "c"}, &cr)(JudgeQuery{Criterion: "crit", Question: "q"})

	if b.total() != 1 {
		t.Fatalf("backend got %d calls, want 1", b.total())
	}
	got := b.ctxs[0]
	if v := got.Value(runCtxKey{}); v != "run-value" {
		t.Errorf("backend context value = %v, want the run context's %q", v, "run-value")
	}
	if err := got.Err(); err != nil {
		t.Fatalf("backend context already done before cancel: %v", err)
	}
	cancel()
	if !errors.Is(got.Err(), context.Canceled) {
		t.Errorf("backend context Err after cancelling the run context = %v, want context.Canceled", got.Err())
	}
}
