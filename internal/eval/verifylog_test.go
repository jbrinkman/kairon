package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	vlAgent       = "selftest"
	vlFixtureRoot = "testdata/verifylog"
	vlShaA        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	vlShaB        = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	vlShaC        = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	vlSuiteS1     = "1111111111111111111111111111111111111111111111111111111111111111"
	vlSuiteS2     = "2222222222222222222222222222222222222222222222222222222222222222"
)

const vlBody = `## Baseline

The baseline score.

## Hypothesis

A clearer prompt helps.

## Change

Rewrote the prompt.

## Results

The measured score.

## Reasoning

The result supports the hypothesis.
`

// ---------- helpers ----------

// vlFixtureOpts returns options pointing at one fixture log set.
func vlFixtureOpts(set string, min int) VerifyLogOptions {
	return VerifyLogOptions{
		IterationsDir: filepath.Join(vlFixtureRoot, set),
		EvalsDir:      filepath.Join(vlFixtureRoot, "evals"),
		MinIterations: min,
	}
}

func vlWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// vlWriteRun writes a run under <evalsDir>/results/<name>. An empty summary or
// agentJSON string leaves that file out.
func vlWriteRun(t *testing.T, evalsDir, name, summary, agentJSON string) {
	t.Helper()
	dir := filepath.Join(evalsDir, "results", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if summary != "" {
		vlWriteFile(t, filepath.Join(dir, "summary.json"), summary)
	}
	if agentJSON != "" {
		vlWriteFile(t, filepath.Join(dir, vlAgent+".json"), agentJSON)
	}
}

// vlRunSpec describes the facts one run records for vlAgent. An empty string
// (or nil Complete) means "not recorded".
type vlRunSpec struct {
	Score      float64
	Prompt     string
	Suite      string
	AgentModel string
	JudgeModel string
	Sandbox    string // "native" or "container"
	Complete   *bool  // recorded together with CaseFilter
	CaseFilter string
	// Checks are the per-check results recorded in <agent>.json. Empty means
	// the run records no checks (cases is []).
	Checks []vlCheck
}

// vlCheck is one check of one criterion of one case. Passed holds one result
// per repeat (index 0 is repeat 1). A check with fewer entries than the case
// has repeats is absent from the later repeats.
type vlCheck struct {
	Case      string
	Criterion string
	Label     string
	Passed    []bool
}

// vlChk builds a check of case "basic", criterion "alpha".
func vlChk(label string, passed ...bool) vlCheck {
	return vlCheck{Case: "basic", Criterion: "alpha", Label: label, Passed: passed}
}

const (
	vlLblFile  = "file_exists path=out.txt"
	vlLblReady = "output_contains pattern=/ready/"
)

// vlFailingStd is the single failing check the std eval-result runs record.
func vlFailingStd() []vlCheck {
	return []vlCheck{vlChk("#1 "+vlLblFile, true, true), vlChk("#2 "+vlLblReady, true, false)}
}

// casesJSON renders Checks as cases[] entries, one per (case, repeat).
func (r vlRunSpec) casesJSON() []any {
	repeats := map[string]int{}
	var order []string
	for _, c := range r.Checks {
		if _, ok := repeats[c.Case]; !ok {
			order = append(order, c.Case)
		}
		if len(c.Passed) > repeats[c.Case] {
			repeats[c.Case] = len(c.Passed)
		}
	}
	out := []any{}
	for _, name := range order {
		for rep := 0; rep < repeats[name]; rep++ {
			var crits []string
			byCrit := map[string][]any{}
			for _, c := range r.Checks {
				if c.Case != name || rep >= len(c.Passed) {
					continue
				}
				if _, ok := byCrit[c.Criterion]; !ok {
					crits = append(crits, c.Criterion)
				}
				idx := 0
				if _, err := fmt.Sscanf(c.Label, "#%d ", &idx); err != nil {
					idx = len(byCrit[c.Criterion]) + 1
				}
				byCrit[c.Criterion] = append(byCrit[c.Criterion], map[string]any{
					"index": idx, "type": "x", "label": c.Label, "passed": c.Passed[rep],
				})
			}
			scores := []any{}
			for _, cr := range crits {
				scores = append(scores, map[string]any{
					"name": cr, "score": 0, "max_score": 0, "deterministic": true, "checks": byCrit[cr],
				})
			}
			out = append(out, map[string]any{"case_name": name, "run": rep + 1, "scores": scores})
		}
	}
	return out
}

func vlBool(b bool) *bool { return &b }

// vlStdSpec is a complete, consistent run: suite S1, fixed models, native
// sandbox, complete with no case filter.
func vlStdSpec(score float64, prompt string) vlRunSpec {
	return vlRunSpec{
		Score: score, Prompt: prompt, Suite: vlSuiteS1,
		AgentModel: "agent-m", JudgeModel: "judge-m", Sandbox: "native",
		Complete: vlBool(true),
	}
}

func vlMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// summaryJSON renders the spec as a summary.json.
func (r vlRunSpec) summaryJSON() string {
	entry := map[string]any{}
	if r.Prompt != "" {
		entry["prompt_sha256"] = r.Prompt
	}
	if r.AgentModel != "" {
		entry["agent_model"] = r.AgentModel
	}
	if r.Suite != "" {
		entry["suite_sha256"] = r.Suite
	}
	if r.Complete != nil {
		entry["complete"] = *r.Complete
		entry["case_filter"] = r.CaseFilter
	}
	sum := map[string]any{
		"git_hash":       "x",
		"agent_verdicts": map[string]any{vlAgent: map[string]any{"score": r.Score, "threshold": 70}},
		"agents":         map[string]any{vlAgent: entry},
	}
	if r.JudgeModel != "" {
		sum["judge_model"] = r.JudgeModel
	}
	if r.Sandbox != "" {
		sum["sandbox"] = r.Sandbox
	}
	return vlMarshal(sum)
}

// agentJSON renders the spec as an <agent>.json (sandbox is a bool there).
func (r vlRunSpec) agentJSON() string {
	a := map[string]any{"agent": vlAgent, "cases": r.casesJSON()}
	if r.Prompt != "" {
		a["prompt_sha256"] = r.Prompt
	}
	if r.AgentModel != "" {
		a["agent_model"] = r.AgentModel
	}
	if r.JudgeModel != "" {
		a["judge_model"] = r.JudgeModel
	}
	if r.Sandbox != "" {
		a["sandbox"] = r.Sandbox == "container"
	}
	if r.Suite != "" {
		a["suite_sha256"] = r.Suite
	}
	if r.Complete != nil {
		a["complete"] = *r.Complete
		a["case_filter"] = r.CaseFilter
	}
	return vlMarshal(a)
}

func vlVerdictSummary(score float64, sha string) string {
	return vlStdSpec(score, sha).summaryJSON()
}

func vlAgentJSON(sha string) string {
	return vlStdSpec(0, sha).agentJSON()
}

// vlWriteSpecRun writes a run whose summary.json and <agent>.json both carry
// spec's facts.
func vlWriteSpecRun(t *testing.T, evalsDir, name string, spec vlRunSpec) {
	t.Helper()
	vlWriteRun(t, evalsDir, name, spec.summaryJSON(), spec.agentJSON())
}

// vlCompleteRun patches the run's existing files so they record the standard
// models, sandbox and coverage plus the given prompt and suite hashes. Used by
// tests that vary only how the score is recorded.
func vlCompleteRun(t *testing.T, evalsDir, name, prompt, suite string) {
	t.Helper()
	dir := filepath.Join(evalsDir, "results", name)
	patch := func(file string, fn func(m map[string]any)) {
		path := filepath.Join(dir, file)
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		fn(m)
		vlWriteFile(t, path, vlMarshal(m))
	}
	patch("summary.json", func(m map[string]any) {
		m["judge_model"], m["sandbox"] = "judge-m", "native"
		agents, _ := m["agents"].(map[string]any)
		if agents == nil {
			agents = map[string]any{}
		}
		agents[vlAgent] = map[string]any{
			"agent_model": "agent-m", "prompt_sha256": prompt, "suite_sha256": suite,
			"complete": true, "case_filter": "",
		}
		m["agents"] = agents
	})
	patch(vlAgent+".json", func(m map[string]any) {
		m["agent_model"], m["judge_model"], m["sandbox"] = "agent-m", "judge-m", false
		m["prompt_sha256"], m["suite_sha256"] = prompt, suite
		m["complete"], m["case_filter"] = true, ""
	})
}

// vlStdRuns writes the standard runs: r0 60.0/A, r1 72.5/B, r2 90.0/C,
// r2b 91.0/C (same prompt as r2), r1dup 73.0/B (same prompt as r1), plus
// r0s 60.0/A and r2s 91.0/C, which have the same prompt as r0 and r2 but a
// changed suite (S2), so they form valid eval iterations with them. The runs
// used as eval results (r0, r0s, r2b, r2s) each record a failing check so the
// red phase holds for them.
func vlStdRuns(t *testing.T, evalsDir string) {
	t.Helper()
	for _, r := range []struct {
		name  string
		score float64
		sha   string
		suite string
	}{
		{"r0", 60.0, vlShaA, vlSuiteS1}, {"r1", 72.5, vlShaB, vlSuiteS1}, {"r2", 90.0, vlShaC, vlSuiteS1},
		{"r2b", 91.0, vlShaC, vlSuiteS1}, {"r1dup", 73.0, vlShaB, vlSuiteS1},
		{"r0s", 60.0, vlShaA, vlSuiteS2}, {"r2s", 91.0, vlShaC, vlSuiteS2},
	} {
		spec := vlStdSpec(r.score, r.sha)
		spec.Suite = r.suite
		switch r.name {
		case "r0s", "r2s", "r2b": // used as eval results, so each fails a check
			spec.Checks = vlFailingStd()
		case "r0":
			// r0 is also an eval result (r0s -> r0 in the sequence tests). It fails
			// a check r0s does not, so both directions of r0 <-> r0s have a new failure.
			spec.Checks = []vlCheck{vlChk("#1 output_contains pattern=/other/", false, false)}
		}
		vlWriteSpecRun(t, evalsDir, r.name, spec)
	}
}

// vlDefaultPhase is the phase vlFM gives a log: baseline for iteration 0, red
// for later eval iterations and refactor for later prompt iterations.
func vlDefaultPhase(iter int, ctype string) string {
	switch {
	case iter == 0:
		return "baseline"
	case ctype == "eval":
		return "red"
	default:
		return "refactor"
	}
}

// vlFM renders front-matter lines (no delimiters) with the default phase for
// the iteration number and change type.
func vlFM(iter int, date, ctype, base, res, bs, rs string) string {
	return vlFMPhase(vlDefaultPhase(iter, ctype), iter, date, ctype, base, res, bs, rs)
}

// vlFMPhase renders front-matter lines (no delimiters) with an explicit
// phase. An empty phase leaves the key out.
func vlFMPhase(phase string, iter int, date, ctype, base, res, bs, rs string) string {
	fm := fmt.Sprintf("iteration: %d\ndate: %s\nchange_type: %s\nbaseline_run: %s\nresult_run: %s\nbaseline_score: %s\nresult_score: %s\n",
		iter, date, ctype, base, res, bs, rs)
	if phase != "" {
		fm += "phase: " + phase + "\n"
	}
	return fm
}

func vlDoc(fm, body string) string { return "---\n" + fm + "---\n\n" + body }

// vlWithout removes the front-matter line for key.
func vlWithout(fm, key string) string {
	var out []string
	for _, l := range strings.SplitAfter(fm, "\n") {
		if !strings.HasPrefix(l, key+":") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "")
}

// vlGoodFM is a valid iteration-00 front-matter over the standard runs.
func vlGoodFM() string {
	return vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5")
}

// vlSetup creates std runs and writes logs (file name -> content) for vlAgent.
func vlSetup(t *testing.T, logs map[string]string) VerifyLogOptions {
	t.Helper()
	root := t.TempDir()
	evalsDir := filepath.Join(root, "evals")
	itersDir := filepath.Join(root, "iterations")
	vlStdRuns(t, evalsDir)
	if err := os.MkdirAll(filepath.Join(itersDir, vlAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range logs {
		vlWriteFile(t, filepath.Join(itersDir, vlAgent, name), content)
	}
	return VerifyLogOptions{IterationsDir: itersDir, EvalsDir: evalsDir}
}

func vlRunVerify(t *testing.T, opts VerifyLogOptions) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := VerifyLog(&buf, vlAgent, opts)
	return buf.String(), err
}

func vlViolations(t *testing.T, err error) []LogViolation {
	t.Helper()
	var ve *VerifyLogError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *VerifyLogError, got %T: %v", err, err)
	}
	return ve.Violations
}

// vlExpectOnly asserts err is a *VerifyLogError with exactly one violation of
// rule whose File ends in fileSuffix (empty suffix: not checked).
func vlExpectOnly(t *testing.T, err error, rule, fileSuffix string) LogViolation {
	t.Helper()
	vs := vlViolations(t, err)
	if len(vs) != 1 {
		t.Fatalf("expected exactly 1 violation (%s), got %d: %v", rule, len(vs), err)
	}
	if vs[0].Rule != rule {
		t.Fatalf("rule = %q, want %q (%v)", vs[0].Rule, rule, vs[0])
	}
	if fileSuffix != "" && !strings.HasSuffix(vs[0].File, fileSuffix) {
		t.Fatalf("File = %q, want suffix %q", vs[0].File, fileSuffix)
	}
	if vs[0].Message == "" {
		t.Fatalf("violation has empty message: %+v", vs[0])
	}
	return vs[0]
}

func vlRules(vs []LogViolation) []string {
	var r []string
	for _, v := range vs {
		r = append(r, v.Rule)
	}
	return r
}

// ---------- fixture table test ----------

func TestVerifyLogFixtureSets(t *testing.T) {
	tests := []struct {
		set        string
		min        int
		wantRule   string // "" => success
		wantSuffix string
		// wantMentions are substrings the single violation's message must contain.
		wantMentions []string
	}{
		{set: "valid", min: 2},
		{set: "missing-run", min: 0, wantRule: "run-exists", wantSuffix: "iteration-01.md"},
		{set: "score-mismatch", min: 0, wantRule: "score-match", wantSuffix: "iteration-01.md"},
		{set: "gap", min: 0, wantRule: "sequence", wantSuffix: "iteration-02.md"},
		{set: "broken-chain", min: 0, wantRule: "chain", wantSuffix: "iteration-01.md"},
		{set: "no-prompt-change", min: 0, wantRule: "prompt-change", wantSuffix: "iteration-01.md"},
		{set: "partial-run", min: 2, wantRule: "run-complete", wantSuffix: "iteration-01.md", wantMentions: []string{"260705-100000-1111111", "complete=false", "stub-basic"}},
		{set: "model-mismatch", min: 2, wantRule: "run-config", wantSuffix: "iteration-01.md", wantMentions: []string{"agent_model"}},
		{set: "sandbox-mismatch", min: 2, wantRule: "run-config", wantSuffix: "iteration-01.md", wantMentions: []string{"sandbox"}},
		{set: "judge-mismatch", min: 2, wantRule: "run-config", wantSuffix: "iteration-01.md", wantMentions: []string{"judge_model"}},
		{set: "suite-changed", min: 2, wantRule: "suite-change", wantSuffix: "iteration-01.md"},
		{set: "eval-prompt-changed", min: 2, wantRule: "prompt-change", wantSuffix: "iteration-01.md"},
		{set: "eval-nothing-changed", min: 2, wantRule: "suite-change", wantSuffix: "iteration-01.md"},
		{set: "legacy-run", min: 2, wantRule: "run-data", wantSuffix: "iteration-01.md", wantMentions: []string{"260705-170000-8888888", "suite_sha256", "complete"}},
		{set: "missing-phase", min: 0, wantRule: "phase", wantSuffix: "iteration-00.md", wantMentions: []string{"phase", "baseline", "red", "green", "refactor"}},
		{set: "baseline-not-first", min: 0, wantRule: "phase", wantSuffix: "iteration-01.md", wantMentions: []string{"baseline", "iteration-00.md"}},
		{set: "red-no-new-failure", min: 0, wantRule: "red", wantSuffix: "iteration-01.md", wantMentions: []string{"260706-110000-aaaaa00", "failing"}},
		{set: "green-no-fix", min: 0, wantRule: "green", wantSuffix: "iteration-02.md", wantMentions: []string{"260707-110000-bbbbb22", "passes"}},
		{set: "green-score-drop", min: 0, wantRule: "green", wantSuffix: "iteration-02.md", wantMentions: []string{"260707-120000-bbbbb33", "70.00", "65.00"}},
		{set: "refactor-score-drop", min: 0, wantRule: "refactor", wantSuffix: "iteration-03.md", wantMentions: []string{"260708-110000-ccccc22", "85.00", "80.00"}},
		{set: "valid", min: 5, wantRule: "min-iterations", wantSuffix: filepath.Join("valid", vlAgent)},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/min=%d", tt.set, tt.min), func(t *testing.T) {
			out, err := vlRunVerify(t, vlFixtureOpts(tt.set, tt.min))
			if tt.wantRule == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if want := "OK selftest: 4 iterations verified\n"; out != want {
					t.Fatalf("output = %q, want %q", out, want)
				}
				return
			}
			v := vlExpectOnly(t, err, tt.wantRule, tt.wantSuffix)
			vlMustMention(t, v.Message, tt.wantMentions...)
			if out != "" {
				t.Fatalf("nothing should be printed on failure, got %q", out)
			}
		})
	}
}

