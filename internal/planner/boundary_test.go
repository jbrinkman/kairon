package planner

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// ---- fakes -----------------------------------------------------------------

// fakeStore is an in-memory provenance.IssueStore that records every call.
type fakeStore struct {
	mu        sync.Mutex
	revisions map[int]string
	bodies    map[int]string
	titles    map[int]string
	nextNum   int
	calls     []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		revisions: map[int]string{},
		bodies:    map[int]string{},
		titles:    map[int]string{},
		nextNum:   100,
	}
}

func (f *fakeStore) seed(number int, revision string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revisions[number] = revision
}

func (f *fakeStore) Revision(_ context.Context, number int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Revision")
	return f.revisions[number], nil
}

func (f *fakeStore) Create(_ context.Context, title, body string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Create")
	f.nextNum++
	f.bodies[f.nextNum] = body
	f.titles[f.nextNum] = title
	return f.nextNum, nil
}

func (f *fakeStore) Update(_ context.Context, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Update")
	f.bodies[number] = body
	return nil
}

// writes counts the store calls that write to GitHub.
func (f *fakeStore) writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == "Create" || c == "Update" {
			n++
		}
	}
	return n
}

// fakeRouter is a provenance.Router that records the conflicts it receives.
type fakeRouter struct {
	intake, punch []provenance.Conflict
	intakeErr     error
}

func (r *fakeRouter) RouteToIntake(_ context.Context, c provenance.Conflict) error {
	r.intake = append(r.intake, c)
	return r.intakeErr
}

func (r *fakeRouter) PunchOut(_ context.Context, c provenance.Conflict) error {
	r.punch = append(r.punch, c)
	return nil
}

// fakeIssues is a failure.Issues that records the order of every call and
// reflects comments and labels back through State.
type fakeIssues struct {
	mu       sync.Mutex
	calls    []string
	comments []failure.Comment
	labels   []string
	posted   []string
}

func (f *fakeIssues) State(_ string, _ int, _ string) (failure.IssueState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "State")
	return failure.IssueState{
		Labels:   slices.Clone(f.labels),
		Comments: slices.Clone(f.comments),
	}, nil
}

func (f *fakeIssues) PostComment(_ string, _ int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "PostComment")
	f.posted = append(f.posted, body)
	f.comments = append(f.comments, failure.Comment{Body: body, CreatedAt: time.Now(), ByViewer: true})
	return nil
}

func (f *fakeIssues) AddLabel(_ string, _ int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "AddLabel "+label)
	f.labels = append(f.labels, label)
	return nil
}

// writeCalls returns the calls that write to GitHub, in order.
func (f *fakeIssues) writeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if c != "State" {
			out = append(out, c)
		}
	}
	return out
}

// ---- fixtures --------------------------------------------------------------

var fixedNow = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

const failLabel = "kairon-failed"

// harness wires a Boundary to fakes.
type harness struct {
	b      *Boundary
	store  *fakeStore
	router *fakeRouter
	issues *fakeIssues
}

func newHarness() *harness {
	h := &harness{store: newFakeStore(), router: &fakeRouter{}, issues: &fakeIssues{}}
	h.b = &Boundary{
		Committer: &provenance.Committer{Store: h.store, Router: h.router, Now: func() time.Time { return fixedNow }},
		Issues:    h.issues,
		Target:    failure.Target{Repo: "owner/repo", Issue: 42, FailLabel: failLabel},
		NewRunID:  func() string { return "run-1" },
	}
	return h
}

func (h *harness) assertNothingWritten(t *testing.T) {
	t.Helper()
	if n := h.store.writes(); n != 0 {
		t.Errorf("store received %d write(s), want 0 (calls %v)", n, h.store.calls)
	}
	if w := h.issues.writeCalls(); len(w) != 0 {
		t.Errorf("issues received writes %v, want none", w)
	}
}

// validBody is a canonical body that passes the structural guard and the
// dependencies-form guard. deps is the text of the Dependencies section.
func validBody(deps string) string {
	return "# Add a JSON Lines exporter\n\n" +
		"## Problem Statement\n\nRun history cannot be piped into jq.\n\n" +
		"## Context\n\nExporters live in internal/export.\n\n" +
		"## Scope\n\n### In Scope\n\n- A jsonl exporter.\n\n### Out of Scope\n\n- The CSV exporter.\n\n" +
		"## Acceptance Criteria\n\n" +
		"1. The exporter writes one JSON object per line.\n   Verify: run `kairon export --format jsonl`; every line parses with `jq -e .`.\n\n" +
		"## Dependencies\n\n" + deps
}

