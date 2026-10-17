package eval

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

const (
	recA = `{"role":"agent","agent":"selftest-step-a","step":"design","model":"stub-model","input_tokens":10,"output_tokens":5,"cost_usd":0.000105,"estimated":false,"duration_ms":1}`
	recB = `{"agent":"selftest-step-b","step":"build","model":"stub-model","input_tokens":20,"output_tokens":10,"cost_usd":0.00021,"estimated":true,"duration_ms":2}`
)

// writeCalls writes content to <dir>/.eval/calls.jsonl.
func writeCalls(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, ".eval", "calls.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadStepRecordsAbsentIsNormal(t *testing.T) {
	dir := t.TempDir()
	recs, err := readStepRecords(dir)
	if err != nil || len(recs) != 0 {
		t.Fatalf("absent file: recs=%v err=%v", recs, err)
	}
	// An absent .eval directory is equally normal.
	if err := os.MkdirAll(filepath.Join(dir, ".eval"), 0o755); err != nil {
		t.Fatal(err)
	}
	if recs, err := readStepRecords(dir); err != nil || len(recs) != 0 {
		t.Fatalf("empty .eval: recs=%v err=%v", recs, err)
	}
}

func TestReadStepRecordsValidLines(t *testing.T) {
	dir := t.TempDir()
	// A blank line is skipped; the harness forces source=workspace whatever the
	// file claims; an empty role means agent; unknown fields are tolerated.
	writeCalls(t, dir, recA+"\n\n"+strings.Replace(recB, `{"agent"`, `{"source":"harness","extra":1,"agent"`, 1)+"\n")
	recs, err := readStepRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2: %+v", len(recs), recs)
	}
	a, b := recs[0], recs[1]
	if a.Agent != "selftest-step-a" || a.Step != "design" || a.Role != "agent" || a.InputTokens != 10 || a.OutputTokens != 5 || a.Estimated {
		t.Errorf("record A = %+v", a)
	}
	if b.Agent != "selftest-step-b" || b.Step != "build" || b.Role != "agent" || !b.Estimated {
		t.Errorf("record B = %+v", b)
	}
	for i, r := range recs {
		if r.Source != inference.SourceWorkspace {
			t.Errorf("record %d Source = %q, want %q", i, r.Source, inference.SourceWorkspace)
		}
	}
}

func TestReadStepRecordsMalformedNamesLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		line    string // expected "line N"
		reason  string
	}{
		{"not json", recA + "\n\nnot json\n", "line 3", ""},
		{"not an object", "[1,2]\n", "line 1", ""},
		{"missing step", recA + "\n" + `{"agent":"a","input_tokens":1}` + "\n", "line 2", "step"},
		{"missing agent", `{"step":"s"}` + "\n", "line 1", "agent"},
		{"blank step", `{"agent":"a","step":"  "}` + "\n", "line 1", "step"},
		{"judge role", recA + "\n" + `{"role":"judge","agent":"a","step":"s"}` + "\n", "line 2", "role"},
		{"negative input tokens", `{"agent":"a","step":"s","input_tokens":-1}` + "\n", "line 1", "negative"},
		{"negative output tokens", `{"agent":"a","step":"s","output_tokens":-1}` + "\n", "line 1", "negative"},
		{"negative cost", `{"agent":"a","step":"s","cost_usd":-0.5}` + "\n", "line 1", "negative"},
		{"negative duration", `{"agent":"a","step":"s","duration_ms":-5}` + "\n", "line 1", "negative"},
		{"wrong type", `{"agent":"a","step":"s","input_tokens":"ten"}` + "\n", "line 1", ""},
		{"trailing garbage", recA + ` x` + "\n", "line 1", ""},
		{"trailing bracket", recA + `]` + "\n", "line 1", ""},
		{"trailing brace", recA + `}` + "\n", "line 1", ""},
		{"cost total overflows to inf", `{"agent":"a","step":"s","cost_usd":1e308}` + "\n" + `{"agent":"a","step":"s","cost_usd":1e308}` + "\n", "line 2", "overflow"},
		{"input token total overflows", `{"agent":"a","step":"s","input_tokens":9223372036854775807}` + "\n" + `{"agent":"a","step":"s","input_tokens":1}` + "\n", "line 2", "overflow"},
		{"output token total overflows", `{"agent":"a","step":"s","output_tokens":9223372036854775807}` + "\n" + `{"agent":"a","step":"s","output_tokens":1}` + "\n", "line 2", "overflow"},
		{"CRLF line numbers", recA + "\r\n" + "{bad\r\n", "line 2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCalls(t, dir, tt.content)
			recs, err := readStepRecords(dir)
			if err == nil {
				t.Fatalf("no error; records = %+v", recs)
			}
			if len(recs) != 0 {
				t.Errorf("records = %+v, want none (all-or-nothing)", recs)
			}
			msg := err.Error()
			if !strings.Contains(msg, ".eval/calls.jsonl "+tt.line+":") {
				t.Errorf("error %q must contain %q", msg, ".eval/calls.jsonl "+tt.line+":")
			}
			if tt.reason != "" && !strings.Contains(msg, tt.reason) {
				t.Errorf("error %q must mention %q", msg, tt.reason)
			}
		})
	}
}

func TestReadStepRecordsRejectsBadFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "real.jsonl")
		if err := os.WriteFile(target, []byte(recA+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".eval"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, ".eval", "calls.jsonl")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		recs, err := readStepRecords(dir)
		if err == nil || !strings.Contains(err.Error(), ".eval/calls.jsonl") || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("err = %v, want a symlink error naming the file", err)
		}
		if len(recs) != 0 {
			t.Errorf("records = %+v", recs)
		}
	})
	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".eval", "calls.jsonl"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := readStepRecords(dir)
		if err == nil || !strings.Contains(err.Error(), ".eval/calls.jsonl") || !strings.Contains(err.Error(), "regular") {
			t.Fatalf("err = %v, want a not-regular error naming the file", err)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		dir := t.TempDir()
		writeCalls(t, dir, strings.Repeat("x", maxCheckFileBytes+1))
		_, err := readStepRecords(dir)
		if err == nil || !strings.Contains(err.Error(), ".eval/calls.jsonl") || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("err = %v, want a size-limit error naming the file", err)
		}
	})
	t.Run("symlinked .eval directory", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "calls.jsonl"), []byte(recA+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, ".eval")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		recs, err := readStepRecords(dir)
		if err == nil || !strings.Contains(err.Error(), ".eval/calls.jsonl") {
			t.Fatalf("err = %v, want an error naming the file", err)
		}
		if len(recs) != 0 {
			t.Errorf("records = %+v", recs)
		}
	})
}

func TestImportStepRecordsAppendsAndCounts(t *testing.T) {
	dir := t.TempDir()
	writeCalls(t, dir, recA+"\n"+recB+"\n")
	cr := CaseResult{
		Calls:     []inference.CallRecord{{Role: "agent", Agent: "outer"}},
		AgentCost: CostInfo{TokensIn: 100, TokensOut: 40, EstimatedUSD: 0.0009, UsageSource: "reported"},
	}
	if err := importStepRecords(&cr, dir); err != nil {
		t.Fatal(err)
	}
	if len(cr.Calls) != 3 || cr.Calls[0].Agent != "outer" || cr.Calls[1].Step != "design" || cr.Calls[2].Step != "build" {
		t.Fatalf("calls = %+v", cr.Calls)
	}
	if cr.AgentCost.TokensIn != 130 || cr.AgentCost.TokensOut != 55 {
		t.Errorf("AgentCost tokens = %d/%d, want 130/55", cr.AgentCost.TokensIn, cr.AgentCost.TokensOut)
	}
	if want := 0.0009 + 0.000105 + 0.00021; math.Abs(cr.AgentCost.EstimatedUSD-want) > 1e-12 {
		t.Errorf("AgentCost USD = %v, want %v", cr.AgentCost.EstimatedUSD, want)
	}
	// One imported record is estimated, so the merged usage source is no
	// longer "reported".
	if cr.AgentCost.UsageSource != "estimated" {
		t.Errorf("UsageSource = %q, want estimated", cr.AgentCost.UsageSource)
	}
}

