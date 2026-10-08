package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	vlFixtureRoot  = "testdata/verifylog"
	vlFixtureEvals = "testdata/verifylog/evals"
	vlAgent        = "selftest"

	vlRunA = "260701-100000-aaaaaaa" // 60.0, sha a
	vlRunB = "260702-100000-bbbbbbb" // 72.5, sha b
	vlRunC = "260703-100000-ccccccc" // 85.0, sha c

	vlBody = `
## Baseline

Baseline numbers.

## Hypothesis

A falsifiable prediction.

## Change

The exact change.

## Results

Measured numbers.

## Reasoning

Why it worked.
`
)

// vlPairs renders violations as sorted "basename|rule" strings so tests can
// assert the exact (file, rule) set.
func vlPairs(vs []Violation) []string {
	pairs := make([]string, 0, len(vs))
	for _, v := range vs {
		pairs = append(pairs, filepath.Base(v.File)+"|"+v.Rule)
	}
	sort.Strings(pairs)
	return pairs
}

func vlAssertPairs(t *testing.T, vs []Violation, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := vlPairs(vs)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("violations = %v, want %v\nfull: %v", got, want, vs)
	}
}

func vlFixtureOpts(set string) VerifyLogOptions {
	return VerifyLogOptions{
		Agent:         vlAgent,
		IterationsDir: filepath.Join(vlFixtureRoot, set),
		EvalsDir:      vlFixtureEvals,
		MinIterations: 0,
	}
}

func TestVerifyLogFixtureSets(t *testing.T) {
	tests := []struct {
		set  string
		want []string
	}{
		{"valid", nil},
		{"missing-run", []string{"iteration-01.md|run-missing"}},
		{"score-mismatch", []string{"iteration-00.md|score-mismatch"}},
		{"gap", []string{"iteration-02.md|numbering"}},
		{"broken-chain", []string{"iteration-01.md|chain"}},
		{"no-prompt-change", []string{"iteration-00.md|prompt-unchanged"}},
	}
	for _, tt := range tests {
		t.Run(tt.set, func(t *testing.T) {
			opts := vlFixtureOpts(tt.set)
			opts.MinIterations = DefaultMinIterations
			vs, err := VerifyLog(opts)
			if err != nil {
				t.Fatalf("VerifyLog: %v", err)
			}
			vlAssertPairs(t, vs, tt.want...)
		})
	}
}

func TestVerifyLogDefaults(t *testing.T) {
	if DefaultIterationsDir != ".kairon/iterations" {
		t.Errorf("DefaultIterationsDir = %q", DefaultIterationsDir)
	}
	if DefaultMinIterations != 2 {
		t.Errorf("DefaultMinIterations = %d", DefaultMinIterations)
	}
}

func TestViolationString(t *testing.T) {
	v := Violation{File: "a/iteration-00.md", Rule: "chain", Message: "boom"}
	if got, want := v.String(), "a/iteration-00.md: [chain] boom"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestVerifyLogMinIterations(t *testing.T) {
	for _, tt := range []struct {
		min  int
		want []string
	}{
		{5, []string{"selftest|min-iterations"}}, // File is the agent directory
		{3, nil},
		{2, nil},
	} {
		opts := vlFixtureOpts("valid")
		opts.MinIterations = tt.min
		vs, err := VerifyLog(opts)
		if err != nil {
			t.Fatalf("min %d: %v", tt.min, err)
		}
		vlAssertPairs(t, vs, tt.want...)
	}

	t.Run("missing agent directory", func(t *testing.T) {
		opts := vlFixtureOpts("valid")
		opts.Agent = "nobody"
		opts.MinIterations = DefaultMinIterations
		vs, err := VerifyLog(opts)
		if err != nil {
			t.Fatal(err)
		}
		vlAssertPairs(t, vs, "nobody|min-iterations")
	})
}

func TestVerifyLogDoesNotTouchGlobalConfig(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.evalsDir = "does/not/exist"
	opts := vlFixtureOpts("valid")
	opts.MinIterations = DefaultMinIterations
	vs, err := VerifyLog(opts)
	if err != nil {
		t.Fatal(err)
	}
	vlAssertPairs(t, vs)
	if cfg.evalsDir != "does/not/exist" {
		t.Errorf("cfg.evalsDir changed to %q", cfg.evalsDir)
	}
}

func TestVerifyLogUsageErrors(t *testing.T) {
	for _, agent := range []string{"", "a/b", `a\b`, "..", "."} {
		opts := vlFixtureOpts("valid")
		opts.Agent = agent
		vs, err := VerifyLog(opts)
		if err == nil {
			t.Errorf("agent %q: expected error, got violations %v", agent, vs)
		}
	}

	opts := vlFixtureOpts("valid")
	opts.MinIterations = -1
	if vs, err := VerifyLog(opts); err == nil {
		t.Errorf("negative MinIterations: expected error, got %v", vs)
	}

	t.Run("agent path is a file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "agent"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyLog(VerifyLogOptions{Agent: "agent", IterationsDir: dir, EvalsDir: dir}); err == nil {
			t.Error("expected an IO error, got nil")
		}
	})
}