// vlLegacyFixtureRuns are the two committed runs that deliberately predate
// the coverage fields (suite_sha256 / complete / case_filter).
var vlLegacyFixtureRuns = map[string]bool{
	"260630-100000-fffffff": true,
	"260705-170000-8888888": true,
}

// TestVerifyLogFixtureLegacyRun replaces the old legacy-fixture test: the
// committed legacy run fffffff must now fail run-data for the missing fields.
func TestVerifyLogFixtureLegacyRun(t *testing.T) {
	root := t.TempDir()
	iters := filepath.Join(root, "iterations")
	vlWriteFile(t, filepath.Join(iters, vlAgent, "iteration-00.md"),
		vlDoc(vlFM(0, "2026-07-01", "prompt", "260630-100000-fffffff", "260701-100000-aaaaaaa", "80.0", "60.0"), vlBody))
	opts := VerifyLogOptions{IterationsDir: iters, EvalsDir: filepath.Join(vlFixtureRoot, "evals")}
	_, err := vlRunVerify(t, opts)
	var found bool
	for _, v := range vlViolations(t, err) {
		if v.Rule != "run-data" || !strings.Contains(v.Message, "260630-100000-fffffff") {
			continue
		}
		found = true
		vlMustMention(t, v.Message, "260630-100000-fffffff", vlAgent, "suite_sha256", "complete")
	}
	if !found {
		t.Fatalf("legacy run fffffff should fail run-data, got %v", err)
	}
}

// TestVerifyLogFixtureRunsRecordCoverage guards the committed runs: every one
// except the two legacy runs must record suite_sha256, complete and
// case_filter for the agent, so a fixture cannot silently drift to legacy shape.
func TestVerifyLogFixtureRunsRecordCoverage(t *testing.T) {
	resultsDir := filepath.Join(vlFixtureRoot, "evals", "results")
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		t.Fatal(err)
	}
	var checked, legacy int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(resultsDir, e.Name(), "summary.json"))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		var sum struct {
			Agents map[string]map[string]json.RawMessage `json:"agents"`
		}
		if err := json.Unmarshal(data, &sum); err != nil {
			t.Errorf("%s: invalid summary.json: %v", e.Name(), err)
			continue
		}
		entry := sum.Agents[vlAgent]
		if vlLegacyFixtureRuns[e.Name()] {
			legacy++
			for _, f := range []string{"suite_sha256", "complete", "case_filter"} {
				if _, ok := entry[f]; ok {
					t.Errorf("legacy run %s must not record %s", e.Name(), f)
				}
			}
			continue
		}
		checked++
		for _, f := range []string{"suite_sha256", "complete", "case_filter"} {
			if _, ok := entry[f]; !ok {
				t.Errorf("run %s: agents.%s.%s is not recorded (only the legacy runs %v may omit it)", e.Name(), vlAgent, f, vlLegacyFixtureRuns)
			}
		}
	}
	if legacy != len(vlLegacyFixtureRuns) {
		t.Errorf("found %d committed legacy runs, want %d", legacy, len(vlLegacyFixtureRuns))
	}
	if checked < 12 {
		t.Errorf("only %d non-legacy runs checked, want at least 12", checked)
	}
}

