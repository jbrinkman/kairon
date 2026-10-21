package guard

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const wellFormedBody = `## Problem
Something is broken.

## Context
Background.

## Scope
In: a. Out: b.

## Acceptance Criteria
1. The thing works. Verify: run the tests.
2. The other thing works. Verify: run the linter.

## Dependencies
None.
`

func canonicalRaw(t *testing.T, stage stageoutput.Stage, body string) string {
	t.Helper()
	raw, err := stageoutput.Marshal(&stageoutput.Document{
		SchemaVersion: stageoutput.CurrentSchemaVersion,
		Stage:         stage,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "377"},
		Generated:     "2026-10-20T20:00:00Z",
		Body:          body,
	})
	require.NoError(t, err)
	return string(raw)
}

func punchOutRaw(t *testing.T) string {
	t.Helper()
	raw, err := stageoutput.Marshal(&stageoutput.Document{
		SchemaVersion: stageoutput.CurrentSchemaVersion,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultPunchOut,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "377"},
		Generated:     "2026-10-20T20:00:00Z",
		Unresolved:    []stageoutput.Unresolved{{Slot: "scope", Reason: "unclear"}},
	})
	require.NoError(t, err)
	return string(raw)
}

func structuralInput(raw string) Input {
	return Input{Stage: stageoutput.StagePlanner, Raw: raw, Attempt: Attempt{Current: 2, Max: 3}}
}