func TestImportStepRecordsMalformedImportsNothing(t *testing.T) {
	dir := t.TempDir()
	writeCalls(t, dir, recA+"\nnope\n")
	cr := CaseResult{AgentCost: CostInfo{TokensIn: 7}}
	err := importStepRecords(&cr, dir)
	if err == nil || !strings.Contains(err.Error(), ".eval/calls.jsonl line 2") {
		t.Fatalf("err = %v", err)
	}
	if len(cr.Calls) != 0 || cr.AgentCost.TokensIn != 7 {
		t.Fatalf("nothing may be imported: calls=%+v cost=%+v", cr.Calls, cr.AgentCost)
	}
}

// stepCase is a stub case whose turn appends the given lines to
// .eval/calls.jsonl before answering.
func stepCase(name string, lines ...string) TestCase {
	var cmds []string
	for _, l := range lines {
		cmds = append(cmds, "printf '%s\\n' '"+l+"' >> .eval/calls.jsonl")
	}
	return TestCase{
		Name:  name,
		Agent: "selftest",
		Input: "do it",
		Stub: &inference.StubScript{Turns: []inference.StubTurn{{
			Response: "answer",
			Commands: cmds,
			Usage:    &inference.StubUsage{InputTokens: 100, OutputTokens: 40},
		}}},
	}
}

func TestExecuteCaseImportsStepRecords(t *testing.T) {
	execEnv(t, false)
	tc := stepCase("steps", recA, recB)
	// The clarity criterion goes to the judge, so a judge call follows the
	// imported records in Calls.
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext != nil || cr.ActualOutput != "answer" {
		t.Fatalf("output=%q ec=%+v\n%s", cr.ActualOutput, cr.ErrorContext, out.String())
	}
	if len(cr.Calls) != 4 {
		t.Fatalf("calls = %d, want agent + 2 imported + judge: %+v", len(cr.Calls), cr.Calls)
	}
	if cr.Calls[0].Role != "agent" || cr.Calls[0].Source != "" || cr.Calls[0].Step != "" {
		t.Errorf("Calls[0] = %+v, want the harness's own agent call", cr.Calls[0])
	}
	for i, want := range []struct{ agent, step string }{{"selftest-step-a", "design"}, {"selftest-step-b", "build"}} {
		c := cr.Calls[i+1]
		if c.Source != inference.SourceWorkspace || c.Agent != want.agent || c.Step != want.step {
			t.Errorf("Calls[%d] = %+v, want workspace record %v", i+1, c, want)
		}
	}
	if cr.Calls[3].Role != "judge" {
		t.Errorf("Calls[3].Role = %q, want judge after the imported records", cr.Calls[3].Role)
	}
	if cr.AgentCost.TokensIn != 130 || cr.AgentCost.TokensOut != 55 {
		t.Errorf("AgentCost tokens = %d/%d, want outer 100/40 + imported 30/15", cr.AgentCost.TokensIn, cr.AgentCost.TokensOut)
	}
	outer := costFromUsage("", inference.Usage{InputTokens: 100, OutputTokens: 40})
	if want := outer.EstimatedUSD + 0.000105 + 0.00021; math.Abs(cr.AgentCost.EstimatedUSD-want) > 1e-12 {
		t.Errorf("AgentCost USD = %v, want %v", cr.AgentCost.EstimatedUSD, want)
	}
}

