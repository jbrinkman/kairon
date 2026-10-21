package failure

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const testFailLabel = "kairon-failed"

var testTarget = Target{Repo: "o/r", Issue: 7, FailLabel: testFailLabel}

func testFC() guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StageBuilder,
		Guard:    "structural",
		Attempt:  guard.Attempt{Current: 4, Max: 4},
		Expected: "all sections present",
		Actual:   "missing Scope",
	}
}

func TestPunchOut_CommentBeforeLabel(t *testing.T) {
	f := newFake()
	if err := PunchOut(f, testTarget, "run1", testFC(), "fix it"); err != nil {
		t.Fatalf("PunchOut: %v", err)
	}
	want := []string{"State", "PostComment", "AddLabel " + testFailLabel}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	ids := f.postedRunIDs()
	if len(ids) != 1 || ids[0] != "run1" {
		t.Fatalf("posted run ids = %v, want [run1]", ids)
	}
	if !strings.Contains(f.comments[0].Body, "fix it") {
		t.Errorf("comment does not carry the remedy:\n%s", f.comments[0].Body)
	}
}

func TestPunchOut_CommentFailureLeavesNoLabel(t *testing.T) {
	f := newFake()
	f.failNext("PostComment", 1)
	err := PunchOut(f, testTarget, "run1", testFC(), "")
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if n := f.count("AddLabel"); n != 0 {
		t.Fatalf("AddLabel called %d times after the comment failed; calls = %v", n, f.calls)
	}
	if len(f.labels) != 0 {
		t.Fatalf("labels = %v, want none", f.labels)
	}
}

func TestPunchOut_RetryAfterPartialPostsNoDuplicate(t *testing.T) {
	f := newFake()
	f.failNext("AddLabel", 1)
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); !errors.Is(err, errBoom) {
		t.Fatalf("first call err = %v, want errBoom", err)
	}
	if got := f.count("PostComment"); got != 1 {
		t.Fatalf("comments after first call = %d, want 1", got)
	}
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := f.count("PostComment"); got != 1 {
		t.Fatalf("comments after second call = %d, want 1 (no duplicate); calls = %v", got, f.calls)
	}
	if !reflect.DeepEqual(f.labels, []string{testFailLabel}) {
		t.Fatalf("labels = %v, want [%s]", f.labels, testFailLabel)
	}
}

func TestPunchOut_LabelAlreadyPresentIsNoop(t *testing.T) {
	f := newFake()
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	before := len(f.writes())
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if after := len(f.writes()); after != before {
		t.Fatalf("second call wrote again: writes = %v", f.writes())
	}
}

func TestPunchOut_NewRunIDPostsFreshComment(t *testing.T) {
	f := newFake()
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err != nil {
		t.Fatalf("run1: %v", err)
	}
	f.clearLabel(testFailLabel) // a human fixes the cause and removes the label
	if err := PunchOut(f, testTarget, "run2", testFC(), ""); err != nil {
		t.Fatalf("run2: %v", err)
	}
	if got := f.postedRunIDs(); !reflect.DeepEqual(got, []string{"run1", "run2"}) {
		t.Fatalf("posted run ids = %v, want [run1 run2]", got)
	}
	if !reflect.DeepEqual(f.labels, []string{testFailLabel}) {
		t.Fatalf("labels = %v, want the fail label applied again", f.labels)
	}
}

func TestPunchOut_IgnoresMarkerNotByViewer(t *testing.T) {
	f := newFake()
	f.seedComment(Marker("run1")+"\n\npasted by someone else", false)
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err != nil {
		t.Fatalf("PunchOut: %v", err)
	}
	if got := f.count("PostComment"); got != 1 {
		t.Fatalf("PostComment calls = %d, want 1: an untrusted marker must not suppress the comment", got)
	}
}

func TestPunchOut_InvalidInputWritesNothing(t *testing.T) {
	badFC := testFC()
	badFC.Expected = ""
	cases := []struct {
		name   string
		iss    Issues
		target Target
		runID  string
		fc     guard.FailureContext
	}{
		{"empty run id", newFake(), testTarget, "", testFC()},
		{"run id with spaces", newFake(), testTarget, "a b", testFC()},
		{"invalid failure context", newFake(), testTarget, "run1", badFC},
		{"empty repo", newFake(), Target{Issue: 7, FailLabel: testFailLabel}, "run1", testFC()},
		{"zero issue", newFake(), Target{Repo: "o/r", FailLabel: testFailLabel}, "run1", testFC()},
		{"empty fail label", newFake(), Target{Repo: "o/r", Issue: 7}, "run1", testFC()},
		{"nil issues", nil, testTarget, "run1", testFC()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := PunchOut(tc.iss, tc.target, tc.runID, tc.fc, "")
			if err == nil {
				t.Fatal("want an error")
			}
			if f, ok := tc.iss.(*fakeIssues); ok && len(f.calls) != 0 {
				t.Fatalf("invalid input touched GitHub: %v", f.calls)
			}
		})
	}
}

func TestPunchOut_StateErrorWritesNothing(t *testing.T) {
	f := newFake()
	f.failNext("State", 1)
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("writes after State error = %v", w)
	}
}

func TestReconcile_AppliesMissingLabelOnly(t *testing.T) {
	f := newFake()
	f.failNext("AddLabel", 1)
	if err := PunchOut(f, testTarget, "run1", testFC(), ""); err == nil {
		t.Fatal("setup: want the label write to fail")
	}
	f.calls = nil

	applied, err := Reconcile(f, testTarget)
	if err != nil || !applied {
		t.Fatalf("Reconcile = (%v, %v), want (true, nil)", applied, err)
	}
	want := []string{"State", "AddLabel " + testFailLabel}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v (no comment)", f.calls, want)
	}
}

func TestReconcile_NoopWhenNotPending(t *testing.T) {
	cases := map[string]func(f *fakeIssues){
		"no marker": func(f *fakeIssues) {},
		"already failed": func(f *fakeIssues) {
			f.seedComment(Marker("run1")+"\n", true)
			f.seedLabel(testFailLabel)
		},
		"done": func(f *fakeIssues) {
			f.seedComment(Marker("run1")+"\n", true)
			f.seedLabel("kairon-done")
		},
		"label cleared after marker": func(f *fakeIssues) {
			f.seedComment(Marker("run1")+"\n", true)
			f.seedLabel(testFailLabel)
			f.clearLabel(testFailLabel)
		},
		"marker not by viewer": func(f *fakeIssues) { f.seedComment(Marker("run1")+"\n", false) },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			setup(f)
			applied, err := Reconcile(f, testTarget)
			if err != nil || applied {
				t.Fatalf("Reconcile = (%v, %v), want (false, nil)", applied, err)
			}
			if w := f.writes(); len(w) != 0 {
				t.Fatalf("writes = %v, want none", w)
			}
		})
	}
}

func TestReconcile_StateErrorAndInvalidTarget(t *testing.T) {
	f := newFake()
	f.failNext("State", 1)
	if applied, err := Reconcile(f, testTarget); !errors.Is(err, errBoom) || applied {
		t.Fatalf("Reconcile = (%v, %v), want (false, errBoom)", applied, err)
	}
	g := newFake()
	if _, err := Reconcile(g, Target{}); !errors.Is(err, guard.ErrInvalid) {
		t.Fatalf("err = %v, want wrapping guard.ErrInvalid", err)
	}
	if len(g.calls) != 0 {
		t.Fatalf("invalid target touched GitHub: %v", g.calls)
	}
}
