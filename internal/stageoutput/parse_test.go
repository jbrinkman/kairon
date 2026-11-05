package stageoutput

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validCanonicalYAML = `schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  ## Problem
  Something.
`

func TestParse_RoundTrip(t *testing.T) {
	canonical := validCanonical()
	canonical.Inferences = []Inference{
		{Slot: "edge_cases", Inferred: "empty input is a no-op", Evidence: "exporters in internal/export"},
		{Slot: "scope", Inferred: "CLI only", Evidence: "issue title"},
	}
	canonicalNoInf := validCanonical()
	punchOut := validPunchOut()
	punchOutInf := validPunchOut()
	punchOutInf.Inferences = []Inference{{Slot: "a", Inferred: "b", Evidence: "c"}}

	for name, d := range map[string]*Document{
		"canonical with inferences":    canonical,
		"canonical without inferences": canonicalNoInf,
		"punch_out":                    punchOut,
		"punch_out with inferences":    punchOutInf,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := Marshal(d)
			require.NoError(t, err)
			got, err := Parse(out)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, d, got)
		})
	}
}

func TestParse_RejectsUnknownKey(t *testing.T) {
	tests := map[string]string{
		"extra top-level key": validCanonicalYAML + "stagee: planner\n",
		"misspelled required key": strings.Replace(validCanonicalYAML,
			"stage: planner", "stagee: planner", 1),
		"unknown key in source": strings.Replace(validCanonicalYAML,
			"  ref: \"142\"\n", "  ref: \"142\"\n  extra: x\n", 1),
		"misspelled key in source": strings.Replace(validCanonicalYAML,
			"  ref: \"142\"", "  reff: \"142\"", 1),
		"unknown key in inferences[0]": validCanonicalYAML +
			"inferences:\n  - slot: a\n    inferred: b\n    evidence: c\n    extra: d\n",
		"misspelled key in inferences[0]": validCanonicalYAML +
			"inferences:\n  - slot: a\n    infered: b\n    evidence: c\n",
		"unknown key in unresolved[0]": `schema_version: 1
stage: planner
result: punch_out
source:
  kind: document
  ref: d.md
generated: "2026-10-20T20:00:00Z"
unresolved:
  - slot: a
    reason: b
    extra: c
`,
		"misspelled key in unresolved[1]": `schema_version: 1
stage: planner
result: punch_out
source:
  kind: document
  ref: d.md
generated: "2026-10-20T20:00:00Z"
unresolved:
  - slot: a
    reason: b
  - slot: c
    reasn: d
`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Parse([]byte(in))
			require.Error(t, err)
			assert.Nil(t, got)
			assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
			assert.False(t, errors.Is(err, ErrUnknownVersion))
		})
	}
}

func TestParse_BodyVerbatim(t *testing.T) {
	body := "## Problem: it's \"broken\"\n" +
		"key: value\n" +
		"other: \"quoted\" # not a comment\n" +
		"- item\n" +
		"\n" +
		"  indented line\n" +
		"\n" +
		"last line\n"
	in := "schema_version: 1\nstage: planner\nresult: canonical\nsource:\n  kind: github_issue\n  ref: \"142\"\n" +
		"generated: \"2026-10-20T20:00:00Z\"\nbody: |\n" +
		"  ## Problem: it's \"broken\"\n" +
		"  key: value\n" +
		"  other: \"quoted\" # not a comment\n" +
		"  - item\n" +
		"\n" +
		"    indented line\n" +
		"\n" +
		"  last line\n"
	got, err := Parse([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, body, got.Body)
}

const jsonShaped = `{"schema_version": 1, "stage": "planner", "result": "canonical",
 "source": {"kind": "github_issue", "ref": "142"},
 "generated": "2026-10-20T20:00:00Z",
 "body": "## Problem\nSomething.\n"}`

func TestParse_JSONShaped(t *testing.T) {
	fromJSON, err := Parse([]byte(jsonShaped))
	require.NoError(t, err)
	fromYAML, err := Parse([]byte(validCanonicalYAML))
	require.NoError(t, err)
	assert.Equal(t, fromYAML, fromJSON)

	bad := strings.Replace(jsonShaped, `"stage"`, `"extra": 1, "stage"`, 1)
	got, err := Parse([]byte(bad))
	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
}

func TestParse_UnknownVersion(t *testing.T) {
	for _, v := range []string{"2", "99"} {
		t.Run("version "+v, func(t *testing.T) {
			in := strings.Replace(validCanonicalYAML, "schema_version: 1", "schema_version: "+v, 1) +
				"model: x\n"
			got, err := Parse([]byte(in))
			require.Error(t, err)
			assert.Nil(t, got)
			assert.True(t, errors.Is(err, ErrUnknownVersion), "got %v", err)
			assert.False(t, errors.Is(err, ErrInvalidDocument))
			var uv *UnknownVersionError
			require.True(t, errors.As(err, &uv))
			want := 2
			if v == "99" {
				want = 99
			}
			assert.Equal(t, UnknownVersionError{Version: want, MaxSupported: 1}, *uv)
			assert.NotContains(t, err.Error(), "model")
		})
	}

	t.Run("same document at version 1 is invalid", func(t *testing.T) {
		got, err := Parse([]byte(validCanonicalYAML + "model: x\n"))
		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
		assert.False(t, errors.Is(err, ErrUnknownVersion))
	})
}

func TestParse_VersionField(t *testing.T) {
	rest := strings.TrimPrefix(validCanonicalYAML, "schema_version: 1\n")
	tests := map[string]string{
		"missing":                  rest,
		"string":                   "schema_version: \"1\"\n" + rest,
		"float":                    "schema_version: 1.5\n" + rest,
		"float1.0":                 "schema_version: 1.0\n" + rest,
		"bool":                     "schema_version: true\n" + rest,
		"null":                     "schema_version: null\n" + rest,
		"empty":                    "schema_version:\n" + rest,
		"zero":                     "schema_version: 0\n" + rest,
		"negative":                 "schema_version: -1\n" + rest,
		"bare schema key":          "schema: 1\n" + rest,
		"schema alongside version": validCanonicalYAML + "schema: 1\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Parse([]byte(in))
			require.Error(t, err)
			assert.Nil(t, got)
			assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
			assert.False(t, errors.Is(err, ErrUnknownVersion))
		})
	}
}