// canonicalReply wraps body in a canonical stage-output document.
func canonicalReply(t *testing.T, body string) string {
	t.Helper()
	raw, err := stageoutput.Marshal(&stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceDocument, Ref: "interactive-request"},
		Generated:     "2026-10-22T12:00:00Z",
		Body:          body,
	})
	if err != nil {
		t.Fatalf("marshal canonical document: %v", err)
	}
	return string(raw)
}

func punchOutReply(t *testing.T) string {
	t.Helper()
	raw, err := stageoutput.Marshal(&stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultPunchOut,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "owner/repo#42"},
		Generated:     "2026-10-22T12:00:00Z",
		Unresolved: []stageoutput.Unresolved{
			{Slot: "performance_target", Reason: "the issue asks for faster export with no measurable target"},
			{Slot: "v2_contract", Reason: "what the v2 export returns is not stated anywhere"},
		},
	})
	if err != nil {
		t.Fatalf("marshal punch_out document: %v", err)
	}
	return string(raw)
}

var v1MarkerEnd = regexp.MustCompile(`<!-- planner-provenance: v1 sha256=[0-9a-f]{64} generated=\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z -->$`)

var attempt = guard.Attempt{Current: 1, Max: 3}

// ---- reference replies from the #380 planner suite -------------------------

