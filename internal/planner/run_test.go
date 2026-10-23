package planner

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeInvoker records every call and replays canned replies, repeating the
// last one when the script runs out.
type fakeInvoker struct {
	mu      sync.Mutex
	agents  []string
	prompts []string
	dirs    []string
	replies []string
	err     error
}

func (f *fakeInvoker) invoke(_ context.Context, agent, prompt, dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents = append(f.agents, agent)
	f.prompts = append(f.prompts, prompt)
	f.dirs = append(f.dirs, dir)
	if f.err != nil {
		return "", f.err
	}
	i := len(f.agents) - 1
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return f.replies[i], nil
}

func automatedRequest(maxRetries int) AutomatedRequest {
	return AutomatedRequest{
		Repo:             "owner/repo",
		Issue:            42,
		FailLabel:        failLabel,
		ObservedRevision: "rev-1",
		MaxRetries:       maxRetries,
		Dir:              "/work/tree",
	}
}

// missingContextBody is a canonical body the structural guard rejects.
func missingContextBody() string {
	return strings.Replace(validBody("Dependencies: none\n"), "## Context\n\nExporters live in internal/export.\n\n", "", 1)
}

func TestRunAutomatedAlwaysInvokesPlannerAutomated(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	inv := &fakeInvoker{replies: []string{canonicalReply(t, validBody("Dependencies: none\n"))}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(2))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomeCommitted || res.Attempts != 1 {
		t.Fatalf("result = %+v, want committed after 1 attempt", res)
	}
	if !slices.Equal(inv.agents, []string{AgentAutomated}) {
		t.Errorf("invoked agents = %v, want exactly [%s]", inv.agents, AgentAutomated)
	}
	if AgentAutomated != "planner-automated" {
		t.Errorf("AgentAutomated = %q", AgentAutomated)
	}
	if inv.dirs[0] != "/work/tree" {
		t.Errorf("dir = %q, want the request dir", inv.dirs[0])
	}
	if !v1MarkerEnd.MatchString(h.store.bodies[42]) {
		t.Errorf("committed body does not end with a v1 marker:\n%s", h.store.bodies[42])
	}
	if !slices.Equal(h.store.calls, []string{"Revision", "Update"}) {
		t.Errorf("store calls = %v, want an update of the observed issue", h.store.calls)
	}
}

func TestRunAutomatedPromptNamesRepoAndIssue(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	inv := &fakeInvoker{replies: []string{canonicalReply(t, validBody("Dependencies: none\n"))}}

	if _, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(1)); err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if !strings.Contains(inv.prompts[0], "owner/repo#42") {
		t.Errorf("prompt does not name the issue as owner/repo#42:\n%s", inv.prompts[0])
	}
	if strings.Contains(strings.ToLower(inv.prompts[0]), "interview") {
		t.Errorf("an unattended prompt must not invite a conversation:\n%s", inv.prompts[0])
	}
}

