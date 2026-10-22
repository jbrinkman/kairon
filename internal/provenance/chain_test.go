package provenance_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
	"github.com/jbrinkman/kairon/internal/watcher"
)

var chainTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// chainStore is an in-memory provenance.IssueStore that counts calls.
type chainStore struct {
	mu          sync.Mutex
	revisions   map[int]string
	bodies      map[int]string
	nextNum     int
	createCalls int
	updateCalls int
}

func newChainStore() *chainStore {
	return &chainStore{revisions: map[int]string{}, bodies: map[int]string{}, nextNum: 100}
}

func (s *chainStore) Revision(_ context.Context, number int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revisions[number], nil
}

func (s *chainStore) Create(_ context.Context, _, body string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	s.nextNum++
	s.bodies[s.nextNum] = body
	return s.nextNum, nil
}

func (s *chainStore) Update(_ context.Context, number int, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls++
	s.bodies[number] = body
	return nil
}

func (s *chainStore) writes() int { return s.createCalls + s.updateCalls }

type chainRouter struct {
	intakeCalls   int
	punchOutCalls int
}

func (r *chainRouter) RouteToIntake(context.Context, provenance.Conflict) error {
	r.intakeCalls++
	return nil
}

func (r *chainRouter) PunchOut(context.Context, provenance.Conflict) error {
	r.punchOutCalls++
	return nil
}

func chainDoc(body string) stageoutput.Document {
	return stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "jbrinkman/kairon#378"},
		Generated:     "2026-03-04T05:06:07Z",
		Body:          body,
	}
}

func newChainCommitter(s *chainStore, r *chainRouter) *provenance.Committer {
	return &provenance.Committer{Store: s, Router: r, Now: func() time.Time { return chainTime }}
}

// recordingAdmit records every raw text it is given and admits it.
type recordingAdmit struct {
	raws []string
	// rejectAt is the zero-based call index that is rejected; -1 rejects none.
	rejectAt int
}

func newRecordingAdmit() *recordingAdmit { return &recordingAdmit{rejectAt: -1} }

func (a *recordingAdmit) admit(raw string) (guard.Result, error) {
	i := len(a.raws)
	a.raws = append(a.raws, raw)
	if i == a.rejectAt {
		return guard.Result{
			Outcome: guard.OutcomeRejected,
			Failure: &guard.FailureContext{
				Stage:    stageoutput.StagePlanner,
				Guard:    "test",
				Attempt:  guard.Attempt{Current: 1, Max: 3},
				Expected: "ok",
				Actual:   "not ok",
			},
		}, nil
	}
	return guard.Result{Outcome: guard.OutcomeReadyToCommit}, nil
}

const (
	firstBody  = "# Schema\n\nAdd the schema.\n"
	secondBody = "# API\n\nUse the schema.\n\nDependencies: #{schema}\n"
)

func TestCommitChain_SubstitutesBeforeHashing(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "schema", Title: "Schema", Doc: chainDoc(firstBody)},
		{Key: "api", Title: "API", Doc: chainDoc(secondBody)},
	}

	res, err := c.CommitChain(context.Background(), items, newRecordingAdmit().admit)
	if err != nil {
		t.Fatalf("CommitChain error: %v", err)
	}
	if len(res.Committed) != 2 || res.StoppedAt != -1 || res.Stopped != nil {
		t.Fatalf("result = %+v, want 2 committed, StoppedAt -1, nil Stopped", res)
	}
	first, second := res.Committed[0].Result.Number, res.Committed[1].Result.Number
	if first == 0 || second == 0 || first == second {
		t.Fatalf("numbers = %d, %d", first, second)
	}

	stored := store.bodies[second]
	want := "Dependencies: #" + itoa(first)
	if !strings.Contains(stored, want) {
		t.Fatalf("stored second body lacks %q:\n%s", want, stored)
	}
	if strings.Contains(stored, "#{") {
		t.Errorf("stored second body still has a placeholder:\n%s", stored)
	}
	if res.Committed[1].Result.Body != stored {
		t.Errorf("Result.Body differs from the stored body")
	}

	got := (&watcher.DependencyParser{}).ParseDependencies(stored)
	if !reflect.DeepEqual(got, []int{first}) {
		t.Errorf("ParseDependencies = %v, want [%d]", got, first)
	}
	if st := provenance.Classify(stored); st != provenance.StateValid {
		t.Errorf("Classify(second) = %v, want valid", st)
	}
	if st := provenance.Classify(store.bodies[first]); st != provenance.StateValid {
		t.Errorf("Classify(first) = %v, want valid", st)
	}
}