// TestVerifyLogFixtureValidEvalIteration pins the shape of the valid set's
// red (eval) iteration, b -> R1: same prompt hash, different suite hash.
func TestVerifyLogFixtureValidEvalIteration(t *testing.T) {
	read := func(run string) (prompt, suite string) {
		data, err := os.ReadFile(filepath.Join(vlFixtureRoot, "evals", "results", run, "summary.json"))
		if err != nil {
			t.Fatal(err)
		}
		var sum struct {
			Agents map[string]struct {
				PromptSHA256 string `json:"prompt_sha256"`
				SuiteSHA256  string `json:"suite_sha256"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(data, &sum); err != nil {
			t.Fatal(err)
		}
		a := sum.Agents[vlAgent]
		return a.PromptSHA256, a.SuiteSHA256
	}
	pb, sb := read("260702-100000-bbbbbbb")
	pr, sr := read("260706-100000-aaaaa11")
	if pb == "" || pb != pr {
		t.Errorf("eval iteration prompt hashes: %q vs %q, want equal and non-empty", pb, pr)
	}
	if sb == "" || sr == "" || sb == sr {
		t.Errorf("eval iteration suite hashes: %q vs %q, want both set and different", sb, sr)
	}
	doc, err := os.ReadFile(filepath.Join(vlFixtureRoot, "valid", vlAgent, "iteration-01.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"change_type: eval", "phase: red"} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("valid iteration-01 should contain %q", want)
		}
	}
}

// TestVerifyLogFixtureValidPhases pins the valid set's four iterations:
// baseline, red, green, refactor, in that order.
func TestVerifyLogFixtureValidPhases(t *testing.T) {
	for i, want := range []string{"baseline", "red", "green", "refactor"} {
		doc, err := os.ReadFile(filepath.Join(vlFixtureRoot, "valid", vlAgent, fmt.Sprintf("iteration-%02d.md", i)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(doc), "\nphase: "+want+"\n") {
			t.Errorf("valid iteration-%02d should have phase %q", i, want)
		}
	}
	if _, err := os.Stat(filepath.Join(vlFixtureRoot, "valid", vlAgent, "iteration-04.md")); err == nil {
		t.Error("valid set must have exactly four iterations")
	}
}

func TestVerifyLogErrorRendersOneLinePerViolation(t *testing.T) {
	e := &VerifyLogError{Violations: []LogViolation{
		{File: "a/iteration-00.md", Rule: "chain", Message: "m1"},
		{File: "a/iteration-01.md", Rule: "score-match", Message: "m2"},
	}}
	want := "a/iteration-00.md: chain: m1\na/iteration-01.md: score-match: m2"
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestVerifyLogConstants(t *testing.T) {
	if DefaultIterationsDir != ".kairon/iterations" {
		t.Errorf("DefaultIterationsDir = %q", DefaultIterationsDir)
	}
	if DefaultMinIterations != 2 {
		t.Errorf("DefaultMinIterations = %d", DefaultMinIterations)
	}
}

// ---------- score tolerance ----------

func TestVerifyLogScoreTolerance(t *testing.T) {
	tests := []struct {
		name    string
		claimed string // result_score for r1 (recorded 72.5)
		wantErr bool
	}{
		{"exact", "72.5", false},
		{"0.1 above passes", "72.6", false},
		{"0.1 below passes", "72.4", false},
		{"0.2 above fails", "72.7", true},
		{"0.2 below fails", "72.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", tt.claimed), vlBody),
			})
			_, err := vlRunVerify(t, opts)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			v := vlExpectOnly(t, err, "score-match", "iteration-00.md")
			for _, s := range []string{"result_score", tt.claimed, "72.5", "r1"} {
				if !strings.Contains(v.Message, s) {
					t.Errorf("message %q should mention %q", v.Message, s)
				}
			}
		})
	}
}

func TestVerifyLogBaselineScoreToleranceBoundary(t *testing.T) {
	// 60.1 against a recorded 60.0 is 0.10000000000000142 in float64; it must
	// still pass as "exactly 0.1".
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.1", "72.5"), vlBody),
	})
	if _, err := vlRunVerify(t, opts); err != nil {
		t.Fatalf("0.1 away should pass: %v", err)
	}
	opts = vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.2", "72.5"), vlBody),
	})
	_, err := vlRunVerify(t, opts)
	v := vlExpectOnly(t, err, "score-match", "iteration-00.md")
	if !strings.Contains(v.Message, "baseline_score") {
		t.Errorf("message %q should name baseline_score", v.Message)
	}
}

// ---------- score source precedence ----------

func TestVerifyLogScoreSources(t *testing.T) {
	recomputeAgent := `{"agent":"selftest","prompt_sha256":"` + vlShaB + `","cases":[
		{"case_name":"c1","actual_output":"x","scores":[{"name":"a","score":14,"max_score":20},{"name":"b","score":15,"max_score":20}]}]}`

	tests := []struct {
		name      string
		summary   string
		agentJSON string
		claimed   string
		wantRule  string // "" => pass
	}{
		{
			name:      "agent_verdicts percent wins over agent_scores",
			summary:   `{"agent_verdicts":{"selftest":{"score":50.0}},"agent_scores":{"selftest":0.9},"agents":{"selftest":{"prompt_sha256":"` + vlShaB + `"}}}`,
			agentJSON: recomputeAgent, claimed: "50.0",
		},
		{
			name:      "agent_scores is ignored when agent_verdicts exists",
			summary:   `{"agent_verdicts":{"selftest":{"score":50.0}},"agent_scores":{"selftest":0.9}}`,
			agentJSON: recomputeAgent, claimed: "90.0", wantRule: "score-match",
		},
		{
			name:      "agent_scores fraction x100 when no verdict",
			summary:   `{"agent_scores":{"selftest":0.725},"agents":{"selftest":{"prompt_sha256":"` + vlShaB + `"}}}`,
			agentJSON: recomputeAgent, claimed: "72.5",
		},
		{
			name:      "agent_scores is a fraction not a percent",
			summary:   `{"agent_scores":{"selftest":0.725}}`,
			agentJSON: recomputeAgent, claimed: "0.7", wantRule: "score-match",
		},
		{
			name:      "recomputed from agent json when summary has neither",
			summary:   `{"git_hash":"x","agents":{"selftest":{"prompt_sha256":"` + vlShaB + `"}}}`,
			agentJSON: recomputeAgent, claimed: "72.5", // (14+15)/(20+20)
		},
		{
			name:      "recomputed from agent json when summary.json is absent",
			summary:   "",
			agentJSON: recomputeAgent, claimed: "72.5",
		},
		{
			name:      "recompute mismatch",
			summary:   `{"git_hash":"x"}`,
			agentJSON: recomputeAgent, claimed: "80.0", wantRule: "score-match",
		},
		{
			name:      "no score anywhere is run-data",
			summary:   `{"git_hash":"x"}`,
			agentJSON: vlAgentJSON(vlShaB), claimed: "72.5", wantRule: "run-data",
		},
		{
			// A present-but-empty verdict object ({}) has no "score"
			// field, so it is not a measured score: it must be ignored and
			// fall through, not accepted as a real 0.0.
			name:      "empty agent_verdicts object is not a 0.0 score",
			summary:   `{"git_hash":"x","agent_verdicts":{"selftest":{}}}`,
			agentJSON: vlAgentJSON(vlShaB), claimed: "0.0", wantRule: "run-data",
		},
		{
			// A present-but-null verdict score decodes the numeric Score
			// field to 0.0; it must not be accepted as a real 0.0 score.
			name:      "null agent_verdicts score is not a 0.0 score",
			summary:   `{"git_hash":"x","agent_verdicts":{"selftest":{"score":null}}}`,
			agentJSON: vlAgentJSON(vlShaB), claimed: "0.0", wantRule: "run-data",
		},
		{
			// A JSON null agent_score decodes to 0.0 with the key
			// present; it must not be accepted as a real 0.0 score.
			name:      "null agent_score is not a 0.0 score",
			summary:   `{"git_hash":"x","agent_scores":{"selftest":null}}`,
			agentJSON: vlAgentJSON(vlShaB), claimed: "0.0", wantRule: "run-data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// eval change type so only the score source is under test.
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r0", "src", "60.0", tt.claimed), vlBody),
			})
			vlWriteRun(t, opts.EvalsDir, "src", tt.summary, tt.agentJSON)
			// Same prompt as r0 and a changed suite, so the eval iteration is
			// otherwise valid and only the score source is under test.
			vlCompleteRun(t, opts.EvalsDir, "src", vlShaA, vlSuiteS2)
			_, err := vlRunVerify(t, opts)
			if tt.wantRule == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			vlExpectOnly(t, err, tt.wantRule, "iteration-00.md")
		})
	}
}

func TestVerifyLogLegacyFixtureRun(t *testing.T) {
	// The committed legacy run records only agent_scores (0.8 => 80.0) and a
	// prompt hash: none of suite_sha256 / complete. It no longer verifies.
	root := t.TempDir()
	iters := filepath.Join(root, "iterations")
	vlWriteFile(t, filepath.Join(iters, vlAgent, "iteration-00.md"),
		vlDoc(vlFM(0, "2026-07-01", "eval", "260630-100000-fffffff", "260701-100000-aaaaaaa", "80.0", "60.0"), vlBody))
	opts := VerifyLogOptions{IterationsDir: iters, EvalsDir: filepath.Join(vlFixtureRoot, "evals")}
	_, err := vlRunVerify(t, opts)
	var found bool
	for _, v := range vlViolations(t, err) {
		if v.Rule != "run-data" {
			continue
		}
		found = true
		for _, want := range []string{"260630-100000-fffffff", vlAgent, "suite_sha256", "complete"} {
			if !strings.Contains(v.Message, want) {
				t.Errorf("run-data message %q should mention %q", v.Message, want)
			}
		}
	}
	if !found {
		t.Fatalf("legacy run should fail run-data, got %v", err)
	}
}

// ---------- prompt hash handling ----------

func TestVerifyLogPromptHashRules(t *testing.T) {
	t.Run("prompt iteration with no prompt_sha256 is run-data", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "nosha", "60.0", "72.5"), vlBody),
		})
		// Same score as r1 but no prompt hash in either file; every other
		// fact is recorded so only the missing hash matters.
		spec := vlStdSpec(72.5, "")
		vlWriteSpecRun(t, opts.EvalsDir, "nosha", spec)
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
		if !strings.Contains(v.Message, "prompt_sha256") || !strings.Contains(v.Message, "nosha") {
			t.Errorf("message %q should name prompt_sha256 and the run", v.Message)
		}
	})

	t.Run("eval iteration with identical prompt_sha256 and a changed suite passes", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r2", "r2s", "90.0", "91.0"), vlBody),
		})
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("eval iteration with no prompt_sha256 is run-data", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r0", "nosha", "60.0", "72.5"), vlBody),
		})
		spec := vlStdSpec(72.5, "")
		spec.Suite = vlSuiteS2
		vlWriteSpecRun(t, opts.EvalsDir, "nosha", spec)
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
		if !strings.Contains(v.Message, "prompt_sha256") || !strings.Contains(v.Message, "nosha") {
			t.Errorf("message %q should name prompt_sha256 and the run", v.Message)
		}
	})

	t.Run("prompt iteration with identical sha is prompt-change", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r2", "r2b", "90.0", "91.0"), vlBody),
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "prompt-change", "iteration-00.md")
	})

	t.Run("agent json hash is preferred over summary hash", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "mixed", "60.0", "72.5"), vlBody),
		})
		// agent json says hash D (differs from r0); summary says hash A (same as r0).
		vlWriteRun(t, opts.EvalsDir, "mixed", vlVerdictSummary(72.5, vlShaA), vlAgentJSON(strings.Repeat("d", 64)))
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("agent json hash should win: %v", err)
		}
	})

	t.Run("summary hash is the fallback", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "sumonly", "60.0", "72.5"), vlBody),
		})
		vlWriteRun(t, opts.EvalsDir, "sumonly", vlVerdictSummary(72.5, vlShaB), "")
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("summary hash fallback should work: %v", err)
		}
	})
}

// ---------- run coverage and configuration rules ----------

// vlPairOpts writes a baseline run "base" and a result run "res" from the given
// specs plus one iteration-00 log of changeType whose claimed scores equal the
// specs' scores.
func vlPairOpts(t *testing.T, changeType string, base, res vlRunSpec) VerifyLogOptions {
	t.Helper()
	root := t.TempDir()
	evalsDir := filepath.Join(root, "evals")
	vlWriteSpecRun(t, evalsDir, "base", base)
	vlWriteSpecRun(t, evalsDir, "res", res)
	fm := vlFM(0, "2026-07-01", changeType, "base", "res", fmt.Sprintf("%.1f", base.Score), fmt.Sprintf("%.1f", res.Score))
	vlWriteFile(t, filepath.Join(root, "iterations", vlAgent, "iteration-00.md"), vlDoc(fm, vlBody))
	return VerifyLogOptions{IterationsDir: filepath.Join(root, "iterations"), EvalsDir: evalsDir}
}

// vlBaseRes returns a standard baseline (prompt A) and a result (prompt B).
func vlBaseRes() (vlRunSpec, vlRunSpec) {
	return vlStdSpec(60.0, vlShaA), vlStdSpec(72.5, vlShaB)
}

func vlMustMention(t *testing.T, msg string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(msg, w) {
			t.Errorf("message %q should mention %q", msg, w)
		}
	}
}

func TestVerifyLogBaselineStdRunsPass(t *testing.T) {
	base, res := vlBaseRes()
	if _, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res)); err != nil {
		t.Fatalf("a prompt iteration over complete, consistent runs should pass: %v", err)
	}
}

func TestVerifyLogRunDataCoverageFields(t *testing.T) {
	for _, ct := range []string{"prompt", "eval"} {
		// An eval iteration needs a changed suite to be otherwise valid.
		mk := func() (vlRunSpec, vlRunSpec) {
			base, res := vlBaseRes()
			if ct == "eval" {
				res.Prompt = base.Prompt
				res.Suite = vlSuiteS2
			}
			return base, res
		}
		t.Run(ct+"/no suite_sha256", func(t *testing.T) {
			base, res := mk()
			res.Suite = ""
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
			vlMustMention(t, v.Message, `"res"`, vlAgent, "suite_sha256")
			if strings.Contains(v.Message, "complete") || strings.Contains(v.Message, "prompt_sha256") {
				t.Errorf("message %q should list only the missing field", v.Message)
			}
		})
		t.Run(ct+"/no complete", func(t *testing.T) {
			base, res := mk()
			base.Complete = nil
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
			vlMustMention(t, v.Message, `"base"`, vlAgent, "complete")
			if strings.Contains(v.Message, "suite_sha256") {
				t.Errorf("message %q should list only the missing field", v.Message)
			}
		})
		t.Run(ct+"/missing fields are listed once per run", func(t *testing.T) {
			base, res := mk()
			res.Suite, res.Complete = "", nil
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
			vlMustMention(t, v.Message, `"res"`, vlAgent, "suite_sha256", "complete")
		})
		t.Run(ct+"/no prompt_sha256", func(t *testing.T) {
			base, res := mk()
			res.Prompt = ""
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
			vlMustMention(t, v.Message, `"res"`, vlAgent, "prompt_sha256")
		})
		t.Run(ct+"/both runs are reported", func(t *testing.T) {
			base, res := mk()
			base.Suite, res.Suite = "", ""
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			vs := vlViolations(t, err)
			if got := vlRules(vs); len(got) != 2 || got[0] != "run-data" || got[1] != "run-data" {
				t.Fatalf("expected two run-data violations, got %v", vs)
			}
			vlMustMention(t, vs[0].Message+vs[1].Message, `"base"`, `"res"`)
		})
	}
}

func TestVerifyLogRunComplete(t *testing.T) {
	for _, ct := range []string{"prompt", "eval"} {
		t.Run(ct, func(t *testing.T) {
			base, res := vlBaseRes()
			if ct == "eval" {
				res.Prompt = base.Prompt
				res.Suite = vlSuiteS2
			}
			res.Complete, res.CaseFilter = vlBool(false), "stub-basic"
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-complete", "iteration-00.md")
			vlMustMention(t, v.Message, `"res"`, vlAgent, "complete=false", "stub-basic")
		})
	}
	t.Run("both runs are reported", func(t *testing.T) {
		base, res := vlBaseRes()
		base.Complete, res.Complete = vlBool(false), vlBool(false)
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		vs := vlViolations(t, err)
		if got := vlRules(vs); len(got) != 2 || got[0] != "run-complete" || got[1] != "run-complete" {
			t.Fatalf("expected two run-complete violations, got %v", vs)
		}
	})
	t.Run("an incomplete run with no case filter still fails", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Complete = vlBool(false)
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		vlExpectOnly(t, err, "run-complete", "iteration-00.md")
	})
}

func TestVerifyLogRunConfig(t *testing.T) {
	// eval iterations compare agent_model and sandbox only; they must also
	// keep the suite changed and the prompt equal to be otherwise valid.
	evalPair := func(mutate func(base, res *vlRunSpec)) (vlRunSpec, vlRunSpec) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		res.Suite = vlSuiteS2
		mutate(&base, &res)
		return base, res
	}
	promptPair := func(mutate func(base, res *vlRunSpec)) (vlRunSpec, vlRunSpec) {
		base, res := vlBaseRes()
		mutate(&base, &res)
		return base, res
	}
	for _, ct := range []string{"prompt", "eval"} {
		pair := promptPair
		if ct == "eval" {
			pair = evalPair
		}
		t.Run(ct+"/agent_model differs", func(t *testing.T) {
			base, res := pair(func(b, r *vlRunSpec) { r.AgentModel = "other-agent" })
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
			vlMustMention(t, v.Message, "agent_model", "agent-m", "other-agent")
		})
		t.Run(ct+"/sandbox differs", func(t *testing.T) {
			base, res := pair(func(b, r *vlRunSpec) { r.Sandbox = "container" })
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
			vlMustMention(t, v.Message, "sandbox", "native", "container")
		})
		t.Run(ct+"/not-recorded vs recorded agent_model is a mismatch", func(t *testing.T) {
			base, res := pair(func(b, r *vlRunSpec) { r.AgentModel = "" })
			_, err := vlRunVerify(t, vlPairOpts(t, ct, base, res))
			v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
			vlMustMention(t, v.Message, "agent_model", "agent-m")
		})
		t.Run(ct+"/not-recorded vs not-recorded agent_model and sandbox are equal", func(t *testing.T) {
			base, res := pair(func(b, r *vlRunSpec) {
				b.AgentModel, r.AgentModel = "", ""
				b.Sandbox, r.Sandbox = "", ""
			})
			if _, err := vlRunVerify(t, vlPairOpts(t, ct, base, res)); err != nil {
				t.Fatalf("not-recorded on both sides should be equal: %v", err)
			}
		})
	}

	t.Run("prompt/judge_model differs", func(t *testing.T) {
		base, res := promptPair(func(b, r *vlRunSpec) { r.JudgeModel = "other-judge" })
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
		vlMustMention(t, v.Message, "judge_model", "judge-m", "other-judge")
	})
	t.Run("prompt/not-recorded vs recorded judge_model is a mismatch", func(t *testing.T) {
		base, res := promptPair(func(b, r *vlRunSpec) { b.JudgeModel = "" })
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
		vlMustMention(t, v.Message, "judge_model", "judge-m")
	})
	t.Run("prompt/not-recorded vs not-recorded judge_model is equal", func(t *testing.T) {
		base, res := promptPair(func(b, r *vlRunSpec) { b.JudgeModel, r.JudgeModel = "", "" })
		if _, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("eval/judge_model differing is not a run-config violation", func(t *testing.T) {
		base, res := evalPair(func(b, r *vlRunSpec) { r.JudgeModel = "other-judge" })
		if _, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyLogSuiteChange(t *testing.T) {
	t.Run("prompt iteration with a different suite fails", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Suite = vlSuiteS2
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		v := vlExpectOnly(t, err, "suite-change", "iteration-00.md")
		vlMustMention(t, v.Message, vlSuiteS1, vlSuiteS2)
	})
	t.Run("prompt iteration with the same suite passes", func(t *testing.T) {
		base, res := vlBaseRes()
		if _, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("eval iteration, same prompt, same suite, same judge fails", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		_, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res))
		v := vlExpectOnly(t, err, "suite-change", "iteration-00.md")
		vlMustMention(t, v.Message, vlSuiteS1, "judge-m")
	})
	t.Run("eval iteration with nothing recorded as judge and the same suite fails", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		base.JudgeModel, res.JudgeModel = "", ""
		_, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res))
		vlExpectOnly(t, err, "suite-change", "iteration-00.md")
	})
	t.Run("eval iteration changing only the suite passes", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		res.Suite = vlSuiteS2
		if _, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("eval iteration changing only the judge passes", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		res.JudgeModel = "other-judge"
		if _, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("eval iteration with a recorded judge vs not-recorded judge counts as changed", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		res.JudgeModel = ""
		if _, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyLogEvalPromptChange(t *testing.T) {
	base, res := vlBaseRes()
	res.Suite = vlSuiteS2 // suite changed, so prompt-change is the only problem
	_, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res))
	v := vlExpectOnly(t, err, "prompt-change", "iteration-00.md")
	vlMustMention(t, v.Message, "eval", vlShaA, vlShaB)
}

func TestVerifyLogPairwiseRulesDoNotCascadeOnMissingFacts(t *testing.T) {
	t.Run("prompt iteration, no suite on one run: only run-data", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Suite = ""
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		vlExpectOnly(t, err, "run-data", "iteration-00.md")
	})
	t.Run("prompt iteration, no prompt on one run: only run-data", func(t *testing.T) {
		base, res := vlBaseRes()
		base.Prompt = ""
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		vlExpectOnly(t, err, "run-data", "iteration-00.md")
	})
	t.Run("prompt iteration, same prompt but no suite: prompt-change and run-data only", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		res.Suite = ""
		_, err := vlRunVerify(t, vlPairOpts(t, "prompt", base, res))
		if got := vlRules(vlViolations(t, err)); len(got) != 2 || got[0] != "run-data" || got[1] != "prompt-change" {
			t.Fatalf("rules = %v, want [run-data prompt-change]", got)
		}
	})
	t.Run("eval iteration, no suite on both runs: only run-data x2", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = base.Prompt
		base.Suite, res.Suite = "", ""
		_, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res))
		if got := vlRules(vlViolations(t, err)); len(got) != 2 || got[0] != "run-data" || got[1] != "run-data" {
			t.Fatalf("rules = %v, want two run-data", got)
		}
	})
	t.Run("eval iteration, no prompt on one run: no prompt-change", func(t *testing.T) {
		base, res := vlBaseRes()
		res.Prompt = ""
		res.Suite = vlSuiteS2
		_, err := vlRunVerify(t, vlPairOpts(t, "eval", base, res))
		vlExpectOnly(t, err, "run-data", "iteration-00.md")
	})
	t.Run("a missing run reports run-exists only", func(t *testing.T) {
		base, res := vlBaseRes()
		opts := vlPairOpts(t, "prompt", base, res)
		if err := os.RemoveAll(filepath.Join(opts.EvalsDir, "results", "res")); err != nil {
			t.Fatal(err)
		}
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "run-exists", "iteration-00.md")
	})
}

func TestVerifyLogFieldwisePrecedence(t *testing.T) {
	// vlPairOpts-like setup where the two files of a run differ.
	setup := func(t *testing.T, ct string, base vlRunSpec, resSummary, resAgent *vlRunSpec, claimed string) VerifyLogOptions {
		t.Helper()
		root := t.TempDir()
		evalsDir := filepath.Join(root, "evals")
		vlWriteSpecRun(t, evalsDir, "base", base)
		var sum, ag string
		if resSummary != nil {
			sum = resSummary.summaryJSON()
		}
		if resAgent != nil {
			ag = resAgent.agentJSON()
		}
		vlWriteRun(t, evalsDir, "res", sum, ag)
		fm := vlFM(0, "2026-07-01", ct, "base", "res", fmt.Sprintf("%.1f", base.Score), claimed)
		vlWriteFile(t, filepath.Join(root, "iterations", vlAgent, "iteration-00.md"), vlDoc(fm, vlBody))
		return VerifyLogOptions{IterationsDir: filepath.Join(root, "iterations"), EvalsDir: evalsDir}
	}
	base, res := vlBaseRes()
	scoreOnly := vlRunSpec{Score: res.Score} // summary.json that records only the verdict

	t.Run("agent json wins over summary", func(t *testing.T) {
		// summary says suite S2 / container / other models; agent json says
		// the standard facts, so the iteration is consistent with base.
		sum := res
		sum.Suite, sum.Sandbox, sum.AgentModel, sum.JudgeModel = vlSuiteS2, "container", "x", "y"
		sum.Complete, sum.CaseFilter = vlBool(false), "f"
		if _, err := vlRunVerify(t, setup(t, "prompt", base, &sum, &res, "72.5")); err != nil {
			t.Fatalf("agent json facts should win: %v", err)
		}
	})
	t.Run("summary is the fallback", func(t *testing.T) {
		if _, err := vlRunVerify(t, setup(t, "prompt", base, &res, nil, "72.5")); err != nil {
			t.Fatalf("summary-only facts should be used: %v", err)
		}
	})
	t.Run("fallback is field by field", func(t *testing.T) {
		// agent json records only the suite and prompt; everything else
		// comes from the summary.
		agentOnly := vlRunSpec{Prompt: res.Prompt, Suite: res.Suite}
		if _, err := vlRunVerify(t, setup(t, "prompt", base, &res, &agentOnly, "72.5")); err != nil {
			t.Fatalf("per-field fallback should complete the facts: %v", err)
		}
	})
	t.Run("agent json complete:false wins over summary complete:true", func(t *testing.T) {
		ag := res
		ag.Complete, ag.CaseFilter = vlBool(false), "stub-basic"
		_, err := vlRunVerify(t, setup(t, "prompt", base, &res, &ag, "72.5"))
		vlExpectOnly(t, err, "run-complete", "iteration-00.md")
	})
	t.Run("sandbox bool in agent json equals string in summary", func(t *testing.T) {
		// base records sandbox only as summary string "container"; result
		// records it only as agent json bool true.
		b := base
		b.Sandbox = "container"
		r := res
		r.Sandbox = "container"
		only := vlRunSpec{Prompt: r.Prompt, Suite: r.Suite, AgentModel: r.AgentModel, JudgeModel: r.JudgeModel, Sandbox: r.Sandbox, Complete: r.Complete}
		if _, err := vlRunVerify(t, setup(t, "prompt", b, &scoreOnly, &only, "72.5")); err != nil {
			t.Fatalf("bool and string sandbox forms should normalise equal: %v", err)
		}
	})
	t.Run("sandbox native bool differs from container string", func(t *testing.T) {
		agentNative := res // agent json sandbox=false => native
		b := base
		b.Sandbox = "container"
		_, err := vlRunVerify(t, setup(t, "prompt", b, &scoreOnly, &agentNative, "72.5"))
		v := vlExpectOnly(t, err, "run-config", "iteration-00.md")
		vlMustMention(t, v.Message, "sandbox")
	})
}

func TestVerifyLogRuleOrderWithinAFile(t *testing.T) {
	base, res := vlBaseRes()
	base.Complete = nil                               // run-data (baseline)
	res.Complete, res.CaseFilter = vlBool(false), "x" // run-complete
	res.AgentModel = "other"                          // run-config
	res.Prompt = base.Prompt                          // prompt-change
	res.Suite = vlSuiteS2                             // suite-change
	opts := vlPairOpts(t, "prompt", base, res)
	// Claim a wrong result score so score-match fires too.
	vlWriteFile(t, filepath.Join(opts.IterationsDir, vlAgent, "iteration-00.md"),
		vlDoc(vlFM(0, "2026-07-01", "prompt", "base", "res", "60.0", "50.0"), vlBody))
	_, err := vlRunVerify(t, opts)
	want := []string{"run-data", "run-complete", "run-config", "score-match", "prompt-change", "suite-change"}
	got := vlRules(vlViolations(t, err))
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

// ---------- front-matter validation ----------

func TestVerifyLogFrontMatterViolations(t *testing.T) {
	good := vlGoodFM()
	tests := []struct {
		name    string
		content string
		wantMsg string // substring of message
	}{
		{"no opening delimiter", good + "\n" + vlBody, ""},
		{"no closing delimiter", "---\n" + good + "\n" + vlBody, ""},
		{"empty file", "", ""},
		{"invalid yaml", "---\nkey: [unclosed\n---\n\n" + vlBody, ""},
		{"missing iteration", vlDoc(vlWithout(good, "iteration"), vlBody), "iteration"},
		{"missing date", vlDoc(vlWithout(good, "date"), vlBody), "date"},
		{"missing change_type", vlDoc(vlWithout(good, "change_type"), vlBody), "change_type"},
		{"missing baseline_run", vlDoc(vlWithout(good, "baseline_run"), vlBody), "baseline_run"},
		{"missing result_run", vlDoc(vlWithout(good, "result_run"), vlBody), "result_run"},
		{"missing baseline_score", vlDoc(vlWithout(good, "baseline_score"), vlBody), "baseline_score"},
		{"missing result_score", vlDoc(vlWithout(good, "result_score"), vlBody), "result_score"},
		{"bad change_type", vlDoc(vlFM(0, "2026-07-01", "tweak", "r0", "r1", "60.0", "72.5"), vlBody), "change_type"},
		{"two-decimal score", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "97.25"), vlBody), "result_score"},
		{"integer score", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "85", "72.5"), vlBody), "baseline_score"},
		{"score above 100", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "101.0"), vlBody), "result_score"},
		{"fraction-looking score", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "0.725"), vlBody), "result_score"},
		{"negative score", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "-1.0", "72.5"), vlBody), "baseline_score"},
		{"non-calendar date", vlDoc(vlFM(0, "2026-02-30", "prompt", "r0", "r1", "60.0", "72.5"), vlBody), "date"},
		{"malformed date", vlDoc(vlFM(0, "07/01/2026", "prompt", "r0", "r1", "60.0", "72.5"), vlBody), "date"},
		{"non-numeric iteration", vlDoc(strings.Replace(good, "iteration: 0", "iteration: abc", 1), vlBody), "iteration"},
		{"run name with slash", vlDoc(vlFM(0, "2026-07-01", "prompt", "a/b", "r1", "60.0", "72.5"), vlBody), "baseline_run"},
		{"run name with backslash", vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", `a\\b`, "60.0", "72.5"), vlBody), "result_run"},
		{"run name dotdot", vlDoc(vlFM(0, "2026-07-01", "prompt", "..", "r1", "60.0", "72.5"), vlBody), "baseline_run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{"iteration-00.md": tt.content})
			_, err := vlRunVerify(t, opts)
			vs := vlViolations(t, err)
			var fm []LogViolation
			for _, v := range vs {
				if v.Rule == "front-matter" {
					fm = append(fm, v)
				}
			}
			if len(fm) == 0 {
				t.Fatalf("expected a front-matter violation, got %v", vs)
			}
			if !strings.HasSuffix(fm[0].File, "iteration-00.md") {
				t.Errorf("File = %q", fm[0].File)
			}
			if tt.wantMsg != "" && !strings.Contains(fm[0].Message, tt.wantMsg) {
				t.Errorf("message %q should mention %q", fm[0].Message, tt.wantMsg)
			}
			// A file with unusable front-matter must not also trip checks that
			// need its fields.
			for _, v := range vs {
				switch v.Rule {
				case "score-match", "run-exists", "run-data", "prompt-change", "chain", "chronology":
					t.Errorf("unexpected cross-file violation for unusable front-matter: %+v", v)
				}
			}
		})
	}
}

func TestVerifyLogFrontMatterBoundaryScoresAccepted(t *testing.T) {
	root := t.TempDir()
	evalsDir := filepath.Join(root, "evals")
	vlWriteRun(t, evalsDir, "lo", vlVerdictSummary(0.0, vlShaA), vlAgentJSON(vlShaA))
	vlWriteRun(t, evalsDir, "hi", vlVerdictSummary(100.0, vlShaB), vlAgentJSON(vlShaB))
	vlWriteFile(t, filepath.Join(root, "it", vlAgent, "iteration-00.md"),
		vlDoc(vlFM(0, "2024-02-29", "prompt", "lo", "hi", "0.0", "100.0"), vlBody))
	if _, err := vlRunVerify(t, VerifyLogOptions{IterationsDir: filepath.Join(root, "it"), EvalsDir: evalsDir}); err != nil {
		t.Fatalf("0.0 / 100.0 / leap day should pass: %v", err)
	}
}

func TestVerifyLogExtraFrontMatterKeysIgnored(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlGoodFM()+"author: someone\ntags: [a, b]\n", vlBody),
	})
	if _, err := vlRunVerify(t, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------- headings ----------

func TestVerifyLogHeadingViolations(t *testing.T) {
	section := func(name, content string) string { return "## " + name + "\n\n" + content + "\n\n" }
	bodyOf := func(parts ...string) string { return strings.Join(parts, "") }
	b, h, c, r, rs := section("Baseline", "b"), section("Hypothesis", "h"), section("Change", "c"), section("Results", "r"), section("Reasoning", "x")

	tests := []struct {
		name    string
		body    string
		wantErr string // substring of message; "" => must pass
	}{
		{"all present", bodyOf(b, h, c, r, rs), ""},
		{"extra headings allowed", bodyOf(b, h, "## Notes\n\nextra\n\n", c, r, rs, "## Appendix\n\nmore\n"), ""},
		{"trailing spaces on heading allowed", strings.Replace(bodyOf(b, h, c, r, rs), "## Change", "## Change  ", 1), ""},
		{
			// Up to 3 leading spaces is still a valid CommonMark ATX heading.
			"indented heading allowed",
			strings.Replace(bodyOf(b, h, c, r, rs), "## Change", "   ## Change", 1), "",
		},
		{
			// Closed-ATX form: the trailing run of # is not part of the name.
			"closed-atx heading allowed",
			strings.Replace(bodyOf(b, h, c, r, rs), "## Change", "## Change ##", 1), "",
		},
		{"missing Baseline", bodyOf(h, c, r, rs), "Baseline"},
		{"missing Reasoning", bodyOf(b, h, c, r), "Reasoning"},
		{"duplicated heading", bodyOf(b, h, c, c, r, rs), "Change"},
		{"out of order", bodyOf(b, c, h, r, rs), "order"},
		{"empty section", bodyOf(b, h, "## Change\n\n", r, rs), "Change"},
		{"whitespace-only section", bodyOf(b, h, "## Change\n   \n\t\n", r, rs), "Change"},
		{"empty last section", bodyOf(b, h, c, r, "## Reasoning\n"), "Reasoning"},
		{"wrong level", strings.Replace(bodyOf(b, h, c, r, rs), "## Results", "### Results", 1), "Results"},
		{
			// A heading inside a fenced code block is not a heading; the real
			// Hypothesis section is missing so this must fail.
			"heading only inside fence",
			bodyOf(b, "## Change\n\nc\n\n", r, rs, "```\n## Hypothesis\n\nfake\n```\n"),
			"Hypothesis",
		},
		{
			// Fenced fake heading must not count as a duplicate.
			"fenced duplicate ignored",
			bodyOf(b, h, c, r, rs, "```\n## Change\nnot a heading\n```\n"),
			"",
		},
		{
			// Fenced content counts as content for the section it is in.
			"fence counts as content",
			bodyOf(b, h, c, r, "## Reasoning\n\n```\ncode\n```\n"),
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{"iteration-00.md": vlDoc(vlGoodFM(), tt.body)})
			_, err := vlRunVerify(t, opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			v := vlExpectOnly(t, err, "headings", "iteration-00.md")
			if !strings.Contains(v.Message, tt.wantErr) {
				t.Errorf("message %q should mention %q", v.Message, tt.wantErr)
			}
		})
	}
}