func TestInvokerArgvIsAutomatedAndNeverTrustsAllTools(t *testing.T) {
	t.Parallel()
	argv := KiroArgs(AgentAutomated, "Canonicalize owner/repo#42")
	want := []string{"chat", "--agent", "planner-automated", "--no-interactive", "Canonicalize owner/repo#42"}
	if !slices.Equal(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
	for _, a := range argv {
		if strings.Contains(a, "trust") {
			t.Errorf("argv contains %q; the manifest's allowedTools must bound the run", a)
		}
	}
	if KiroCommand != "kiro-cli" {
		t.Errorf("KiroCommand = %q, want kiro-cli", KiroCommand)
	}
}

func TestInvokerProductionRunsKiroAndReturnsStdout(t *testing.T) {
	// Not parallel: it rewrites PATH.
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf 'args:'\nfor a in \"$@\"; do printf ' [%s]' \"$a\"; done\nprintf '\\ncwd:%s\\n' \"$PWD\"\n"
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kiro-cli: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	work := t.TempDir()
	out, err := KiroInvoker(context.Background(), AgentAutomated, "do it", work)
	if err != nil {
		t.Fatalf("KiroInvoker: %v", err)
	}
	if !strings.Contains(out, "args: [chat] [--agent] [planner-automated] [--no-interactive] [do it]") {
		t.Errorf("output = %q, want the production argv", out)
	}
	if strings.Contains(out, "--trust-all-tools") {
		t.Errorf("output = %q, must not trust all tools", out)
	}
	real, _ := filepath.EvalSymlinks(work)
	if !strings.Contains(out, "cwd:"+real) && !strings.Contains(out, "cwd:"+work) {
		t.Errorf("output = %q, want the command run in %s", out, work)
	}
}

func TestInvokerProductionReportsExitFailure(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho boom >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kiro-cli: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := KiroInvoker(context.Background(), AgentAutomated, "x", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want one carrying stderr", err)
	}
}

func TestRunAutomatedStripsANSIBeforeExtract(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	doc := canonicalReply(t, validBody("Dependencies: none\n"))
	colored := "\x1b[0m\x1b[38;5;141m" + strings.ReplaceAll(doc, "\n", "\x1b[0m\n\x1b[1m") + "\x1b[K\x1b]0;title\x07"
	if _, err := Extract(colored); err == nil {
		t.Fatal("test setup: the colored reply should not extract without stripping")
	}
	inv := &fakeInvoker{replies: []string{colored}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(1))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %v, want committed after stripping ANSI (failure %+v)", res.Outcome, res.Failure)
	}
	if strings.Contains(h.store.bodies[42], "\x1b") {
		t.Errorf("committed body still holds an escape sequence")
	}
}

func TestStripANSI(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"plain":                                 "plain",
		"\x1b[31mred\x1b[0m":                    "red",
		"a\x1b[2K\x1b[1Gb":                      "ab",
		"\x1b[?25lx\x1b[?25h":                   "x",
		"\x1b]0;title\x07after":                 "after",
		"\x1b]8;;http://x\x1b\\l\x1b]8;;\x1b\\": "l",
	}
	for in, want := range cases {
		if got := StripANSI(in); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunAutomatedRetriesRecoverableRejectionThenPunchesOut(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	inv := &fakeInvoker{replies: []string{canonicalReply(t, missingContextBody())}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(2))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomePunchedOut {
		t.Fatalf("outcome = %v, want punched out after the retries", res.Outcome)
	}
	if res.Attempts != 3 || len(inv.agents) != 3 {
		t.Errorf("attempts = %d, invocations = %d, want 3 each (max_retries 2 + the first run)", res.Attempts, len(inv.agents))
	}
	for _, a := range inv.agents {
		if a != AgentAutomated {
			t.Errorf("a retry invoked %q, want %q every time", a, AgentAutomated)
		}
	}
	// Each attempt is a new attempt: the prompt says which one it is.
	for i, p := range inv.prompts {
		want := "attempt " + string(rune('1'+i)) + " of 3"
		if !strings.Contains(strings.ToLower(p), want) {
			t.Errorf("prompt %d does not say %q:\n%s", i+1, want, p)
		}
	}
	// A retry tells the agent why the last reply was rejected.
	if !strings.Contains(inv.prompts[1], "structural") {
		t.Errorf("retry prompt does not carry the previous failure:\n%s", inv.prompts[1])
	}
	if h.store.writes() != 0 {
		t.Errorf("a rejected document reached the store: %v", h.store.calls)
	}
	w := h.issues.writeCalls()
	if len(w) != 2 || w[0] != "PostComment" || w[1] != "AddLabel "+failLabel {
		t.Errorf("writes = %v, want a comment and then the fail label", w)
	}
	if res.Failure == nil || res.Failure.Attempt.Current != 3 || res.Failure.Attempt.Max != 3 {
		t.Errorf("failure = %+v, want it stamped with the final attempt 3 of 3", res.Failure)
	}
}

func TestRunAutomatedRecoversOnRetry(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	inv := &fakeInvoker{replies: []string{
		canonicalReply(t, missingContextBody()),
		canonicalReply(t, validBody("Dependencies: none\n")),
	}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(3))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomeCommitted || res.Attempts != 2 {
		t.Fatalf("result = %+v, want committed on attempt 2", res)
	}
	if w := h.issues.writeCalls(); len(w) != 0 {
		t.Errorf("a retry wrote to the failure port: %v", w)
	}
}

func TestRunAutomatedPunchOutReplyIsTerminalAndNotRetried(t *testing.T) {
	t.Parallel()
	h := newHarness()
	inv := &fakeInvoker{replies: []string{punchOutReply(t)}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(5))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomePunchedOut || res.Attempts != 1 {
		t.Fatalf("result = %+v, want punched out after exactly 1 attempt", res)
	}
	if len(inv.agents) != 1 {
		t.Errorf("agent invoked %d times, want 1: a punch_out reply is not retried", len(inv.agents))
	}
	w := h.issues.writeCalls()
	if len(w) != 2 || w[0] != "PostComment" || w[1] != "AddLabel "+failLabel {
		t.Errorf("writes = %v, want one comment and then one label (no duplicate punch-out)", w)
	}
	if h.store.writes() != 0 {
		t.Errorf("a punch_out wrote an issue body")
	}
	if !strings.Contains(h.issues.posted[0], "performance_target") {
		t.Errorf("comment does not name the unresolved slot:\n%s", h.issues.posted[0])
	}
}

func TestRunAutomatedInvokerFailureIsRetried(t *testing.T) {
	t.Parallel()
	h := newHarness()
	inv := &fakeInvoker{err: errors.New("kiro-cli exited 1")}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(1))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomePunchedOut || res.Attempts != 2 {
		t.Fatalf("result = %+v, want a punch-out after 2 attempts", res)
	}
	if res.Failure == nil || !strings.Contains(res.Failure.Actual, "kiro-cli exited 1") {
		t.Errorf("failure = %+v, want it to carry the invoker error", res.Failure)
	}
}