func TestCommitChain_AdmitSeesSubstitutedDocument(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "schema", Title: "Schema", Doc: chainDoc(firstBody)},
		{Key: "api", Title: "API", Doc: chainDoc(secondBody)},
	}
	adm := newRecordingAdmit()

	res, err := c.CommitChain(context.Background(), items, adm.admit)
	if err != nil {
		t.Fatalf("CommitChain error: %v", err)
	}
	if len(adm.raws) != 2 {
		t.Fatalf("admit called %d times, want 2", len(adm.raws))
	}
	raw := adm.raws[1]
	if strings.Contains(raw, "#{") {
		t.Errorf("admit saw a placeholder:\n%s", raw)
	}
	if want := "Dependencies: #" + itoa(res.Committed[0].Result.Number); !strings.Contains(raw, want) {
		t.Errorf("admit text lacks %q:\n%s", want, raw)
	}
	doc, err := stageoutput.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("admit text does not parse as a stage-output document: %v", err)
	}
	if doc.Body == "" || strings.Contains(doc.Body, "#{") {
		t.Errorf("parsed admit body = %q", doc.Body)
	}
	if items[1].Doc.Body != secondBody {
		t.Errorf("caller's item was mutated: %q", items[1].Doc.Body)
	}
}

func TestCommitChain_UnknownPlaceholderWritesNothing(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "schema", Title: "Schema", Doc: chainDoc(firstBody)},
		// "later" is a forward reference: it is committed after this item.
		{Key: "api", Title: "API", Doc: chainDoc("# API\n\nDependencies: #{later}\n")},
		{Key: "later", Title: "Later", Doc: chainDoc("# Later\n\nbody\n")},
	}
	adm := newRecordingAdmit()

	res, err := c.CommitChain(context.Background(), items, adm.admit)
	if !errors.Is(err, provenance.ErrUnknownPlaceholder) {
		t.Fatalf("err = %v, want ErrUnknownPlaceholder", err)
	}
	if len(adm.raws) != 1 {
		t.Errorf("admit called %d times, want 1 (only the first item)", len(adm.raws))
	}
	if store.writes() != 1 {
		t.Errorf("store writes = %d, want 1 (only the first item)", store.writes())
	}
	if len(res.Committed) != 1 || res.StoppedAt != 1 {
		t.Errorf("result = %+v, want 1 committed and StoppedAt 1", res)
	}
}

func TestCommitChain_StopsOnRejection(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")},
		{Key: "b", Title: "B", Doc: chainDoc("# B\n\nbody\n")},
		{Key: "c", Title: "C", Doc: chainDoc("# C\n\nbody\n")},
	}
	adm := newRecordingAdmit()
	adm.rejectAt = 1

	res, err := c.CommitChain(context.Background(), items, adm.admit)
	if err != nil {
		t.Fatalf("CommitChain error: %v", err)
	}
	if len(adm.raws) != 2 {
		t.Errorf("admit called %d times, want 2 (item 3 never processed)", len(adm.raws))
	}
	if store.createCalls != 1 {
		t.Errorf("creates = %d, want 1 (item 1 stays committed)", store.createCalls)
	}
	if len(res.Committed) != 1 || res.Committed[0].Key != "a" {
		t.Errorf("Committed = %+v, want only a", res.Committed)
	}
	if res.StoppedAt != 1 {
		t.Errorf("StoppedAt = %d, want 1", res.StoppedAt)
	}
	if res.Stopped == nil || res.Stopped.Admission.Outcome != guard.OutcomeRejected || res.Stopped.Admission.Failure == nil {
		t.Errorf("Stopped = %+v, want the rejected admission", res.Stopped)
	}
}