// referenceReply returns the last stub reply of the named #380 planner case
// that begins a stage-output document. It reports false when the case carries
// none.
func referenceReply(t *testing.T, caseName string) (string, bool) {
	t.Helper()
	file := filepath.Join(repoRoot(t), ".kairon", "evals", "cases", "planner", caseName+".yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var c plannerCase
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for i := len(c.Stub.Turns) - 1; i >= 0; i-- {
		if beginsDocument(c.Stub.Turns[i].Response) {
			return c.Stub.Turns[i].Response, true
		}
	}
	return "", false
}

var plannerCaseNames = []string{
	"automated-best-effort-fill",
	"automated-bug-classification",
	"automated-dependencies-line",
	"automated-punch-out",
	"interactive-decline-implementation",
	"interactive-document-handoff",
	"interactive-option-rules",
	"interactive-path-only-message",
	"interactive-return-document",
	"interactive-single-decision",
	"multi-issue-split",
}

// ---- canonical path --------------------------------------------------------

func TestBoundaryCanonicalReachesCommitWithV1Marker(t *testing.T) {
	t.Parallel()
	h := newHarness()

	res, err := h.b.Process(context.Background(), Request{
		Reply:   canonicalReply(t, validBody("Dependencies: none\n")),
		Attempt: attempt,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %v, want committed (failure %+v)", res.Outcome, res.Failure)
	}
	if res.Number != 101 || res.Hash == "" {
		t.Errorf("number/hash = %d/%q, want 101 and a hash", res.Number, res.Hash)
	}
	if got := h.store.calls; !slices.Equal(got, []string{"Create"}) {
		t.Errorf("store calls = %v, want exactly [Create]", got)
	}
	stored := h.store.bodies[101]
	if !v1MarkerEnd.MatchString(stored) {
		t.Errorf("stored body does not end with a v1 provenance marker:\n%s", stored)
	}
	if !strings.Contains(stored, "sha256="+res.Hash) {
		t.Errorf("stored marker does not carry the reported hash %s", res.Hash)
	}
	if res.Body != stored {
		t.Errorf("Result.Body differs from the body the store received")
	}
	if got := h.store.titles[101]; got != "Add a JSON Lines exporter" {
		t.Errorf("title = %q, want the body's first heading", got)
	}
	if w := h.issues.writeCalls(); len(w) != 0 {
		t.Errorf("a committed document wrote to the failure port: %v", w)
	}
}

func TestBoundaryExplicitTitleWins(t *testing.T) {
	t.Parallel()
	h := newHarness()
	res, err := h.b.Process(context.Background(), Request{
		Reply:   canonicalReply(t, validBody("Dependencies: none\n")),
		Attempt: attempt,
		Title:   "Explicit title",
	})
	if err != nil || res.Outcome != OutcomeCommitted {
		t.Fatalf("Process = %+v, %v", res, err)
	}
	if got := h.store.titles[res.Number]; got != "Explicit title" {
		t.Errorf("title = %q, want the request title", got)
	}
}

func TestBoundaryCanonicalInFenceUpdatesExistingIssue(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(7, "rev-1")

	res, err := h.b.Process(context.Background(), Request{
		Reply:   "```yaml\n" + canonicalReply(t, validBody("Dependencies: #12, #15\n")) + "```\n",
		Attempt: attempt,
		Target:  provenance.Target{Number: 7, ObservedRevision: "rev-1"},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeCommitted || res.Number != 7 {
		t.Fatalf("result = %+v, want committed issue 7", res)
	}
	if !slices.Equal(h.store.calls, []string{"Revision", "Update"}) {
		t.Errorf("store calls = %v, want [Revision Update]", h.store.calls)
	}
	if !v1MarkerEnd.MatchString(h.store.bodies[7]) {
		t.Errorf("updated body does not end with a v1 marker")
	}
}

func TestBoundaryReferenceCanonicalRepliesCommit(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"automated-best-effort-fill",
		"automated-bug-classification",
		"automated-dependencies-line",
		"interactive-document-handoff",
		"interactive-return-document",
		"multi-issue-split",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reply, ok := referenceReply(t, name)
			if !ok {
				t.Fatalf("%s carries no document", name)
			}
			h := newHarness()
			res, err := h.b.Process(context.Background(), Request{Reply: reply, Attempt: attempt})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if res.Outcome != OutcomeCommitted {
				t.Fatalf("outcome = %v, want committed (failure %+v)", res.Outcome, res.Failure)
			}
			if !v1MarkerEnd.MatchString(h.store.bodies[res.Number]) {
				t.Errorf("stored body does not end with a v1 marker")
			}
		})
	}
}

// ---- punch_out path --------------------------------------------------------

func TestBoundaryPunchOutCommentsThenLabelsAndWritesNoBody(t *testing.T) {
	t.Parallel()
	h := newHarness()

	res, err := h.b.Process(context.Background(), Request{Reply: punchOutReply(t), Attempt: attempt})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomePunchedOut {
		t.Fatalf("outcome = %v, want punched out", res.Outcome)
	}
	if want := []string{"PostComment", "AddLabel " + failLabel}; !slices.Equal(h.issues.writeCalls(), want) {
		t.Fatalf("issue writes = %v, want %v (comment first, then label)", h.issues.writeCalls(), want)
	}
	if len(h.store.calls) != 0 {
		t.Errorf("a punch-out touched the issue store: %v", h.store.calls)
	}
	comment := h.issues.posted[0]
	for _, want := range []string{
		failure.Marker("run-1"), "performance_target", "no measurable target", "v2_contract", "not stated anywhere",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("punch-out comment lacks %q:\n%s", want, comment)
		}
	}
	if res.Failure == nil {
		t.Fatal("punch-out result has no FailureContext")
	}
	fc := res.Failure
	if fc.Stage != stageoutput.StagePlanner || fc.Guard != "intake" || fc.Attempt != (guard.Attempt{Current: 1, Max: 1}) {
		t.Errorf("failure context = %+v, want planner / intake / 1 of 1", fc)
	}
	if res.Disposition != failure.Unrecoverable {
		t.Errorf("disposition = %v, want unrecoverable", res.Disposition)
	}
}

func TestBoundaryReferencePunchOutReplyPunchesOut(t *testing.T) {
	t.Parallel()
	reply, ok := referenceReply(t, "automated-punch-out")
	if !ok {
		t.Fatal("automated-punch-out carries no document")
	}
	h := newHarness()
	res, err := h.b.Process(context.Background(), Request{Reply: reply, Attempt: attempt})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomePunchedOut {
		t.Fatalf("outcome = %v, want punched out", res.Outcome)
	}
	if want := []string{"PostComment", "AddLabel " + failLabel}; !slices.Equal(h.issues.writeCalls(), want) {
		t.Errorf("issue writes = %v, want %v", h.issues.writeCalls(), want)
	}
	if len(h.store.calls) != 0 {
		t.Errorf("store calls = %v, want none", h.store.calls)
	}
}

func TestBoundaryPunchOutWithoutTargetIssueIsContractError(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.b.Target.Issue = 0

	res, err := h.b.Process(context.Background(), Request{Reply: punchOutReply(t), Attempt: attempt})
	if err == nil {
		t.Fatalf("Process returned no error, result %+v", res)
	}
	if !errors.Is(err, ErrNoPunchOutTarget) || !errors.Is(err, guard.ErrInvalid) {
		t.Errorf("error = %v, want ErrNoPunchOutTarget matching guard.ErrInvalid", err)
	}
	if res.Outcome != OutcomeInvalid {
		t.Errorf("outcome = %v, want invalid alongside the error", res.Outcome)
	}
	h.assertNothingWritten(t)
	if len(h.issues.calls) != 0 {
		t.Errorf("failure port was called: %v", h.issues.calls)
	}
}

