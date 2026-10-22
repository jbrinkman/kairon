package failure

import (
	"sync"

	"github.com/jbrinkman/kairon/internal/guard"
)

// StageResult is what one execution of a stage reports. A nil Failure means
// the stage succeeded.
type StageResult struct {
	// Failure is why the stage's output was rejected. Nil means success.
	Failure *guard.FailureContext
	// Disposition says whether the failure may be retried.
	Disposition Disposition
	// Remedy is optional text for the punch-out comment. Empty uses the default.
	Remedy string
}

// Route connects a guard's failure method to the mechanism. guard.FailureHandler
// is func(FailureContext) error, so the recoverable-versus-terminal decision
// cannot travel in the context. The guard owns that decision by choosing which
// handler to install: Recoverable or Terminal. Route records the context and
// the disposition, and the stage returns Result to the Runner, which enacts it.
//
// The zero value is ready to use. A Route holds one outcome; use a new Route
// for each execution of a stage. If a handler is called more than once, the
// last call wins. It is safe for concurrent use.
type Route struct {
	mu          sync.Mutex
	failure     *guard.FailureContext
	disposition Disposition
	remedy      string
}

// Recoverable returns a handler that records fc as a failure that may be
// retried while attempts remain.
func (r *Route) Recoverable() guard.FailureHandler {
	return func(fc guard.FailureContext) error {
		r.record(fc, Recoverable, "")
		return nil
	}
}

// Terminal returns a handler that records fc as a failure that must punch out
// immediately, with no retries. remedy is optional text for the comment. A
// planner punch-out calls it with Attempt{1, 1} (requirements 3.5).
func (r *Route) Terminal(remedy string) guard.FailureHandler {
	return func(fc guard.FailureContext) error {
		r.record(fc, Unrecoverable, remedy)
		return nil
	}
}

// Result returns what the handlers recorded. With nothing recorded, Failure is
// nil, which the Runner treats as success. The returned Failure is a copy.
func (r *Route) Result() StageResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := StageResult{Disposition: r.disposition, Remedy: r.remedy}
	if r.failure != nil {
		fc := *r.failure
		res.Failure = &fc
	}
	return res
}

func (r *Route) record(fc guard.FailureContext, d Disposition, remedy string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := fc
	r.failure, r.disposition, r.remedy = &c, d, remedy
}
