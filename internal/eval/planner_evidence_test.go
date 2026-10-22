package eval

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// The planner suite asks a judge whether the planner's evidence is real. A
// judge can only answer that if it is shown the sources the evidence points
// at, and a judge check is shown only the workspace files it lists in `files`.
// These tests pin three things:
//
//   - every evidence-verification judge check in the planner suite lists
//     files (TestPlannerEvidenceChecksHaveSources);
//   - those files are the very bytes the agent was given in its prompt, held
//     in one place (TestPlannerEvidenceSourcesAreTheSetupBytes);
//   - at run time the production path (newCaseWorkspace -> EvaluateChecks)
//     hands them to the judge, so a fabricated citation is caught and a real
//     one is not (TestPlannerEvidenceJudgeLoadsSources and
//     TestPlannerEvidenceFabricatedReplyFails).
//
// A scripted stub.judge answer cannot prove any of this: it answers yes or no
// whatever it is shown. So the runtime tests use a deterministic
// citation-grounding judge, which stands in for the model and answers from
// the sources it was given.

// evidenceCaseName is the planner case whose evidence check the runtime tests
// run.
const evidenceCaseName = "automated-best-effort-fill"

// evidenceFabricatedReply is the recorded reply that cites evidence absent
// from the fixtures, relative to this package.
var evidenceFabricatedReply = filepath.Join("testdata", "planner-fabricated", evidenceCaseName+".txt")

// evidenceQuestion selects the judge checks that verify evidence against a
// source: the question asks whether something is real, grounded or evidenced.
var evidenceQuestion = regexp.MustCompile(`(?i)\bevidence\b|\bgrounded\b|\breal (quote|file)`)

