package planner

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
	"github.com/jbrinkman/kairon/internal/watcher"
)

const (
	testRepo  = "owner/repo"
	testLabel = "kairon-failed"
	testRunID = "run-381"
)

type harness struct {
	store  *fakeStore
	router *fakeRouter
	issues *fakeIssues
	b      *Boundary
}

func newHarness() *harness {
	h := &harness{store: newFakeStore(), router: &fakeRouter{}, issues: newFakeIssues()}
	h.b = &Boundary{Store: h.store, Router: h.router, Issues: h.issues}
	return h
}

func (h *harness) automated(issue int, replies ...string) Request {
	return Request{
		Mode:             ModeAutomated,
		Replies:          replies,
		Issue:            issue,
		ObservedRevision: "rev-1",
		Repo:             testRepo,
		FailLabel:        testLabel,
		RunID:            testRunID,
	}
}

func interactive(replies ...string) Request {
	return Request{Mode: ModeInteractive, Replies: replies}
}

// assertNoWrites fails if the store or the issues fake was written to.
func (h *harness) assertNoWrites(t *testing.T) {
	t.Helper()
	if n := h.store.total(); n != 0 {
		t.Errorf("store was called %d times (%v), want none", n, h.store.calls)
	}
	if w := h.issues.writes(); len(w) != 0 {
		t.Errorf("issues were written: %v", w)
	}
	if h.router.intake+h.router.punchOut != 0 {
		t.Errorf("router was called")
	}
}

func assertRejected(t *testing.T, res Result, wantGuard string) {
	t.Helper()
	if res.Outcome != OutcomeRejected {
		t.Fatalf("outcome = %v, want rejected", res.Outcome)
	}
	if res.Disposition != failure.Recoverable {
		t.Errorf("disposition = %v, want Recoverable", res.Disposition)
	}
	fc := res.Failure
	if fc == nil {
		t.Fatal("rejection has no FailureContext")
	}
	if err := fc.Validate(); err != nil {
		t.Errorf("FailureContext is not filled: %v (%+v)", err, *fc)
	}
	if fc.Stage != stageoutput.StagePlanner {
		t.Errorf("stage = %q, want planner", fc.Stage)
	}
	if wantGuard != "" && fc.Guard != wantGuard {
		t.Errorf("guard = %q, want %q", fc.Guard, wantGuard)
	}
	if len(res.Committed) != 0 {
		t.Errorf("rejection committed %d issues", len(res.Committed))
	}
}

func TestBoundaryRejectsMalformedReplies(t *testing.T) {
	good := interactiveCanonical(t, canonicalBody("Add retry", "none"))
	cases := []struct {
		name      string
		reply     string
		wantGuard string
	}{
		{"prose before", "Here is the issue:\n" + good, BoundaryGuard},
		{"prose after", good + "\nLet me know if you want changes.", BoundaryGuard},
		{"two documents", good + "---\n" + good, BoundaryGuard},
		{"two fenced documents", "```yaml\n" + good + "```\n\n```yaml\n" + good + "```", BoundaryGuard},
		{"not a document", "I need more information first.", BoundaryGuard},
		{"empty", "", BoundaryGuard},
		{"fails the schema", "schema_version: 1\nstage: planner\nresult: canonical\n", guard.StructuralName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			res, err := h.b.Handle(context.Background(), interactive(tc.reply))
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			assertRejected(t, res, tc.wantGuard)
			if res.Failure.Attempt != (guard.Attempt{Current: 1, Max: 1}) {
				t.Errorf("attempt = %+v, want zero value to default to {1,1}", res.Failure.Attempt)
			}
			h.assertNoWrites(t)
		})
	}
}

