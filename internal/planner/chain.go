package planner

import (
	"context"
	"fmt"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// ChainRequest is one issue of a split, in dependency order.
type ChainRequest struct {
	// Key names the issue so a later issue can refer to it as "#{Key}" on its
	// Dependencies: line. Empty means no later issue can refer to it.
	Key string
	// Reply is the agent's whole reply for this issue: exactly one canonical
	// stage-output document.
	Reply string
	// Title is the issue title. Empty means the first "# " heading of the
	// body.
	Title string
	// Target says where the body goes, as for Request.Target.
	Target provenance.Target
}

// chainAttempt is the attempt the guard chain sees for every item. A split is
// committed as one unit by the caller, who retries the whole split; this
// package never counts attempts.
var chainAttempt = guard.Attempt{Current: 1, Max: 1}

// ProcessChain commits an ordered multi-issue split.
//
// Every reply is extracted and parsed before anything is written, so a reply
// that is not exactly one canonical document fails the call with
// guard.ErrInvalid and leaves GitHub untouched. The documents then go to
// provenance.CommitChain, which resolves each "#{key}" dependency into the
// single-line "Dependencies: #N" form from the numbers committed so far,
// passes the substituted document to the guard chain (Boundary.Guards, else
// the default chain followed by DependenciesForm) and only then hashes and
// writes it. This package adds no substitution, hashing or commit logic.
//
// A guard rejection or a revision conflict stops the chain without an error:
// the returned ChainResult says where (StoppedAt, Stopped) and lists the
// issues already committed, which stay committed. A forward reference or an
// unknown key stops it with provenance.ErrUnknownPlaceholder.
func (b *Boundary) ProcessChain(ctx context.Context, items []ChainRequest) (provenance.ChainResult, error) {
	none := provenance.ChainResult{StoppedAt: -1}
	if b.Committer == nil {
		return none, fmt.Errorf("%w: boundary has no committer", provenance.ErrInvalid)
	}
	if len(items) == 0 {
		return none, fmt.Errorf("%w: a chain needs at least one item", provenance.ErrInvalid)
	}

	chain := make([]provenance.ChainItem, 0, len(items))
	for i, it := range items {
		text, err := Extract(it.Reply)
		if err != nil {
			return none, fmt.Errorf("%w: chain item %d (%q): %v", guard.ErrInvalid, i, it.Key, err)
		}
		doc, err := stageoutput.Parse([]byte(text))
		if err != nil {
			return none, fmt.Errorf("%w: chain item %d (%q): %v", guard.ErrInvalid, i, it.Key, err)
		}
		if doc.Result != stageoutput.ResultCanonical {
			return none, fmt.Errorf("%w: chain item %d (%q): result %q is not canonical; a split commits canonical documents only",
				guard.ErrInvalid, i, it.Key, doc.Result)
		}
		title := it.Title
		if title == "" {
			title = titleFromBody(doc.Body)
		}
		chain = append(chain, provenance.ChainItem{Key: it.Key, Title: title, Doc: *doc, Target: it.Target})
	}

	build := b.Guards
	if build == nil {
		build = defaultGuards
	}
	admit := func(raw string) (guard.Result, error) {
		route := &failure.Route{}
		return guard.Run(guard.Input{Stage: stageoutput.StagePlanner, Raw: raw, Attempt: chainAttempt}, build(route.Recoverable()))
	}
	return b.Committer.CommitChain(ctx, chain, admit)
}
