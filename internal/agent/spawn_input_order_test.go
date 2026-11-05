package agent

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
)

// A Spawn that is refused because another run holds the issue's slot must not
// touch the input document that run reads. Two overlapping Spawn calls for the
// same issue write the document exactly once, from the call that got the slot.
func TestOverlappingSpawnWritesInputDocumentOnce(t *testing.T) {
	h := newCapHarness(t, 3, 3)
	h.put(10)

	var writes atomic.Int32
	firstInWrite := make(chan struct{})
	releaseFirst := make(chan struct{})
	h.m.writeInput = func(issue int, doc *boundary.InputDocument) (string, error) {
		if writes.Add(1) == 1 {
			close(firstInWrite)
			<-releaseFirst
		}
		return writeInputDocument(issue, doc)
	}

	errs := make(chan error, 2)
	go func() { _, err := h.m.Spawn(10, testRepo); errs <- err }()
	<-firstInWrite

	secondDone := make(chan struct{})
	go func() { _, err := h.m.Spawn(10, testRepo); errs <- err; close(secondDone) }()
	// Give the second call time to run as far as it can while the first is
	// inside its write.
	select {
	case <-secondDone:
	case <-time.After(300 * time.Millisecond):
	}
	close(releaseFirst)

	var ok, refused int
	for range 2 {
		select {
		case err := <-errs:
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrNotAdmitted):
				refused++
			default:
				t.Errorf("Spawn(#10) = %v, want nil or ErrNotAdmitted", err)
			}
		case <-time.After(capWait):
			t.Fatal("Spawn did not return")
		}
	}
	if ok != 1 || refused != 1 {
		t.Errorf("results: %d started, %d refused; want 1 and 1", ok, refused)
	}
	if n := writes.Load(); n != 1 {
		t.Errorf("input document written %d times, want once (by the call that got the slot)", n)
	}
}