func TestStructural_Admit(t *testing.T) {
	tests := []struct {
		name       string
		raw        func(t *testing.T) string
		wantPass   bool
		actualHas  []string
		actualNot  []string
		expectedIn []string
	}{
		{
			name:     "well-formed canonical document",
			raw:      func(t *testing.T) string { return canonicalRaw(t, stageoutput.StagePlanner, wellFormedBody) },
			wantPass: true,
		},
		{
			name: "missing Acceptance Criteria section",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Acceptance Criteria", "## Other", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas:  []string{"Acceptance Criteria"},
			actualNot:  []string{"Problem"},
			expectedIn: []string{"Problem", "Context", "Scope", "Acceptance Criteria", "Dependencies"},
		},
		{
			name: "missing Dependencies section",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Dependencies\nNone.\n", "", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas: []string{"Dependencies"},
			actualNot: []string{"Problem", "Context"},
		},
		{
			name: "multiple missing sections listed in required order",
			raw: func(t *testing.T) string {
				return canonicalRaw(t, stageoutput.StagePlanner, "## Problem\nx\n\n## Scope\ny\n")
			},
			actualHas: []string{"Context, Acceptance Criteria, Dependencies"},
		},
		{
			name: "second criterion lacks Verify",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, " Verify: run the linter.", "", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas: []string{"2", "of 2"},
			actualNot: []string{"1,"},
		},
		{
			name: "acceptance criteria heading with no list items",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody,
					"1. The thing works. Verify: run the tests.\n2. The other thing works. Verify: run the linter.\n",
					"Just prose here.\n", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas: []string{"no list items"},
		},
		{
			name: "Verify on a continuation line",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody,
					"1. The thing works. Verify: run the tests.",
					"1. The thing works\n   across two lines.\n   Verify: run the tests.", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "nested list marker is continuation text",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody,
					"1. The thing works. Verify: run the tests.",
					"1. The thing works\n   - sub detail\n   - Verify: run the tests.", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "heading inside fenced code block does not count",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Context\nBackground.\n",
					"```\n## Context\n```\n", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas: []string{"Context"},
			actualNot: []string{"Problem"},
		},
		{
			name: "Problem Statement satisfies Problem and Scope em dash variant satisfies Scope",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Problem", "## Problem Statement", 1)
				body = strings.Replace(body, "## Scope", "## Scope — In and Out", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "heading prefix must end at a word boundary",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Scope", "## Scoped", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			actualHas: []string{"Scope"},
		},
		{
			name: "case-insensitive heading with trailing colon",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "## Context", "### CONTEXT:", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "bold Verify accepted",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "Verify: run the tests.", "**Verify:** run the tests.", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "lower-case verify accepted",
			raw: func(t *testing.T) string {
				body := strings.Replace(wellFormedBody, "Verify: run the linter.", "verify: run the linter.", 1)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name: "dash bullets with Verify",
			raw: func(t *testing.T) string {
				body := strings.NewReplacer("1. ", "- ", "2. ", "* ").Replace(wellFormedBody)
				return canonicalRaw(t, stageoutput.StagePlanner, body)
			},
			wantPass: true,
		},
		{
			name:     "punch_out passes",
			raw:      punchOutRaw,
			wantPass: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := structuralInput(tt.raw(t))
			res := NewStructural(nil).Admit(in)

			if tt.wantPass {
				assert.True(t, res.Pass)
				assert.Nil(t, res.Failure)
				return
			}
			require.False(t, res.Pass)
			require.NotNil(t, res.Failure)
			require.NoError(t, res.Validate())

			fc := res.Failure
			assert.Equal(t, in.Stage, fc.Stage)
			assert.Equal(t, StructuralName, fc.Guard)
			assert.Equal(t, "structural", fc.Guard)
			assert.Equal(t, in.Attempt, fc.Attempt)
			assert.NotEmpty(t, fc.Expected)
			assert.NotEmpty(t, fc.Actual)
			for _, s := range tt.actualHas {
				assert.Contains(t, fc.Actual, s)
			}
			for _, s := range tt.actualNot {
				assert.NotContains(t, fc.Actual, s)
			}
			for _, s := range tt.expectedIn {
				assert.Contains(t, fc.Expected, s)
			}
		})
	}
}

func TestStructural_MissingVerifyNamesCriterionNumbers(t *testing.T) {
	body := strings.Replace(wellFormedBody, " Verify: run the linter.", "", 1)
	res := NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.NotNil(t, res.Failure)
	assert.Equal(t, "every acceptance criterion carries a Verify: line", res.Failure.Expected)
	assert.Equal(t, "criteria without Verify: 2 (of 2)", res.Failure.Actual)

	body = strings.NewReplacer(" Verify: run the tests.", "", " Verify: run the linter.", "").Replace(wellFormedBody)
	res = NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.NotNil(t, res.Failure)
	assert.Equal(t, "criteria without Verify: 1, 2 (of 2)", res.Failure.Actual)
}

func TestStructural_MissingSectionMessages(t *testing.T) {
	body := "## Problem\nx\n\n## Scope\ny\n"
	res := NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.NotNil(t, res.Failure)
	assert.Equal(t, "sections present: Problem, Context, Scope, Acceptance Criteria, Dependencies", res.Failure.Expected)
	assert.Equal(t, "missing sections: Context, Acceptance Criteria, Dependencies", res.Failure.Actual)
}

func TestStructural_ChecksEveryAcceptanceCriteriaSection(t *testing.T) {
	// A second peer Acceptance Criteria section whose item lacks Verify: must
	// be rejected, even though the first section is fully verified.
	body := wellFormedBody + `
## Acceptance Criteria
3. A later unverified criterion.
`
	res := NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.NotNil(t, res.Failure)
	assert.False(t, res.Pass)
	assert.Equal(t, "every acceptance criterion carries a Verify: line", res.Failure.Expected)
	assert.Contains(t, res.Failure.Actual, "criteria without Verify:")
}

func TestStructural_AcceptanceCriteriaSectionEndsAtNextHeading(t *testing.T) {
	// The unverified item belongs to Dependencies, not Acceptance Criteria.
	body := strings.Replace(wellFormedBody, "None.\n", "- #12 must merge first\n", 1)
	res := NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	assert.True(t, res.Pass)
	assert.Nil(t, res.Failure)
}

func TestStructural_RejectsUnparseable(t *testing.T) {
	valid := canonicalRaw(t, stageoutput.StagePlanner, wellFormedBody)
	tests := []struct {
		name      string
		raw       string
		actualHas string
	}{
		{"empty string", "", ""},
		{"non-YAML text", "this is not [a document", ""},
		{"unknown key", "stagee: planner\n" + valid, ""},
		{"schema_version 2", strings.Replace(valid, "schema_version: 1", "schema_version: 2", 1), "highest supported version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := structuralInput(tt.raw)
			_, parseErr := stageoutput.Parse([]byte(tt.raw))
			require.Error(t, parseErr)

			var res AdmissionResult
			require.NotPanics(t, func() { res = NewStructural(nil).Admit(in) })
			require.False(t, res.Pass)
			require.NotNil(t, res.Failure)
			require.NoError(t, res.Validate())

			assert.Equal(t, "document parses under the strict stage-output schema (stageoutput.Parse)", res.Failure.Expected)
			assert.Contains(t, res.Failure.Expected, "strict stage-output schema")
			assert.Equal(t, parseErr.Error(), res.Failure.Actual)
			if tt.actualHas != "" {
				assert.Contains(t, res.Failure.Actual, tt.actualHas)
			}
			assert.Equal(t, StructuralName, res.Failure.Guard)
			assert.Equal(t, in.Stage, res.Failure.Stage)
			assert.Equal(t, in.Attempt, res.Failure.Attempt)
		})
	}
}

