package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const commitTestBody = "# Title\r\n\r\nDo the thing.   \r\n\r\nDependencies: #314\r\n\r\n"

func canonicalDoc(body string) *stageoutput.Document {
	return &stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "jbrinkman/kairon#378"},
		Generated:     "2026-03-04T05:06:07Z",
		Body:          body,
	}
}

func punchOutDoc() *stageoutput.Document {
	return &stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultPunchOut,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "jbrinkman/kairon#378"},
		Generated:     "2026-03-04T05:06:07Z",
		Unresolved:    []stageoutput.Unresolved{{Slot: "scope", Reason: "unclear"}},
	}
}

func readyResult() guard.Result {
	return guard.Result{Outcome: guard.OutcomeReadyToCommit}
}

func newTestCommitter(s *fakeStore, r *fakeRouter) *Committer {
	return &Committer{Store: s, Router: r, Now: func() time.Time { return fixedTime }}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func TestCommit_WritesV1MarkerAsLastLine(t *testing.T) {
	store, router := newFakeStore(), &fakeRouter{}
	c := newTestCommitter(store, router)

	res, err := c.Commit(context.Background(), Request{
		Doc: canonicalDoc(commitTestBody), Title: "T", Admission: readyResult(),
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Outcome != OutcomeCommitted {
		t.Fatalf("Outcome = %v, want OutcomeCommitted", res.Outcome)
	}
	body := store.body(res.Number)
	if body == "" {
		t.Fatalf("store has no body for issue %d", res.Number)
	}
	if body != res.Body {
		t.Errorf("Result.Body differs from the body written to the store")
	}

	// Independent computation of the canonical body and its sha256.
	canonical := "# Title\n\nDo the thing.\n\nDependencies: #314"
	sum := sha256.Sum256([]byte(canonical))
	wantHash := hex.EncodeToString(sum[:])
	wantMarker := "<!-- planner-provenance: v1 sha256=" + wantHash + " generated=2026-03-04T05:06:07Z -->"

	if got := lastLine(body); got != wantMarker {
		t.Errorf("last line = %q, want %q", got, wantMarker)
	}
	if !markerRe.MatchString(lastLine(body)) {
		t.Errorf("last line %q does not match the documented marker form", lastLine(body))
	}
	if want := canonical + "\n" + wantMarker; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if res.Hash != wantHash {
		t.Errorf("Result.Hash = %q, want %q", res.Hash, wantHash)
	}
}

func TestCommit_DefaultClockIsTimeNow(t *testing.T) {
	store := newFakeStore()
	c := &Committer{Store: store, Router: &fakeRouter{}} // nil Now
	before := time.Now().UTC().Truncate(time.Second)
	res, err := c.Commit(context.Background(), Request{Doc: canonicalDoc("x"), Title: "T", Admission: readyResult()})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)
	m := lastLine(store.body(res.Number))
	const key = "generated="
	i := strings.Index(m, key)
	if i < 0 {
		t.Fatalf("marker %q has no generated field", m)
	}
	ts, err := time.Parse(time.RFC3339, strings.Fields(m[i+len(key):])[0])
	if err != nil {
		t.Fatalf("generated is not RFC 3339: %v", err)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("generated = %v, want between %v and %v", ts, before, after)
	}
}

func TestRoundTrip_CommittedBodyClassifiesValid(t *testing.T) {
	store, router := newFakeStore(), &fakeRouter{}
	c := newTestCommitter(store, router)

	created, err := c.Commit(context.Background(), Request{
		Doc: canonicalDoc(commitTestBody), Title: "T", Admission: readyResult(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := Classify(store.body(created.Number)); got != StateValid {
		t.Errorf("created body classifies %v, want valid", got)
	}

	store.seed(7, "rev-1", "old body")
	updated, err := c.Commit(context.Background(), Request{
		Doc: canonicalDoc("updated\r\nbody  "), Target: Target{Number: 7, ObservedRevision: "rev-1"},
		Admission: readyResult(),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := Classify(store.body(7)); got != StateValid {
		t.Errorf("updated body classifies %v, want valid", got)
	}
	if updated.Number != 7 {
		t.Errorf("Result.Number = %d, want 7", updated.Number)
	}
}

func TestCommit_RefusesWithoutAllPass(t *testing.T) {
	failure := &guard.FailureContext{Stage: stageoutput.StagePlanner, Guard: "structural", Expected: "e", Actual: "a"}
	cases := []struct {
		name string
		adm  guard.Result
	}{
		{"zero result", guard.Result{}},
		{"rejected", guard.Result{Outcome: guard.OutcomeRejected, Failure: failure}},
		{"rejected without failure", guard.Result{Outcome: guard.OutcomeRejected}},
		{"ready but carries a failure", guard.Result{Outcome: guard.OutcomeReadyToCommit, Failure: failure}},
	}
	targets := map[string]Target{
		"create": {},
		"update": {Number: 7, ObservedRevision: "rev-1"},
	}
	for _, tc := range cases {
		for tname, target := range targets {
			t.Run(tc.name+"/"+tname, func(t *testing.T) {
				store, router := newFakeStore(), &fakeRouter{}
				store.seed(7, "rev-1", "old")
				c := newTestCommitter(store, router)
				res, err := c.Commit(context.Background(), Request{
					Doc: canonicalDoc("body"), Title: "T", Target: target, Admission: tc.adm,
				})
				if !errors.Is(err, ErrNotReady) {
					t.Fatalf("err = %v, want ErrNotReady", err)
				}
				if res.Outcome == OutcomeCommitted {
					t.Errorf("Outcome = committed on a refused request")
				}
				if n := store.totalCalls(); n != 0 {
					t.Errorf("store received %d calls, want 0", n)
				}
				if router.intakeCalls+router.punchOutCalls != 0 {
					t.Errorf("router was called on a refused request")
				}
			})
		}
	}
}

func TestCommit_ExactlyOneWrite(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		store, router := newFakeStore(), &fakeRouter{}
		c := newTestCommitter(store, router)
		res, err := c.Commit(context.Background(), Request{Doc: canonicalDoc("b"), Title: "My title", Admission: readyResult()})
		if err != nil || res.Outcome != OutcomeCommitted {
			t.Fatalf("Commit = %+v, %v", res, err)
		}
		if store.createCalls != 1 || store.updateCalls != 0 || store.revisionCalls != 0 {
			t.Errorf("calls create=%d update=%d revision=%d, want 1/0/0", store.createCalls, store.updateCalls, store.revisionCalls)
		}
		if store.titles[res.Number] != "My title" {
			t.Errorf("title = %q, want %q", store.titles[res.Number], "My title")
		}
	})
	t.Run("update", func(t *testing.T) {
		store, router := newFakeStore(), &fakeRouter{}
		store.seed(7, "rev-1", "old")
		c := newTestCommitter(store, router)
		res, err := c.Commit(context.Background(), Request{
			Doc: canonicalDoc("b"), Target: Target{Number: 7, ObservedRevision: "rev-1"}, Admission: readyResult(),
		})
		if err != nil || res.Outcome != OutcomeCommitted {
			t.Fatalf("Commit = %+v, %v", res, err)
		}
		if store.updateCalls != 1 || store.createCalls != 0 {
			t.Errorf("calls update=%d create=%d, want 1/0", store.updateCalls, store.createCalls)
		}
		if router.intakeCalls+router.punchOutCalls != 0 {
			t.Errorf("router was called without a conflict")
		}
	})
	t.Run("failing create is attempted once", func(t *testing.T) {
		store, router := newFakeStore(), &fakeRouter{}
		boom := errors.New("boom")
		store.createErr = boom
		c := newTestCommitter(store, router)
		res, err := c.Commit(context.Background(), Request{Doc: canonicalDoc("b"), Title: "T", Admission: readyResult()})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want to wrap boom", err)
		}
		if res.Outcome != OutcomeInvalid {
			t.Errorf("Outcome = %v, want OutcomeInvalid", res.Outcome)
		}
		if store.createCalls != 1 {
			t.Errorf("createCalls = %d, want 1", store.createCalls)
		}
	})
	t.Run("failing update is attempted once", func(t *testing.T) {
		store, router := newFakeStore(), &fakeRouter{}
		boom := errors.New("boom")
		store.seed(7, "rev-1", "old")
		store.updateErr = boom
		c := newTestCommitter(store, router)
		res, err := c.Commit(context.Background(), Request{
			Doc: canonicalDoc("b"), Target: Target{Number: 7, ObservedRevision: "rev-1"}, Admission: readyResult(),
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want to wrap boom", err)
		}
		if res.Outcome != OutcomeInvalid {
			t.Errorf("Outcome = %v, want OutcomeInvalid", res.Outcome)
		}
		if store.updateCalls != 1 {
			t.Errorf("updateCalls = %d, want 1", store.updateCalls)
		}
		if store.body(7) != "old" {
			t.Errorf("stored body changed despite failed update")
		}
	})
}

func TestCommit_ConcurrentEditAbortsWrite(t *testing.T) {
	store, router := newFakeStore(), &fakeRouter{}
	store.seed(7, "rev-2", "human edit")
	c := newTestCommitter(store, router)

	res, err := c.Commit(context.Background(), Request{
		Doc: canonicalDoc("planned"), Target: Target{Number: 7, ObservedRevision: "rev-1"}, Admission: readyResult(),
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Outcome != OutcomeRoutedToIntake {
		t.Errorf("Outcome = %v, want OutcomeRoutedToIntake", res.Outcome)
	}
	if store.updateCalls != 0 || store.createCalls != 0 {
		t.Errorf("writes: update=%d create=%d, want 0/0", store.updateCalls, store.createCalls)
	}
	if store.body(7) != "human edit" {
		t.Errorf("stored body = %q, want it unchanged", store.body(7))
	}
	if router.intakeCalls != 1 || router.punchOutCalls != 0 {
		t.Errorf("router intake=%d punchOut=%d, want 1/0", router.intakeCalls, router.punchOutCalls)
	}
	want := Conflict{Number: 7, Observed: "rev-1", Current: "rev-2"}
	if len(router.conflicts) != 1 || router.conflicts[0] != want {
		t.Errorf("conflicts = %+v, want [%+v]", router.conflicts, want)
	}
	if res.Number != 7 {
		t.Errorf("Result.Number = %d, want 7", res.Number)
	}
	if res.Body != "" || res.Hash != "" {
		t.Errorf("Result carries a body or hash for a write that did not happen: %+v", res)
	}
}

func TestCommit_ConflictFallsBackToPunchOut(t *testing.T) {
	conflicted := func() (*fakeStore, *fakeRouter, *Committer) {
		store, router := newFakeStore(), &fakeRouter{}
		store.seed(7, "rev-2", "human edit")
		return store, router, newTestCommitter(store, router)
	}
	req := Request{
		Doc: canonicalDoc("planned"), Target: Target{Number: 7, ObservedRevision: "rev-1"}, Admission: readyResult(),
	}

	t.Run("intake fails, punch out succeeds", func(t *testing.T) {
		store, router, c := conflicted()
		router.intakeErr = errors.New("intake down")
		res, err := c.Commit(context.Background(), req)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if res.Outcome != OutcomePunchedOut {
			t.Errorf("Outcome = %v, want OutcomePunchedOut", res.Outcome)
		}
		if router.intakeCalls != 1 || router.punchOutCalls != 1 {
			t.Errorf("router intake=%d punchOut=%d, want 1/1", router.intakeCalls, router.punchOutCalls)
		}
		if store.updateCalls != 0 {
			t.Errorf("updateCalls = %d, want 0", store.updateCalls)
		}
	})
	t.Run("both fail", func(t *testing.T) {
		store, router, c := conflicted()
		router.intakeErr = errors.New("intake down")
		router.punchOutErr = errors.New("punch down")
		res, err := c.Commit(context.Background(), req)
		if !errors.Is(err, ErrUnroutable) {
			t.Fatalf("err = %v, want ErrUnroutable", err)
		}
		if !errors.Is(err, router.intakeErr) || !errors.Is(err, router.punchOutErr) {
			t.Errorf("err = %v, want it to join both router errors", err)
		}
		if res.Outcome != OutcomeInvalid {
			t.Errorf("Outcome = %v, want OutcomeInvalid", res.Outcome)
		}
		if router.intakeCalls != 1 || router.punchOutCalls != 1 {
			t.Errorf("router intake=%d punchOut=%d, want 1/1", router.intakeCalls, router.punchOutCalls)
		}
		if store.updateCalls != 0 || store.body(7) != "human edit" {
			t.Errorf("a write happened on an unroutable conflict")
		}
	})
	t.Run("no router configured", func(t *testing.T) {
		store := newFakeStore()
		store.seed(7, "rev-2", "human edit")
		c := &Committer{Store: store, Now: func() time.Time { return fixedTime }}
		_, err := c.Commit(context.Background(), req)
		if !errors.Is(err, ErrUnroutable) {
			t.Fatalf("err = %v, want ErrUnroutable", err)
		}
		if store.updateCalls != 0 {
			t.Errorf("updateCalls = %d, want 0", store.updateCalls)
		}
	})
	t.Run("revision error", func(t *testing.T) {
		store, router, c := conflicted()
		boom := errors.New("gh down")
		store.revisionErr = boom
		res, err := c.Commit(context.Background(), req)
		if !errors.Is(err, ErrRevisionUnavailable) {
			t.Fatalf("err = %v, want ErrRevisionUnavailable", err)
		}
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want it to wrap the store error", err)
		}
		if res.Outcome != OutcomeInvalid {
			t.Errorf("Outcome = %v, want OutcomeInvalid", res.Outcome)
		}
		if store.updateCalls != 0 || store.createCalls != 0 {
			t.Errorf("a write happened without a revision check")
		}
		if router.intakeCalls+router.punchOutCalls != 0 {
			t.Errorf("router called on a revision error")
		}
	})
}

func TestCommit_InvalidRequests(t *testing.T) {
	cases := []struct {
		name string
		req  Request
	}{
		{"nil document", Request{Title: "T", Admission: readyResult()}},
		{"punch_out document", Request{Doc: punchOutDoc(), Title: "T", Admission: readyResult()}},
		{"empty body", Request{Doc: canonicalDoc(""), Title: "T", Admission: readyResult()}},
		{"blank body", Request{Doc: canonicalDoc(" \n\t\n"), Title: "T", Admission: readyResult()}},
		{"marker-only body", Request{Doc: canonicalDoc(Marker(strings.Repeat("a", 64), fixedTime)), Title: "T", Admission: readyResult()}},
		{"structurally invalid document", Request{Doc: &stageoutput.Document{Result: stageoutput.ResultCanonical, Body: "b"}, Title: "T", Admission: readyResult()}},
		{"create with empty title", Request{Doc: canonicalDoc("b"), Admission: readyResult()}},
		{"create with blank title", Request{Doc: canonicalDoc("b"), Title: "  \t", Admission: readyResult()}},
		{"update with empty observed revision", Request{Doc: canonicalDoc("b"), Target: Target{Number: 7}, Admission: readyResult()}},
		{"negative target number", Request{Doc: canonicalDoc("b"), Title: "T", Target: Target{Number: -1, ObservedRevision: "r"}, Admission: readyResult()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, router := newFakeStore(), &fakeRouter{}
			c := newTestCommitter(store, router)
			_, err := c.Commit(context.Background(), tc.req)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if errors.Is(err, ErrNotReady) {
				t.Errorf("err matches ErrNotReady; a ready request must not")
			}
			if n := store.totalCalls(); n != 0 {
				t.Errorf("store received %d calls, want 0", n)
			}
		})
	}
}

func TestCommit_NotReadyBeatsInvalid(t *testing.T) {
	// The gate runs first, so an invalid request that was never admitted is
	// reported as not ready, and nothing is validated against the store.
	store := newFakeStore()
	c := newTestCommitter(store, &fakeRouter{})
	_, err := c.Commit(context.Background(), Request{})
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
}

func TestCommit_MissingStoreIsInvalid(t *testing.T) {
	c := &Committer{Now: func() time.Time { return fixedTime }}
	_, err := c.Commit(context.Background(), Request{Doc: canonicalDoc("b"), Title: "T", Admission: readyResult()})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestCommit_DoesNotMutateDocument(t *testing.T) {
	doc := canonicalDoc(commitTestBody)
	before := *doc
	c := newTestCommitter(newFakeStore(), &fakeRouter{})
	if _, err := c.Commit(context.Background(), Request{Doc: doc, Title: "T", Admission: readyResult()}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if doc.Body != before.Body {
		t.Errorf("Commit mutated Doc.Body: %q", doc.Body)
	}
}

func TestCommit_MarkedInputBodyKeepsSingleMarker(t *testing.T) {
	// A body that already carries a (stale) marker must not end up with two.
	stale := "text\n" + Marker(strings.Repeat("0", 64), fixedTime.Add(-time.Hour))
	store := newFakeStore()
	c := newTestCommitter(store, &fakeRouter{})
	res, err := c.Commit(context.Background(), Request{Doc: canonicalDoc(stale), Title: "T", Admission: readyResult()})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	body := store.body(res.Number)
	if n := strings.Count(body, MarkerPrefix); n != 1 {
		t.Errorf("body has %d markers, want 1: %q", n, body)
	}
	if Classify(body) != StateValid {
		t.Errorf("body classifies %v, want valid", Classify(body))
	}
}