// ---------- sequence, chain, chronology ----------

func TestVerifyLogSequenceViolations(t *testing.T) {
	doc := func(n int, base, res string, date string) string {
		return vlDoc(vlFM(n, date, "eval", base, res, "60.0", "60.0"), vlBody)
	}
	// r0 and r0s share a prompt and score but differ in suite, so r0 -> r0s
	// and r0s -> r0 are valid eval iterations that chain into each other.
	t.Run("does not start at 00", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{"iteration-01.md": doc(1, "r0", "r0s", "2026-07-01")})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-01.md")
	})
	t.Run("duplicate numbers 00 and 000", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md":  doc(0, "r0", "r0s", "2026-07-01"),
			"iteration-000.md": doc(0, "r0s", "r0", "2026-07-01"),
		})
		_, err := vlRunVerify(t, opts)
		vs := vlViolations(t, err)
		seq := 0
		for _, v := range vs {
			if v.Rule == "sequence" {
				seq++
			}
		}
		if seq == 0 {
			t.Fatalf("expected a sequence violation for duplicates, got %v", vs)
		}
	})
	t.Run("front-matter iteration differs from file number", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{"iteration-00.md": doc(3, "r0", "r0s", "2026-07-01")})
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "sequence", "iteration-00.md")
		if !strings.Contains(v.Message, "3") {
			t.Errorf("message %q should name the front-matter value", v.Message)
		}
	})
	t.Run("misnamed iteration-1.md", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": doc(0, "r0", "r0s", "2026-07-01"),
			"iteration-1.md":  doc(1, "r0s", "r0", "2026-07-01"),
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-1.md")
	})
	t.Run("misnamed iteration-ab.md", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md":   doc(0, "r0", "r0s", "2026-07-01"),
			"iteration-ab.md":   "junk",
			"iteration-01.md.x": "ignored",
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-ab.md")
	})
	t.Run("gap flags only the first out-of-place file", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": doc(0, "r0", "r0s", "2026-07-01"),
			"iteration-02.md": doc(2, "r0s", "r0", "2026-07-01"),
			"iteration-03.md": doc(3, "r0", "r0s", "2026-07-01"),
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-02.md")
	})
	t.Run("three digit numbers accepted when contiguous", func(t *testing.T) {
		files := map[string]string{}
		for i := 0; i < 3; i++ {
			base, res := "r0", "r0s"
			if i%2 == 1 {
				base, res = "r0s", "r0"
			}
			files[fmt.Sprintf("iteration-%02d.md", i)] = doc(i, base, res, "2026-07-01")
		}
		opts := vlSetup(t, files)
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyLogChain(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
		"iteration-01.md": vlDoc(vlFM(1, "2026-07-02", "prompt", "r0", "r2", "60.0", "90.0"), vlBody),
	})
	_, err := vlRunVerify(t, opts)
	v := vlExpectOnly(t, err, "chain", "iteration-01.md")
	if !strings.Contains(v.Message, "r1") || !strings.Contains(v.Message, "r0") {
		t.Errorf("message %q should name both runs", v.Message)
	}
}