func TestBoundaryInteractiveRejectsPunchOut(t *testing.T) {
	h := newHarness()
	reply := punchOutDoc(t,
		stageoutput.Source{Kind: stageoutput.SourceDocument, Ref: "interactive-request"},
		[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})
	res, err := h.b.Handle(context.Background(), interactive(reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	containsAll(t, res.Failure.Actual, "punch_out")
	h.assertNoWrites(t)
}

func TestBoundaryAutomatedModeContract(t *testing.T) {
	const issue = 42
	good := automatedCanonical(t, issue, canonicalBody("Add retry", "none"))
	cases := []struct {
		name    string
		replies []string
	}{
		{"no replies", nil},
		{"two replies", []string{good, good}},
		{"document source", []string{interactiveCanonical(t, canonicalBody("Add retry", "none"))}},
		{"wrong issue ref", []string{automatedCanonical(t, 43, canonicalBody("Add retry", "none"))}},
		{"punch out with wrong ref", []string{punchOutDoc(t,
			stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "7"},
			[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})}},
		{"punch out with document source", []string{punchOutDoc(t,
			stageoutput.Source{Kind: stageoutput.SourceDocument, Ref: "42"},
			[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			res, err := h.b.Handle(context.Background(), h.automated(issue, tc.replies...))
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			assertRejected(t, res, BoundaryGuard)
			h.assertNoWrites(t)
		})
	}
}

func TestBoundaryRejectsWrongStage(t *testing.T) {
	h := newHarness()
	reply := strings.Replace(interactiveCanonical(t, canonicalBody("T", "none")), "stage: planner", "stage: architect", 1)
	res, err := h.b.Handle(context.Background(), interactive(reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	h.assertNoWrites(t)
}

func TestBoundaryRejectsMarkerInBody(t *testing.T) {
	h := newHarness()
	body := canonicalBody("Add retry", "none") + "\n" + provenance.MarkerPrefix + " v1 sha256=" + strings.Repeat("a", 64) + " generated=2026-01-01T00:00:00Z -->\n"
	res, err := h.b.Handle(context.Background(), interactive(interactiveCanonical(t, body)))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	containsAll(t, res.Failure.Actual, provenance.MarkerPrefix)
	h.assertNoWrites(t)
}

func TestBoundaryRejectsMissingTitleOnCreate(t *testing.T) {
	h := newHarness()
	body := strings.TrimPrefix(canonicalBody("Add retry", "none"), "# Add retry\n\n")
	res, err := h.b.Handle(context.Background(), interactive(interactiveCanonical(t, body)))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	containsAll(t, res.Failure.Expected, "H1")
	h.assertNoWrites(t)
}

var markerHashRe = regexp.MustCompile(`sha256=([0-9a-f]{64})`)

func TestBoundaryIntegrationCommitsCanonical(t *testing.T) {
	h := newHarness()
	reply := "```yaml\n" + interactiveCanonical(t, canonicalBody("Add retry to export", "none")) + "```"
	res, err := h.b.Handle(context.Background(), interactive(reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %v (failure %+v), want committed", res.Outcome, res.Failure)
	}
	if len(res.Committed) != 1 || res.Committed[0].Key != "issue-1" {
		t.Fatalf("committed = %+v, want one item keyed issue-1", res.Committed)
	}
	number := res.Committed[0].Result.Number
	if got := h.store.titles[number]; got != "Add retry to export" {
		t.Errorf("created title = %q, want the H1 text", got)
	}
	committed := h.store.bodies[number]
	if strings.Contains(committed, "# Add retry to export") {
		t.Errorf("committed body still contains the H1:\n%s", committed)
	}
	if !strings.HasPrefix(committed, "## Problem Statement") {
		t.Errorf("committed body should start at the first section:\n%s", committed)
	}
	m := markerHashRe.FindStringSubmatch(committed)
	if m == nil {
		t.Fatalf("committed body has no marker hash:\n%s", committed)
	}
	if want := provenance.Hash(committed); m[1] != want {
		t.Errorf("marker hash %s does not cover the committed body (hash %s)", m[1], want)
	}
	if res.Committed[0].Result.Hash != m[1] {
		t.Errorf("result hash %s != marker hash %s", res.Committed[0].Result.Hash, m[1])
	}
	if h.store.count("Create") != 1 || h.store.count("Update") != 0 {
		t.Errorf("store calls = %v, want one Create", h.store.calls)
	}
	if w := h.issues.writes(); len(w) != 0 {
		t.Errorf("issues written on a canonical commit: %v", w)
	}
}

func TestBoundaryIntegrationUpdatesExistingIssue(t *testing.T) {
	h := newHarness()
	h.store.seed(42, "rev-1", "original")
	// An automated update keeps the existing title; the H1 is optional.
	body := strings.TrimPrefix(canonicalBody("ignored", "none"), "# ignored\n\n")
	res, err := h.b.Handle(context.Background(), h.automated(42, automatedCanonical(t, 42, body)))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %v (failure %+v), want committed", res.Outcome, res.Failure)
	}
	if h.store.count("Update") != 1 || h.store.count("Create") != 0 {
		t.Errorf("store calls = %v, want one Update", h.store.calls)
	}
	if got := h.store.bodies[42]; !strings.Contains(got, "<!-- planner-provenance: v1 ") {
		t.Errorf("updated body has no marker:\n%s", got)
	}
}

func TestBoundaryIntegrationStripsH1OnUpdate(t *testing.T) {
	h := newHarness()
	h.store.seed(42, "rev-1", "original")
	res, err := h.b.Handle(context.Background(), h.automated(42, automatedCanonical(t, 42, canonicalBody("Retitled", "none"))))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %v, want committed", res.Outcome)
	}
	if strings.Contains(h.store.bodies[42], "# Retitled") {
		t.Errorf("the H1 should not be written into an updated body:\n%s", h.store.bodies[42])
	}
}

func TestBoundaryAutomatedPunchOutRoutesToTier2(t *testing.T) {
	h := newHarness()
	un := []stageoutput.Unresolved{
		{Slot: "goal.target", Reason: "the issue gives no measurable export speed target"},
		{Slot: "constraints.v2_contract", Reason: "the v2 export response shape is not in the repository"},
	}
	reply := punchOutDoc(t, stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "42"}, un)
	res, err := h.b.Handle(context.Background(), h.automated(42, reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomePunchedOut {
		t.Fatalf("outcome = %v, want punched out", res.Outcome)
	}
	if res.Failure == nil {
		t.Fatal("no FailureContext")
	}
	if res.Failure.Attempt != (guard.Attempt{Current: 1, Max: 1}) {
		t.Errorf("attempt = %+v, want {1,1}", res.Failure.Attempt)
	}
	if err := res.Failure.Validate(); err != nil {
		t.Errorf("FailureContext invalid: %v", err)
	}
	if res.Failure.Stage != stageoutput.StagePlanner {
		t.Errorf("stage = %q", res.Failure.Stage)
	}

	// failure.PunchOut ran once: the comment before the label, nothing else.
	w := h.issues.writes()
	if len(w) != 2 || w[0] != "PostComment" || w[1] != "AddLabel "+testLabel {
		t.Fatalf("issue writes = %v, want [PostComment, AddLabel %s]", w, testLabel)
	}
	comment := h.issues.bodies[0]
	for _, u := range un {
		containsAll(t, comment, u.Slot, u.Reason)
	}
	if id, ok := failure.ParseMarker(comment); !ok || id != testRunID {
		t.Errorf("comment marker run id = %q (ok %v), want %q", id, ok, testRunID)
	}
	containsAll(t, comment, "1 of 1")

	if n := h.store.total(); n != 0 {
		t.Errorf("IssueStore was called %d times on a punch-out: %v", n, h.store.calls)
	}
	if h.router.intake+h.router.punchOut != 0 {
		t.Errorf("router was called on a punch-out")
	}
}

func TestBoundaryAutomatedPunchOutIgnoresRequestAttempt(t *testing.T) {
	h := newHarness()
	reply := punchOutDoc(t, stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "42"},
		[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})
	req := h.automated(42, reply)
	req.Attempt = guard.Attempt{Current: 2, Max: 3}
	res, err := h.b.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Failure == nil || res.Failure.Attempt != (guard.Attempt{Current: 1, Max: 1}) {
		t.Errorf("failure = %+v, want Attempt {1,1} (a planner punch-out is terminal)", res.Failure)
	}
}

func TestBoundaryAutomatedPunchOutWriteFailureIsPending(t *testing.T) {
	h := newHarness()
	h.b.Issues = failingIssues{h.issues}
	reply := punchOutDoc(t, stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "42"},
		[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})
	res, err := h.b.Handle(context.Background(), h.automated(42, reply))
	if err == nil {
		t.Fatal("expected the write error")
	}
	if res.Outcome != OutcomePunchOutPending {
		t.Errorf("outcome = %v, want punch-out pending", res.Outcome)
	}
}

type failingIssues struct{ *fakeIssues }

func (failingIssues) PostComment(string, int, string) error { return errors.New("boom") }

func TestBoundaryAutomatedPunchOutNeedsTarget(t *testing.T) {
	h := newHarness()
	reply := punchOutDoc(t, stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "42"},
		[]stageoutput.Unresolved{{Slot: "scope.size", Reason: "too large"}})
	req := h.automated(42, reply)
	req.RunID = ""
	res, err := h.b.Handle(context.Background(), req)
	if !errors.Is(err, guard.ErrInvalid) {
		t.Fatalf("err = %v, want guard.ErrInvalid", err)
	}
	if res.Outcome == OutcomePunchedOut {
		t.Error("must not report a punch-out that did not happen")
	}
	if w := h.issues.writes(); len(w) != 0 {
		t.Errorf("issues written: %v", w)
	}
}

func TestBoundaryIntegrationRunnerRetriesMalformedReply(t *testing.T) {
	h := newHarness()
	good := interactiveCanonical(t, canonicalBody("Add retry", "none"))
	replies := []string{"Sure! Here is the issue:\n" + good, good}

	var attempts []guard.Attempt
	runner := failure.Runner{
		Issues: h.issues,
		Target: failure.Target{Repo: testRepo, Issue: 42, FailLabel: testLabel},
		Policy: failure.PolicyFromRetries(1),
		RunID:  testRunID,
	}
	var last Result
	run, err := runner.Run(func(a guard.Attempt) failure.StageResult {
		attempts = append(attempts, a)
		req := interactive(replies[a.Current-1])
		req.Attempt = a
		res, err := h.b.Handle(context.Background(), req)
		if err != nil {
			t.Fatalf("Handle() error: %v", err)
		}
		last = res
		if res.Outcome == OutcomeRejected {
			return failure.StageResult{Failure: res.Failure, Disposition: res.Disposition}
		}
		return failure.StageResult{}
	})
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}
	if run.Outcome != failure.OutcomeSucceeded || run.Attempts != 2 {
		t.Fatalf("run = %+v, want succeeded after 2 attempts", run)
	}
	if len(attempts) != 2 || attempts[0] != (guard.Attempt{Current: 1, Max: 2}) || attempts[1] != (guard.Attempt{Current: 2, Max: 2}) {
		t.Errorf("attempts = %+v", attempts)
	}
	if last.Outcome != OutcomeCommitted {
		t.Errorf("final outcome = %v, want committed", last.Outcome)
	}
	if h.store.count("Create") != 1 {
		t.Errorf("store calls = %v, want exactly one Create (after the retry)", h.store.calls)
	}
	if w := h.issues.writes(); len(w) != 0 {
		t.Errorf("a retry must not write to GitHub issues: %v", w)
	}
}

func TestBoundaryIntegrationMalformedReplyAloneWritesNothing(t *testing.T) {
	h := newHarness()
	res, err := h.b.Handle(context.Background(), interactive("Here you go:\nschema_version: 1\n"))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	h.assertNoWrites(t)
}

func TestBoundaryChainResolvesDependencies(t *testing.T) {
	h := newHarness()
	first := interactiveCanonical(t, canonicalBody("Add the store", "none"))
	second := interactiveCanonical(t, canonicalBody("Use the store", "#{issue-1}"))
	res, err := h.b.Handle(context.Background(), interactive(first, second))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeCommitted || len(res.Committed) != 2 {
		t.Fatalf("outcome = %v committed = %d (failure %+v), want 2 committed", res.Outcome, len(res.Committed), res.Failure)
	}
	if res.Committed[0].Key != "issue-1" || res.Committed[1].Key != "issue-2" {
		t.Errorf("keys = %q, %q", res.Committed[0].Key, res.Committed[1].Key)
	}
	n1, n2 := res.Committed[0].Result.Number, res.Committed[1].Result.Number
	if n1 >= n2 {
		t.Fatalf("issues not created in order: %d, %d", n1, n2)
	}

	body := h.store.bodies[n2]
	if strings.Contains(body, "#{") {
		t.Errorf("placeholder left in the committed body:\n%s", body)
	}
	var dp watcher.DependencyParser
	deps := dp.ParseDependencies(body)
	if len(deps) != 1 || deps[0] != n1 {
		t.Errorf("second body parses to dependencies %v, want [%d]", deps, n1)
	}
	if got := dp.ParseDependencies(h.store.bodies[n1]); len(got) != 0 {
		t.Errorf("first body should have no dependencies, got %v", got)
	}
	// The hash covers the substituted text.
	m := markerHashRe.FindStringSubmatch(body)
	if m == nil || m[1] != provenance.Hash(body) {
		t.Errorf("hash does not cover the substituted body")
	}
}

func TestBoundaryChainUsesKnownKeys(t *testing.T) {
	h := newHarness()
	second := interactiveCanonical(t, canonicalBody("Use the store", "#{issue-1}"))
	req := interactive(second)
	req.Known = map[string]int{"issue-1": 412}
	res, err := h.b.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeCommitted || len(res.Committed) != 1 {
		t.Fatalf("outcome = %v, want one committed", res.Outcome)
	}
	if res.Committed[0].Key != "issue-2" {
		t.Errorf("key = %q, want issue-2 (continues after the known keys)", res.Committed[0].Key)
	}
	var dp watcher.DependencyParser
	if deps := dp.ParseDependencies(h.store.bodies[res.Committed[0].Result.Number]); len(deps) != 1 || deps[0] != 412 {
		t.Errorf("dependencies = %v, want [412]", deps)
	}
	if len(req.Known) != 1 {
		t.Errorf("caller's Known map was modified: %v", req.Known)
	}
}

func TestBoundaryChainUnknownPlaceholderRejectedBeforeAnyWrite(t *testing.T) {
	cases := map[string]string{
		"unknown key":       "#{issue-9}",
		"forward reference": "#{issue-2}",
		"self reference":    "#{issue-1}",
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			first := interactiveCanonical(t, canonicalBody("First", deps))
			second := interactiveCanonical(t, canonicalBody("Second", "none"))
			res, err := h.b.Handle(context.Background(), interactive(first, second))
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			assertRejected(t, res, BoundaryGuard)
			if res.Index != 0 {
				t.Errorf("index = %d, want 0", res.Index)
			}
			h.assertNoWrites(t)
		})
	}
}

