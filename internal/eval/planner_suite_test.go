package eval

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/checks"
	"github.com/jbrinkman/kairon/internal/stageoutput"
	"gopkg.in/yaml.v3"
)

// The planner suite (.kairon/evals/rubrics/planner.yaml and
// .kairon/evals/cases/planner/*.yaml) certifies the stage-output contract of
// the planner. These tests make its RED/GREEN claim reproducible offline:
//
//   - GREEN: the reference transcripts shipped in each case's stub.turns pass
//     the suite and every document in them is valid per internal/stageoutput.
//   - RED: transcripts of what the pre-contract planner prompt produces
//     (testdata/planner-legacy/<case>.txt) fail the suite, and fail the two
//     criteria that carry the contract.
//
// Everything runs on the stub backend inside a temp project, so no model is
// called and the repository's .kairon/evals/results/ is neither read nor
// written.

// plannerCaseNames are the seven shipped planner cases. A case that is
// removed or renamed makes TestPlannerSuiteRegistration fail.
var plannerCaseNames = []string{
	"automated-best-effort-fill",
	"automated-bug-classification",
	"automated-punch-out",
	"interactive-option-rules",
	"interactive-return-document",
	"interactive-single-decision",
	"multi-issue-split",
}

// plannerFloor is the rubric's min_criterion_score, in percent.
const plannerFloor = 60.0

// legacyTurnSeparator splits a legacy transcript into agent turns.
const legacyTurnSeparator = "=== TURN ==="

// plannerPaths are the absolute locations of the shipped suite and the legacy
// transcripts. They are resolved before a test changes directory.
type plannerPaths struct {
	evals  string // .kairon/evals of the repository (read only)
	legacy string // internal/eval/testdata/planner-legacy
}

func plannerSuitePaths(t *testing.T) plannerPaths {
	t.Helper()
	evals, err := filepath.Abs(filepath.Join("..", "..", ".kairon", "evals"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := filepath.Abs(filepath.Join("testdata", "planner-legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(evals, "rubrics", "planner.yaml")); err != nil {
		t.Fatalf("planner rubric not found: %v", err)
	}
	return plannerPaths{evals: evals, legacy: legacy}
}

// copyPlannerFile copies src to dst, creating parent directories.
func copyPlannerFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, dst, string(data))
}

// copyPlannerSuite copies the shipped rubric, cases and planner fixtures into
// a fresh evals directory and returns it. The results directory is never
// copied.
func copyPlannerSuite(t *testing.T, p plannerPaths) string {
	t.Helper()
	dst := t.TempDir()
	copyPlannerFile(t, filepath.Join(p.evals, "rubrics", "planner.yaml"), filepath.Join(dst, "rubrics", "planner.yaml"))
	for _, pattern := range []string{
		filepath.Join(p.evals, "cases", "planner", "*.yaml"),
		filepath.Join(p.evals, "fixtures", "planner-*.md"),
	} {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			rel, err := filepath.Rel(p.evals, f)
			if err != nil {
				t.Fatal(err)
			}
			copyPlannerFile(t, f, filepath.Join(dst, rel))
		}
	}
	return dst
}

// readLegacyTurns reads the transcript of a case and splits it into turns.
func readLegacyTurns(t *testing.T, p plannerPaths, caseName string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.legacy, caseName+".txt"))
	if err != nil {
		t.Fatalf("legacy transcript for %s: %v", caseName, err)
	}
	var turns []string
	for _, part := range strings.Split(string(data), "\n"+legacyTurnSeparator+"\n") {
		if s := strings.TrimSpace(part); s != "" {
			turns = append(turns, s+"\n")
		}
	}
	if len(turns) == 0 {
		t.Fatalf("legacy transcript for %s is empty", caseName)
	}
	return turns
}

