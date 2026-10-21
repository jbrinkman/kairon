package stageoutput

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMarshal_CanonicalKeyOrderAndBlockScalar(t *testing.T) {
	d := validCanonical()
	d.Body = "## Problem\nSomething: with a colon.\n\n- item # not a comment\n"
	d.Inferences = []Inference{{Slot: "edge_cases", Inferred: "no-op", Evidence: "exporters"}}

	out, err := Marshal(d)
	require.NoError(t, err)
	text := string(out)

	want := `schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  ## Problem
  Something: with a colon.

  - item # not a comment
inferences:
  - slot: edge_cases
    inferred: no-op
    evidence: exporters
`
	assert.Equal(t, want, text)
}

func TestMarshal_PunchOutShape(t *testing.T) {
	out, err := Marshal(validPunchOut())
	require.NoError(t, err)

	want := `schema_version: 1
stage: planner
result: punch_out
source:
  kind: document
  ref: docs/design.md
generated: "2026-10-20T20:00:00Z"
unresolved:
  - slot: acceptance
    reason: no testable outcome stated
  - slot: scope
    reason: two readings possible
`
	assert.Equal(t, want, string(out))
	assert.NotContains(t, string(out), "body")
}

func TestMarshal_TopLevelKeyOrder(t *testing.T) {
	d := validPunchOut()
	d.Inferences = []Inference{{Slot: "s", Inferred: "i", Evidence: "e"}}
	out, err := Marshal(d)
	require.NoError(t, err)

	var root yaml.Node
	require.NoError(t, yaml.Unmarshal(out, &root))
	require.Equal(t, yaml.DocumentNode, root.Kind)
	m := root.Content[0]
	var keys []string
	for i := 0; i < len(m.Content); i += 2 {
		keys = append(keys, m.Content[i].Value)
	}
	assert.Equal(t, []string{"schema_version", "stage", "result", "source", "generated", "unresolved", "inferences"}, keys)
}

func TestMarshal_MultilineBodyIsLiteralBlock(t *testing.T) {
	d := validCanonical()
	d.Body = "line one\nline two\n"
	out, err := Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), "body: |\n")
	assert.NotContains(t, string(out), `\n`)
}

func TestMarshal_InvalidDocumentReturnsNoBytes(t *testing.T) {
	d := validCanonical()
	d.Unresolved = []Unresolved{{Slot: "a", Reason: "b"}}
	out, err := Marshal(d)
	assert.Nil(t, out)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
	assert.Contains(t, err.Error(), "unresolved")

	out, err = Marshal(nil)
	assert.Nil(t, out)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestMarshal_RejectsUnregisteredVersion(t *testing.T) {
	d := validCanonical()
	d.SchemaVersion = CurrentSchemaVersion + 1
	out, err := Marshal(d)
	assert.Nil(t, out)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownVersion))
	var uv *UnknownVersionError
	require.True(t, errors.As(err, &uv))
	assert.Equal(t, CurrentSchemaVersion+1, uv.Version)
	assert.Equal(t, CurrentSchemaVersion, uv.MaxSupported)
}

// The emitted text must be valid YAML that decodes back to the same body for
// awkward content. Parse arrives in a later task, so this decodes with yaml.v3
// directly.
func TestMarshal_BodyDecodesVerbatim(t *testing.T) {
	bodies := []string{
		"a\n",
		"a",
		"colons: here\nquote \"x\" and 'y'\n# hash line\n  leading space line\n- item\n\nafter blank\n",
		"trailing newline\n",
		"keep trailing blank lines\n\n",
		"line with trailing space \nnext\n",
	}
	for _, body := range bodies {
		d := validCanonical()
		d.Body = body
		out, err := Marshal(d)
		require.NoError(t, err, "%q", body)

		var got struct {
			Body string `yaml:"body"`
		}
		require.NoError(t, yaml.Unmarshal(out, &got), "%q\n%s", body, out)
		assert.Equal(t, body, got.Body, "emitted:\n%s", out)
	}
}

func TestMarshal_QuotesStringsThatLookLikeOtherTypes(t *testing.T) {
	d := validCanonical()
	d.Source.Ref = "142"
	out, err := Marshal(d)
	require.NoError(t, err)
	text := string(out)
	assert.True(t, strings.Contains(text, `ref: "142"`), text)
	assert.True(t, strings.Contains(text, `generated: "2026-10-20T20:00:00Z"`), text)
}
