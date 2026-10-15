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

func vlVerdictSummary(score float64, sha string) string {
	return fmt.Sprintf(`{"git_hash":"x","agent_verdicts":{%q:{"score":%v,"threshold":70}},"agents":{%q:{"prompt_sha256":%q}}}`,
		vlAgent, score, vlAgent, sha)
}

func vlAgentJSON(sha string) string {
	return fmt.Sprintf(`{"agent":%q,"prompt_sha256":%q,"cases":[]}`, vlAgent, sha)
}

// vlStdRuns writes the standard runs: r0 60.0/A, r1 72.5/B, r2 90.0/C,
// r2b 91.0/C (same prompt as r2), r1dup 73.0/B (same prompt as r1).
func vlStdRuns(t *testing.T, evalsDir string) {
	t.Helper()
	for _, r := range []struct {
		name  string
		score float64
		sha   string
	}{
		{"r0", 60.0, vlShaA}, {"r1", 72.5, vlShaB}, {"r2", 90.0, vlShaC},
		{"r2b", 91.0, vlShaC}, {"r1dup", 73.0, vlShaB},
	} {
		vlWriteRun(t, evalsDir, r.name, vlVerdictSummary(r.score, r.sha), vlAgentJSON(r.sha))
	}
}

// vlFM renders front-matter lines (no delimiters).
func vlFM(iter int, date, ctype, base, res, bs, rs string) string {
	return fmt.Sprintf("iteration: %d\ndate: %s\nchange_type: %s\nbaseline_run: %s\nresult_run: %s\nbaseline_score: %s\nresult_score: %s\n",
		iter, date, ctype, base, res, bs, rs)
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
	}{
		{set: "valid", min: 2},
		{set: "missing-run", min: 0, wantRule: "run-exists", wantSuffix: "iteration-01.md"},
		{set: "score-mismatch", min: 0, wantRule: "score-match", wantSuffix: "iteration-01.md"},
		{set: "gap", min: 0, wantRule: "sequence", wantSuffix: "iteration-02.md"},
		{set: "broken-chain", min: 0, wantRule: "chain", wantSuffix: "iteration-01.md"},
		{set: "no-prompt-change", min: 0, wantRule: "prompt-change", wantSuffix: "iteration-01.md"},
		{set: "valid", min: 5, wantRule: "min-iterations", wantSuffix: filepath.Join("valid", vlAgent)},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/min=%d", tt.set, tt.min), func(t *testing.T) {
			out, err := vlRunVerify(t, vlFixtureOpts(tt.set, tt.min))
			if tt.wantRule == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if want := "OK selftest: 3 iterations verified\n"; out != want {
					t.Fatalf("output = %q, want %q", out, want)
				}
				return
			}
			vlExpectOnly(t, err, tt.wantRule, tt.wantSuffix)
			if out != "" {
				t.Fatalf("nothing should be printed on failure, got %q", out)
			}
		})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// eval change type so only the score source is under test.
			opts := vlSetup(t, map[string]string{
				"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r0", "src", "60.0", tt.claimed), vlBody),
			})
			vlWriteRun(t, opts.EvalsDir, "src", tt.summary, tt.agentJSON)
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
	// The committed legacy run records only agent_scores (0.8 => 80.0).
	root := t.TempDir()
	iters := filepath.Join(root, "iterations")
	vlWriteFile(t, filepath.Join(iters, vlAgent, "iteration-00.md"),
		vlDoc(vlFM(0, "2026-07-01", "eval", "260630-100000-fffffff", "260701-100000-aaaaaaa", "80.0", "60.0"), vlBody))
	opts := VerifyLogOptions{IterationsDir: iters, EvalsDir: filepath.Join(vlFixtureRoot, "evals")}
	if _, err := vlRunVerify(t, opts); err != nil {
		t.Fatalf("legacy run should verify: %v", err)
	}
}

// ---------- prompt hash handling ----------

func TestVerifyLogPromptHashRules(t *testing.T) {
	t.Run("prompt iteration with no prompt_sha256 is run-data", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "prompt", "r0", "nosha", "60.0", "72.5"), vlBody),
		})
		// Same score as r1 but no prompt hash in either file.
		vlWriteRun(t, opts.EvalsDir, "nosha",
			fmt.Sprintf(`{"agent_verdicts":{%q:{"score":72.5}}}`, vlAgent),
			fmt.Sprintf(`{"agent":%q,"cases":[]}`, vlAgent))
		// baseline must differ from result so only the missing hash matters.
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "run-data", "iteration-00.md")
		if !strings.Contains(v.Message, "prompt_sha256") || !strings.Contains(v.Message, "nosha") {
			t.Errorf("message %q should name prompt_sha256 and the run", v.Message)
		}
	})

	t.Run("eval iteration with identical prompt_sha256 passes", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r2", "r2b", "90.0", "91.0"), vlBody),
		})
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("eval iteration needs no prompt_sha256", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": vlDoc(vlFM(0, "2026-07-01", "eval", "r0", "nosha", "60.0", "72.5"), vlBody),
		})
		vlWriteRun(t, opts.EvalsDir, "nosha", fmt.Sprintf(`{"agent_verdicts":{%q:{"score":72.5}}}`, vlAgent), "")
		if _, err := vlRunVerify(t, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
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
	// vlSetup runs all have fixed scores; use eval type and matching scores via
	// r0 -> r0 so chain checks stay trivially satisfied.
	t.Run("does not start at 00", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{"iteration-01.md": doc(1, "r0", "r0", "2026-07-01")})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-01.md")
	})
	t.Run("duplicate numbers 00 and 000", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md":  doc(0, "r0", "r0", "2026-07-01"),
			"iteration-000.md": doc(0, "r0", "r0", "2026-07-01"),
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
		opts := vlSetup(t, map[string]string{"iteration-00.md": doc(3, "r0", "r0", "2026-07-01")})
		_, err := vlRunVerify(t, opts)
		v := vlExpectOnly(t, err, "sequence", "iteration-00.md")
		if !strings.Contains(v.Message, "3") {
			t.Errorf("message %q should name the front-matter value", v.Message)
		}
	})
	t.Run("misnamed iteration-1.md", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": doc(0, "r0", "r0", "2026-07-01"),
			"iteration-1.md":  doc(1, "r0", "r0", "2026-07-01"),
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-1.md")
	})
	t.Run("misnamed iteration-ab.md", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md":   doc(0, "r0", "r0", "2026-07-01"),
			"iteration-ab.md":   "junk",
			"iteration-01.md.x": "ignored",
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-ab.md")
	})
	t.Run("gap flags only the first out-of-place file", func(t *testing.T) {
		opts := vlSetup(t, map[string]string{
			"iteration-00.md": doc(0, "r0", "r0", "2026-07-01"),
			"iteration-02.md": doc(2, "r0", "r0", "2026-07-01"),
			"iteration-03.md": doc(3, "r0", "r0", "2026-07-01"),
		})
		_, err := vlRunVerify(t, opts)
		vlExpectOnly(t, err, "sequence", "iteration-02.md")
	})
	t.Run("three digit numbers accepted when contiguous", func(t *testing.T) {
		files := map[string]string{}
		for i := 0; i < 3; i++ {
			files[fmt.Sprintf("iteration-%02d.md", i)] = doc(i, "r0", "r0", "2026-07-01")
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
