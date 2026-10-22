// Package planner is the deterministic output boundary for the planner agent.
//
// The planner never writes to GitHub. Its final reply is one stage-output
// document (docs/stage-output.md). Boundary.Handle takes that reply and drives
// the parts built in internal/stageoutput, internal/guard, internal/provenance
// and internal/failure:
//
//  1. Extract: the reply must be exactly one document, optionally inside one
//     yaml fence, with no other text.
//  2. Mode contract: an interactive reply is never a punch_out; an automated
//     intake returns exactly one document whose source is the issue.
//  3. guard.Run with the default chain. A rejection is Recoverable, so the
//     caller's failure.Runner decides between a retry and a punch-out.
//  4. punch_out documents go to failure.PunchOut (Tier 2) with Attempt{1,1}.
//  5. canonical documents lose their H1 line (it becomes the issue title) and
//     are committed through provenance.CommitChainWith, which resolves
//     "#{issue-N}" placeholders, hashes the committed body and writes it once.
//
// The package is pure Go behind interfaces (provenance.IssueStore,
// provenance.Router, failure.Issues). It starts no process and makes no
// network call, which deps_test.go enforces.
package planner
