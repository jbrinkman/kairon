package provenance

import (
	"context"
	"fmt"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// ChainItem is one proposed issue in a chain.
//
// Key names the item so a later item can refer to it as "#{Key}" on its
// dependencies line. Items with an empty Key cannot be referred to.
type ChainItem struct {
	Key    string
	Title  string
	Doc    stageoutput.Document
	Target Target
}

// Admit is the boundary's wrapper around guard.Run. It receives the exact
// stage-output text for one item, after dependency substitution, and returns
// the guard chain's result. A non-nil error means the chain could not run.
type Admit func(raw string) (guard.Result, error)

// Committed is an item the chain wrote.
type Committed struct {
	Key    string
	Result Result
}

// StopReason describes the item a chain stopped at.
//
// If the guard chain did not return OutcomeReadyToCommit, Admission holds
// that result and Commit is the zero Result. If the guard chain passed but
// Commit did not write (the issue changed under the planner), Admission holds
// the passing result and Commit holds the routed Result.
type StopReason struct {
	Admission guard.Result
	Commit    Result
}

// ChainResult reports how far CommitChain got.
type ChainResult struct {
	// Committed lists the issues written, in order. A caller can use it to
	// resume. Nothing is rolled back.
	Committed []Committed
	// StoppedAt is the index of the item that did not commit, or -1 if every
	// item committed.
	StoppedAt int
	// Stopped is nil when every item committed. It is also nil when the chain
	// stopped on an error before the guard chain or Commit produced a result;
	// the error is returned alongside the ChainResult.
	Stopped *StopReason
}

// CommitChain commits a chain of proposed issues in order.
//
// For each item it:
//
//  1. substitutes "#{key}" placeholders on dependencies lines using the
//     numbers committed so far (SubstituteDependencies), onto a copy of the
//     item's document;
//  2. marshals that copy and passes the text to admit, so the guard chain
//     sees the already substituted document;
//  3. stops if the guard result is not guard.OutcomeReadyToCommit (the guard
//     already ran its OnFailure);
//  4. calls Commit with the substituted document, so Mark hashes the
//     substituted body;
//  5. stops if Commit did not return OutcomeCommitted (a revision conflict),
//     otherwise records the new issue number under item.Key.
//
// Later items are not attempted after a stop, because they may depend on the
// item that stopped. GitHub has no multi-write transaction, so issues already
// committed stay committed. The returned ChainResult lists them so the caller
// can resume.
//
// A stop caused by a guard rejection or a conflict returns a nil error. An
// error (ErrInvalid for a malformed chain, ErrUnknownPlaceholder or
// ErrBadDependencyLine from substitution, or an error from admit, Marshal or
// Commit) stops the chain at that item and is returned wrapped together with
// the ChainResult so far. Items are never written when the chain is
// malformed.
func (c *Committer) CommitChain(ctx context.Context, items []ChainItem, admit Admit) (ChainResult, error) {
	res := ChainResult{StoppedAt: -1}
	if admit == nil {
		return res, fmt.Errorf("%w: no admit function", ErrInvalid)
	}
	if err := validateChainKeys(items); err != nil {
		return res, err
	}

	numbers := make(map[string]int, len(items))
	for i, item := range items {
		fail := func(err error) (ChainResult, error) {
			res.StoppedAt = i
			return res, fmt.Errorf("provenance: chain item %d (%q): %w", i, item.Key, err)
		}

		body, err := SubstituteDependencies(item.Doc.Body, numbers)
		if err != nil {
			return fail(err)
		}
		doc := item.Doc // copy: the caller's item is not modified
		doc.Body = body

		raw, err := stageoutput.Marshal(&doc)
		if err != nil {
			return fail(err)
		}
		admission, err := admit(string(raw))
		if err != nil {
			return fail(fmt.Errorf("admit: %w", err))
		}
		if admission.Outcome != guard.OutcomeReadyToCommit {
			res.StoppedAt = i
			res.Stopped = &StopReason{Admission: admission}
			return res, nil
		}

		out, err := c.Commit(ctx, Request{Doc: &doc, Title: item.Title, Target: item.Target, Admission: admission})
		if err != nil {
			return fail(err)
		}
		if out.Outcome != OutcomeCommitted {
			res.StoppedAt = i
			res.Stopped = &StopReason{Admission: admission, Commit: out}
			return res, nil
		}

		res.Committed = append(res.Committed, Committed{Key: item.Key, Result: out})
		if item.Key != "" {
			numbers[item.Key] = out.Number
		}
	}
	return res, nil
}

// validateChainKeys rejects a chain in which two items share a non-empty key,
// because the later number would silently replace the earlier one. It also
// rejects a non-empty key that cannot form a valid "#{key}" placeholder, since
// a later item could never refer to it and the unsubstituted reference would
// reach the committed body.
func validateChainKeys(items []ChainItem) error {
	seen := make(map[string]int, len(items))
	for i, item := range items {
		if item.Key == "" {
			continue
		}
		if !chainKeyRe.MatchString(item.Key) {
			return fmt.Errorf("%w: item %d key %q is not a valid placeholder key", ErrInvalid, i, item.Key)
		}
		if first, dup := seen[item.Key]; dup {
			return fmt.Errorf("%w: items %d and %d share key %q", ErrInvalid, first, i, item.Key)
		}
		seen[item.Key] = i
	}
	return nil
}