// swapInLegacyTranscripts rewrites each case file in evalsDir so its stub
// replays the legacy transcript instead of the reference transcript. The
// checks, setup and everything else stay exactly as shipped. A simulated-user
// case runs for as many turns as the transcript has: max_turns follows it and
// every routing decision is "none", so the legacy planner is answered with the
// default reply.
func swapInLegacyTranscripts(t *testing.T, p plannerPaths, evalsDir string) {
	t.Helper()
	for _, name := range plannerCaseNames {
		path := filepath.Join(evalsDir, "cases", "planner", name+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		turns := readLegacyTurns(t, p, name)

		var stubTurns []any
		for _, reply := range turns {
			stubTurns = append(stubTurns, map[string]any{"response": reply})
		}
		stub := map[string]any{"turns": stubTurns}
		if old, ok := doc["stub"].(map[string]any); ok {
			if judge, ok := old["judge"].([]any); ok {
				// A legacy reply never satisfies a judge question.
				answers := make([]any, len(judge))
				for i := range judge {
					answers[i] = "no"
				}
				stub["judge"] = answers
			}
		}
		if su, ok := doc["simulated_user"].(map[string]any); ok {
			su["max_turns"] = len(turns)
			routes := make([]any, len(turns)-1)
			for i := range routes {
				routes[i] = "none"
			}
			if len(routes) > 0 {
				stub["routes"] = routes
			}
		}
		doc["stub"] = stub

		out, err := yaml.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// runPlannerSuite runs the planner suite in evalsDir on the stub backend, in a
// throwaway project directory, and returns the run error and the recorded
// agent result. It fails the test if kiro-cli was invoked.
func runPlannerSuite(t *testing.T, evalsDir string) (error, AgentResult) {
	t.Helper()
	chdirTemp(t)
	initTempGitRepo(t)
	writeProjectFile(t, ".kiro/agents/planner.json", `{"name":"planner","model":"claude-sonnet-5.5","prompt":"inline"}`)
	gitTrack(t, ".kiro/agents/planner.json")
	t.Cleanup(resetConfig)
	_, calls := installFakeKiroCLI(t, pinFakeKiroBody)

	runErr := RunWithOptions("planner", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})

	if got := readCalls(t, calls); len(got) != 0 {
		t.Fatalf("kiro-cli was invoked: %q", got)
	}
	dirs, err := filepath.Glob(filepath.Join(evalsDir, "results", "*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("results dirs = %v (err %v), want exactly one under %s (run error: %v)", dirs, err, evalsDir, runErr)
	}
	data, err := os.ReadFile(filepath.Join(dirs[0], "planner.json"))
	if err != nil {
		t.Fatalf("run recorded no planner result (run error: %v): %v", runErr, err)
	}
	var res AgentResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	return runErr, res
}

func criterionPercent(t *testing.T, v AgentVerdict, name string) float64 {
	t.Helper()
	for _, c := range v.Criteria {
		if c.Name == name {
			return c.Percent
		}
	}
	t.Fatalf("criterion %q was not scored in any case; scored: %+v", name, v.Criteria)
	return 0
}

func TestPlannerSuiteRegistration(t *testing.T) {
	p := plannerSuitePaths(t)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: p.evals}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	got, err := ListAvailableTestCases("planner")
	if err != nil {
		t.Fatalf("ListAvailableTestCases(planner): %v", err)
	}
	sort.Strings(got)
	want := append([]string(nil), plannerCaseNames...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("planner cases = %v, want exactly %v", got, want)
	}
}

func TestPlannerSuiteCaseFilesDeclarePlannerAgent(t *testing.T) {
	p := plannerSuitePaths(t)
	files, err := filepath.Glob(filepath.Join(p.evals, "cases", "planner", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(plannerCaseNames) {
		t.Errorf("found %d case files, want %d", len(files), len(plannerCaseNames))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var head struct {
			Name  string `yaml:"name"`
			Agent string `yaml:"agent"`
		}
		if err := yaml.Unmarshal(data, &head); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		base := strings.TrimSuffix(filepath.Base(f), ".yaml")
		if head.Agent != "planner" {
			t.Errorf("%s: agent = %q, want planner", base, head.Agent)
		}
		if head.Name != base {
			t.Errorf("%s: name = %q, want it to match the file name", base, head.Name)
		}
	}
}

func TestPlannerSuiteGreenReferenceTranscriptsPass(t *testing.T) {
	p := plannerSuitePaths(t)
	evalsDir := copyPlannerSuite(t, p)

	runErr, res := runPlannerSuite(t, evalsDir)
	v := agentVerdict(res)

	if runErr != nil {
		t.Errorf("run error = %v, want nil", runErr)
	}
	if !v.Passed {
		t.Errorf("agent verdict failed: score %.1f%% (threshold %.1f%%), criteria below floor %v",
			v.Score, v.Threshold, v.CriteriaBelowFloor)
	}
	if len(res.Cases) != len(plannerCaseNames) {
		t.Errorf("ran %d cases, want %d", len(res.Cases), len(plannerCaseNames))
	}
	for _, cr := range res.Cases {
		if cr.Passed == nil || !*cr.Passed {
			t.Errorf("case %s failed in the reference run", cr.CaseName)
		}
	}
}

// stubDocuments returns, per case, the stub reply turns that contain a
// stage-output document.
func stubDocuments(t *testing.T, p plannerPaths) map[string][]string {
	t.Helper()
	if err := configure(RunOptions{Backend: "stub", EvalsDir: p.evals}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	cases, err := loadCases("planner")
	if err != nil {
		t.Fatal(err)
	}
	docStart := regexp.MustCompile(`(?m)^schema_version:`)
	out := map[string][]string{}
	for _, tc := range cases {
		out[tc.Name] = nil
		if tc.Stub == nil {
			continue
		}
		for _, turn := range tc.Stub.Turns {
			if docStart.MatchString(turn.Response) {
				out[tc.Name] = append(out[tc.Name], turn.Response)
			}
		}
	}
	return out
}

// stripYAMLFence removes one optional ```yaml fence around a document.
var plannerYAMLFence = regexp.MustCompile("(?s)\\A\\s*```ya?ml[ \\t]*\\n(.*?)\\n```\\s*\\z")

func stripYAMLFence(s string) string {
	if m := plannerYAMLFence.FindStringSubmatch(s); m != nil {
		return m[1] + "\n"
	}
	return s
}

func TestPlannerSuiteStubDocumentsAreValid(t *testing.T) {
	p := plannerSuitePaths(t)
	docs := stubDocuments(t, p)

	// Which cases must return a document, and which must not.
	wantDocs := map[string]int{
		"automated-best-effort-fill":   1,
		"automated-bug-classification": 1,
		"automated-punch-out":          1,
		"interactive-return-document":  1,
		"multi-issue-split":            1,
		"interactive-option-rules":     0,
		"interactive-single-decision":  0,
	}
	for _, name := range plannerCaseNames {
		replies := docs[name]
		if len(replies) != wantDocs[name] {
			t.Errorf("%s: %d stub replies carry a document, want %d", name, len(replies), wantDocs[name])
		}
		for i, reply := range replies {
			doc, err := stageoutput.Parse([]byte(stripYAMLFence(reply)))
			if err != nil {
				t.Errorf("%s reply %d does not parse: %v", name, i, err)
				continue
			}
			if err := doc.Validate(); err != nil {
				t.Errorf("%s reply %d does not validate: %v", name, i, err)
			}
			if doc.Stage != stageoutput.StagePlanner {
				t.Errorf("%s: stage = %q, want planner", name, doc.Stage)
			}
			if doc.Result == stageoutput.ResultPunchOut {
				if len(doc.Unresolved) == 0 {
					t.Errorf("%s: punch_out document has no unresolved entries", name)
				}
				if doc.Body != "" {
					t.Errorf("%s: punch_out document has a body", name)
				}
			}
		}
	}
}

func TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments(t *testing.T) {
	p := plannerSuitePaths(t)
	files, err := filepath.Glob(filepath.Join(p.legacy, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(plannerCaseNames) {
		t.Errorf("found %d legacy transcripts, want %d (one per case)", len(files), len(plannerCaseNames))
	}
	docStart := regexp.MustCompile(`(?m)^\s*(schema_version|stage|result)\s*:`)
	for _, name := range plannerCaseNames {
		for i, turn := range readLegacyTurns(t, p, name) {
			if docStart.MatchString(turn) {
				t.Errorf("%s turn %d contains a stage-output document", name, i+1)
			}
		}
	}
}

func TestPlannerSuiteRedLegacyTranscriptsFail(t *testing.T) {
	p := plannerSuitePaths(t)
	evalsDir := copyPlannerSuite(t, p)
	swapInLegacyTranscripts(t, p, evalsDir)

	runErr, res := runPlannerSuite(t, evalsDir)
	v := agentVerdict(res)

	if v.Passed {
		t.Errorf("agent verdict passed with legacy transcripts: score %.1f%%, want a failing verdict", v.Score)
	}
	if !errors.Is(runErr, ErrThresholdFailed) {
		t.Errorf("run error = %v, want ErrThresholdFailed", runErr)
	}
	for _, name := range []string{"output_document_validity", "two_mode_behavior"} {
		if pct := criterionPercent(t, v, name); pct >= plannerFloor {
			t.Errorf("%s = %.1f%% with legacy transcripts, want below the %.0f%% floor", name, pct, plannerFloor)
		}
		found := false
		for _, below := range v.CriteriaBelowFloor {
			found = found || below == name
		}
		if !found {
			t.Errorf("%s is not reported below the floor: %v", name, v.CriteriaBelowFloor)
		}
	}
	if len(res.Cases) != len(plannerCaseNames) {
		t.Errorf("ran %d cases, want %d", len(res.Cases), len(plannerCaseNames))
	}
	for _, cr := range res.Cases {
		if cr.Passed != nil && *cr.Passed {
			t.Errorf("case %s passed with its legacy transcript", cr.CaseName)
		}
	}
}

func TestPlannerSuiteRegexesCompile(t *testing.T) {
	p := plannerSuitePaths(t)
	files, err := filepath.Glob(filepath.Join(p.evals, "cases", "planner", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var tc struct {
			Checks []checks.Check `yaml:"checks"`
		}
		if err := yaml.Unmarshal(data, &tc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for i, c := range tc.Checks {
			switch c.Type {
			case checks.CheckOutputContains, checks.CheckOutputNotContains:
				seen++
				if _, err := regexp.Compile(c.Pattern); err != nil {
					t.Errorf("%s check #%d (%s): pattern %q does not compile: %v",
						filepath.Base(f), i+1, c.Type, c.Pattern, err)
				}
			}
		}
	}
	if seen == 0 {
		t.Error("no output_contains/output_not_contains pattern found in the planner suite")
	}
}
