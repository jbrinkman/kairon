package watcher

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/github"
)

// newSpawnWatcher returns a test watcher whose poll yields one eligible issue
// (number 7, no dependencies) and whose spawn seam returns spawnErr.
func newSpawnWatcher(t *testing.T, spawnErr error) (*Watcher, *[]int) {
	t.Helper()
	w, _ := newTestWatcher(t)
	w.selectIssues = func(repo, label, assignee string) (github.Selection, error) {
		return github.Selection{Eligible: []github.Issue{{Number: 7, Title: "t"}}}, nil
	}
	var spawned []int
	w.spawn = func(issue int, repo string) (*agent.Agent, error) {
		spawned = append(spawned, issue)
		return nil, spawnErr
	}
	return w, &spawned
}

func TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount(t *testing.T) {
	w, spawned := newSpawnWatcher(t, fmt.Errorf("%w: skipped (done)", agent.ErrNotAdmitted))

	w.checkIssues()

	if want := []int{7}; !reflect.DeepEqual(*spawned, want) {
		t.Fatalf("spawned = %v, want %v", *spawned, want)
	}
	if got := trackedKeys(w); len(got) != 0 {
		t.Errorf("tracked = %v, want issue untracked after ErrNotAdmitted", got)
	}
	if got := w.getCurrentRetryCount(7); got != 0 {
		t.Errorf("retry count = %d, want 0 (refunded)", got)
	}
	if fileExists(".kairon/retries/issue-7.count") {
		t.Error("retry file should be removed when the refunded count reaches zero")
	}
}

func TestCheckIssues_NotAdmittedRefundKeepsPriorAttempts(t *testing.T) {
	w, _ := newSpawnWatcher(t, fmt.Errorf("%w: punched out", agent.ErrNotAdmitted))
	writeRetryFile(t, 7, 1) // one earlier real attempt

	w.checkIssues()

	if got := w.getCurrentRetryCount(7); got != 1 {
		t.Errorf("retry count = %d, want 1 (only this attempt refunded)", got)
	}
	if got := trackedKeys(w); len(got) != 0 {
		t.Errorf("tracked = %v, want untracked", got)
	}
}

func TestCheckIssues_AdmissionUnavailableUntracksAndRefunds(t *testing.T) {
	w, _ := newSpawnWatcher(t, fmt.Errorf("%w: gh down", agent.ErrAdmissionUnavailable))

	w.checkIssues()

	if got := trackedKeys(w); len(got) != 0 {
		t.Errorf("tracked = %v, want issue untracked after ErrAdmissionUnavailable", got)
	}
	if got := w.getCurrentRetryCount(7); got != 0 {
		t.Errorf("retry count = %d, want 0 (refunded)", got)
	}
	if fileExists(".kairon/retries/issue-7.count") {
		t.Error("retry file should be removed at zero")
	}
}

func TestCheckIssues_OtherSpawnErrorStaysTracked(t *testing.T) {
	w, _ := newSpawnWatcher(t, errors.New("worktree create failed"))

	w.checkIssues()

	if got, want := trackedKeys(w), []int{7}; !reflect.DeepEqual(got, want) {
		t.Errorf("tracked = %v, want %v", got, want)
	}
	if got := w.getCurrentRetryCount(7); got != 1 {
		t.Errorf("retry count = %d, want 1 (unchanged)", got)
	}
}

func TestCheckIssues_SuccessfulSpawnStaysTracked(t *testing.T) {
	w, _ := newSpawnWatcher(t, nil)

	w.checkIssues()

	if got, want := trackedKeys(w), []int{7}; !reflect.DeepEqual(got, want) {
		t.Errorf("tracked = %v, want %v", got, want)
	}
	if got := w.getCurrentRetryCount(7); got != 1 {
		t.Errorf("retry count = %d, want 1", got)
	}
}

func TestDecrementGlobalRetryCount(t *testing.T) {
	w, _ := newTestWatcher(t)

	// Missing file: no error, no file created.
	w.decrementGlobalRetryCount(3)
	if fileExists(".kairon/retries/issue-3.count") {
		t.Error("decrement of a missing count must not create a file")
	}

	p := writeRetryFile(t, 3, 2)
	w.decrementGlobalRetryCount(3)
	if got := w.getCurrentRetryCount(3); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}
	w.decrementGlobalRetryCount(3)
	if fileExists(p) {
		t.Error("file should be removed when count reaches zero")
	}
	w.decrementGlobalRetryCount(3) // floor at zero, no panic
	if got := w.getCurrentRetryCount(3); got != 0 {
		t.Errorf("count = %d, want 0", got)
	}

	// Garbage content: floor, file removed, no error.
	if err := os.MkdirAll(".kairon/retries", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".kairon/retries/issue-4.count", []byte("junk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.decrementGlobalRetryCount(4)
	if got := w.getCurrentRetryCount(4); got != 0 {
		t.Errorf("count = %d, want 0 for unparsable file", got)
	}
}