// ---- rejection -------------------------------------------------------------

func TestBoundaryGuardFailureIsRecoverableAndWritesNothing(t *testing.T) {
	t.Parallel()
	// The criterion carries no Verify: line, so the structural guard rejects.
	body := strings.Replace(validBody("Dependencies: none\n"),
		"   Verify: run `kairon export --format jsonl`; every line parses with `jq -e .`.\n", "", 1)
	h := newHarness()

	res, err := h.b.Process(context.Background(), Request{Reply: canonicalReply(t, body), Attempt: guard.Attempt{Current: 2, Max: 3}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeRejected {
		t.Fatalf("outcome = %v, want rejected", res.Outcome)
	}
	if res.Disposition != failure.Recoverable {
		t.Errorf("disposition = %v, want recoverable", res.Disposition)
	}
	if res.Failure == nil {
		t.Fatal("rejected result has no FailureContext")
	}
	if res.Failure.Guard != guard.StructuralName || res.Failure.Stage != stageoutput.StagePlanner {
		t.Errorf("failure = %+v, want the structural guard on stage planner", res.Failure)
	}
	if res.Failure.Attempt != (guard.Attempt{Current: 2, Max: 3}) {
		t.Errorf("attempt = %+v, want the caller's 2 of 3", res.Failure.Attempt)
	}
	if err := res.Failure.Validate(); err != nil {
		t.Errorf("failure context is incomplete: %v", err)
	}
	h.assertNothingWritten(t)
}

func TestBoundaryRejectedResultFeedsFailureRunner(t *testing.T) {
	t.Parallel()
	body := strings.Replace(validBody("Dependencies: none\n"), "## Context\n\nExporters live in internal/export.\n\n", "", 1)
	h := newHarness()
	runner := failure.Runner{
		Issues: h.issues,
		Target: h.b.Target,
		Policy: failure.Policy{MaxAttempts: 3},
		RunID:  "run-1",
	}

	var attempts []guard.Attempt
	res, err := runner.Run(func(a guard.Attempt) failure.StageResult {
		attempts = append(attempts, a)
		out, perr := h.b.Process(context.Background(), Request{Reply: canonicalReply(t, body), Attempt: a})
		if perr != nil {
			t.Fatalf("Process: %v", perr)
		}
		return failure.StageResult{Failure: out.Failure, Disposition: out.Disposition}
	})
	if err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if res.Outcome != failure.OutcomePunchedOut || res.Attempts != 3 || len(attempts) != 3 {
		t.Errorf("runner result = %+v after %d attempts, want a punch-out after 3", res, len(attempts))
	}
	if h.store.writes() != 0 {
		t.Errorf("a rejected document reached the store")
	}
}

func TestBoundaryExtractFailureIsRejectedAndRecoverable(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]string{
		"prose around the document": "Here is the issue:\n\n" + canonicalReply(t, validBody("Dependencies: none\n")),
		"empty reply":               "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness()
			res, err := h.b.Process(context.Background(), Request{Reply: reply, Attempt: attempt})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if res.Outcome != OutcomeRejected || res.Disposition != failure.Recoverable {
				t.Fatalf("result = %+v, want rejected and recoverable", res)
			}
			if res.Failure == nil || res.Failure.Guard != "extract" {
				t.Fatalf("failure = %+v, want guard extract", res.Failure)
			}
			if err := res.Failure.Validate(); err != nil {
				t.Errorf("failure context is incomplete: %v", err)
			}
			h.assertNothingWritten(t)
		})
	}
}

func TestBoundaryUnparseableDocumentIsRejectedByStructuralGuard(t *testing.T) {
	t.Parallel()
	reply := strings.Replace(canonicalReply(t, validBody("Dependencies: none\n")), "result: canonical", "result: maybe", 1)
	h := newHarness()

	res, err := h.b.Process(context.Background(), Request{Reply: reply, Attempt: attempt})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeRejected || res.Disposition != failure.Recoverable {
		t.Fatalf("result = %+v, want rejected and recoverable", res)
	}
	if res.Failure == nil || res.Failure.Guard != guard.StructuralName {
		t.Fatalf("failure = %+v, want the structural guard", res.Failure)
	}
	h.assertNothingWritten(t)
}