func TestRunAutomatedHaltsWhenIssueAlreadyFailed(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.issues.labels = []string{failLabel}
	inv := &fakeInvoker{replies: []string{punchOutReply(t)}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(2))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomeHalted {
		t.Errorf("outcome = %v, want halted", res.Outcome)
	}
	if len(inv.agents) != 0 {
		t.Errorf("the agent ran on a failed issue")
	}
}

func TestRunAutomatedRoutesConflictToIntakeWithoutRetry(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-2") // changed since rev-1 was observed
	inv := &fakeInvoker{replies: []string{canonicalReply(t, validBody("Dependencies: none\n"))}}

	res, err := RunAutomated(context.Background(), h.b, inv.invoke, automatedRequest(3))
	if err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if res.Outcome != OutcomeRoutedToIntake || res.Attempts != 1 {
		t.Errorf("result = %+v, want routed to intake after 1 attempt", res)
	}
	if h.store.writes() != 0 {
		t.Errorf("a conflicting issue was written: %v", h.store.calls)
	}
}

func TestRunAutomatedRejectsBadRequests(t *testing.T) {
	t.Parallel()
	good := func() AutomatedRequest { return automatedRequest(1) }
	cases := map[string]func(*AutomatedRequest){
		"no repo":        func(r *AutomatedRequest) { r.Repo = "" },
		"no issue":       func(r *AutomatedRequest) { r.Issue = 0 },
		"no revision":    func(r *AutomatedRequest) { r.ObservedRevision = "" },
		"no fail label":  func(r *AutomatedRequest) { r.FailLabel = "" },
		"negative retry": func(r *AutomatedRequest) { r.MaxRetries = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness()
			inv := &fakeInvoker{replies: []string{punchOutReply(t)}}
			req := good()
			mutate(&req)
			if _, err := RunAutomated(context.Background(), h.b, inv.invoke, req); err == nil {
				t.Fatal("RunAutomated accepted a bad request")
			}
			if len(inv.agents) != 0 {
				t.Errorf("the agent ran for a bad request")
			}
		})
	}
	t.Run("nil invoker", func(t *testing.T) {
		t.Parallel()
		if _, err := RunAutomated(context.Background(), newHarness().b, nil, good()); err == nil {
			t.Fatal("RunAutomated accepted a nil invoker")
		}
	})
	t.Run("nil boundary", func(t *testing.T) {
		t.Parallel()
		inv := &fakeInvoker{replies: []string{"x"}}
		if _, err := RunAutomated(context.Background(), nil, inv.invoke, good()); err == nil {
			t.Fatal("RunAutomated accepted a nil boundary")
		}
	})
}