// ---- temp-dir helpers ----

type vlEnv struct {
	t        *testing.T
	iterDir  string
	evalsDir string
}

func newVLEnv(t *testing.T) *vlEnv {
	t.Helper()
	root := t.TempDir()
	e := &vlEnv{t: t, iterDir: filepath.Join(root, "iterations"), evalsDir: filepath.Join(root, "evals")}
	score := func(f float64) *float64 { return &f }
	e.addRun(vlRunA, score(0.6), strings.Repeat("a", 64))
	e.addRun(vlRunB, score(0.725), strings.Repeat("b", 64))
	e.addRun(vlRunC, score(0.85), strings.Repeat("c", 64))
	return e
}

// addRun writes <evals>/results/<run>/summary.json. A nil score omits the
// agent from agent_scores; an empty sha omits the agents block.
func (e *vlEnv) addRun(run string, score *float64, sha string) {
	e.t.Helper()
	summary := map[string]any{"git_hash": "x", "agent_scores": map[string]float64{}}
	if score != nil {
		summary["agent_scores"] = map[string]float64{vlAgent: *score}
	}
	if sha != "" {
		summary["agents"] = map[string]any{vlAgent: map[string]any{"agent_model": "m", "prompt_sha256": sha}}
	}
	data, err := json.Marshal(summary)
	if err != nil {
		e.t.Fatal(err)
	}
	dir := filepath.Join(e.evalsDir, "results", run)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *vlEnv) writeRaw(name, content string) {
	e.t.Helper()
	dir := filepath.Join(e.iterDir, vlAgent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// vlFrontMatter renders default front-matter for iteration n (A→B, prompt),
// applying overrides; an override value of "!" removes the key.
func vlFrontMatter(n int, over map[string]string) string {
	keys := []string{"iteration", "date", "change_type", "baseline_run", "result_run", "baseline_score", "result_score"}
	vals := map[string]string{
		"iteration":      fmt.Sprint(n),
		"date":           "2026-07-01",
		"change_type":    "prompt",
		"baseline_run":   vlRunA,
		"result_run":     vlRunB,
		"baseline_score": "60.0",
		"result_score":   "72.5",
	}
	for k, v := range over {
		vals[k] = v
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range keys {
		if vals[k] == "!" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", k, vals[k])
	}
	b.WriteString("---\n")
	return b.String()
}

func (e *vlEnv) writeIter(n int, over map[string]string, body string) {
	e.t.Helper()
	e.writeRaw(fmt.Sprintf("iteration-%02d.md", n), vlFrontMatter(n, over)+body)
}

func (e *vlEnv) verify(min int) []Violation {
	e.t.Helper()
	vs, err := VerifyLog(VerifyLogOptions{Agent: vlAgent, IterationsDir: e.iterDir, EvalsDir: e.evalsDir, MinIterations: min})
	if err != nil {
		e.t.Fatalf("VerifyLog: %v", err)
	}
	return vs
}

// ---- temp-dir tests ----

func TestVerifyLogBaselineIterationValid(t *testing.T) {
	e := newVLEnv(t)
	e.writeIter(0, nil, vlBody)
	vlAssertPairs(t, e.verify(1))
}

func TestVerifyLogScoreTolerance(t *testing.T) {
	tests := []struct {
		name   string
		result string
		fail   bool
	}{
		{"exact", "72.5", false},
		{"plus 0.1", "72.6", false},
		{"minus 0.1", "72.4", false},
		{"plus 0.2", "72.7", true},
		{"minus 0.2", "72.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newVLEnv(t)
			e.writeIter(0, map[string]string{"result_score": tt.result}, vlBody)
			if tt.fail {
				vlAssertPairs(t, e.verify(1), "iteration-00.md|score-mismatch")
			} else {
				vlAssertPairs(t, e.verify(1))
			}
		})
	}

	t.Run("baseline score is checked too", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(0, map[string]string{"baseline_score": "61.0"}, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-00.md|score-mismatch")
	})
}

func TestVerifyLogFrontMatter(t *testing.T) {
	override := func(k, v string) map[string]string { return map[string]string{k: v} }
	tests := []struct {
		name string
		over map[string]string
	}{
		{"score without decimal", override("result_score", "87")},
		{"score two decimals", override("result_score", "87.55")},
		{"score above range", override("result_score", "101.0")},
		{"score negative", override("result_score", "-1.0")},
		{"score text", override("result_score", "high")},
		{"score quoted", override("result_score", `"72.5"`)},
		{"baseline score two decimals", override("baseline_score", "60.00")},
		{"date wrong format", override("date", "2026-7-1")},
		{"date impossible", override("date", "2026-02-30")},
		{"date text", override("date", "yesterday")},
		{"change_type unknown", override("change_type", "rubric")},
		{"iteration not an integer", override("iteration", "zero")},
		{"run name parent traversal", override("baseline_run", "../x")},
		{"run name with slash", override("result_run", "a/b")},
		{"run name with backslash", override("result_run", `a\b`)},
		{"run name dot-dot", override("baseline_run", "..")},
		{"run name empty", override("baseline_run", `""`)},
		{"missing iteration", override("iteration", "!")},
		{"missing date", override("date", "!")},
		{"missing change_type", override("change_type", "!")},
		{"missing baseline_run", override("baseline_run", "!")},
		{"missing result_run", override("result_run", "!")},
		{"missing baseline_score", override("baseline_score", "!")},
		{"missing result_score", override("result_score", "!")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newVLEnv(t)
			e.writeIter(0, tt.over, vlBody)
			// An invalid iteration skips run rules and is reported once.
			vlAssertPairs(t, e.verify(1), "iteration-00.md|front-matter")
		})
	}

	raw := map[string]string{
		"no front-matter":        "## Baseline\n\ntext\n",
		"unterminated":           "---\niteration: 0\n",
		"invalid yaml":           "---\niteration: [0\n---\n" + vlBody,
		"not a mapping":          "---\n- a\n- b\n---\n" + vlBody,
		"leading blank line":     "\n" + vlFrontMatter(0, nil) + vlBody,
		"empty front-matter":     "---\n---\n" + vlBody,
		"duplicate key":          strings.Replace(vlFrontMatter(0, nil), "date: 2026-07-01\n", "date: 2026-07-01\ndate: 2026-07-02\n", 1) + vlBody,
		"body marker not exact":  strings.TrimSuffix(vlFrontMatter(0, nil), "---\n") + "--- \n" + vlBody,
		"empty file":             "",
		"only opening delimiter": "---\n",
	}
	for name, content := range raw {
		t.Run(name, func(t *testing.T) {
			e := newVLEnv(t)
			e.writeRaw("iteration-00.md", content)
			vlAssertPairs(t, e.verify(1), "iteration-00.md|front-matter")
		})
	}

	t.Run("score 100.0 and 0.0 are accepted", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun("run-zero", vlPtr(0.0), "")
		e.addRun("run-full", vlPtr(1.0), "")
		e.writeIter(0, map[string]string{
			"change_type": "eval", "baseline_run": "run-zero", "result_run": "run-full",
			"baseline_score": "0.0", "result_score": "100.0",
		}, vlBody)
		vlAssertPairs(t, e.verify(1))
	})

	t.Run("unknown keys are ignored", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeRaw("iteration-00.md", strings.Replace(vlFrontMatter(0, nil), "---\n", "---\nextra: whatever\n", 1)+vlBody)
		vlAssertPairs(t, e.verify(1))
	})

	t.Run("CRLF line endings are accepted", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeRaw("iteration-00.md", strings.ReplaceAll(vlFrontMatter(0, nil)+vlBody, "\n", "\r\n"))
		vlAssertPairs(t, e.verify(1))
	})
}

