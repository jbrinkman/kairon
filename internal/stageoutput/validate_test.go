package stageoutput

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validCanonical() *Document {
	return &Document{
		SchemaVersion: 1,
		Stage:         StagePlanner,
		Result:        ResultCanonical,
		Source:        Source{Kind: SourceGitHubIssue, Ref: "142"},
		Generated:     "2026-10-20T20:00:00Z",
		Body:          "## Problem\nSomething.\n",
	}
}

func validPunchOut() *Document {
	return &Document{
		SchemaVersion: 1,
		Stage:         StagePlanner,
		Result:        ResultPunchOut,
		Source:        Source{Kind: SourceDocument, Ref: "docs/design.md"},
		Generated:     "2026-10-20T20:00:00Z",
		Unresolved: []Unresolved{
			{Slot: "acceptance", Reason: "no testable outcome stated"},
			{Slot: "scope", Reason: "two readings possible"},
		},
	}
}

// assertInvalid asserts err is an ErrInvalidDocument (and not an unknown
// version) whose message names every field in fields.
func assertInvalid(t *testing.T, err error, fields ...string) {
	t.Helper()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument), "want ErrInvalidDocument, got %v", err)
	assert.False(t, errors.Is(err, ErrUnknownVersion))
	for _, f := range fields {
		assert.Contains(t, err.Error(), f)
	}
}

func TestValidate_ResultUnion(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *Document)
		base    func() *Document
		wantErr []string // empty means valid
	}{
		{name: "valid canonical", base: validCanonical},
		{name: "valid punch_out", base: validPunchOut},
		{
			name: "canonical with unresolved", base: validCanonical,
			mutate:  func(d *Document) { d.Unresolved = []Unresolved{{Slot: "a", Reason: "b"}} },
			wantErr: []string{"unresolved"},
		},
		{
			name: "punch_out with body", base: validPunchOut,
			mutate:  func(d *Document) { d.Body = "text" },
			wantErr: []string{"body"},
		},
		{
			name: "punch_out with empty unresolved", base: validPunchOut,
			mutate:  func(d *Document) { d.Unresolved = []Unresolved{} },
			wantErr: []string{"unresolved"},
		},
		{
			name: "punch_out with nil unresolved", base: validPunchOut,
			mutate:  func(d *Document) { d.Unresolved = nil },
			wantErr: []string{"unresolved"},
		},
		{
			name: "canonical with empty body", base: validCanonical,
			mutate:  func(d *Document) { d.Body = "" },
			wantErr: []string{"body"},
		},
		{
			name: "canonical with whitespace-only body", base: validCanonical,
			mutate:  func(d *Document) { d.Body = " \n\t\n" },
			wantErr: []string{"body"},
		},
		{
			name: "punch_out entry missing slot", base: validPunchOut,
			mutate:  func(d *Document) { d.Unresolved[1].Slot = "" },
			wantErr: []string{"unresolved[1].slot"},
		},
		{
			name: "punch_out entry missing reason", base: validPunchOut,
			mutate:  func(d *Document) { d.Unresolved[0].Reason = "  " },
			wantErr: []string{"unresolved[0].reason"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.base()
			if tc.mutate != nil {
				tc.mutate(d)
			}
			err := d.Validate()
			if len(tc.wantErr) == 0 {
				assert.NoError(t, err)
				return
			}
			assertInvalid(t, err, tc.wantErr...)
		})
	}
}

