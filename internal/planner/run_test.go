package planner

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func TestKiroArgsIsAutomatedAndNeverTrustsAllTools(t *testing.T) {
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

func TestAutomatedCommandRunsPlannerAutomatedInDir(t *testing.T) {
	t.Parallel()
	cmd := AutomatedCommand("do it", "/work/tree")
	if got := cmd.Args; !slices.Equal(got, append([]string{KiroCommand}, KiroArgs(AgentAutomated, "do it")...)) {
		t.Errorf("args = %v, want kiro-cli with the automated argv", got)
	}
	if cmd.Dir != "/work/tree" {
		t.Errorf("dir = %q, want /work/tree", cmd.Dir)
	}
	if cmd.Process != nil {
		t.Error("AutomatedCommand must not start the process")
	}
}

func TestAutomatedPromptNamesRepoIssueAndAttempt(t *testing.T) {
	t.Parallel()
	p := AutomatedPrompt("owner/repo", 42, guard.Attempt{Current: 1, Max: 3}, nil)
	for _, want := range []string{"owner/repo#42", "gh issue view 42 --repo owner/repo", "attempt 1 of 3"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt does not contain %q:\n%s", want, p)
		}
	}
	if strings.Contains(strings.ToLower(p), "interview") || strings.Contains(p, "rejected") {
		t.Errorf("a first unattended prompt must not invite a conversation or mention a rejection:\n%s", p)
	}
}

func TestAutomatedPromptCarriesThePreviousFailure(t *testing.T) {
	t.Parallel()
	prev := &guard.FailureContext{Guard: "structure", Expected: "a Context section", Actual: "no Context section"}
	p := AutomatedPrompt("owner/repo", 42, guard.Attempt{Current: 2, Max: 3}, prev)
	for _, want := range []string{"attempt 2 of 3", "structure", "a Context section", "no Context section"} {
		if !strings.Contains(p, want) {
			t.Errorf("retry prompt does not contain %q:\n%s", want, p)
		}
	}
}

func TestRejectionFailureUsesTheRecordedFailure(t *testing.T) {
	t.Parallel()
	fc := &guard.FailureContext{Stage: stageoutput.StagePlanner, Guard: "structure", Attempt: guard.Attempt{Current: 1, Max: 2}, Expected: "e", Actual: "a"}
	if got := RejectionFailure(Result{Outcome: OutcomeRejected, Failure: fc}, guard.Attempt{Current: 1, Max: 2}); got != *fc {
		t.Errorf("RejectionFailure = %+v, want %+v", got, *fc)
	}
	got := RejectionFailure(Result{Outcome: OutcomeRejected}, guard.Attempt{Current: 2, Max: 3})
	if got.Guard != ExtractGuardName || got.Attempt != (guard.Attempt{Current: 2, Max: 3}) {
		t.Errorf("synthesized failure = %+v, want the extract guard at attempt 2 of 3", got)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("synthesized failure does not validate: %v", err)
	}
}

func TestInvokeFailureNamesTheInvokeGuard(t *testing.T) {
	t.Parallel()
	got := InvokeFailure(guard.Attempt{Current: 1, Max: 2}, errors.New("exit status 3: not logged in"))
	if got.Guard != InvokeGuardName || got.Stage != stageoutput.StagePlanner {
		t.Errorf("failure = %+v, want stage planner, guard %s", got, InvokeGuardName)
	}
	if !strings.Contains(got.Actual, "not logged in") {
		t.Errorf("Actual = %q, want the run error", got.Actual)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("failure does not validate: %v", err)
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

func TestStrippedColoredReplyExtracts(t *testing.T) {
	t.Parallel()
	doc := canonicalReply(t, validBody("Dependencies: none\n"))
	colored := "\x1b[0m\x1b[38;5;141m" + strings.ReplaceAll(doc, "\n", "\x1b[0m\n\x1b[1m") + "\x1b[K\x1b]0;title\x07"
	if _, err := Extract(colored); err == nil {
		t.Fatal("test setup: the colored reply should not extract without stripping")
	}
	if _, err := Extract(StripANSI(colored)); err != nil {
		t.Errorf("Extract(StripANSI(colored)) = %v, want the document", err)
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

// TestAutomatedSiteNamesOnlyTheAutomatedAgent pins the unattended invocation
// site: run.go references AgentAutomated and never AgentInteractive.
func TestAutomatedSiteNamesOnlyTheAutomatedAgent(t *testing.T) {
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