func TestParse_StreamShape(t *testing.T) {
	tests := map[string]string{
		"empty":                                "",
		"whitespace only":                      " \n\t\n  \n",
		"comment only":                         "# nothing here\n",
		"top-level list":                       "- a\n- b\n",
		"scalar":                               "just text\n",
		"multi-document":                       validCanonicalYAML + "---\n" + validCanonicalYAML,
		"leading separator then two documents": "---\n" + validCanonicalYAML + "---\n" + validCanonicalYAML,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Parse([]byte(in))
			require.Error(t, err)
			assert.Nil(t, got)
			assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
		})
	}

	t.Run("single document with leading separator is accepted", func(t *testing.T) {
		got, err := Parse([]byte("---\n" + validCanonicalYAML))
		require.NoError(t, err)
		assert.Equal(t, validCanonical(), got)
	})
}

func TestParse_PresenceNotEmptiness(t *testing.T) {
	t.Run("canonical with unresolved: []", func(t *testing.T) {
		got, err := Parse([]byte(validCanonicalYAML + "unresolved: []\n"))
		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
		assert.Contains(t, err.Error(), "unresolved")
	})
	t.Run("punch_out with body: \"\"", func(t *testing.T) {
		in := `schema_version: 1
stage: planner
result: punch_out
source:
  kind: document
  ref: d.md
generated: "2026-10-20T20:00:00Z"
body: ""
unresolved:
  - slot: a
    reason: b
`
		got, err := Parse([]byte(in))
		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
		assert.Contains(t, err.Error(), "body")
	})
	t.Run("null body is treated as absent", func(t *testing.T) {
		in := strings.Replace(validCanonicalYAML, "body: |\n  ## Problem\n  Something.\n", "body: null\n", 1)
		got, err := Parse([]byte(in))
		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, ErrInvalidDocument), "got %v", err)
		assert.Contains(t, err.Error(), "body")
	})
}

// The two example documents below are copies of the examples in the planner
// design document, kept in testdata/ so the tests do not read the document
// itself. They pin the parser against the shapes the design describes.

// readTestdata returns the contents of testdata/<name>.
func readTestdata(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(raw)
}

func TestParse_CanonicalExampleFromSpec(t *testing.T) {
	in := readTestdata(t, "canonical-example.yaml")
	got, err := Parse([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, 1, got.SchemaVersion)
	assert.Equal(t, StagePlanner, got.Stage)
	assert.Equal(t, ResultCanonical, got.Result)
	assert.Equal(t, Source{Kind: SourceGitHubIssue, Ref: "142"}, got.Source)
	assert.Equal(t, "2026-10-20T20:00:00Z", got.Generated)
	assert.Equal(t, "## Problem\n...the canonical issue text per §5-§8...\n", got.Body)
	require.Len(t, got.Inferences, 1)
	assert.Equal(t, "edge_cases", got.Inferences[0].Slot)
	assert.Equal(t, "handle empty input as a no-op", got.Inferences[0].Inferred)
}

func TestParse_PunchOutExampleFromSpec(t *testing.T) {
	in := readTestdata(t, "punch-out-example.yaml")
	got, err := Parse([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, ResultPunchOut, got.Result)
	assert.Equal(t, "2026-10-20T20:00:00Z", got.Generated)
	assert.Empty(t, got.Body)
	require.Len(t, got.Unresolved, 2)
	assert.Equal(t, "acceptance_criteria", got.Unresolved[0].Slot)
	assert.Equal(t, "The issue states the goal (\"make export faster\") but gives no measurable target; "+
		"a materially different acceptance criterion follows from each plausible target.", got.Unresolved[0].Reason)
	assert.Equal(t, "scope.external_contract", got.Unresolved[1].Slot)
	assert.Equal(t, "Whether the v2 API must stay backward compatible is a product decision the "+
		"repository cannot settle.", got.Unresolved[1].Reason)
}

func TestParse_UnquotedScalars(t *testing.T) {
	in := `schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: 142
generated: 2026-10-20T20:00:00Z
body: |
  x
`
	got, err := Parse([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, "142", got.Source.Ref)
	assert.Equal(t, "2026-10-20T20:00:00Z", got.Generated)
}
