package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github"
)

type reconcileCall struct {
	repo      string
	issue     int
	failLabel string
}

// newTestWatcher returns a watcher with stubbed seams and chdirs to a temp dir,
// because retry count files are relative to the working directory.
func newTestWatcher(t *testing.T) (*Watcher, *[]reconcileCall) {
	t.Helper()
	t.Chdir(t.TempDir())
	cfg := &config.Config{Repo: "o/r", Label: "kairon", MaxRetries: 3, PollInterval: time.Minute}
	w := New(cfg, nil, "")
	var calls []reconcileCall
	w.reconcile = func(repo string, issue int, failLabel string) error {
		calls = append(calls, reconcileCall{repo, issue, failLabel})
		return nil
	}
	w.selectIssues = func(repo, label, assignee string) (github.Selection, error) {
		t.Fatalf("selectIssues must be stubbed by the test that needs it")
		return github.Selection{}, nil
	}
	return w, &calls
}

func writeRetryFile(t *testing.T, issue, count int) string {
	t.Helper()
	dir := filepath.Join(".kairon", "retries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, fmt.Sprintf("issue-%d.count", issue))
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d\n", count)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func trackedKeys(w *Watcher) []int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var keys []int
	for k := range w.tracked {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func TestReconcilePending_CallsReconcileAndResetsRetryCount(t *testing.T) {
	w, calls := newTestWatcher(t)
	p1 := writeRetryFile(t, 5, 3)
	p2 := writeRetryFile(t, 6, 2)

	w.reconcilePending([]github.Issue{{Number: 5}, {Number: 6}})

	want := []reconcileCall{
		{"o/r", 5, "kairon-failed"},
		{"o/r", 6, "kairon-failed"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("reconcile calls = %v, want %v", *calls, want)
	}
	if fileExists(p1) || fileExists(p2) {
		t.Errorf("retry files should be removed after reconcile: %s exists=%v, %s exists=%v", p1, fileExists(p1), p2, fileExists(p2))
	}
}

func TestReconcilePending_ErrorDoesNotStopOthers(t *testing.T) {
	w, _ := newTestWatcher(t)
	p5 := writeRetryFile(t, 5, 3)
	p6 := writeRetryFile(t, 6, 3)
	p7 := writeRetryFile(t, 7, 3)
	var seen []int
	w.reconcile = func(repo string, issue int, failLabel string) error {
		seen = append(seen, issue)
		if issue == 6 {
			return errors.New("boom")
		}
		return nil
	}

	w.reconcilePending([]github.Issue{{Number: 5}, {Number: 6}, {Number: 7}})

	if want := []int{5, 6, 7}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("reconciled = %v, want %v", seen, want)
	}
	if fileExists(p5) || fileExists(p7) {
		t.Error("retry files of successfully reconciled issues should be removed")
	}
	if !fileExists(p6) {
		t.Error("retry file of the issue whose reconcile failed must be kept")
	}
}

func TestUntrackAbsent_DropsExcludedKeepsEligible(t *testing.T) {
	w, _ := newTestWatcher(t)
	for _, n := range []int{1, 2, 3} {
		w.tracked[n] = true
	}

	w.untrackAbsent([]github.Issue{{Number: 2}, {Number: 9}})

	if got, want := trackedKeys(w), []int{2}; !reflect.DeepEqual(got, want) {
		t.Errorf("tracked after untrackAbsent = %v, want %v", got, want)
	}
}

func TestCheckIssues_PollErrorLeavesTrackedUnchanged(t *testing.T) {
	w, calls := newTestWatcher(t)
	w.tracked[1] = true
	w.tracked[2] = true
	w.selectIssues = func(repo, label, assignee string) (github.Selection, error) {
		return github.Selection{}, errors.New("gh down")
	}

	w.checkIssues()

	if got, want := trackedKeys(w), []int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("tracked after poll error = %v, want %v", got, want)
	}
	if len(*calls) != 0 {
		t.Errorf("no reconcile expected on poll error, got %v", *calls)
	}
}

func TestCheckIssues_UntracksAbsentAndNeverSpawnsTerminalPending(t *testing.T) {
	// The manager is nil: any Spawn call would panic, so reaching the end of
	// checkIssues proves terminal-pending issues are not passed to Spawn.
	w, calls := newTestWatcher(t)
	w.tracked[4] = true // punched out and labeled since: absent from the poll
	w.tracked[8] = true // also absent from the poll
	w.selectIssues = func(repo, label, assignee string) (github.Selection, error) {
		return github.Selection{
			TerminalPending: []github.Issue{{Number: 5}},
		}, nil
	}

	w.checkIssues()

	if want := []reconcileCall{{"o/r", 5, "kairon-failed"}}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("reconcile calls = %v, want %v", *calls, want)
	}
	if got := trackedKeys(w); len(got) != 0 {
		t.Errorf("tracked = %v, want all dropped (none eligible)", got)
	}
}

func TestRestartedIssueReentersWithFreshRetryBudget(t *testing.T) {
	w, _ := newTestWatcher(t)
	w.incrementGlobalRetryCount(5)
	w.incrementGlobalRetryCount(5)
	w.incrementGlobalRetryCount(5)
	if !w.hasExceededGlobalRetries(5) {
		t.Fatal("precondition: budget should be exhausted after 3 attempts")
	}

	w.reconcilePending([]github.Issue{{Number: 5}})

	if w.hasExceededGlobalRetries(5) {
		t.Error("hasExceededGlobalRetries should be false after the retry count is reset")
	}
	if got := w.getCurrentRetryCount(5); got != 0 {
		t.Errorf("retry count = %d, want 0", got)
	}
}