func TestCommitChain_StopsOnConflict(t *testing.T) {
	store := newChainStore()
	store.revisions[50] = "rev-edited"
	store.bodies[50] = "human edit"
	router := &chainRouter{}
	c := newChainCommitter(store, router)
	items := []provenance.ChainItem{
		{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")},
		{Key: "b", Doc: chainDoc("# B\n\nbody\n"), Target: provenance.Target{Number: 50, ObservedRevision: "rev-planned"}},
		{Key: "c", Title: "C", Doc: chainDoc("# C\n\nbody\n")},
	}
	adm := newRecordingAdmit()

	res, err := c.CommitChain(context.Background(), items, adm.admit)
	if err != nil {
		t.Fatalf("CommitChain error: %v", err)
	}
	if store.updateCalls != 0 || store.bodies[50] != "human edit" {
		t.Errorf("conflicting issue was written: updates=%d body=%q", store.updateCalls, store.bodies[50])
	}
	if store.createCalls != 1 {
		t.Errorf("creates = %d, want 1 (item 3 never processed)", store.createCalls)
	}
	if router.intakeCalls != 1 || router.punchOutCalls != 0 {
		t.Errorf("router calls: intake=%d punchOut=%d, want 1 and 0", router.intakeCalls, router.punchOutCalls)
	}
	if len(res.Committed) != 1 || res.StoppedAt != 1 {
		t.Errorf("result = %+v, want 1 committed and StoppedAt 1", res)
	}
	if res.Stopped == nil || res.Stopped.Commit.Outcome != provenance.OutcomeRoutedToIntake || res.Stopped.Commit.Number != 50 {
		t.Errorf("Stopped = %+v, want routed-to-intake for #50", res.Stopped)
	}
}

func TestCommitChain_EmptyAndInvalidInput(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})

	res, err := c.CommitChain(context.Background(), nil, newRecordingAdmit().admit)
	if err != nil || len(res.Committed) != 0 || res.StoppedAt != -1 || res.Stopped != nil {
		t.Errorf("empty chain: res=%+v err=%v", res, err)
	}

	items := []provenance.ChainItem{{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")}}
	if _, err := c.CommitChain(context.Background(), items, nil); !errors.Is(err, provenance.ErrInvalid) {
		t.Errorf("nil admit: err = %v, want ErrInvalid", err)
	}

	dup := []provenance.ChainItem{
		{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")},
		{Key: "a", Title: "A2", Doc: chainDoc("# A2\n\nbody\n")},
	}
	if _, err := c.CommitChain(context.Background(), dup, newRecordingAdmit().admit); !errors.Is(err, provenance.ErrInvalid) {
		t.Errorf("duplicate key: err = %v, want ErrInvalid", err)
	}
	if store.writes() != 0 {
		t.Errorf("store writes = %d, want 0 for invalid input", store.writes())
	}
}

func TestCommitChain_RejectsUnsupportedKeyFormat(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})

	// "schema.v2" cannot form a valid "#{schema.v2}" placeholder (the dot is
	// outside the key character class), so a later item could never refer to
	// it and the literal reference would reach the committed body. Reject it
	// before any issue is written.
	items := []provenance.ChainItem{
		{Key: "schema.v2", Title: "A", Doc: chainDoc("# A\n\nbody\n")},
	}
	if _, err := c.CommitChain(context.Background(), items, newRecordingAdmit().admit); !errors.Is(err, provenance.ErrInvalid) {
		t.Errorf("unsupported key format: err = %v, want ErrInvalid", err)
	}
	if store.writes() != 0 {
		t.Errorf("store writes = %d, want 0 for invalid input", store.writes())
	}
}