func TestBoundaryInvalidAttemptIsAnError(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, err := h.b.Process(context.Background(), Request{Reply: canonicalReply(t, validBody("Dependencies: none\n"))})
	if !errors.Is(err, guard.ErrInvalid) {
		t.Fatalf("error = %v, want guard.ErrInvalid for a zero attempt", err)
	}
	h.assertNothingWritten(t)
}

func TestBoundaryDependenciesFormRejectsThroughDefaultChain(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		deps string
		want string
	}{
		"bullet list under a Dependencies header": {"- #314 must merge first\n- #317 must merge first\n", "bullet"},
		"missing Dependencies line":               {"None.\n", "no Dependencies:"},
		"unresolved placeholder":                  {"Dependencies: #{store}\n", "unresolved"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness()
			res, err := h.b.Process(context.Background(), Request{
				Reply:   canonicalReply(t, validBody(tc.deps)),
				Attempt: attempt,
			})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if res.Outcome != OutcomeRejected || res.Disposition != failure.Recoverable {
				t.Fatalf("result = %+v, want rejected and recoverable", res)
			}
			if res.Failure == nil || res.Failure.Guard != DependenciesFormName {
				t.Fatalf("failure = %+v, want guard %s", res.Failure, DependenciesFormName)
			}
			if !strings.Contains(res.Failure.Actual, tc.want) {
				t.Errorf("actual = %q, want it to mention %q", res.Failure.Actual, tc.want)
			}
			h.assertNothingWritten(t)
		})
	}
}