func vlPtr[T any](v T) *T { return &v }

func TestVerifyLogRunNameNotSinglePathElement(t *testing.T) {
	e := newVLEnv(t)
	// A real run exists one level up; traversal must not resolve it.
	e.writeIter(0, map[string]string{"baseline_run": "../results/" + vlRunA}, vlBody)
	vlAssertPairs(t, e.verify(1), "iteration-00.md|front-matter")
}

func TestVerifyLogHeadings(t *testing.T) {
	without := func(h string) string {
		return strings.Replace(vlBody, "## "+h+"\n", "", 1)
	}
	tests := []struct {
		name string
		body string
		msg  string
	}{
		{"missing Baseline", without("Baseline"), "Baseline"},
		{"missing Reasoning", without("Reasoning"), "Reasoning"},
		{"duplicate", vlBody + "\n## Reasoning\n\nAgain.\n", "Reasoning"},
		{"out of order", "\n## Baseline\n\nb\n\n## Change\n\nc\n\n## Hypothesis\n\nh\n\n## Results\n\nr\n\n## Reasoning\n\nx\n", "order"},
		{"empty content", strings.Replace(vlBody, "Measured numbers.", "", 1), "Results"},
		{"whitespace only content", strings.Replace(vlBody, "Measured numbers.", "   \t", 1), "Results"},
		{"heading only in code fence", strings.Replace(vlBody, "## Change\n\nThe exact change.", "```\n## Change\n```\n\nSee above.", 1), "Change"},
		{"no body", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newVLEnv(t)
			e.writeIter(0, nil, tt.body)
			vs := e.verify(1)
			if len(vs) == 0 {
				t.Fatal("expected headings violation, got none")
			}
			for _, v := range vs {
				if v.Rule != "headings" || filepath.Base(v.File) != "iteration-00.md" {
					t.Fatalf("unexpected violation %v", v)
				}
			}
			if tt.msg != "" && !strings.Contains(vs[0].Message, tt.msg) {
				t.Errorf("message %q does not mention %q", vs[0].Message, tt.msg)
			}
		})
	}

	t.Run("heading with trailing space and extra sections are fine", func(t *testing.T) {
		e := newVLEnv(t)
		body := strings.Replace(vlBody, "## Change\n", "## Change  \n", 1) + "\n## Extra\n\nnotes\n"
		e.writeIter(0, nil, body)
		vlAssertPairs(t, e.verify(1))
	})

	t.Run("code fence containing heading does not count as content break", func(t *testing.T) {
		e := newVLEnv(t)
		body := strings.Replace(vlBody, "The exact change.", "```diff\n## Baseline\n+x\n```", 1)
		e.writeIter(0, nil, body)
		vlAssertPairs(t, e.verify(1))
	})
}