func TestVerifyLogChronology(t *testing.T) {
	mk := func(secondDate string) VerifyLogOptions {
		return vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-05", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
			"iteration-01.md": vlDoc(vlFM(1, secondDate, "prompt", "r1", "r2", "72.5", "90.0"), vlBody),
		})
	}
	t.Run("earlier date fails", func(t *testing.T) {
		_, err := vlRunVerify(t, mk("2026-07-04"))
		vlExpectOnly(t, err, "chronology", "iteration-01.md")
	})
	t.Run("same date passes", func(t *testing.T) {
		if _, err := vlRunVerify(t, mk("2026-07-05")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("later date passes", func(t *testing.T) {
		if _, err := vlRunVerify(t, mk("2026-07-06")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// ---------- run existence / data ----------

func TestVerifyLogRunExists(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "nope-a", "nope-b", "60.0", "72.5"), vlBody),
	})
	_, err := vlRunVerify(t, opts)
	vs := vlViolations(t, err)
	if got := vlRules(vs); len(got) != 2 || got[0] != "run-exists" || got[1] != "run-exists" {
		t.Fatalf("expected two run-exists violations, got %v", vs)
	}
	// A run that is a file, not a directory, is not a run.
	opts = vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r0", "afile", "60.0", "72.5"), vlBody),
	})
	vlWriteFile(t, filepath.Join(opts.EvalsDir, "results", "afile"), "not a dir")
	_, err = vlRunVerify(t, opts)
	vlExpectOnly(t, err, "run-exists", "iteration-00.md")
}

func TestVerifyLogDefaultsAreUsed(t *testing.T) {
	// IterationsDir/EvalsDir "" resolve relative to the cwd: run in a TempDir.
	root := t.TempDir()
	vlStdRuns(t, filepath.Join(root, ".kairon", "evals"))
	vlWriteFile(t, filepath.Join(root, ".kairon", "iterations", vlAgent, "iteration-00.md"),
		vlDoc(vlGoodFM(), vlBody))
	t.Chdir(root)
	out, err := vlRunVerify(t, VerifyLogOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "OK selftest: 1 iterations verified\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

// ---------- no-logs, min-iterations, options ----------

func TestVerifyLogNoLogs(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		opts := vlSetup(t, nil)
		opts.IterationsDir = filepath.Join(t.TempDir(), "absent")
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", "")
	})
	t.Run("empty directory", func(t *testing.T) {
		opts := vlSetup(t, nil)
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", vlAgent)
	})
	t.Run("only non-log files", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{"v2.md": "candidate", "notes.txt": "n"})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", vlAgent)
	})
	t.Run("reported even when min-iterations is 0", func(t *testing.T) {
		opts := vlSetup(t, nil)
		opts.MinIterations = 0
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", "")
	})
}