func TestBoundaryCustomGuardsReplaceTheDefaultChain(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.b.Guards = func(handler guard.FailureHandler) []guard.Guard {
		return []guard.Guard{rejectAll{handler: handler}}
	}
	res, err := h.b.Process(context.Background(), Request{
		Reply:   canonicalReply(t, validBody("Dependencies: none\n")),
		Attempt: attempt,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeRejected || res.Failure == nil || res.Failure.Guard != "reject-all" {
		t.Fatalf("result = %+v, want rejected by the injected guard", res)
	}
	h.assertNothingWritten(t)
}

type rejectAll struct{ handler guard.FailureHandler }

func (rejectAll) Admit(in guard.Input) guard.AdmissionResult {
	return in.Reject("reject-all", "never", "always")
}
func (g rejectAll) OnFailure(fc guard.FailureContext) error { return g.handler(fc) }

// ---- revision conflict -----------------------------------------------------

func TestBoundaryRevisionChangeRoutesToIntakeAndWritesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.store.seed(7, "rev-2") // a human edited the issue after the planner read rev-1

	res, err := h.b.Process(context.Background(), Request{
		Reply:   canonicalReply(t, validBody("Dependencies: none\n")),
		Attempt: attempt,
		Target:  provenance.Target{Number: 7, ObservedRevision: "rev-1"},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomeRoutedToIntake || res.Number != 7 {
		t.Fatalf("result = %+v, want routed to intake for issue 7", res)
	}
	if len(h.router.intake) != 1 || len(h.router.punch) != 0 {
		t.Errorf("router saw intake %d, punch %d; want 1 and 0", len(h.router.intake), len(h.router.punch))
	}
	if c := h.router.intake[0]; c.Number != 7 || c.Observed != "rev-1" || c.Current != "rev-2" {
		t.Errorf("conflict = %+v, want issue 7 rev-1 to rev-2", c)
	}
	if res.Body != "" || res.Hash != "" {
		t.Errorf("a routed result carries body/hash: %+v", res)
	}
	h.assertNothingWritten(t)
}

func TestBoundaryRevisionChangeWithFailingIntakePunchesOut(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.router.intakeErr = errors.New("intake unavailable")
	h.store.seed(7, "rev-2")

	res, err := h.b.Process(context.Background(), Request{
		Reply:   canonicalReply(t, validBody("Dependencies: none\n")),
		Attempt: attempt,
		Target:  provenance.Target{Number: 7, ObservedRevision: "rev-1"},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Outcome != OutcomePunchedOut || res.Number != 7 || len(h.router.punch) != 1 {
		t.Fatalf("result = %+v (punch calls %d), want punched out for issue 7", res, len(h.router.punch))
	}
	if h.store.writes() != 0 {
		t.Errorf("a conflict wrote to the store")
	}
}

func TestBoundaryConflictRouterAdapter(t *testing.T) {
	t.Parallel()
	issues := &fakeIssues{}
	r := &ConflictRouter{
		Issues:   issues,
		Target:   failure.Target{Repo: "owner/repo", FailLabel: failLabel},
		NewRunID: func() string { return "run-9" },
	}
	c := provenance.Conflict{Number: 7, Observed: "rev-1", Current: "rev-2"}

	if err := r.RouteToIntake(context.Background(), c); err != nil {
		t.Fatalf("RouteToIntake: %v", err)
	}
	if w := issues.writeCalls(); len(w) != 0 {
		t.Errorf("RouteToIntake wrote %v, want nothing (the watcher routes)", w)
	}
	if err := r.PunchOut(context.Background(), c); err != nil {
		t.Fatalf("PunchOut: %v", err)
	}
	if want := []string{"PostComment", "AddLabel " + failLabel}; !slices.Equal(issues.writeCalls(), want) {
		t.Errorf("PunchOut writes = %v, want %v", issues.writeCalls(), want)
	}
	if !strings.Contains(issues.posted[0], "rev-2") {
		t.Errorf("comment does not name the revisions:\n%s", issues.posted[0])
	}
}

// ---- structural properties -------------------------------------------------

func TestBoundaryProcessHasNoModeParameterAndNoGhOrHash(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "boundary.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse boundary.go: %v", err)
	}

	var process *ast.FuncDecl
	requestFields := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == "Process" && d.Recv != nil {
				process = d
			}
		case *ast.TypeSpec:
			if st, ok := d.Type.(*ast.StructType); ok && d.Name.Name == "Request" {
				for _, f := range st.Fields.List {
					for _, name := range f.Names {
						requestFields[strings.ToLower(name.Name)] = true
					}
				}
			}
		}
		return true
	})
	if process == nil {
		t.Fatal("Boundary.Process not found in boundary.go")
	}
	var params []string
	for _, f := range process.Type.Params.List {
		for _, n := range f.Names {
			params = append(params, strings.ToLower(n.Name))
		}
	}
	if len(params) != 2 {
		t.Errorf("Process parameters = %v, want exactly a context and a request", params)
	}
	for _, p := range params {
		if strings.Contains(p, "mode") || strings.Contains(p, "agent") {
			t.Errorf("Process has a mode-like parameter %q", p)
		}
	}
	for f := range requestFields {
		if strings.Contains(f, "mode") || strings.Contains(f, "agent") {
			t.Errorf("Request has a mode-like field %q", f)
		}
	}

	// The boundary never shells out, never calls gh and never writes a hash:
	// it neither imports those packages nor calls the marker/hash producers.
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		switch path {
		case "os/exec", "crypto/sha256", "crypto/sha1", "crypto/md5", "github.com/jbrinkman/kairon/internal/github":
			t.Errorf("boundary.go imports %s", path)
		}
	}
	src, err := os.ReadFile("boundary.go")
	if err != nil {
		t.Fatalf("read boundary.go: %v", err)
	}
	for _, banned := range []string{"provenance.Mark", "provenance.Hash", "provenance.Marker", "gh issue", "exec.Command"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("boundary.go contains %q", banned)
		}
	}
}

func TestBoundaryNoStoreWriteIsMadeOutsideTheCommitter(t *testing.T) {
	t.Parallel()
	// Every path through Process other than a successful canonical commit must
	// leave the store untouched; the commit itself is the only write.
	replies := map[string]string{
		"punch_out": punchOutReply(t),
		"garbage":   "not a document",
	}
	for name, reply := range replies {
		h := newHarness()
		if _, err := h.b.Process(context.Background(), Request{Reply: reply, Attempt: attempt}); err != nil {
			t.Fatalf("%s: Process: %v", name, err)
		}
		if h.store.writes() != 0 {
			t.Errorf("%s: store received writes %v", name, h.store.calls)
		}
	}
}

// ---- DependenciesForm ------------------------------------------------------

func admitBody(t *testing.T, body string) guard.AdmissionResult {
	t.Helper()
	g := NewDependenciesForm(func(guard.FailureContext) error { return nil })
	return g.Admit(guard.Input{
		Stage:   stageoutput.StagePlanner,
		Raw:     canonicalReply(t, body),
		Attempt: attempt,
	})
}