func TestBoundaryChainRejectsLaterMalformedDocumentBeforeAnyWrite(t *testing.T) {
	h := newHarness()
	first := interactiveCanonical(t, canonicalBody("First", "none"))
	res, err := h.b.Handle(context.Background(), interactive(first, "not a document"))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	if res.Index != 1 {
		t.Errorf("index = %d, want 1", res.Index)
	}
	h.assertNoWrites(t)
}

func TestBoundaryRevisionConflictRoutesAndWritesNothing(t *testing.T) {
	h := newHarness()
	h.store.seed(42, "rev-2-edited-by-human", "human edit")
	reply := automatedCanonical(t, 42, canonicalBody("Add retry", "none"))
	res, err := h.b.Handle(context.Background(), h.automated(42, reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeRoutedToIntake {
		t.Fatalf("outcome = %v, want routed to intake", res.Outcome)
	}
	if h.router.intake != 1 || h.router.punchOut != 0 {
		t.Errorf("router calls: intake %d, punch-out %d, want 1 and 0", h.router.intake, h.router.punchOut)
	}
	if h.store.count("Update") != 0 || h.store.count("Create") != 0 {
		t.Errorf("store was written: %v", h.store.calls)
	}
	if h.store.bodies[42] != "human edit" {
		t.Errorf("the human's edit was overwritten: %q", h.store.bodies[42])
	}
	if len(res.Committed) != 0 {
		t.Errorf("committed = %+v, want none", res.Committed)
	}
	if w := h.issues.writes(); len(w) != 0 {
		t.Errorf("issues written: %v", w)
	}
}

func TestBoundaryRejectsBadRequests(t *testing.T) {
	good := interactiveCanonical(t, canonicalBody("T", "none"))
	cases := []struct {
		name string
		req  Request
	}{
		{"interactive with an issue", Request{Mode: ModeInteractive, Replies: []string{good}, Issue: 5}},
		{"automated without an issue", Request{Mode: ModeAutomated, Replies: []string{good}}},
		{"invalid attempt", Request{Mode: ModeInteractive, Replies: []string{good}, Attempt: guard.Attempt{Current: 3, Max: 2}}},
		{"unknown mode", Request{Mode: Mode(9), Replies: []string{good}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			_, err := h.b.Handle(context.Background(), tc.req)
			if !errors.Is(err, guard.ErrInvalid) {
				t.Fatalf("err = %v, want guard.ErrInvalid", err)
			}
			h.assertNoWrites(t)
		})
	}
}

func TestBoundaryPassesAttemptToRejection(t *testing.T) {
	h := newHarness()
	req := interactive("prose only")
	req.Attempt = guard.Attempt{Current: 2, Max: 3}
	res, err := h.b.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	if res.Failure.Attempt != req.Attempt {
		t.Errorf("attempt = %+v, want %+v", res.Failure.Attempt, req.Attempt)
	}
}

func TestBoundaryOutcomeString(t *testing.T) {
	for o := OutcomeInvalid; o <= OutcomeRejected; o++ {
		if s := o.String(); s == "" || strings.Contains(s, "%") {
			t.Errorf("Outcome(%d).String() = %q", int(o), s)
		}
	}
}