func TestRunAutomatedDoesNotMutateTheCallersBoundary(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(42, "rev-1")
	before := h.b.Target
	inv := &fakeInvoker{replies: []string{canonicalReply(t, validBody("Dependencies: none\n"))}}
	req := automatedRequest(1)
	req.Issue = 42
	req.Repo = "other/repo"
	if _, err := RunAutomated(context.Background(), h.b, inv.invoke, req); err != nil {
		t.Fatalf("RunAutomated: %v", err)
	}
	if h.b.Target != before {
		t.Errorf("Boundary.Target changed from %+v to %+v", before, h.b.Target)
	}
}

// ---- structural properties -------------------------------------------------

// TestNoModeRoutingInPlannerPackage proves AC5 structurally: the only
// mode-dependent values are the two agent-name constants; no function maps a
// mode to an agent name, and no router-for-mode type exists.
func TestNoModeRoutingInPlannerPackage(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		if name != "agents.go" {
			for _, lit := range []string{`"planner"`, `"planner-automated"`} {
				if strings.Contains(string(src), lit) {
					t.Errorf("%s contains the agent-name literal %s; only agents.go names agents", name, lit)
				}
			}
		}
		if lower := strings.ToLower(name); strings.Contains(lower, "mode") || strings.Contains(lower, "router") {
			t.Errorf("file %s looks like a mode-routing component", name)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				lname := strings.ToLower(d.Name.Name)
				if strings.Contains(lname, "mode") || strings.Contains(lname, "agentfor") || strings.Contains(lname, "selectagent") || strings.Contains(lname, "pickagent") {
					t.Errorf("%s: function %s maps a mode to an agent", name, d.Name.Name)
				}
				if d.Type.Params != nil {
					for _, f := range d.Type.Params.List {
						for _, p := range f.Names {
							if strings.Contains(strings.ToLower(p.Name), "mode") {
								t.Errorf("%s: function %s has a mode parameter %q", name, d.Name.Name, p.Name)
							}
						}
					}
				}
			case *ast.TypeSpec:
				lname := strings.ToLower(d.Name.Name)
				if strings.Contains(lname, "mode") {
					t.Errorf("%s: type %s looks like a mode-routing component", name, d.Name.Name)
				}
			case *ast.SwitchStmt:
				// A switch whose cases are agent-name constants is a router.
				ast.Inspect(d, func(m ast.Node) bool {
					if id, ok := m.(*ast.Ident); ok && (id.Name == "AgentInteractive" || id.Name == "AgentAutomated") {
						t.Errorf("%s: a switch uses %s; that would be a mode router", name, id.Name)
					}
					return true
				})
			}
			return true
		})
	}
}

// TestRunAutomatedSourceNamesOnlyTheAutomatedAgent pins the call site: it
// references AgentAutomated and never AgentInteractive.
func TestRunAutomatedSourceNamesOnlyTheAutomatedAgent(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "AgentAutomated") {
		t.Error("run.go does not select AgentAutomated")
	}
	if strings.Contains(s, "AgentInteractive") {
		t.Error("run.go references AgentInteractive; the unattended site must never select it")
	}
}