func TestVerifyLogDateOrder(t *testing.T) {
	e := newVLEnv(t)
	e.writeIter(0, map[string]string{"date": "2026-07-02", "result_run": vlRunB}, vlBody)
	e.writeIter(1, map[string]string{"date": "2026-07-01", "baseline_run": vlRunB, "result_run": vlRunC, "baseline_score": "72.5", "result_score": "85.0"}, vlBody)
	vlAssertPairs(t, e.verify(2), "iteration-01.md|date-order")

	t.Run("same date is fine", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(0, nil, vlBody)
		e.writeIter(1, map[string]string{"baseline_run": vlRunB, "result_run": vlRunC, "baseline_score": "72.5", "result_score": "85.0"}, vlBody)
		vlAssertPairs(t, e.verify(2))
	})
}

func TestVerifyLogRunNoScore(t *testing.T) {
	e := newVLEnv(t)
	e.addRun(vlRunB, nil, strings.Repeat("b", 64)) // run exists, agent has no score
	e.writeIter(0, nil, vlBody)
	vlAssertPairs(t, e.verify(1), "iteration-00.md|run-no-score")
}

func TestVerifyLogRunMissing(t *testing.T) {
	e := newVLEnv(t)
	e.writeIter(0, map[string]string{"baseline_run": "260799-000000-nonexist"}, vlBody)
	vlAssertPairs(t, e.verify(1), "iteration-00.md|run-missing")
}