// evidenceLine matches one `evidence:` line of a stage-output document, and
// evidenceCitation one path:line token inside it.
var (
	evidenceLine     = regexp.MustCompile(`(?m)^\s*evidence:.*$`)
	evidenceCitation = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(?:go|md|yaml|yml|json|sh):\d+`)
)

const (
	evidenceNoSources = "no source files were provided to verify the evidence"
	evidenceMissing   = "not found in the provided sources"
)

// groundingJudge is a deterministic stand-in for the model judge. It answers
// yes only if every path:line token on the reply's `evidence:` lines appears
// in the content of the files it was shown. With no files it says so, and for
// a citation it cannot find it names that citation, so a test can tell the
// two failures apart.
func groundingJudge() JudgeFunc {
	return func(q JudgeQuery) (JudgeVerdict, error) {
		if len(q.Files) == 0 {
			return JudgeVerdict{Yes: false, Reasoning: evidenceNoSources}, nil
		}
		var sources strings.Builder
		for _, f := range q.Files {
			sources.WriteString(f.Content)
			sources.WriteString("\n")
		}
		var citations []string
		for _, line := range evidenceLine.FindAllString(q.Output, -1) {
			citations = append(citations, evidenceCitation.FindAllString(line, -1)...)
		}
		if len(citations) == 0 {
			return JudgeVerdict{Yes: false, Reasoning: "the reply cites no path:line evidence"}, nil
		}
		for _, c := range citations {
			if !strings.Contains(sources.String(), c) {
				return JudgeVerdict{Yes: false, Reasoning: fmt.Sprintf("citation %s %s", c, evidenceMissing)}, nil
			}
		}
		return JudgeVerdict{Yes: true, Reasoning: "every cited path:line appears in the provided sources"}, nil
	}
}

// evidenceCheck is one evidence-verification judge check and the case it is in.
type evidenceCheck struct {
	tc    TestCase
	check Check
}

// loadPlannerEvidenceChecks loads the shipped planner cases and returns every
// judge check that verifies evidence against a source.
func loadPlannerEvidenceChecks(t *testing.T) []evidenceCheck {
	t.Helper()
	p := plannerSuitePaths(t)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: p.evals}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	cases, err := loadCases("planner")
	if err != nil {
		t.Fatalf("loadCases(planner): %v", err)
	}
	var out []evidenceCheck
	for _, tc := range cases {
		for _, c := range tc.Checks {
			if c.Type == CheckJudge && evidenceQuestion.MatchString(c.Question) {
				out = append(out, evidenceCheck{tc: tc, check: c})
			}
		}
	}
	return out
}

func TestPlannerEvidenceChecksHaveSources(t *testing.T) {
	found := loadPlannerEvidenceChecks(t)

	// The audit must not pass vacuously: the suite has (at least) the
	// best-effort-fill and the return-document evidence checks.
	if len(found) < 2 {
		t.Fatalf("found %d evidence-verification judge checks in the planner suite, want at least 2", len(found))
	}
	cases := map[string]bool{}
	for _, e := range found {
		cases[e.tc.Name] = true
		if len(e.check.Files) == 0 {
			t.Errorf("case %s: evidence-verification judge check lists no files, so the judge cannot verify any citation: %q",
				e.tc.Name, e.check.Question)
		}
	}
	for _, want := range []string{"automated-best-effort-fill", "interactive-return-document"} {
		if !cases[want] {
			t.Errorf("no evidence-verification judge check found in case %s", want)
		}
	}
}

func TestPlannerEvidenceSourcesAreTheSetupBytes(t *testing.T) {
	found := loadPlannerEvidenceChecks(t)
	if len(found) == 0 {
		t.Fatal("no evidence-verification judge checks found")
	}
	for _, e := range found {
		tc := e.tc
		if tc.Workspace == "" {
			t.Errorf("case %s: no workspace fixture is set, so the judge's files cannot resolve", tc.Name)
			continue
		}
		prompt, err := assemblePrompt(tc.Setup, tc.Input)
		if err != nil {
			t.Fatalf("case %s: assemblePrompt: %v", tc.Name, err)
		}
		fixtureRel := ".kairon/evals/fixtures/workspaces/" + tc.Workspace + "/"
		for _, f := range e.check.Files {
			fixture := filepath.Join(workspaceFixtureDir(tc.Workspace), filepath.FromSlash(f))
			want, err := os.ReadFile(fixture)
			if err != nil {
				t.Errorf("case %s: judge file %s is not in the workspace fixture: %v", tc.Name, f, err)
				continue
			}
			// The agent's prompt must be built from this same file.
			var setup *SetupEntry
			for i := range tc.Setup {
				if tc.Setup[i].Type == "file" && tc.Setup[i].Path == fixtureRel+f {
					setup = &tc.Setup[i]
				}
			}
			if setup == nil {
				t.Errorf("case %s: no setup entry has path %s, so the agent and the judge do not read one file", tc.Name, fixtureRel+f)
				continue
			}
			if got, err := os.ReadFile(rebaseEvalsPath(setup.Path)); err != nil || string(got) != string(want) {
				t.Errorf("case %s: setup file %s differs from the judge's file (read error: %v)", tc.Name, setup.Path, err)
			}
			if !strings.Contains(prompt, string(want)) {
				t.Errorf("case %s: the agent prompt does not contain the exact bytes of %s", tc.Name, f)
			}
		}
	}
}

// recordingJudge wraps a JudgeFunc and records every query it is asked.
type recordingJudge struct {
	inner   JudgeFunc
	queries []JudgeQuery
}

func (r *recordingJudge) Func() JudgeFunc {
	return func(q JudgeQuery) (JudgeVerdict, error) {
		r.queries = append(r.queries, q)
		return r.inner(q)
	}
}

// evidenceRun is the outcome of running the best-effort-fill evidence check on
// one reply against a real case workspace.
type evidenceRun struct {
	result  CheckResult
	queries []JudgeQuery
	tc      TestCase
}

// runEvidenceCheck stages the real workspace of the best-effort-fill case and
// runs its real evidence judge check on output through EvaluateChecks, with the
// grounding judge as the judge. A nil output means the case's reference reply.
func runEvidenceCheck(t *testing.T, output *string) evidenceRun {
	t.Helper()
	var ec *evidenceCheck
	found := loadPlannerEvidenceChecks(t)
	for i := range found {
		if found[i].tc.Name == evidenceCaseName {
			ec = &found[i]
			break
		}
	}
	if ec == nil {
		t.Fatalf("case %s has no evidence-verification judge check", evidenceCaseName)
	}
	reply := ""
	if output != nil {
		reply = *output
	} else {
		if ec.tc.Stub == nil || len(ec.tc.Stub.Turns) == 0 {
			t.Fatalf("case %s has no stub reply", ec.tc.Name)
		}
		reply = ec.tc.Stub.Turns[0].Response
	}
	ws, err := newCaseWorkspace(ec.tc)
	if err != nil {
		t.Fatalf("newCaseWorkspace(%s): %v", ec.tc.Name, err)
	}
	t.Cleanup(func() { _ = ws.Remove() })

	rec := &recordingJudge{inner: groundingJudge()}
	results := EvaluateChecks([]Check{ec.check}, CheckInput{
		Dir:    ws.Dir,
		Output: reply,
		Input:  judgeCaseInput(ec.tc),
		Judge:  rec.Func(),
	})
	if len(results) != 1 {
		t.Fatalf("got %d check results, want 1", len(results))
	}
	return evidenceRun{result: results[0], queries: rec.queries, tc: ec.tc}
}

func TestPlannerEvidenceJudgeLoadsSources(t *testing.T) {
	run := runEvidenceCheck(t, nil)

	if !run.result.Passed {
		t.Errorf("the reference reply fails its evidence check: %s", run.result.Detail)
	}
	if strings.Contains(run.result.Detail, "file does not exist") {
		t.Errorf("a judge file is missing from the workspace: %s", run.result.Detail)
	}
	if len(run.queries) != 1 {
		t.Fatalf("the judge was asked %d times, want 1 (check detail: %s)", len(run.queries), run.result.Detail)
	}
	files := run.queries[0].Files
	wantPaths := []string{"planner-intake-feature-issue.md", "planner-repo-context.md"}
	var gotPaths []string
	for _, f := range files {
		gotPaths = append(gotPaths, f.Path)
	}
	if strings.Join(gotPaths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("the judge was shown files %v, want %v", gotPaths, wantPaths)
	}
	for _, f := range files {
		// Each file the judge saw is the file the agent's prompt was built from.
		var want []byte
		for _, s := range run.tc.Setup {
			if s.Type == "file" && filepath.Base(s.Path) == f.Path {
				b, err := os.ReadFile(rebaseEvalsPath(s.Path))
				if err != nil {
					t.Fatal(err)
				}
				want = b
			}
		}
		if want == nil {
			t.Errorf("no setup file named %s", f.Path)
			continue
		}
		if f.Content != string(want) || f.Truncated {
			t.Errorf("the judge saw different content for %s than the setup file (truncated: %v)", f.Path, f.Truncated)
		}
	}
}

// plannerFixtureText returns the concatenated text of every planner fixture
// (planner-*.md) below the shipped evals fixtures directory, wherever it sits.
func plannerFixtureText(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", ".kairon", "evals", "fixtures")
	var sb strings.Builder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasPrefix(d.Name(), "planner-") && strings.HasSuffix(d.Name(), ".md") {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sb.Write(b)
			sb.WriteString("\n")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestPlannerEvidenceFabricatedReplyFails(t *testing.T) {
	data, err := os.ReadFile(evidenceFabricatedReply)
	if err != nil {
		t.Fatal(err)
	}
	fabricated := string(data)

	// The sample must be a well-formed planner document, so that only its
	// evidence is wrong.
	doc, err := stageoutput.Parse(data)
	if err != nil {
		t.Fatalf("the fabricated sample does not parse: %v", err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("the fabricated sample does not validate: %v", err)
	}
	if doc.Stage != stageoutput.StagePlanner || doc.Result != stageoutput.ResultCanonical {
		t.Fatalf("the fabricated sample is %s/%s, want planner/canonical", doc.Stage, doc.Result)
	}

	// It cites a file, a line and a quote that the fixtures do not have.
	fixtures := plannerFixtureText(t)
	if fixtures == "" {
		t.Fatal("found no planner fixtures to compare the fabricated sample against")
	}
	for _, invented := range []string{
		"internal/export/ndjson.go:12",
		"internal/export/csv.go:99",
		"every exporter must be covered by a table-driven test with a golden file",
	} {
		if !strings.Contains(fabricated, invented) {
			t.Errorf("the fabricated sample does not cite %q", invented)
		}
		if strings.Contains(fixtures, invented) {
			t.Errorf("%q is present in the fixtures, so it is not fabricated", invented)
		}
	}

	// The unmodified reference reply passes, so the failure below is
	// attributable to the fabricated citations only.
	if ref := runEvidenceCheck(t, nil); !ref.result.Passed {
		t.Errorf("the unmodified reference reply fails the evidence check: %s", ref.result.Detail)
	}

	run := runEvidenceCheck(t, &fabricated)
	if run.result.Passed {
		t.Errorf("the fabricated reply passes the evidence check: %s", run.result.Detail)
	}
	for _, want := range []string{"judge answered no", evidenceMissing, "internal/export/ndjson.go:12"} {
		if !strings.Contains(run.result.Detail, want) {
			t.Errorf("check detail does not contain %q: %s", want, run.result.Detail)
		}
	}
	if strings.Contains(run.result.Detail, "no source files") {
		t.Errorf("the judge was given no sources, so it did not reject the reply on its citations: %s", run.result.Detail)
	}
}