func TestCommitChain_AdmitErrorStops(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	boom := errors.New("boom")
	items := []provenance.ChainItem{{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")}}

	res, err := c.CommitChain(context.Background(), items, func(string) (guard.Result, error) {
		return guard.Result{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if store.writes() != 0 || res.StoppedAt != 0 || len(res.Committed) != 0 {
		t.Errorf("res=%+v writes=%d", res, store.writes())
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestCommitChainWithKnown_SubstitutesSeededNumber(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "b", Title: "B", Doc: chainDoc("# B\n\nUse a.\n\nDependencies: #{a}\n")},
	}
	adm := newRecordingAdmit()

	res, err := c.CommitChainWith(context.Background(), items, adm.admit, map[string]int{"a": 412})
	if err != nil {
		t.Fatalf("CommitChainWith error: %v", err)
	}
	if len(res.Committed) != 1 || res.StoppedAt != -1 || res.Stopped != nil {
		t.Fatalf("result = %+v, want 1 committed", res)
	}
	stored := store.bodies[res.Committed[0].Result.Number]
	if !strings.Contains(stored, "Dependencies: #412") {
		t.Errorf("stored body lacks the real number:\n%s", stored)
	}
	if strings.Contains(stored, "#{") {
		t.Errorf("stored body still has a placeholder:\n%s", stored)
	}
	if len(adm.raws) != 1 || strings.Contains(adm.raws[0], "#{") {
		t.Errorf("admit did not see the substituted document: %v", adm.raws)
	}
	// Classify validates the stored hash against the committed body.
	if st := provenance.Classify(stored); st != provenance.StateValid {
		t.Errorf("Classify(stored) = %v, want valid", st)
	}
	got := (&watcher.DependencyParser{}).ParseDependencies(stored)
	if !reflect.DeepEqual(got, []int{412}) {
		t.Errorf("ParseDependencies = %v, want [412]", got)
	}
}

func TestCommitChainWithKnown_ItemCanUseSeededAndCommittedKeys(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	items := []provenance.ChainItem{
		{Key: "b", Title: "B", Doc: chainDoc("# B\n\nbody\n")},
		{Key: "c", Title: "C", Doc: chainDoc("# C\n\nbody\n\nDependencies: #{a}, #{b}\n")},
	}

	res, err := c.CommitChainWith(context.Background(), items, newRecordingAdmit().admit, map[string]int{"a": 7})
	if err != nil {
		t.Fatalf("CommitChainWith error: %v", err)
	}
	b, cc := res.Committed[0].Result.Number, res.Committed[1].Result.Number
	if want := "Dependencies: #7, #" + itoa(b); !strings.Contains(store.bodies[cc], want) {
		t.Errorf("stored body lacks %q:\n%s", want, store.bodies[cc])
	}
}

func TestCommitChainWithKnown_DuplicateKeyWritesNothing(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	adm := newRecordingAdmit()
	items := []provenance.ChainItem{
		{Key: "x", Title: "X", Doc: chainDoc("# X\n\nbody\n")},
		{Key: "a", Title: "A", Doc: chainDoc("# A\n\nbody\n")},
	}

	res, err := c.CommitChainWith(context.Background(), items, adm.admit, map[string]int{"a": 412})
	if !errors.Is(err, provenance.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if store.writes() != 0 || len(adm.raws) != 0 || len(res.Committed) != 0 {
		t.Errorf("writes=%d admits=%d res=%+v, want nothing", store.writes(), len(adm.raws), res)
	}
}

func TestCommitChainWithKnown_InvalidKnownEntryWritesNothing(t *testing.T) {
	for name, known := range map[string]map[string]int{
		"bad key":         {"schema.v2": 5},
		"zero number":     {"a": 0},
		"negative number": {"a": -3},
	} {
		t.Run(name, func(t *testing.T) {
			store := newChainStore()
			c := newChainCommitter(store, &chainRouter{})
			items := []provenance.ChainItem{{Key: "b", Title: "B", Doc: chainDoc("# B\n\nbody\n")}}

			_, err := c.CommitChainWith(context.Background(), items, newRecordingAdmit().admit, known)
			if !errors.Is(err, provenance.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if store.writes() != 0 {
				t.Errorf("store writes = %d, want 0", store.writes())
			}
		})
	}
}

func TestCommitChainWithKnown_CallerMapNotMutated(t *testing.T) {
	store := newChainStore()
	c := newChainCommitter(store, &chainRouter{})
	known := map[string]int{"a": 412}
	items := []provenance.ChainItem{{Key: "b", Title: "B", Doc: chainDoc("# B\n\nbody\n")}}

	if _, err := c.CommitChainWith(context.Background(), items, newRecordingAdmit().admit, known); err != nil {
		t.Fatalf("CommitChainWith error: %v", err)
	}
	if !reflect.DeepEqual(known, map[string]int{"a": 412}) {
		t.Errorf("known was mutated: %v", known)
	}
}