func TestDependenciesFormAccepts(t *testing.T) {
	t.Parallel()
	for name, deps := range map[string]string{
		"none":                "Dependencies: none\n",
		"none, any case":      "dependencies: None\n",
		"one issue":           "Dependencies: #12\n",
		"several issues":      "Dependencies: #12, #15, #300\n",
		"trailing whitespace": "Dependencies: #12, #15   \n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := admitBody(t, validBody(deps))
			if !res.Pass {
				t.Fatalf("rejected: %+v", res.Failure)
			}
			if err := res.Validate(); err != nil {
				t.Errorf("admission result breaks the contract: %v", err)
			}
		})
	}
}

func TestDependenciesFormAcceptsBulletsOutsideTheDependenciesSection(t *testing.T) {
	t.Parallel()
	// validBody already has bullets under Scope; a Dependencies line followed
	// by a later section with bullets is also fine.
	body := validBody("Dependencies: #12\n") + "\n## Notes\n\n- a bullet in another section\n"
	if res := admitBody(t, body); !res.Pass {
		t.Fatalf("rejected: %+v", res.Failure)
	}
}

func TestDependenciesFormIgnoresFencedExamples(t *testing.T) {
	t.Parallel()
	body := validBody("Dependencies: #12\n") +
		"\n## Notes\n\n```markdown\n## Dependencies\n- #1\nDependencies: #{x}\n```\n"
	if res := admitBody(t, body); !res.Pass {
		t.Fatalf("rejected a fenced example: %+v", res.Failure)
	}
}

func TestDependenciesFormRejects(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body string
		want string
	}{
		"bullet list under the header": {
			validBody("- #314 must merge first\n- #317 must merge first\n"), "bullet",
		},
		"numbered list under the header": {
			validBody("1. #314 must merge first\n"), "bullet",
		},
		"bullets after a valid line": {
			validBody("Dependencies: #314\n\n- #317 must merge first\n"), "bullet",
		},
		"bullet that carries the keyword": {
			validBody("- Dependencies: #314\n"), "bullet",
		},
		"missing Dependencies line": {
			validBody("None.\n"), "no Dependencies:",
		},
		"header with nothing under it": {
			validBody(""), "no Dependencies:",
		},
		"two Dependencies lines": {
			validBody("Dependencies: #1\nDependencies: #2\n"), "2 Dependencies:",
		},
		"unresolved placeholder": {
			validBody("Dependencies: #{store}\n"), "unresolved",
		},
		"unresolved placeholder beside a number": {
			validBody("Dependencies: #12, #{store}\n"), "unresolved",
		},
		"empty value": {
			validBody("Dependencies:\n"), "malformed",
		},
		"prose value": {
			validBody("Dependencies: the store issue\n"), "malformed",
		},
		"number without hash": {
			validBody("Dependencies: 12\n"), "malformed",
		},
		"none mixed with a number": {
			validBody("Dependencies: none, #12\n"), "malformed",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := admitBody(t, tc.body)
			if res.Pass {
				t.Fatal("admitted a body the guard must reject")
			}
			if err := res.Validate(); err != nil {
				t.Fatalf("admission result breaks the contract: %v", err)
			}
			if res.Failure.Guard != DependenciesFormName {
				t.Errorf("guard = %q, want %q", res.Failure.Guard, DependenciesFormName)
			}
			if !strings.Contains(res.Failure.Actual, tc.want) {
				t.Errorf("actual = %q, want it to mention %q", res.Failure.Actual, tc.want)
			}
		})
	}
}

func TestDependenciesFormPassesPunchOutAndRejectsUnparseable(t *testing.T) {
	t.Parallel()
	g := NewDependenciesForm(nil)
	if res := g.Admit(guard.Input{Stage: stageoutput.StagePlanner, Raw: punchOutReply(t), Attempt: attempt}); !res.Pass {
		t.Errorf("punch_out rejected: %+v", res.Failure)
	}
	res := g.Admit(guard.Input{Stage: stageoutput.StagePlanner, Raw: "not: a document", Attempt: attempt})
	if res.Pass || res.Failure == nil || res.Failure.Guard != DependenciesFormName {
		t.Errorf("unparseable input result = %+v, want a rejection by %s", res, DependenciesFormName)
	}
}