func TestValidate_Envelope(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *Document)
		wantErr string
	}{
		{"schema_version zero", func(d *Document) { d.SchemaVersion = 0 }, "schema_version"},
		{"schema_version negative", func(d *Document) { d.SchemaVersion = -1 }, "schema_version"},
		{"stage empty", func(d *Document) { d.Stage = "" }, "stage"},
		{"stage not in enum", func(d *Document) { d.Stage = "designer" }, "stage"},
		{"result empty", func(d *Document) { d.Result = "" }, "result"},
		{"result not in enum", func(d *Document) { d.Result = "partial" }, "result"},
		{"source kind empty", func(d *Document) { d.Source.Kind = "" }, "source.kind"},
		{"source kind not in enum", func(d *Document) { d.Source.Kind = "jira" }, "source.kind"},
		{"source ref empty", func(d *Document) { d.Source.Ref = "" }, "source.ref"},
		{"source ref whitespace", func(d *Document) { d.Source.Ref = "  \t" }, "source.ref"},
		{"generated empty", func(d *Document) { d.Generated = "" }, "generated"},
		{"generated not RFC 3339", func(d *Document) { d.Generated = "2026-10-20 20:00:00" }, "generated"},
		{"generated date only", func(d *Document) { d.Generated = "2026-10-20" }, "generated"},
		{"generated non-UTC offset", func(d *Document) { d.Generated = "2026-10-20T20:00:00-04:00" }, "generated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validCanonical()
			tc.mutate(d)
			assertInvalid(t, d.Validate(), tc.wantErr)
		})
	}

	t.Run("every stage is accepted", func(t *testing.T) {
		for _, s := range []Stage{StagePlanner, StageArchitect, StageKrewLead, StageBuilder, StageValidator, StageDocumenter} {
			d := validCanonical()
			d.Stage = s
			assert.NoError(t, d.Validate(), string(s))
		}
	})

	t.Run("both source kinds are accepted", func(t *testing.T) {
		for _, k := range []SourceKind{SourceGitHubIssue, SourceDocument} {
			d := validCanonical()
			d.Source.Kind = k
			assert.NoError(t, d.Validate(), string(k))
		}
	})

	t.Run("generated with explicit zero offset and fraction is accepted", func(t *testing.T) {
		for _, g := range []string{"2026-10-20T20:00:00Z", "2026-10-20T20:00:00.123Z", "2026-10-20T20:00:00+00:00"} {
			d := validCanonical()
			d.Generated = g
			assert.NoError(t, d.Validate(), g)
		}
	})

	t.Run("all problems are reported together", func(t *testing.T) {
		d := validCanonical()
		d.Stage = "nope"
		d.Source.Ref = ""
		d.Generated = "yesterday"
		err := d.Validate()
		assertInvalid(t, err, "stage", "source.ref", "generated")
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.Len(t, ve.Problems, 3)
	})

	t.Run("nil document", func(t *testing.T) {
		var d *Document
		assertInvalid(t, d.Validate())
	})
}

func TestValidate_Inferences(t *testing.T) {
	good := Inference{Slot: "edge_cases", Inferred: "empty input is a no-op", Evidence: "internal/export/*.go"}

	t.Run("canonical without inferences is valid", func(t *testing.T) {
		assert.NoError(t, validCanonical().Validate())
	})

	t.Run("canonical with inferences is valid", func(t *testing.T) {
		d := validCanonical()
		d.Inferences = []Inference{good, good}
		assert.NoError(t, d.Validate())
	})

	t.Run("punch_out with inferences is valid", func(t *testing.T) {
		d := validPunchOut()
		d.Inferences = []Inference{good}
		assert.NoError(t, d.Validate())
	})

	t.Run("empty inferences slice is valid", func(t *testing.T) {
		d := validCanonical()
		d.Inferences = []Inference{}
		assert.NoError(t, d.Validate())
	})

	for _, tc := range []struct {
		field  string
		mutate func(i *Inference)
	}{
		{"slot", func(i *Inference) { i.Slot = "" }},
		{"inferred", func(i *Inference) { i.Inferred = "" }},
		{"evidence", func(i *Inference) { i.Evidence = " " }},
	} {
		t.Run("entry missing "+tc.field, func(t *testing.T) {
			bad := good
			tc.mutate(&bad)
			d := validCanonical()
			d.Inferences = []Inference{good, bad}
			assertInvalid(t, d.Validate(), "inferences[1]."+tc.field)
		})
	}
}

func TestEnumValid(t *testing.T) {
	assert.True(t, StagePlanner.Valid())
	assert.False(t, Stage("").Valid())
	assert.True(t, ResultPunchOut.Valid())
	assert.False(t, Result("x").Valid())
	assert.True(t, SourceDocument.Valid())
	assert.False(t, SourceKind("x").Valid())
}

func TestUnknownVersionError(t *testing.T) {
	var err error = &UnknownVersionError{Version: 2, MaxSupported: 1}
	assert.True(t, errors.Is(err, ErrUnknownVersion))
	assert.False(t, errors.Is(err, ErrInvalidDocument))
	assert.Contains(t, err.Error(), "2")
	assert.Contains(t, err.Error(), "1")
	var uv *UnknownVersionError
	require.True(t, errors.As(err, &uv))
	assert.Equal(t, 2, uv.Version)
}
