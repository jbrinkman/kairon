package checks

import (
	"regexp"
	"strings"
	"testing"
)

const canonicalDoc = `schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  # Add retry budget to the watcher

  Goal: cap the number of attempts per issue.
inferences:
  - slot: retry_scope
    inferred: per issue, not per agent
    evidence: "Issue text: 'stop retrying the same issue forever'"
`

const punchOutDoc = `schema_version: 1
stage: planner
result: punch_out
source:
  kind: github_issue
  ref: "143"
generated: "2026-10-20T20:05:00Z"
unresolved:
  - slot: target_branch
    reason: The issue names two integration branches.
`

// monthNinetyNineDoc is a canonical document whose only defect is an impossible
// month in `generated`.
func monthNinetyNineDoc() string {
	return strings.Replace(canonicalDoc, `"2026-10-20T20:00:00Z"`, `"2026-99-21T12:00:00Z"`, 1)
}

// plannerGeneratedRegex is the regex envelope the planner suite uses for the
// `generated` line. It is copied here only to show that the regex alone cannot
// catch a month-99 timestamp.
var plannerGeneratedRegex = regexp.MustCompile(`(?m)^generated: "?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z"?\s*$`)

func docCheck() Check {
	return Check{Criterion: "output_document_validity", Type: CheckOutputDocumentValid}
}

func TestDocumentCheckWellFormedPasses(t *testing.T) {
	for name, doc := range map[string]string{
		"canonical":                          canonicalDoc,
		"punch_out":                          punchOutDoc,
		"fenced":                             "```yaml\n" + canonicalDoc + "```\n",
		"fenced yml, surrounding whitespace": "\n  ```yml\n" + punchOutDoc + "```\n\n",
		"fenced, CRLF line endings":          strings.ReplaceAll("```yaml\n"+canonicalDoc+"```\n", "\n", "\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			r := evalOne(t, docCheck(), Input{Output: doc})
			wantPass(t, r)
			if r.Detail != "" {
				t.Errorf("Detail = %q, want empty on pass", r.Detail)
			}
		})
	}
}

func TestDocumentCheckMonthNinetyNineFailsWithParserReason(t *testing.T) {
	doc := monthNinetyNineDoc()

	// The point of the type: the regex envelope accepts this document.
	if !plannerGeneratedRegex.MatchString(doc) {
		t.Fatalf("test premise broken: planner generated-regex must match the month-99 document:\n%s", doc)
	}

	r := evalOne(t, docCheck(), Input{Output: doc})
	wantFail(t, r, "RFC 3339")
	if !strings.Contains(r.Detail, "generated") {
		t.Errorf("Detail %q should name the generated field", r.Detail)
	}
	if strings.Contains(r.Detail, "pattern /") {
		t.Errorf("Detail %q is a regex mismatch, want the parser's reason", r.Detail)
	}
}

func TestDocumentCheckRejectsInvalidOutput(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string // substring of the parser's message
	}{
		{"prose", "Here is the plan you asked for. I will add a retry budget.", "invalid stage-output document"},
		{"empty", "", "document is empty"},
		{"whitespace only", " \n\n  \n", "document is empty"},
		{"unknown key", strings.Replace(canonicalDoc, "stage: planner\n", "stage: planner\nbogus: 1\n", 1), "bogus"},
		{"two documents", canonicalDoc + "---\n" + canonicalDoc, "invalid stage-output document"},
		{"schema_version 2", strings.Replace(canonicalDoc, "schema_version: 1", "schema_version: 2", 1), "schema_version"},
		{"missing body", strings.Replace(punchOutDoc, "result: punch_out", "result: canonical", 1), "body"},
		{"punch_out with body", strings.Replace(punchOutDoc, "unresolved:", "body: x\nunresolved:", 1), "body"},
		{"unknown stage", strings.Replace(canonicalDoc, "stage: planner", "stage: wizard", 1), "stage"},
		{"two fences", "```yaml\n" + canonicalDoc + "```\n```yaml\n" + canonicalDoc + "```\n", ""},
		{"fence with prose before", "Here you go:\n```yaml\n" + canonicalDoc + "```\n", ""},
		{"nested fence", "```yaml\n```yaml\n" + canonicalDoc + "```\n```\n", ""},
		{"non-yaml fence", "```json\n" + canonicalDoc + "```\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := evalOne(t, docCheck(), Input{Output: tc.output})
			wantFail(t, r, "output is not a valid stage-output document: ")
			if tc.want != "" && !strings.Contains(r.Detail, tc.want) {
				t.Errorf("Detail %q does not contain %q", r.Detail, tc.want)
			}
			if strings.Contains(r.Detail, "pattern /") {
				t.Errorf("Detail %q is a regex mismatch, want the parser's reason", r.Detail)
			}
		})
	}
}