func TestStructural_DoesNotMutate(t *testing.T) {
	cases := map[string]string{
		"valid":   canonicalRaw(t, stageoutput.StagePlanner, wellFormedBody),
		"invalid": canonicalRaw(t, stageoutput.StagePlanner, "## Problem\nx\n"),
		"garbage": "nope: [",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			g := NewStructural(nil)
			in := structuralInput(raw)
			orig := in

			first := g.Admit(in)
			second := g.Admit(in)

			assert.Equal(t, first, second, "Admit must be repeatable")
			assert.Equal(t, orig, in, "Input must be unchanged")
			if first.Failure != nil {
				assert.NotSame(t, first.Failure, second.Failure)
			}
		})
	}
}

func TestStructural_OnFailure(t *testing.T) {
	fc := structuralInput("x").Reject(StructuralName, "exp", "act").Failure
	require.NotNil(t, fc)

	t.Run("handler receives exact context and its error is returned", func(t *testing.T) {
		boom := errors.New("boom")
		var got []FailureContext
		g := NewStructural(func(c FailureContext) error {
			got = append(got, c)
			return boom
		})
		err := g.OnFailure(*fc)
		assert.Same(t, boom, err)
		require.Len(t, got, 1)
		assert.Equal(t, *fc, got[0])
	})
	t.Run("handler nil error", func(t *testing.T) {
		g := NewStructural(func(FailureContext) error { return nil })
		assert.NoError(t, g.OnFailure(*fc))
	})
	t.Run("nil handler", func(t *testing.T) {
		err := NewStructural(nil).OnFailure(*fc)
		assert.ErrorIs(t, err, ErrNoFailureHandler)
	})
}

func TestStructural_CustomRequiredSections(t *testing.T) {
	g := NewStructural(nil)
	g.RequiredSections = []string{"Problem"}

	res := g.Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, "## Problem\nonly this\n")))
	assert.True(t, res.Pass, "no Acceptance Criteria section means no Verify check")
	assert.Nil(t, res.Failure)

	res = g.Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, "## Context\nnope\n")))
	require.NotNil(t, res.Failure)
	assert.Equal(t, "sections present: Problem", res.Failure.Expected)
	assert.Equal(t, "missing sections: Problem", res.Failure.Actual)

	// Defaults are not modified by a custom list.
	assert.Equal(t, []string{"Problem", "Context", "Scope", "Acceptance Criteria", "Dependencies"}, DefaultRequiredSections)
}

func TestStructural_VerifyCheckRunsWhenAcceptanceCriteriaOptional(t *testing.T) {
	g := NewStructural(nil)
	g.RequiredSections = []string{"Problem"}

	body := "## Problem\nx\n\n## Acceptance Criteria\n- no marker here\n"
	res := g.Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.NotNil(t, res.Failure)
	assert.Equal(t, "criteria without Verify: 1 (of 1)", res.Failure.Actual)
}

func TestStructural_InChain(t *testing.T) {
	t.Run("valid document is ready to commit", func(t *testing.T) {
		calls := 0
		in := structuralInput(canonicalRaw(t, stageoutput.StagePlanner, wellFormedBody))
		res, err := Run(in, DefaultChain(func(FailureContext) error { calls++; return nil }))
		require.NoError(t, err)
		assert.Equal(t, OutcomeReadyToCommit, res.Outcome)
		assert.Nil(t, res.Failure)
		assert.Equal(t, 0, calls)
	})
	t.Run("invalid document is rejected and handler runs once", func(t *testing.T) {
		var got []FailureContext
		in := structuralInput(canonicalRaw(t, stageoutput.StagePlanner, "## Problem\nx\n"))
		res, err := Run(in, DefaultChain(func(fc FailureContext) error { got = append(got, fc); return nil }))
		require.NoError(t, err)
		assert.Equal(t, OutcomeRejected, res.Outcome)
		require.NotNil(t, res.Failure)
		assert.NoError(t, res.FailureErr)
		require.Len(t, got, 1)
		assert.Equal(t, *res.Failure, got[0])
		assert.Equal(t, StructuralName, got[0].Guard)
	})
	t.Run("nil handler surfaces ErrNoFailureHandler in FailureErr", func(t *testing.T) {
		in := structuralInput("")
		res, err := Run(in, DefaultChain(nil))
		require.NoError(t, err)
		assert.Equal(t, OutcomeRejected, res.Outcome)
		assert.ErrorIs(t, res.FailureErr, ErrNoFailureHandler)
	})
}