func TestVerifyLogPromptRules(t *testing.T) {
	t.Run("unrecorded sha on result run", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun(vlRunB, vlPtr(0.725), "")
		e.writeIter(0, nil, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-00.md|prompt-unrecorded")
	})

	t.Run("unrecorded sha on baseline run", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun(vlRunA, vlPtr(0.6), "")
		e.writeIter(0, nil, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-00.md|prompt-unrecorded")
	})

	t.Run("sha only in agent file is found", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun(vlRunB, vlPtr(0.725), "")
		agentFile := fmt.Sprintf(`{"agent":%q,"prompt_sha256":%q,"cases":[]}`, vlAgent, strings.Repeat("b", 64))
		path := filepath.Join(e.evalsDir, "results", vlRunB, vlAgent+".json")
		if err := os.WriteFile(path, []byte(agentFile), 0o644); err != nil {
			t.Fatal(err)
		}
		e.writeIter(0, nil, vlBody)
		vlAssertPairs(t, e.verify(1))
	})

	t.Run("same sha is unchanged", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun(vlRunB, vlPtr(0.725), strings.Repeat("a", 64))
		e.writeIter(0, nil, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-00.md|prompt-unchanged")
	})

	t.Run("eval change type is exempt", func(t *testing.T) {
		e := newVLEnv(t)
		e.addRun(vlRunB, vlPtr(0.725), strings.Repeat("a", 64)) // same sha
		e.writeIter(0, map[string]string{"change_type": "eval"}, vlBody)
		vlAssertPairs(t, e.verify(1))

		e.addRun(vlRunB, vlPtr(0.725), "") // no sha at all
		vlAssertPairs(t, e.verify(1))
	})
}