func TestExecuteCaseNoStepRecordsLeavesCostAndCallsAlone(t *testing.T) {
	execEnv(t, false)
	tc := stepCase("none")
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "answer"}}
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if len(cr.Calls) != 1 || cr.AgentCost.TokensIn != 100 || cr.AgentCost.TokensOut != 40 {
		t.Fatalf("calls=%+v cost=%+v", cr.Calls, cr.AgentCost)
	}
}

func TestExecuteCaseMalformedStepRecordsFailClosed(t *testing.T) {
	execEnv(t, false)
	tc := stepCase("bad", recA, "", "not json")
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ActualOutput != "" {
		t.Errorf("ActualOutput = %q, want empty", cr.ActualOutput)
	}
	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, ".eval/calls.jsonl line 3") {
		t.Fatalf("ErrorContext = %+v", cr.ErrorContext)
	}
	if !strings.Contains(out.String(), ".eval/calls.jsonl line 3") {
		t.Errorf("case output must show the error:\n%s", out.String())
	}
	for _, c := range cr.Calls {
		if c.Source == inference.SourceWorkspace {
			t.Errorf("nothing may be imported from a malformed file: %+v", cr.Calls)
		}
	}
	if cr.AgentCost.TokensIn != 100 || cr.AgentCost.TokensOut != 40 {
		t.Errorf("AgentCost = %+v, want only the outer usage", cr.AgentCost)
	}
	if len(cr.Scores) == 0 {
		t.Fatal("no scores")
	}
	for _, s := range cr.Scores {
		if s.Score != 0 {
			t.Errorf("criterion %s scored %d, want 0", s.Name, s.Score)
		}
	}
}

func TestExecuteCaseImportsOnAgentFailureKeepingErrorContext(t *testing.T) {
	execEnv(t, false)
	tc := stepCase("fail", recA)
	tc.Stub.Turns[0].Commands = append(tc.Stub.Turns[0].Commands, "echo boom >&2; exit 3")
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, "boom") {
		t.Fatalf("original ErrorContext lost: %+v\n%s", cr.ErrorContext, out.String())
	}
	var imported []inference.CallRecord
	for _, c := range cr.Calls {
		if c.Source == inference.SourceWorkspace {
			imported = append(imported, c)
		}
	}
	if len(imported) != 1 || imported[0].Step != "design" {
		t.Fatalf("records of a failed run must still be imported: %+v", cr.Calls)
	}
	if cr.AgentCost.TokensIn < 10 || cr.AgentCost.TokensOut < 5 {
		t.Errorf("imported tokens not counted: %+v", cr.AgentCost)
	}
}

func TestExecuteCaseMalformedOnAgentFailureAppendsToErrorContext(t *testing.T) {
	execEnv(t, false)
	tc := stepCase("failbad", "not json")
	tc.Stub.Turns[0].Commands = append(tc.Stub.Turns[0].Commands, "echo boom >&2; exit 3")
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext == nil {
		t.Fatal("no ErrorContext")
	}
	s := cr.ErrorContext.Stderr
	if !strings.Contains(s, "boom") || !strings.Contains(s, ".eval/calls.jsonl line 1") {
		t.Errorf("Stderr = %q, want the original error with the import error appended", s)
	}
	if strings.Index(s, "boom") > strings.Index(s, ".eval/calls.jsonl") {
		t.Errorf("import error must be appended after the original: %q", s)
	}
}

func TestExecuteCaseMultiTurnImportsOnce(t *testing.T) {
	execEnv(t, false)
	// The file is cumulative across turns: turn 2 sees turn 1's line.
	tc := turnsCase("multi", []string{"one", "two"},
		inference.StubTurn{Response: "R1", Commands: []string{"printf '%s\\n' '" + recA + "' >> .eval/calls.jsonl"}},
		inference.StubTurn{Response: "R2", Commands: []string{"printf '%s\\n' '" + recB + "' >> .eval/calls.jsonl"}},
	)
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "R2"}}
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	var steps []string
	for _, c := range cr.Calls {
		if c.Source == inference.SourceWorkspace {
			steps = append(steps, c.Step)
		}
	}
	if strings.Join(steps, ",") != "design,build" {
		t.Fatalf("imported steps = %v, want [design build] exactly once\n%s", steps, out.String())
	}
}

