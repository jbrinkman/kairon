package stageoutput

import (
	"errors"
	"fmt"
	"strings"
)

// CurrentSchemaVersion is the highest schema_version this package understands.
const CurrentSchemaVersion = 1

// Stage names the pipeline stage that produced a document.
type Stage string

// Stages a document may declare.
const (
	StagePlanner    Stage = "planner"
	StageArchitect  Stage = "architect"
	StageKrewLead   Stage = "krew-lead"
	StageBuilder    Stage = "builder"
	StageValidator  Stage = "validator"
	StageDocumenter Stage = "documenter"
)

// Valid reports whether s is one of the defined stages.
func (s Stage) Valid() bool {
	switch s {
	case StagePlanner, StageArchitect, StageKrewLead, StageBuilder, StageValidator, StageDocumenter:
		return true
	}
	return false
}

// Result is the discriminator between the two document variants.
type Result string

// Results a document may declare.
const (
	// ResultCanonical carries a body and no unresolved list.
	ResultCanonical Result = "canonical"
	// ResultPunchOut carries a non-empty unresolved list and no body.
	ResultPunchOut Result = "punch_out"
)

// Valid reports whether r is one of the defined results.
func (r Result) Valid() bool {
	return r == ResultCanonical || r == ResultPunchOut
}

// SourceKind says what kind of input a document was derived from.
type SourceKind string

// Source kinds a document may declare.
const (
	SourceGitHubIssue SourceKind = "github_issue"
	SourceDocument    SourceKind = "document"
)

// Valid reports whether k is one of the defined source kinds.
func (k SourceKind) Valid() bool {
	return k == SourceGitHubIssue || k == SourceDocument
}

// Source identifies the input a document was derived from.
type Source struct {
	Kind SourceKind
	Ref  string
}

// Inference records a slot that automated intake filled by inference, and the
// evidence the inference was grounded in.
type Inference struct {
	Slot     string
	Inferred string
	Evidence string
}

// Unresolved records a slot a punch_out document could not resolve, and why.
type Unresolved struct {
	Slot   string
	Reason string
}

// Document is a stage-output document.
//
// Document carries no struct tags. The YAML key names live on the unexported
// per-version wire structs (see wireV1), so the in-memory type stays
// independent of any one schema version.
//
// For the Go-level checks in Validate, a variant-specific field is "present"
// when Body is non-empty or Unresolved has at least one entry. In YAML, an
// explicit null for body or unresolved is treated as absent.
type Document struct {
	SchemaVersion int
	Stage         Stage
	Result        Result
	Source        Source
	// Generated is an RFC 3339 timestamp in UTC. It is kept as a string so it
	// round-trips verbatim.
	Generated string
	// Body is the substantive content. canonical only.
	Body string
	// Unresolved lists the slots that could not be resolved. punch_out only.
	Unresolved []Unresolved
	// Inferences is an optional audit record, allowed on either variant.
	Inferences []Inference
}

// ErrUnknownVersion is matched (errors.Is) by any error caused by a
// schema_version this package does not understand.
var ErrUnknownVersion = errors.New("unknown schema_version")

// ErrInvalidDocument is matched (errors.Is) by any error caused by a
// structurally invalid or malformed document.
var ErrInvalidDocument = errors.New("invalid stage-output document")

// UnknownVersionError reports a schema_version that is not in the parser
// registry. It matches ErrUnknownVersion and does not match ErrInvalidDocument.
type UnknownVersionError struct {
	// Version is the schema_version the document declared.
	Version int
	// MaxSupported is the highest schema_version this package understands.
	MaxSupported int
}

func (e *UnknownVersionError) Error() string {
	return fmt.Sprintf("%s: document declares %d, highest supported version is %d",
		ErrUnknownVersion.Error(), e.Version, e.MaxSupported)
}

// Is makes errors.Is(err, ErrUnknownVersion) true.
func (e *UnknownVersionError) Is(target error) bool {
	return target == ErrUnknownVersion
}

// ValidationError lists every problem Validate found. Each problem names the
// offending field path, for example "unresolved[1].reason". It matches
// ErrInvalidDocument.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return ErrInvalidDocument.Error() + ": " + strings.Join(e.Problems, "; ")
}

// Unwrap makes errors.Is(err, ErrInvalidDocument) true.
func (e *ValidationError) Unwrap() error {
	return ErrInvalidDocument
}