func TestVerifyLogNumbering(t *testing.T) {
	t.Run("file name without zero padding", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(0, nil, vlBody)
		e.writeRaw("iteration-1.md", vlFrontMatter(1, map[string]string{"baseline_run": vlRunB, "result_run": vlRunC, "baseline_score": "72.5", "result_score": "85.0"})+vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-1.md|numbering")
	})

	t.Run("starts at 01", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(1, nil, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-01.md|numbering")
	})

	t.Run("iteration differs from file name", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeRaw("iteration-00.md", vlFrontMatter(5, nil)+vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-00.md|numbering")
	})

	t.Run("only the first break of a gap is reported", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(0, nil, vlBody)
		e.writeIter(2, map[string]string{"baseline_run": vlRunB, "result_run": vlRunC, "baseline_score": "72.5", "result_score": "85.0"}, vlBody)
		e.writeIter(3, map[string]string{"baseline_run": vlRunC, "result_run": vlRunC, "baseline_score": "85.0", "result_score": "85.0", "change_type": "eval"}, vlBody)
		vlAssertPairs(t, e.verify(1), "iteration-02.md|numbering")
	})

	t.Run("three digit numbers are allowed", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeRaw("iteration-000.md", vlFrontMatter(0, nil)+vlBody)
		vlAssertPairs(t, e.verify(1))
	})

	t.Run("unrelated files are ignored", func(t *testing.T) {
		e := newVLEnv(t)
		e.writeIter(0, nil, vlBody)
		e.writeRaw("README.md", "notes")
		e.writeRaw("iteration-00.md.bak", "old")
		if err := os.MkdirAll(filepath.Join(e.iterDir, vlAgent, "iteration-99.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		vlAssertPairs(t, e.verify(1), "iteration-00.md.bak|numbering")
	})
}

func TestVerifyLogChain(t *testing.T) {
	e := newVLEnv(t)
	e.writeIter(0, nil, vlBody)
	e.writeIter(1, map[string]string{"baseline_run": vlRunA, "result_run": vlRunC, "baseline_score": "60.0", "result_score": "85.0"}, vlBody)
	vlAssertPairs(t, e.verify(2), "iteration-01.md|chain")
}

func TestVerifyLogCollectsAllViolationsSorted(t *testing.T) {
	e := newVLEnv(t)
	// 00: score-mismatch + prompt-unchanged; 01: broken chain + front-matter-free
	// date-order; plus min-iterations on the directory.
	e.addRun(vlRunB, vlPtr(0.725), strings.Repeat("a", 64))
	e.writeIter(0, map[string]string{"date": "2026-07-09", "result_score": "90.0"}, vlBody)
	e.writeIter(1, map[string]string{"date": "2026-07-01", "baseline_run": vlRunA, "result_run": vlRunC, "baseline_score": "60.0", "result_score": "85.0"}, vlBody)
	vs := e.verify(5)

	vlAssertPairs(t, vs,
		"iteration-00.md|prompt-unchanged",
		"iteration-00.md|score-mismatch",
		"iteration-01.md|chain",
		"iteration-01.md|date-order",
		"selftest|min-iterations",
	)
	if !sort.SliceIsSorted(vs, func(i, j int) bool {
		if vs[i].File != vs[j].File {
			return vs[i].File < vs[j].File
		}
		return vs[i].Rule < vs[j].Rule
	}) {
		t.Errorf("violations not sorted by file then rule: %v", vs)
	}
}

func TestVerifyLogMissingIterationsDirWithZeroMinimum(t *testing.T) {
	e := newVLEnv(t)
	vlAssertPairs(t, e.verify(0))
}

// ---- README example ----

const iterationsReadmePath = "../../.kairon/iterations/README.md"

// extractReadmeExample returns the content of the first four-backtick
// markdown fence in the README whose first line is the front-matter opener.
func extractReadmeExample(t *testing.T, readme string) string {
	t.Helper()
	lines := strings.Split(readme, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t\r") != "````markdown" {
			continue
		}
		var block []string
		closed := false
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimRight(lines[j], " \t\r") == "````" {
				closed = true
				i = j
				break
			}
			block = append(block, lines[j])
		}
		if closed && len(block) > 0 && block[0] == "---" {
			return strings.Join(block, "\n") + "\n"
		}
	}
	t.Fatal("no four-backtick markdown fence starting with --- found in README")
	return ""
}

// TestIterationsReadmeExampleParses checks that the complete example in
// .kairon/iterations/README.md is accepted by the verifier's own front-matter
// and heading parsing. Run names are illustrative, so run-dependent rules are
// not asserted.
func TestIterationsReadmeExampleParses(t *testing.T) {
	data, err := os.ReadFile(iterationsReadmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	example := extractReadmeExample(t, string(data))

	fmText, body, err := splitFrontMatter(example)
	if err != nil {
		t.Fatalf("splitFrontMatter: %v", err)
	}
	if _, problems := parseFrontMatter(fmText); len(problems) > 0 {
		t.Errorf("example front-matter invalid: %v", problems)
	}
	if msgs := checkHeadings(body); len(msgs) > 0 {
		t.Errorf("example headings invalid: %v", msgs)
	}

	// Run it through the full verifier as iteration-00.md as well: the
	// per-file format rules must not fire.
	e := newVLEnv(t)
	e.writeRaw("iteration-00.md", example)
	for _, v := range e.verify(0) {
		switch v.Rule {
		case ruleFrontMatter, ruleHeadings, ruleNumbering:
			t.Errorf("example violates format rule: %s", v)
		}
	}
}
