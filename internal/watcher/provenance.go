package watcher

import "github.com/jbrinkman/kairon/internal/github"

// Provenance is the watcher's view of an issue body's planner provenance.
type Provenance int

const (
	ProvenanceUnknown  Provenance = iota // zero value, invalid: fail closed
	ProvenanceValid                      // body carries a planner marker whose hash matches
	ProvenanceNone                       // no planner marker (manually created issue)
	ProvenanceMismatch                   // marker present, hash does not match (edited after generation)
)

// String returns the lower-case name of the provenance state. Any value that
// is not one of the three valid states reports "unknown".
func (p Provenance) String() string {
	switch p {
	case ProvenanceValid:
		return "valid"
	case ProvenanceNone:
		return "none"
	case ProvenanceMismatch:
		return "mismatch"
	default:
		return "unknown"
	}
}

// ProvenanceClassifier is satisfied by the #367 classify logic (directly or via an adapter).
type ProvenanceClassifier interface {
	Classify(issueBody string) Provenance
}

// ProvenanceClassifierFunc adapts a plain function to ProvenanceClassifier.
type ProvenanceClassifierFunc func(issueBody string) Provenance

// Classify calls f.
func (f ProvenanceClassifierFunc) Classify(issueBody string) Provenance { return f(issueBody) }

// PlannerRunner starts one planner run for an issue and blocks until it ends.
type PlannerRunner interface {
	Run(issue github.Issue, provenance Provenance) error
}