// readRunAgentResult reads the single run's <agent>.json under evalsDir.
func readRunAgentResult(t *testing.T, evalsDir, agent string) AgentResult {
	t.Helper()
	var ar AgentResult
	readJSON(t, filepath.Join(onlyRunDir(t, evalsDir), agent+".json"), &ar)
	return ar
}

func TestSelftestWorkflowStubStepRecordsCase(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest-workflow", "stub-step-records", RunOptions{})
	if err != nil {
		t.Fatalf("selftest-workflow stub-step-records must pass: %v\n%s", err, out)
	}
	ar := readRunAgentResult(t, evalsDir, "selftest-workflow")
	if len(ar.Cases) != 1 {
		t.Fatalf("cases = %d", len(ar.Cases))
	}
	cr := ar.Cases[0]
	if cr.ErrorContext != nil {
		t.Fatalf("ErrorContext = %+v", cr.ErrorContext)
	}
	var steps []string
	for _, c := range cr.Calls {
		if c.Source == inference.SourceWorkspace {
			steps = append(steps, c.Agent+"/"+c.Step)
		}
	}
	if strings.Join(steps, ",") != "selftest-step-a/design,selftest-step-b/build" {
		t.Errorf("imported records = %v", steps)
	}
	if cr.Calls[0].Source != "" || cr.Calls[0].Role != "agent" {
		t.Errorf("Calls[0] = %+v, want the harness's agent call first", cr.Calls[0])
	}
	// Outer stub usage 100/40 plus the two imported records (10/5 and 20/10).
	if cr.AgentCost.TokensIn != 130 || cr.AgentCost.TokensOut != 55 {
		t.Errorf("agent_cost tokens = %d/%d, want 130/55", cr.AgentCost.TokensIn, cr.AgentCost.TokensOut)
	}
	outer := costFromUsage("", inference.Usage{InputTokens: 100, OutputTokens: 40})
	if want := outer.EstimatedUSD + 0.000105 + 0.00021; math.Abs(cr.AgentCost.EstimatedUSD-want) > 1e-12 {
		t.Errorf("agent_cost usd = %v, want %v", cr.AgentCost.EstimatedUSD, want)
	}
	// The imported steps are also in the persisted JSON.
	raw := readFileT(t, filepath.Join(onlyRunDir(t, evalsDir), "selftest-workflow.json"))
	if !strings.Contains(raw, `"source": "workspace"`) && !strings.Contains(raw, `"source":"workspace"`) {
		t.Errorf("result JSON lacks source=workspace records")
	}
}

func TestSelftestFailCallsMalformedLineCase(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest-fail", "calls-malformed-line", RunOptions{})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed\n%s", err, out)
	}
	if !strings.Contains(out, ".eval/calls.jsonl line 3") {
		t.Errorf("output must name line 3:\n%s", out)
	}
	cr := readRunAgentResult(t, evalsDir, "selftest-fail").Cases[0]
	if cr.ActualOutput != "" {
		t.Errorf("ActualOutput = %q, want empty", cr.ActualOutput)
	}
	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, ".eval/calls.jsonl line 3") {
		t.Fatalf("ErrorContext = %+v", cr.ErrorContext)
	}
	for _, c := range cr.Calls {
		if c.Source == inference.SourceWorkspace {
			t.Errorf("nothing may be imported: %+v", cr.Calls)
		}
	}
	for _, s := range cr.Scores {
		if s.Score != 0 {
			t.Errorf("criterion %s scored %d, want 0", s.Name, s.Score)
		}
	}
}
