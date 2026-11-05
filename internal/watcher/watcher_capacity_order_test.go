package watcher

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/github"
)

// AC2 with a slot that frees mid-poll: once an issue waits for a slot, no
// younger issue starts in the same poll, even if a slot frees before the
// younger issue reaches the gate. The freed slot goes to the older issue on
// the next poll.
func TestCheckIssues_SlotFreedMidPollDoesNotJumpTheQueue(t *testing.T) {
	twoIssues := func(w *Watcher) {
		w.selectIssues = func(_, _, _ string) (github.Selection, error) {
			return github.Selection{Eligible: []github.Issue{{Number: 10, Title: "a"}, {Number: 11, Title: "b"}}}, nil
		}
	}

	t.Run("gate", func(t *testing.T) {
		w, spawned := newSpawnWatcher(t, nil)
		twoIssues(w)
		// Full when #10 is checked; a run exits before #11 is checked. After
		// that the only slot is taken by whatever this watcher starts.
		var reads int
		w.slots = func() (int, int) {
			reads++
			if reads == 1 {
				return 1, 1
			}
			return len(*spawned), 1
		}
		buf := captureLog(t)

		w.checkIssues()

		if len(*spawned) != 0 {
			t.Fatalf("spawned = %v, want none in this poll (#10 is older and waiting)", *spawned)
		}
		for _, n := range []int{10, 11} {
			want := fmt.Sprintf("issue #%d waiting for an agent slot (1/1 running)", n)
			if !strings.Contains(strings.Join(buf.lines(), "\n"), want) {
				t.Errorf("log has no %q:\n%s", want, strings.Join(buf.lines(), "\n"))
			}
		}
		if got := trackedKeys(w); len(got) != 0 {
			t.Errorf("tracked = %v, want none", got)
		}

		w.checkIssues()
		if want := []int{10}; !reflect.DeepEqual(*spawned, want) {
			t.Errorf("next poll spawned = %v, want %v", *spawned, want)
		}
	})

	t.Run("spawn refused at capacity", func(t *testing.T) {
		w, spawned := newSpawnWatcher(t, nil)
		twoIssues(w)
		w.slots = func() (int, int) { return 0, 1 }
		w.spawn = func(issue int, _ string) (*agent.Agent, error) {
			*spawned = append(*spawned, issue)
			if issue == 10 {
				return nil, fmt.Errorf("%w (1/1 running)", agent.ErrAtCapacity)
			}
			return nil, nil
		}

		w.checkIssues()

		if want := []int{10}; !reflect.DeepEqual(*spawned, want) {
			t.Errorf("spawned = %v, want %v (#11 must not start after #10 was refused)", *spawned, want)
		}
	})
}