func TestDependenciesFormOnFailureCallsHandlerOrReports(t *testing.T) {
	t.Parallel()
	var got guard.FailureContext
	boom := errors.New("boom")
	g := NewDependenciesForm(func(fc guard.FailureContext) error { got = fc; return boom })
	fc := guard.FailureContext{Stage: stageoutput.StagePlanner, Guard: DependenciesFormName, Attempt: attempt, Expected: "e", Actual: "a"}
	if err := g.OnFailure(fc); !errors.Is(err, boom) || got != fc {
		t.Errorf("OnFailure = %v with context %+v, want the handler's error and context", err, got)
	}
	if err := NewDependenciesForm(nil).OnFailure(fc); !errors.Is(err, guard.ErrNoFailureHandler) {
		t.Errorf("OnFailure without a handler = %v, want guard.ErrNoFailureHandler", err)
	}
}

func TestDependenciesFormRunsInTheChainAfterStructural(t *testing.T) {
	t.Parallel()
	g := defaultGuards(func(guard.FailureContext) error { return nil })
	if len(g) != 2 {
		t.Fatalf("default chain has %d guards, want 2", len(g))
	}
	if _, ok := g[0].(*guard.Structural); !ok {
		t.Errorf("guard 0 is %T, want *guard.Structural", g[0])
	}
	if _, ok := g[1].(*DependenciesForm); !ok {
		t.Errorf("guard 1 is %T, want *DependenciesForm", g[1])
	}
}

// ---- same contract ---------------------------------------------------------

func TestSameContractReferenceRepliesParseToPlannerDocuments(t *testing.T) {
	t.Parallel()
	wantDocument := map[string]stageoutput.Result{
		"automated-best-effort-fill":   stageoutput.ResultCanonical,
		"automated-bug-classification": stageoutput.ResultCanonical,
		"automated-dependencies-line":  stageoutput.ResultCanonical,
		"automated-punch-out":          stageoutput.ResultPunchOut,
		"interactive-document-handoff": stageoutput.ResultCanonical,
		"interactive-return-document":  stageoutput.ResultCanonical,
		"multi-issue-split":            stageoutput.ResultCanonical,
	}

	types := map[string]reflect.Type{}
	for _, name := range plannerCaseNames {
		reply, ok := referenceReply(t, name)
		want, shouldCarry := wantDocument[name]
		if ok != shouldCarry {
			t.Errorf("%s: carries a document = %v, want %v", name, ok, shouldCarry)
			continue
		}
		if !ok {
			continue
		}
		text, err := Extract(reply)
		if err != nil {
			t.Errorf("%s: Extract: %v", name, err)
			continue
		}
		doc, err := stageoutput.Parse([]byte(text))
		if err != nil {
			t.Errorf("%s: Parse: %v", name, err)
			continue
		}
		if doc.Stage != stageoutput.StagePlanner {
			t.Errorf("%s: stage = %q, want planner", name, doc.Stage)
		}
		if doc.Result != want {
			t.Errorf("%s: result = %q, want %q", name, doc.Result, want)
		}
		types[name] = reflect.TypeOf(doc)
	}
	if len(types) != len(wantDocument) {
		t.Fatalf("parsed %d reference documents, want %d", len(types), len(wantDocument))
	}

	interactive := types["interactive-return-document"]
	for _, name := range []string{"automated-best-effort-fill", "automated-bug-classification", "automated-punch-out"} {
		if types[name] != interactive {
			t.Errorf("%s parses to %v, interactive parses to %v; they must be the same Go type", name, types[name], interactive)
		}
	}
	if want := reflect.TypeOf(&stageoutput.Document{}); interactive != want {
		t.Errorf("document type = %v, want %v", interactive, want)
	}
}

func TestSameContractBoundaryResultIsOneTypeForBothAgents(t *testing.T) {
	t.Parallel()
	interactive, _ := referenceReply(t, "interactive-return-document")
	automated, _ := referenceReply(t, "automated-best-effort-fill")

	h := newHarness()
	a, err := h.b.Process(context.Background(), Request{Reply: interactive, Attempt: attempt})
	if err != nil {
		t.Fatalf("interactive Process: %v", err)
	}
	b, err := h.b.Process(context.Background(), Request{Reply: automated, Attempt: attempt})
	if err != nil {
		t.Fatalf("automated Process: %v", err)
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) || a.Outcome != b.Outcome {
		t.Errorf("interactive %+v and automated %+v results differ in type or outcome", a.Outcome, b.Outcome)
	}
	if a.Document == nil || b.Document == nil || reflect.TypeOf(a.Document) != reflect.TypeOf(b.Document) {
		t.Errorf("results do not both carry a *stageoutput.Document")
	}
}