func TestVerifyLogMinIterations(t *testing.T) {
	logs := map[string]string{"iteration-00.md": vlDoc(vlGoodFM(), vlBody)}
	t.Run("below minimum", func(t *testing.T) {
		opts := vlSetup(t, logs)
		opts.MinIterations = 2
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "min-iterations", vlAgent)
		if !strings.Contains(v.Message, "1") || !strings.Contains(v.Message, "2") {
			t.Errorf("message %q should name found and required counts", v.Message)
		}
	})
	t.Run("equal to minimum passes", func(t *testing.T) {
		opts := vlSetup(t, logs)
		opts.MinIterations = 1
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("zero disables the minimum", func(t *testing.T) {
		opts := vlSetup(t, logs)
		opts.MinIterations = 0
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("negative is an error, not a violation", func(t *testing.T) {
		opts := vlSetup(t, logs)
		opts.MinIterations = -1
		_, err := vlRunVerify(t, opts)
		if err == nil {
			t.Fatal("expected an error")
		}
		var ve *VerifyLogError
		if errors.As(err, &ve) {
			t.Fatalf("negative min should be a plain error, got violations %v", ve.Violations)
		}
	})
}

func TestVerifyLogIgnoresNonLogFiles(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlGoodFM(), vlBody),
		"v2.md":           "candidate prompt, not a log",
		"notes.md":        "# notes",
		"README.md":       "readme",
	})
	if err := os.MkdirAll(filepath.Join(opts.IterationsDir, vlAgent, "iteration-05.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := vlRunVerify(t, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "OK selftest: 1 iterations verified\n" {
		t.Fatalf("output = %q", out)
	}
}

// ---------- path traversal ----------

func TestVerifyLogRejectsBadAgentNames(t *testing.T) {
	opts := vlSetup(t, nil)
	for _, agent := range []string{"", "..", "../x", "a/b", `a\b`, "x/../y", "/abs"} {
		t.Run(fmt.Sprintf("%q", agent), func(t *testing.T) {
			var buf bytes.Buffer
			err := VerifyLog(&buf, agent, opts)
			if err == nil {
				t.Fatal("expected an error")
			}
			var ve *VerifyLogError
			if errors.As(err, &ve) {
				t.Fatalf("a bad agent name is a usage error, not a log violation: %v", err)
			}
			if buf.Len() != 0 {
				t.Errorf("nothing should be printed, got %q", buf.String())
			}
		})
	}
}

func TestVerifyLogRejectsRunNamesOutsideResults(t *testing.T) {
	for _, run := range []string{"../outside", "..", "a/b", `a\b`, "r0/../r1"} {
		t.Run(run, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", run, "r1", "60.0", "72.5"), vlBody),
			})
			// A perfectly good run just outside results/ that a traversal
			// would reach: it must never be read.
			vlWriteFile(t, filepath.Join(opts.EvalsDir, "outside", "summary.json"), vlVerdictSummary(60.0, vlShaA))
			vlWriteFile(t, filepath.Join(opts.EvalsDir, "outside", vlAgent+".json"), vlAgentJSON(vlShaA))

			_, err := vlRunVerify(t, opts)
			vs := vlViolations(t, err)
			if len(vs) != 1 || vs[0].Rule != "front-matter" {
				t.Fatalf("expected a single front-matter violation, got %v", vs)
			}
			if !strings.Contains(vs[0].Message, "baseline_run") {
				t.Errorf("message %q should name baseline_run", vs[0].Message)
			}
		})
	}
}

func TestVerifyLogDoesNotFollowRunNameOutOfResults(t *testing.T) {
	// The result_run escape is rejected too, and a score file outside results
	// never satisfies score-match.
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "../outside", "60.0", "50.0"), vlBody),
	})
	vlWriteFile(t, filepath.Join(opts.EvalsDir, "outside", "summary.json"), vlVerdictSummary(50.0, vlShaB))
	_, err := vlRunVerify(t, opts)
	vs := vlViolations(t, err)
	if len(vs) != 1 || vs[0].Rule != "front-matter" || !strings.Contains(vs[0].Message, "result_run") {
		t.Fatalf("expected one front-matter violation naming result_run, got %v", vs)
	}
}

// ---------- multiple violations ----------

func TestVerifyLogReportsAllViolations(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		// 00: score mismatch on result (72.5 recorded, 80.0 claimed) + missing section.
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-05", "prompt", "r0", "r1", "60.0", "80.0"),
			strings.Replace(vlBody, "## Reasoning\n\nThe result supports the hypothesis.\n", "", 1)),
		// 01: broken chain (baseline r0 != r1), earlier date, same-prompt prompt change (r2 -> r2b).
		"iteration-01.md": vlDoc(vlFM(1, "2026-07-01", "prompt", "r2", "r2b", "90.0", "91.0"), vlBody),
		// 03: gap.
		"iteration-03.md": vlDoc(vlFM(3, "2026-07-06", "eval", "gone", "r2b", "60.0", "91.0"), vlBody),
	})
	opts.MinIterations = 5
	_, err := vlRunVerify(t, opts)
	vs := vlViolations(t, err)

	has := func(rule, suffix string) bool {
		for _, v := range vs {
			if v.Rule == rule && strings.HasSuffix(v.File, suffix) {
				return true
			}
		}
		return false
	}
	for _, w := range []struct{ rule, suffix string }{
		{"score-match", "iteration-00.md"},
		{"headings", "iteration-00.md"},
		{"chain", "iteration-01.md"},
		{"chronology", "iteration-01.md"},
		{"prompt-change", "iteration-01.md"},
		{"sequence", "iteration-03.md"},
		{"run-exists", "iteration-03.md"},
		{"min-iterations", vlAgent},
	} {
		if !has(w.rule, w.suffix) {
			t.Errorf("missing violation %s on %s; got %v", w.rule, w.suffix, vs)
		}
	}

	lines := strings.Split(err.Error(), "\n")
	if len(lines) != len(vs) {
		t.Fatalf("Error() has %d lines for %d violations:\n%s", len(lines), len(vs), err.Error())
	}
	for i, v := range vs {
		want := fmt.Sprintf("%s: %s: %s", v.File, v.Rule, v.Message)
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
		if strings.Contains(v.Message, "\n") {
			t.Errorf("violation message must be a single line: %q", v.Message)
		}
	}

	// Output is sorted by file.
	for i := 1; i < len(vs); i++ {
		if vs[i-1].File > vs[i].File {
			t.Errorf("violations not sorted by file: %q before %q", vs[i-1].File, vs[i].File)
		}
	}
}

// ---------- output ----------

func TestVerifyLogOutputCountsIterations(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
		"iteration-01.md": vlDoc(vlFM(1, "2026-07-02", "prompt", "r1", "r2", "72.5", "90.0"), vlBody),
	})
	out, err := vlRunVerify(t, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "OK selftest: 2 iterations verified\n" {
		t.Fatalf("output = %q", out)
	}
}

// ensure the helper JSON builders produce valid JSON (guards test fixtures).
func TestVerifyLogTestHelpersProduceValidJSON(t *testing.T) {
	for _, s := range []string{vlVerdictSummary(72.5, vlShaB), vlAgentJSON(vlShaB)} {
		var v map[string]any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// ---------- phase ----------

// vlPhaseViolations verifies a set of logs and returns the violations (nil on
// success).
func vlPhaseViolations(t *testing.T, logs map[string]string) []LogViolation {
	t.Helper()
	_, err := vlRunVerify(t, vlSetup(t, logs))
	if err == nil {
		return nil
	}
	return vlViolations(t, err)
}

func TestVerifyLogPhaseMissing(t *testing.T) {
	fm := vlFMPhase("", 0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5")
	_, err := vlRunVerify(t, vlSetup(t, map[string]string{"iteration-00.md": vlDoc(fm, vlBody)}))
	v := vlExpectOnly(t, err, "phase", "iteration-00.md")
	vlMustMention(t, v.Message, "phase", "baseline", "red", "green", "refactor")
}

func TestVerifyLogPhaseUnknownValue(t *testing.T) {
	for _, bad := range []string{"tweak", "Red", "3", `""`} {
		t.Run(bad, func(t *testing.T) {
			fm := vlFMPhase(bad, 0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5")
			_, err := vlRunVerify(t, vlSetup(t, map[string]string{"iteration-00.md": vlDoc(fm, vlBody)}))
			v := vlExpectOnly(t, err, "phase", "iteration-00.md")
			vlMustMention(t, v.Message, "phase", "baseline", "red", "green", "refactor")
		})
	}
}

func TestVerifyLogPhaseAllowedValues(t *testing.T) {
	// baseline in 00, then each of red/green/refactor in a later file.
	for _, ph := range []string{"red", "green", "refactor"} {
		t.Run(ph, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
				"iteration-01.md": vlDoc(vlFMPhase(ph, 1, "2026-07-02", "prompt", "r1", "r2", "72.5", "90.0"), vlBody),
			})
			// Whether the phase's evidence holds is not this test's concern: only
			// the phase key itself must be accepted.
			_, err := vlRunVerify(t, opts)
			var ve *VerifyLogError
			if errors.As(err, &ve) {
				for _, v := range ve.Violations {
					if v.Rule == "phase" {
						t.Fatalf("phase %q rejected: %v", ph, v)
					}
				}
			}
		})
	}
}

func TestVerifyLogPhaseBaselineOnlyInIterationZero(t *testing.T) {
	// baseline is fine in iteration-00.md ...
	fm0 := vlFMPhase("baseline", 0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5")
	if _, err := vlRunVerify(t, vlSetup(t, map[string]string{"iteration-00.md": vlDoc(fm0, vlBody)})); err != nil {
		t.Fatalf("baseline in iteration-00.md rejected: %v", err)
	}
	// ... but not in a later file, and the violation names that file.
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(fm0, vlBody),
		"iteration-01.md": vlDoc(vlFMPhase("baseline", 1, "2026-07-02", "prompt", "r1", "r2", "72.5", "90.0"), vlBody),
	})
	_, err := vlRunVerify(t, opts)
	v := vlExpectOnly(t, err, "phase", "iteration-01.md")
	vlMustMention(t, v.Message, "baseline", "iteration-00.md")
}

// A bad phase must not stop the chain, run or score checks of the same file,
// and must not set the front-matter rule.
func TestVerifyLogPhaseViolationDoesNotSuppressOtherChecks(t *testing.T) {
	opts := vlSetup(t, map[string]string{
		"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
		// no phase; broken chain (baseline r0 != r1); wrong claimed score; run r2b
		// has the same prompt as r2 so prompt-change fires too.
		"iteration-01.md": vlDoc(vlFMPhase("", 1, "2026-07-02", "prompt", "r2", "r2b", "90.0", "50.0"), vlBody),
	})
	_, err := vlRunVerify(t, opts)
	var got []string
	for _, v := range vlViolations(t, err) {
		if !strings.HasSuffix(v.File, "iteration-01.md") {
			t.Errorf("unexpected violation on another file: %+v", v)
			continue
		}
		got = append(got, v.Rule)
	}
	want := []string{"chain", "score-match", "prompt-change", "phase"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rules on iteration-01.md = %v, want %v", got, want)
	}
}

func TestVerifyLogPhaseDoesNotMaskFrontMatterViolations(t *testing.T) {
	// A log with another bad key and no phase reports both rules.
	fm := vlWithout(vlFMPhase("", 0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), "date")
	vs := vlPhaseViolations(t, map[string]string{"iteration-00.md": vlDoc(fm, vlBody)})
	if got := vlRules(vs); fmt.Sprint(got) != fmt.Sprint([]string{"front-matter", "phase"}) {
		t.Fatalf("rules = %v, want [front-matter phase]", got)
	}
}

func TestVerifyLogPhaseNonScalarValueDoesNotSuppressOtherChecks(t *testing.T) {
	// A non-scalar phase value (a YAML list or map) is valid YAML but is not a
	// scalar string. It must be reported as its own phase violation, not abort
	// front-matter parsing with a generic front-matter error that masks the
	// file's other problems.
	for _, bad := range []string{"[red, green]", "{a: b}"} {
		t.Run(bad, func(t *testing.T) {
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5"), vlBody),
				// non-scalar phase; broken chain (baseline r2 != r1); wrong claimed
				// score; run r2b has the same prompt as r2 so prompt-change fires too.
				"iteration-01.md": vlDoc(vlFMPhase(bad, 1, "2026-07-02", "prompt", "r2", "r2b", "90.0", "50.0"), vlBody),
			})
			_, err := vlRunVerify(t, opts)
			var got []string
			for _, v := range vlViolations(t, err) {
				if !strings.HasSuffix(v.File, "iteration-01.md") {
					t.Errorf("unexpected violation on another file: %+v", v)
					continue
				}
				got = append(got, v.Rule)
			}
			want := []string{"chain", "score-match", "prompt-change", "phase"}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("rules on iteration-01.md = %v, want %v", got, want)
			}
		})
	}
}

func TestVerifyLogPhaseRuleIDsAreRegisteredInOrder(t *testing.T) {
	ids := []string{"phase", "red", "green", "refactor", "cycle"}
	prev := ruleOrder["suite-change"]
	for _, id := range ids {
		rank, ok := ruleOrder[id]
		if !ok {
			t.Errorf("rule %q is not registered in ruleOrder", id)
			continue
		}
		if rank <= prev {
			t.Errorf("rule %q has rank %d, want greater than the previous rule's %d", id, rank, prev)
		}
		prev = rank
	}
	if min := ruleOrder["min-iterations"]; prev >= min {
		t.Errorf("min-iterations rank %d must come after cycle rank %d", min, prev)
	}
}

// ---------- red / green / refactor evidence ----------

// vlPhaseOpts writes runs "base" and "res" and one iteration-00 log with the
// given phase and change type (claimed scores equal the recorded ones).
func vlPhaseOpts(t *testing.T, phase, changeType string, base, res vlRunSpec) VerifyLogOptions {
	t.Helper()
	root := t.TempDir()
	evalsDir := filepath.Join(root, "evals")
	vlWriteSpecRun(t, evalsDir, "base", base)
	vlWriteSpecRun(t, evalsDir, "res", res)
	fm := vlFMPhase(phase, 0, "2026-07-01", changeType, "base", "res", fmt.Sprintf("%.1f", base.Score), fmt.Sprintf("%.1f", res.Score))
	vlWriteFile(t, filepath.Join(root, "iterations", vlAgent, "iteration-00.md"), vlDoc(fm, vlBody))
	return VerifyLogOptions{IterationsDir: filepath.Join(root, "iterations"), EvalsDir: evalsDir}
}