// Exactly one wrapping fence is removed: a document whose body merely contains
// a fence is untouched, and a fence that does not wrap the whole reply is not
// a wrapper.
func TestDocumentCheckFenceInsideBodyIsNotStripped(t *testing.T) {
	doc := strings.Replace(canonicalDoc, "  Goal: cap the number of attempts per issue.\n",
		"  Example:\n\n  ```yaml\n  max_retries: 3\n  ```\n", 1)
	wantPass(t, evalOne(t, docCheck(), Input{Output: doc}))
}

func TestDocumentCheckTurnSelection(t *testing.T) {
	in := Input{
		Output: "top-level prose",
		Turns: []TurnEvidence{
			{Output: canonicalDoc},
			{Output: "a question for the user?"},
		},
	}

	t.Run("turn 1 reads the document", func(t *testing.T) {
		c := docCheck()
		c.Turn = turnPtr(1)
		wantPass(t, evalOne(t, c, in))
	})
	t.Run("turn 2 reads the prose", func(t *testing.T) {
		c := docCheck()
		c.Turn = turnPtr(2)
		wantFail(t, evalOne(t, c, in), "not a valid stage-output document")
	})
	t.Run("no turn reads the last turn", func(t *testing.T) {
		wantFail(t, evalOne(t, docCheck(), in), "not a valid stage-output document")

		last := Input{Turns: []TurnEvidence{{Output: "question?"}, {Output: punchOutDoc}}}
		wantPass(t, evalOne(t, docCheck(), last))
	})
	t.Run("out of range", func(t *testing.T) {
		c := docCheck()
		c.Turn = turnPtr(3)
		r := evalOne(t, c, in)
		wantFail(t, r, "turn 3 is out of range")
		if !strings.Contains(r.Detail, "2 turn(s)") {
			t.Errorf("Detail %q should mention the number of turns", r.Detail)
		}
	})
}

func TestDocumentCheckValidate(t *testing.T) {
	ok := decodeChecks(t, "- {criterion: c, type: output_document_valid}\n- {criterion: c, type: output_document_valid, turn: 2}\n")
	if err := Validate("f.yaml", ok, ValidateOptions{}); err != nil {
		t.Fatalf("output_document_valid with no fields / turn should validate: %v", err)
	}

	for field, src := range map[string]string{
		"pattern":     "{criterion: c, type: output_document_valid, pattern: 'x'}",
		"path":        "{criterion: c, type: output_document_valid, path: a.yaml}",
		"run":         "{criterion: c, type: output_document_valid, run: 'true'}",
		"question":    "{criterion: c, type: output_document_valid, question: q}",
		"fact":        "{criterion: c, type: output_document_valid, fact: f}",
		"allow":       "{criterion: c, type: output_document_valid, allow: []}",
		"expect_exit": "{criterion: c, type: output_document_valid, expect_exit: 0}",
	} {
		t.Run("rejects "+field, func(t *testing.T) {
			err := Validate("f.yaml", decodeChecks(t, "- "+src+"\n"), ValidateOptions{})
			if err == nil {
				t.Fatalf("%s must be rejected on output_document_valid", field)
			}
			want := field + " is not valid for type output_document_valid"
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		})
	}

	t.Run("turn below one", func(t *testing.T) {
		for _, tc := range []struct{ typ, extra, turn string }{
			{"output_document_valid", "", "0"},
			{"output_contains", ", pattern: x", "0"},
			{"output_contains", ", pattern: x", "-1"},
		} {
			t.Run(tc.typ+" "+tc.turn, func(t *testing.T) {
				src := "- {criterion: c, type: " + tc.typ + tc.extra + ", turn: " + tc.turn + "}\n"
				err := Validate("f.yaml", decodeChecks(t, src), ValidateOptions{})
				if err == nil {
					t.Fatalf("turn: %s on %s must be rejected", tc.turn, tc.typ)
				}
				if !strings.Contains(err.Error(), "turn") || !strings.Contains(err.Error(), ">= 1") {
					t.Errorf("error %q should say turn must be >= 1", err)
				}
			})
		}
	})
}
