package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

// The workflow documentation (docs/evaluation.md, "Workflows: sub-agent
// provenance and step records") is the contract for the .eval/calls.jsonl
// format, sub-agent provenance and workflow_sha256. These tests pin the terms
// an eval author depends on, so removing one fails the test.

const (
	wfDocHeading   = "### Workflows: sub-agent provenance and step records"
	wfDocEndMarker = "### Resume refusal"
)

// wfDocSlice returns the text of doc from the first occurrence of start up to
// (not including) the next occurrence of end after it, or to the end of doc
// when end is empty or absent. It fails the test if start is missing.
func wfDocSlice(t *testing.T, doc, start, end string) string {
	t.Helper()
	i := strings.Index(doc, start)
	if i < 0 {
		t.Fatalf("docs/evaluation.md has no %q", start)
	}
	rest := doc[i+len(start):]
	if end != "" {
		if j := strings.Index(rest, end); j >= 0 {
			rest = rest[:j]
		}
	}
	return start + rest
}

// wfDocRequire fails once per missing term, naming the section it was
// expected in.
func wfDocRequire(t *testing.T, section, where string, terms ...string) {
	t.Helper()
	for _, term := range terms {
		if !strings.Contains(section, term) {
			t.Errorf("docs/evaluation.md (%s) does not contain %q", where, term)
		}
	}
}

func TestWorkflowDocsSectionExists(t *testing.T) {
	doc := readEvaluationDoc(t)
	section := wfDocSlice(t, doc, wfDocHeading, wfDocEndMarker)
	if !strings.Contains(doc, wfDocEndMarker) {
		t.Fatalf("docs/evaluation.md has no %q after the Workflows section", wfDocEndMarker)
	}
	wfDocRequire(t, section, "Workflows section",
		"#### Sub-agent provenance (`sub_agents`)",
		"#### `workflow_sha256`",
		"#### Step records (`.eval/calls.jsonl`)",
		"toolsSettings.subagent.trustedAgents",
	)
}

// The call-record table must list the two new fields.
func TestWorkflowDocsCallRecordTable(t *testing.T) {
	doc := readEvaluationDoc(t)
	for _, row := range []string{"| `step` |", "| `source` |"} {
		if !strings.Contains(doc, "\n"+row) {
			t.Errorf("docs/evaluation.md per-call record table has no row starting with %q", row)
		}
	}
	table := wfDocSlice(t, doc, "#### Per-call records (`calls`)", "#### Run-level fields")
	wfDocRequire(t, table, "per-call records", "| `step` |", "| `source` |", "`workspace`", ".eval/calls.jsonl")
}

// The file format and its validation rules.
func TestWorkflowDocsStepRecordsFormat(t *testing.T) {
	doc := readEvaluationDoc(t)
	section := wfDocSlice(t, doc, wfDocHeading, wfDocEndMarker)
	steps := wfDocSlice(t, section, "#### Step records (`.eval/calls.jsonl`)", "\n#### Verifying with the self-test")

	wfDocRequire(t, steps, "step records",
		// format
		".eval/calls.jsonl",
		"One record per line.",
		"| `step` | **yes** |",
		"| `agent` | **yes** |",
		"`role`",
		"`input_tokens`",
		"`output_tokens`",
		"`cost_usd`",
		"`estimated`",
		"`duration_ms`",
		// validation
		"An absent file is normal.",
		"A malformed file scores 0 and names the line.",
		".eval/calls.jsonl line N: <reason>",
		"1-based",
		"all-or-nothing",
		"`error_context.stderr`",
		// source workspace
		"`source` to `workspace`",
		// cost accounting and the double-count caveat
		"Cost accounting.",
		"`agent_cost`",
		"Double-count caveat.",
		"steps only",
	)
}

// Sub-agent provenance and the staged-workspace rule.
func TestWorkflowDocsSubAgentProvenance(t *testing.T) {
	doc := readEvaluationDoc(t)
	section := wfDocSlice(t, doc, wfDocHeading, wfDocEndMarker)
	subs := wfDocSlice(t, section, "#### Sub-agent provenance (`sub_agents`)", "\n#### `workflow_sha256`")

	wfDocRequire(t, subs, "sub-agent provenance",
		"toolsSettings.subagent.trustedAgents",
		"transitive closure",
		"| `agent_model` |",
		"| `prompt_sha256` |",
		"| `resources_present` |",
		"recorded, not enforced",
		"Staged-workspace rule.",
		"never from the repository working directory",
		"`--prompt-file`",
		"agents.<agent>.sub_agents",
		"not** keys of the `agents` map",
	)
}

// workflow_sha256: framing, omission and independence of the agent's own hash.
func TestWorkflowDocsWorkflowSHA256(t *testing.T) {
	doc := readEvaluationDoc(t)
	section := wfDocSlice(t, doc, wfDocHeading, wfDocEndMarker)
	wf := wfDocSlice(t, section, "#### `workflow_sha256`", "\n#### Step records")

	wfDocRequire(t, wf, "workflow_sha256",
		"length-framed",
		"`<kind>\\x00<decimal length>\\x00<bytes>`",
		"`workflow-agent`",
		"`prompt-sha256`",
		"sorted by name",
		"Omitted when there is nothing to hash.",
		"no `workflow_sha256`",
		"The dispatching agent's own `prompt_sha256` is unaffected.",
		"Resume is refused when it differs.",
	)
}

// Resume refusal must name workflow_sha256, and the prompt_sha256 section must
// say sub-agents are not part of an agent's own hash.
func TestWorkflowDocsResumeAndPromptHash(t *testing.T) {
	doc := readEvaluationDoc(t)
	resume := wfDocSlice(t, doc, wfDocEndMarker, "\n### ")
	wfDocRequire(t, resume, "Resume refusal",
		"workflow_sha256",
		"A result file without a `workflow_sha256`",
	)

	hash := wfDocSlice(t, doc, "#### How `prompt_sha256` is computed", wfDocHeading)
	wfDocRequire(t, hash, "How prompt_sha256 is computed",
		"Sub-agents are not part of an agent's own hash.",
		"`sub_agents`",
		"`workflow_sha256`",
	)
}

// The documented algorithm, implemented independently here from the doc's
// words, must produce what the code produces: the documented framing, order
// (dispatching agent first, then sub-agents sorted by name) and part kinds.
func TestWorkflowDocsAlgorithmMatchesCode(t *testing.T) {
	frame := func(kind, data string) string {
		return kind + "\x00" + strconv.Itoa(len(data)) + "\x00" + data
	}
	subs := map[string]SubAgentProvenance{
		"zeta":  {PromptSHA256: strings.Repeat("b", 64)},
		"alpha": {PromptSHA256: strings.Repeat("a", 64)},
	}
	dispatcherSHA := strings.Repeat("0", 64)

	var b strings.Builder
	b.WriteString(frame("workflow-agent", "krew"))
	b.WriteString(frame("prompt-sha256", dispatcherSHA))
	b.WriteString(frame("workflow-agent", "alpha"))
	b.WriteString(frame("prompt-sha256", subs["alpha"].PromptSHA256))
	b.WriteString(frame("workflow-agent", "zeta"))
	b.WriteString(frame("prompt-sha256", subs["zeta"].PromptSHA256))
	sum := sha256.Sum256([]byte(b.String()))
	want := hex.EncodeToString(sum[:])

	if got := workflowSHA256("krew", dispatcherSHA, subs); got != want {
		t.Errorf("workflowSHA256 = %s, documented algorithm gives %s", got, want)
	}
}