// vlEvalPair is an eval iteration pair: same prompt, suite S1 -> S2.
func vlEvalPair(baseChecks, resChecks []vlCheck) (vlRunSpec, vlRunSpec) {
	base := vlStdSpec(70.0, vlShaB)
	res := vlStdSpec(70.0, vlShaB)
	res.Suite = vlSuiteS2
	base.Checks, res.Checks = baseChecks, resChecks
	return base, res
}

// vlPromptPair is a prompt iteration pair: prompt B -> C, same suite.
func vlPromptPair(baseScore, resScore float64, baseChecks, resChecks []vlCheck) (vlRunSpec, vlRunSpec) {
	base := vlStdSpec(baseScore, vlShaB)
	res := vlStdSpec(resScore, vlShaC)
	base.Checks, res.Checks = baseChecks, resChecks
	return base, res
}

func vlPhaseOK(t *testing.T, opts VerifyLogOptions) {
	t.Helper()
	if out, err := vlRunVerify(t, opts); err != nil {
		t.Fatalf("expected success, got: %v (out %q)", err, out)
	}
}

func TestVerifyLogRed(t *testing.T) {
	ready1 := func(passed ...bool) vlCheck { return vlChk("#1 "+vlLblReady, passed...) }
	t.Run("new failing check passes", func(t *testing.T) {
		base, res := vlEvalPair(nil, []vlCheck{ready1(false, false)})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("a check failing in one repeat only counts as failing", func(t *testing.T) {
		base, res := vlEvalPair(nil, []vlCheck{ready1(true, false)})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("a check failing in the first repeat only counts as failing", func(t *testing.T) {
		base, res := vlEvalPair(nil, []vlCheck{ready1(false, true)})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("no failing check in the result fails", func(t *testing.T) {
		base, res := vlEvalPair(nil, []vlCheck{ready1(true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "red", "eval", base, res))
		v := vlExpectOnly(t, err, "red", "iteration-00.md")
		vlMustMention(t, v.Message, "base", "res")
	})
	t.Run("no checks recorded at all fails", func(t *testing.T) {
		base, res := vlEvalPair(nil, nil)
		_, err := vlRunVerify(t, vlPhaseOpts(t, "red", "eval", base, res))
		vlExpectOnly(t, err, "red", "iteration-00.md")
	})
	t.Run("failing check already failing in the baseline fails", func(t *testing.T) {
		base, res := vlEvalPair([]vlCheck{ready1(false, false)}, []vlCheck{ready1(false, false)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "red", "eval", base, res))
		vlExpectOnly(t, err, "red", "iteration-00.md")
	})
	t.Run("baseline fails one check, result fails it and another: passes", func(t *testing.T) {
		base, res := vlEvalPair(
			[]vlCheck{ready1(false, false), vlChk("#2 "+vlLblFile, true, true)},
			[]vlCheck{ready1(false, false), vlChk("#2 "+vlLblFile, false, false)})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("change_type prompt fails", func(t *testing.T) {
		base, res := vlPromptPair(70, 70, nil, []vlCheck{ready1(false, false)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "red", "prompt", base, res))
		v := vlExpectOnly(t, err, "red", "iteration-00.md")
		vlMustMention(t, v.Message, "eval", "prompt")
	})
	t.Run("a lower score does not matter", func(t *testing.T) {
		base, res := vlEvalPair(nil, []vlCheck{ready1(false, false)})
		res.Score = 10
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
}

func TestVerifyLogCheckIdentity(t *testing.T) {
	t.Run("an index shift does not make an existing check new", func(t *testing.T) {
		// The baseline fails "#1 output_contains ..."; a check inserted before it
		// moves it to #2 in the result. It is the same check, so nothing is new.
		base, res := vlEvalPair(
			[]vlCheck{vlChk("#1 "+vlLblReady, false, false)},
			[]vlCheck{vlChk("#1 "+vlLblFile, true, true), vlChk("#2 "+vlLblReady, false, false)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "red", "eval", base, res))
		vlExpectOnly(t, err, "red", "iteration-00.md")
	})
	t.Run("a different label is a different check", func(t *testing.T) {
		base, res := vlEvalPair(
			[]vlCheck{vlChk("#1 "+vlLblReady, false, false)},
			[]vlCheck{vlChk("#1 output_contains pattern=/done/", false, false)})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("a different criterion is a different check", func(t *testing.T) {
		other := vlChk("#1 "+vlLblReady, false, false)
		other.Criterion = "beta"
		base, res := vlEvalPair([]vlCheck{vlChk("#1 "+vlLblReady, false, false)}, []vlCheck{other})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("a different case is a different check", func(t *testing.T) {
		other := vlChk("#1 "+vlLblReady, false, false)
		other.Case = "second"
		base, res := vlEvalPair([]vlCheck{vlChk("#1 "+vlLblReady, false, false)}, []vlCheck{other})
		vlPhaseOK(t, vlPhaseOpts(t, "red", "eval", base, res))
	})
	t.Run("a skipped criterion contributes nothing", func(t *testing.T) {
		// A failing check under a skipped criterion is not evidence.
		base, res := vlEvalPair(nil, []vlCheck{vlChk("#1 "+vlLblReady, false, false)})
		opts := vlPhaseOpts(t, "red", "eval", base, res)
		path := filepath.Join(opts.EvalsDir, "results", "res", vlAgent+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		patched := strings.ReplaceAll(string(data), `"deterministic":true`, `"deterministic":true,"skipped":true`)
		if patched == string(data) {
			t.Fatal("test setup: skipped flag not injected")
		}
		vlWriteFile(t, path, patched)
		_, err = vlRunVerify(t, opts)
		vlExpectOnly(t, err, "red", "iteration-00.md")
	})
}

func TestVerifyLogGreen(t *testing.T) {
	failing := func() []vlCheck { return []vlCheck{vlChk("#1 "+vlLblReady, false, false)} }
	t.Run("fix and no score drop passes", func(t *testing.T) {
		base, res := vlPromptPair(70, 85, failing(), []vlCheck{vlChk("#1 "+vlLblReady, true, true)})
		vlPhaseOK(t, vlPhaseOpts(t, "green", "prompt", base, res))
	})
	t.Run("equal score passes", func(t *testing.T) {
		base, res := vlPromptPair(70, 70, failing(), []vlCheck{vlChk("#1 "+vlLblReady, true, true)})
		vlPhaseOK(t, vlPhaseOpts(t, "green", "prompt", base, res))
	})
	t.Run("an index shift still finds the fixed check", func(t *testing.T) {
		base, res := vlPromptPair(70, 85, failing(),
			[]vlCheck{vlChk("#1 "+vlLblFile, true, true), vlChk("#2 "+vlLblReady, true, true)})
		vlPhaseOK(t, vlPhaseOpts(t, "green", "prompt", base, res))
	})
	t.Run("one fixed check among several is enough", func(t *testing.T) {
		base, res := vlPromptPair(70, 85,
			[]vlCheck{vlChk("#1 "+vlLblReady, false, false), vlChk("#2 "+vlLblFile, false, false)},
			[]vlCheck{vlChk("#1 "+vlLblReady, false, true), vlChk("#2 "+vlLblFile, true, true)})
		vlPhaseOK(t, vlPhaseOpts(t, "green", "prompt", base, res))
	})
	t.Run("still failing in one repeat is not fixed", func(t *testing.T) {
		base, res := vlPromptPair(70, 85, failing(), []vlCheck{vlChk("#1 "+vlLblReady, true, false)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "prompt", base, res))
		v := vlExpectOnly(t, err, "green", "iteration-00.md")
		vlMustMention(t, v.Message, "base", "res")
	})
	t.Run("a check missing from a repeat is not a pass", func(t *testing.T) {
		// The check is recorded in repeat 1 only; the case has two repeats
		// because another check is recorded twice.
		base, res := vlPromptPair(70, 85, failing(),
			[]vlCheck{vlChk("#1 "+vlLblReady, true), vlChk("#2 "+vlLblFile, true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "prompt", base, res))
		vlExpectOnly(t, err, "green", "iteration-00.md")
	})
	t.Run("a check absent from the result is not a pass", func(t *testing.T) {
		base, res := vlPromptPair(70, 85, failing(), []vlCheck{vlChk("#1 "+vlLblFile, true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "prompt", base, res))
		vlExpectOnly(t, err, "green", "iteration-00.md")
	})
	t.Run("a baseline with no failing check has nothing to fix", func(t *testing.T) {
		base, res := vlPromptPair(70, 85, []vlCheck{vlChk("#1 "+vlLblReady, true, true)}, []vlCheck{vlChk("#1 "+vlLblReady, true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "prompt", base, res))
		vlExpectOnly(t, err, "green", "iteration-00.md")
	})
	t.Run("score drop fails", func(t *testing.T) {
		base, res := vlPromptPair(70, 65, failing(), []vlCheck{vlChk("#1 "+vlLblReady, true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "prompt", base, res))
		v := vlExpectOnly(t, err, "green", "iteration-00.md")
		vlMustMention(t, v.Message, "base", "res", "70.00", "65.00")
	})
	t.Run("change_type eval fails", func(t *testing.T) {
		base, res := vlEvalPair(failing(), []vlCheck{vlChk("#1 "+vlLblReady, true, true)})
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "eval", base, res))
		v := vlExpectOnly(t, err, "green", "iteration-00.md")
		vlMustMention(t, v.Message, "prompt", "eval")
	})
	t.Run("each failing condition is its own violation", func(t *testing.T) {
		base, res := vlEvalPair(failing(), failing())
		res.Score = 60
		_, err := vlRunVerify(t, vlPhaseOpts(t, "green", "eval", base, res))
		vs := vlViolations(t, err)
		if got := vlRules(vs); fmt.Sprint(got) != fmt.Sprint([]string{"green", "green", "green"}) {
			t.Fatalf("rules = %v, want three green", got)
		}
	})
}

func TestVerifyLogRefactor(t *testing.T) {
	t.Run("higher score passes", func(t *testing.T) {
		base, res := vlPromptPair(70, 80, nil, nil)
		vlPhaseOK(t, vlPhaseOpts(t, "refactor", "prompt", base, res))
	})
	t.Run("equal score passes", func(t *testing.T) {
		base, res := vlPromptPair(70, 70, nil, nil)
		vlPhaseOK(t, vlPhaseOpts(t, "refactor", "prompt", base, res))
	})
	t.Run("a tiny drop fails", func(t *testing.T) {
		// Claimed scores round to the same value; the recorded ones differ by 0.01.
		base, res := vlPromptPair(70, 69.99, nil, nil)
		_, err := vlRunVerify(t, vlPhaseOpts(t, "refactor", "prompt", base, res))
		v := vlExpectOnly(t, err, "refactor", "iteration-00.md")
		vlMustMention(t, v.Message, "base", "res", "70.00", "69.99")
	})
	t.Run("a drop beyond the claimed-score tolerance fails", func(t *testing.T) {
		base, res := vlPromptPair(70, 60, nil, nil)
		_, err := vlRunVerify(t, vlPhaseOpts(t, "refactor", "prompt", base, res))
		vlExpectOnly(t, err, "refactor", "iteration-00.md")
	})
	t.Run("change_type eval fails", func(t *testing.T) {
		base, res := vlEvalPair(nil, nil)
		_, err := vlRunVerify(t, vlPhaseOpts(t, "refactor", "eval", base, res))
		v := vlExpectOnly(t, err, "refactor", "iteration-00.md")
		vlMustMention(t, v.Message, "prompt", "eval")
	})
	t.Run("check results are not required", func(t *testing.T) {
		base, res := vlPromptPair(70, 75, []vlCheck{vlChk("#1 "+vlLblReady, false, false)}, []vlCheck{vlChk("#1 "+vlLblReady, false, false)})
		vlPhaseOK(t, vlPhaseOpts(t, "refactor", "prompt", base, res))
	})
}

func TestVerifyLogBaselinePhaseHasNoEvidenceRule(t *testing.T) {
	for _, ct := range []string{"prompt", "eval"} {
		t.Run(ct, func(t *testing.T) {
			var base, res vlRunSpec
			if ct == "eval" {
				base, res = vlEvalPair(nil, nil)
			} else {
				base, res = vlPromptPair(70, 40, nil, nil)
			}
			vlPhaseOK(t, vlPhaseOpts(t, "baseline", ct, base, res))
		})
	}
}

func TestVerifyLogEvidenceRulesDoNotCascade(t *testing.T) {
	failing := []vlCheck{vlChk("#1 "+vlLblReady, false, false)}
	fixed := []vlCheck{vlChk("#1 "+vlLblReady, true, true)}
	dropVerdict := func(t *testing.T, opts VerifyLogOptions, run string) {
		t.Helper()
		vlWriteFile(t, filepath.Join(opts.EvalsDir, "results", run, "summary.json"),
			`{"git_hash":"x","judge_model":"judge-m","sandbox":"native","agents":{"selftest":{"agent_model":"agent-m","prompt_sha256":"`+vlShaC+`","suite_sha256":"`+vlSuiteS1+`","complete":true,"case_filter":""}}}`)
	}
	t.Run("missing result run: only run-exists", func(t *testing.T) {
		for _, ph := range []string{"red", "green", "refactor"} {
			ct := "prompt"
			if ph == "red" {
				ct = "eval"
			}
			base, res := vlPromptPair(70, 60, failing, fixed)
			opts := vlPhaseOpts(t, ph, ct, base, res)
			if err := os.RemoveAll(filepath.Join(opts.EvalsDir, "results", "res")); err != nil {
				t.Fatal(err)
			}
			_, err := vlRunVerify(t, opts)
			if v := vlExpectOnly(t, err, "run-exists", "iteration-00.md"); v.Rule != "run-exists" {
				t.Fatalf("%s: %+v", ph, v)
			}
		}
	})
	t.Run("missing score: green and refactor report only run-data", func(t *testing.T) {
		for _, ph := range []string{"green", "refactor"} {
			base, res := vlPromptPair(70, 60, failing, fixed)
			opts := vlPhaseOpts(t, ph, "prompt", base, res)
			dropVerdict(t, opts, "res")
			_, err := vlRunVerify(t, opts)
			vlExpectOnly(t, err, "run-data", "iteration-00.md")
		}
	})
	t.Run("missing baseline score: refactor reports only run-data", func(t *testing.T) {
		base, res := vlPromptPair(70, 80, nil, nil)
		opts := vlPhaseOpts(t, "refactor", "prompt", base, res)
		vlWriteFile(t, filepath.Join(opts.EvalsDir, "results", "base", "summary.json"),
			`{"git_hash":"x","judge_model":"judge-m","sandbox":"native","agents":{"selftest":{"agent_model":"agent-m","prompt_sha256":"`+vlShaB+`","suite_sha256":"`+vlSuiteS1+`","complete":true,"case_filter":""}}}`)
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "run-data", "iteration-00.md")
	})
	t.Run("an invalid front-matter log gets no phase rules", func(t *testing.T) {
		base, res := vlPromptPair(70, 60, failing, failing)
		opts := vlPhaseOpts(t, "green", "prompt", base, res)
		path := filepath.Join(opts.IterationsDir, vlAgent, "iteration-00.md")
		data, _ := os.ReadFile(path)
		vlWriteFile(t, path, strings.Replace(string(data), "date: 2026-07-01", "date: nope", 1))
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "front-matter", "iteration-00.md")
	})
	t.Run("an invalid phase gets no evidence rules", func(t *testing.T) {
		base, res := vlPromptPair(70, 60, failing, failing)
		_, err := vlRunVerify(t, vlPhaseOpts(t, "tweak", "prompt", base, res))
		vlExpectOnly(t, err, "phase", "iteration-00.md")
	})
}

// vlFixtureCheckStates reads a committed run's <agent>.json and returns, per
// repeat, whether the check labelled "#2 ..." passed.
func vlFixtureCheckStates(t *testing.T, run string) []bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vlFixtureRoot, "evals", "results", run, vlAgent+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var ar AgentResult
	if err := json.Unmarshal(data, &ar); err != nil {
		t.Fatal(err)
	}
	var out []bool
	for _, c := range ar.Cases {
		for _, sc := range c.Scores {
			if sc.Name != "alpha" || c.CaseName != "basic" {
				continue
			}
			for _, ck := range sc.Checks {
				if strings.HasPrefix(ck.Label, "#2 output_contains") {
					out = append(out, ck.Passed)
				}
			}
		}
	}
	return out
}

// TestVerifyLogFixtureEvidenceRuns pins the committed runs that carry checks.
func TestVerifyLogFixtureEvidenceRuns(t *testing.T) {
	runs := map[string][]bool{
		"260706-100000-aaaaa11": {true, false}, // R1: fails in repeat 2 only
		"260706-110000-aaaaa00": {true, true},  // R0
		"260707-100000-bbbbb11": {true, true},  // G1
		"260707-110000-bbbbb22": {true, false}, // Gx
		"260707-120000-bbbbb33": {true, true},  // Gd
		"260708-100000-ccccc11": {true, true},  // F1
		"260708-110000-ccccc22": {true, true},  // Fd
	}
	for run, want := range runs {
		if got := vlFixtureCheckStates(t, run); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("run %s: check #2 per repeat = %v, want %v", run, got, want)
		}
	}
	score := func(run string) float64 {
		return loadRunRecord(filepath.Join(vlFixtureRoot, "evals", "results"), run, vlAgent).score
	}
	for run, want := range map[string]float64{
		"260706-100000-aaaaa11": 70, "260706-110000-aaaaa00": 70, "260707-100000-bbbbb11": 85,
		"260707-110000-bbbbb22": 85, "260707-120000-bbbbb33": 65, "260708-100000-ccccc11": 85, "260708-110000-ccccc22": 80,
	} {
		if got := score(run); got != want {
			t.Errorf("run %s: score = %v, want %v", run, got, want)
		}
	}
}

// ---------- cycle rule and RequireCycle ----------

// vlCycleVerify writes one log per phase (iteration-00, -01, ...; an empty
// phase leaves the key out) over the standard runs and verifies the set with
// RequireCycle as given. The logs need not satisfy their own phase rules: the
// cycle rule only looks at the phase labels. It returns every violation (nil on
// success).
func vlCycleVerify(t *testing.T, requireCycle bool, phases ...string) []LogViolation {
	t.Helper()
	logs := map[string]string{}
	for i, p := range phases {
		fm := vlFMPhase(p, i, "2026-07-01", "prompt", "r0", "r1", "60.0", "72.5")
		logs[fmt.Sprintf("iteration-%02d.md", i)] = vlDoc(fm, vlBody)
	}
	opts := vlSetup(t, logs)
	opts.RequireCycle = requireCycle
	_, err := vlRunVerify(t, opts)
	if err == nil {
		return nil
	}
	return vlViolations(t, err)
}

// vlCycleOnly returns the cycle violations of vs.
func vlCycleOnly(vs []LogViolation) []LogViolation {
	var out []LogViolation
	for _, v := range vs {
		if v.Rule == "cycle" {
			out = append(out, v)
		}
	}
	return out
}

func TestVerifyLogCycleRequired(t *testing.T) {
	tests := []struct {
		name   string
		phases []string
		// wantMentions has one entry per expected cycle violation: substrings
		// that violation's message must contain.
		wantMentions [][]string
	}{
		{name: "red green refactor", phases: []string{"baseline", "red", "green", "refactor"}},
		{name: "refactor may come first", phases: []string{"baseline", "refactor", "red", "green"}},
		{name: "repeated cycle", phases: []string{"baseline", "red", "green", "refactor", "red", "green"}},
		{name: "red not directly before green", phases: []string{"baseline", "red", "refactor", "green"}},
		{name: "no green", phases: []string{"baseline", "red", "refactor"}, wantMentions: [][]string{{"red", "green"}}},
		{name: "no red", phases: []string{"baseline", "green", "refactor"}, wantMentions: [][]string{{"red", "green"}}},
		{name: "green before red", phases: []string{"baseline", "green", "red", "refactor"}, wantMentions: [][]string{{"red", "green"}}},
		{name: "no refactor", phases: []string{"baseline", "red", "green"}, wantMentions: [][]string{{"refactor"}}},
		{name: "neither cycle nor refactor", phases: []string{"baseline"}, wantMentions: [][]string{{"red", "green"}, {"refactor"}}},
		{name: "green before red and no refactor", phases: []string{"baseline", "green", "red"}, wantMentions: [][]string{{"red", "green"}, {"refactor"}}},
		{name: "log without a phase does not count", phases: []string{"baseline", "red", "", "refactor"}, wantMentions: [][]string{{"red", "green"}}},
		{name: "log with an unknown phase does not count", phases: []string{"baseline", "red", "tweak", "refactor"}, wantMentions: [][]string{{"red", "green"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := vlCycleOnly(vlCycleVerify(t, true, tt.phases...))
			if len(vs) != len(tt.wantMentions) {
				t.Fatalf("got %d cycle violation(s), want %d: %v", len(vs), len(tt.wantMentions), vs)
			}
			for i, v := range vs {
				if !strings.HasSuffix(v.File, vlAgent) {
					t.Errorf("cycle violation File = %q, want the agent directory (suffix %q)", v.File, vlAgent)
				}
				vlMustMention(t, v.Message, tt.wantMentions[i]...)
			}
		})
	}
}

// TestVerifyLogCycleOffByDefault pins that the zero value of RequireCycle is
// false: the same sets that fail the cycle with it on report no cycle violation
// with it off.
func TestVerifyLogCycleOffByDefault(t *testing.T) {
	for _, phases := range [][]string{
		{"baseline"},
		{"baseline", "red", "green"},
		{"baseline", "red", "refactor"},
		{"baseline", "green", "red"},
	} {
		t.Run(strings.Join(phases, ","), func(t *testing.T) {
			if vs := vlCycleOnly(vlCycleVerify(t, false, phases...)); len(vs) != 0 {
				t.Fatalf("RequireCycle=false must not report cycle violations, got %v", vs)
			}
		})
	}
	if (VerifyLogOptions{}).RequireCycle {
		t.Error("the zero value of RequireCycle must be false")
	}
}

// TestVerifyLogCycleIsSetLevel pins that the cycle rule reports once against
// the agent directory and does not suppress or duplicate per-file violations.
func TestVerifyLogCycleIsSetLevel(t *testing.T) {
	vs := vlCycleVerify(t, true, "baseline")
	cycle := vlCycleOnly(vs)
	if len(cycle) != 2 {
		t.Fatalf("want 2 cycle violations, got %v", vs)
	}
	for _, v := range cycle {
		if strings.Contains(v.File, "iteration-") {
			t.Errorf("cycle violation must not name a log file, got File %q", v.File)
		}
	}
	// The set also has per-log violations (its runs are the std ones, with the
	// min-iterations default off): they are unaffected by RequireCycle.
	withoutCycle := vlCycleVerify(t, false, "baseline")
	if len(vs)-len(cycle) != len(withoutCycle) {
		t.Errorf("RequireCycle changed non-cycle violations: with=%v without=%v", vs, withoutCycle)
	}
}

func TestVerifyLogCycleNoLogsReportsOnlyNoLogs(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		opts := vlSetup(t, nil)
		opts.IterationsDir = filepath.Join(t.TempDir(), "absent")
		opts.RequireCycle = true
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", "")
	})
	t.Run("empty directory", func(t *testing.T) {
		opts := vlSetup(t, nil)
		opts.RequireCycle = true
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", vlAgent)
	})
	t.Run("only non-log files", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{"notes.txt": "n"})
		opts.RequireCycle = true
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "no-logs", vlAgent)
	})
}

func TestVerifyLogCycleFixtureSets(t *testing.T) {
	cycleOpts := func(set string, require bool) VerifyLogOptions {
		o := vlFixtureOpts(set, 0)
		o.RequireCycle = require
		return o
	}
	t.Run("valid passes with RequireCycle", func(t *testing.T) {
		out, err := vlRunVerify(t, cycleOpts("valid", true))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "OK selftest: 4 iterations verified\n"; out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
	})
	t.Run("no-refactor fails on cycle", func(t *testing.T) {
		out, err := vlRunVerify(t, cycleOpts("no-refactor", true))
		v := vlExpectOnly(t, err, "cycle", filepath.Join("no-refactor", vlAgent))
		vlMustMention(t, v.Message, "refactor")
		if out != "" {
			t.Fatalf("nothing should be printed on failure, got %q", out)
		}
	})
	t.Run("no-refactor passes with RequireCycle false", func(t *testing.T) {
		out, err := vlRunVerify(t, cycleOpts("no-refactor", false))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "OK selftest: 3 iterations verified\n"; out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
	})
	t.Run("no-refactor has red then green and no refactor", func(t *testing.T) {
		for i, want := range []string{"baseline", "red", "green"} {
			doc, err := os.ReadFile(filepath.Join(vlFixtureRoot, "no-refactor", vlAgent, fmt.Sprintf("iteration-%02d.md", i)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(doc), "\nphase: "+want+"\n") {
				t.Errorf("no-refactor iteration-%02d should have phase %q", i, want)
			}
		}
	})
}
