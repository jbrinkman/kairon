package eval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runFixturesDir holds committed result-directory fixtures. It is deliberately
// not under testdata/evals/results/, which is git-ignored run output.
const runFixturesDir = "testdata/evals/fixtures/runs"

// copyRunFixture copies the committed fixture run `fixture` into the temp
// evals results dir of the cwd (call chdirTemp first) as `name`, so tests
// never read or write .kairon/evals/results/ of the repository.
func copyRunFixture(t *testing.T, fixture, name string) {
	t.Helper()
	// chdirTemp has already moved the cwd, so resolve the fixture against the
	// package directory captured at init time.
	src := filepath.Join(packageDir, runFixturesDir, fixture)
	dst := filepath.Join(".kairon", "evals", "results", name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
}

// packageDir is the package source directory, captured before any test
// changes the working directory.
var packageDir = func() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}()

// provRun describes a synthetic result directory for diff tests.
type provRun struct {
	summary Summary
	agents  map[string]AgentResult
	// raw agent files written verbatim (e.g. malformed or legacy shapes)
	raw map[string]string
}

// writeProvRun writes run under the default evals results dir of the cwd.
func writeProvRun(t *testing.T, name string, r provRun) {
	t.Helper()
	dir := filepath.Join(".kairon", "evals", "results", name)
	data, err := json.Marshal(r.summary)
	if err != nil {
		t.Fatal(err)
	}
	writeCfgFile(t, filepath.Join(dir, "summary.json"), string(data))
	for agent, res := range r.agents {
		data, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		writeCfgFile(t, filepath.Join(dir, agent+".json"), string(data))
	}
	for file, content := range r.raw {
		writeCfgFile(t, filepath.Join(dir, file), content)
	}
}

func provSummary(mode string, agentScores map[string]float64) Summary {
	return Summary{
		GitHash:     "abc",
		AgentScores: agentScores,
		JudgeModel:  "judge-1",
		Sandbox:     mode,
		Agents: map[string]AgentProvenance{
			"a1": {AgentModel: "model-1", PromptSHA256: "sha-1"},
		},
	}
}

func trustContainment(trust ...string) *Containment {
	if trust == nil {
		trust = []string{}
	}
	return &Containment{ToolTrust: trust, FakeGH: true, ReadOnlyFS: true, Network: "unrestricted"}
}

func runDiff(t *testing.T, a, b string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := diffTo(&buf, a, b); err != nil {
		t.Fatalf("diffTo(%s, %s): %v", a, b, err)
	}
	return buf.String()
}

func TestDiffNativeVsContainerReportsModeDifference(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	writeProvRun(t, "native-run", provRun{summary: provSummary("native", scores)})
	cs := provSummary("container", scores)
	cs.Containment = map[string]Containment{"a1": *trustContainment("read")}
	writeProvRun(t, "container-run", provRun{summary: cs})

	out := runDiff(t, "native-run", "container-run")

	for _, want := range []string{
		"Run Provenance:",
		"native-run: mode native",
		"container-run: mode container",
		"sandbox: native → container",
		"scores are not directly comparable",
		"⚠",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The provenance block precedes the existing deltas.
	if strings.Index(out, "Run Provenance:") > strings.Index(out, "a1: 0.500") {
		t.Errorf("provenance block must come before the per-agent deltas:\n%s", out)
	}
	for _, want := range []string{"Cost Delta:", "Cost Trends:"} {
		if !strings.Contains(out, want) {
			t.Errorf("existing output %q missing:\n%s", want, out)
		}
	}
}

func TestDiffSameModeNoWarning(t *testing.T) {
	for _, mode := range []string{"native", "container"} {
		t.Run(mode, func(t *testing.T) {
			chdirTemp(t)
			scores := map[string]float64{"a1": 0.5}
			writeProvRun(t, "r1", provRun{summary: provSummary(mode, scores)})
			writeProvRun(t, "r2", provRun{summary: provSummary(mode, scores)})

			out := runDiff(t, "r1", "r2")
			if !strings.Contains(out, "r1: mode "+mode) || !strings.Contains(out, "r2: mode "+mode) {
				t.Errorf("both modes should be printed:\n%s", out)
			}
			for _, bad := range []string{"⚠", "not directly comparable", "sandbox:"} {
				if strings.Contains(out, bad) {
					t.Errorf("same-mode diff must not contain %q:\n%s", bad, out)
				}
			}
			if !strings.Contains(out, "No provenance differences.") {
				t.Errorf("expected no-differences line:\n%s", out)
			}
		})
	}
}

func TestDiffListsContainmentDifference(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	sa := provSummary("container", scores)
	sa.Containment = map[string]Containment{"a1": *trustContainment("read")}
	sb := provSummary("container", scores)
	other := trustContainment("read", "shell")
	other.FakeGH = false
	sb.Containment = map[string]Containment{"a1": *other}
	writeProvRun(t, "r1", provRun{summary: sa})
	writeProvRun(t, "r2", provRun{summary: sb})

	out := runDiff(t, "r1", "r2")
	for _, want := range []string{
		"a1 containment.tool_trust: [read] → [read,shell]",
		"a1 containment.fake_gh: true → false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "⚠") {
		t.Errorf("same mode must not warn:\n%s", out)
	}
}

func TestDiffContainmentNoneVsPresent(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	sb := provSummary("container", scores)
	sb.Containment = map[string]Containment{"a1": *trustContainment()}
	writeProvRun(t, "r1", provRun{summary: provSummary("native", scores)})
	writeProvRun(t, "r2", provRun{summary: sb})

	out := runDiff(t, "r1", "r2")
	want := "a1 containment.record: none → tool_trust=[] fake_gh=true read_only_fs=true network=unrestricted"
	if !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestDiffListsProvenanceDifferencesPerAgent(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5, "a2": 0.6}
	sa := provSummary("native", scores)
	sa.Agents["a2"] = AgentProvenance{AgentModel: "model-x", PromptSHA256: "sha-x"}
	sb := provSummary("native", scores)
	sb.JudgeModel = "judge-2"
	sb.Agents = map[string]AgentProvenance{
		"a1": {AgentModel: "model-2", PromptSHA256: "sha-1"}, // model only
		"a2": {AgentModel: "model-x", PromptSHA256: "sha-y"}, // sha only
	}
	writeProvRun(t, "r1", provRun{summary: sa})
	writeProvRun(t, "r2", provRun{summary: sb})

	out := runDiff(t, "r1", "r2")
	for _, want := range []string{
		"judge_model: judge-1 → judge-2",
		"a1 agent_model: model-1 → model-2",
		"a2 prompt_sha256: sha-x → sha-y",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"a1 prompt_sha256", "a2 agent_model", "⚠"} {
		if strings.Contains(out, bad) {
			t.Errorf("unexpected %q:\n%s", bad, out)
		}
	}
}

func TestDiffLegacyRunShowsUnknownMode(t *testing.T) {
	chdirTemp(t)
	// Written before any provenance or mode tracking.
	legacy := provRun{
		summary: Summary{GitHash: "old", AgentScores: map[string]float64{"a1": 0.4}},
		raw:     map[string]string{"a1.json": `{"agent":"a1","git_hash":"old","cases":[]}`},
	}
	writeProvRun(t, "legacy", legacy)
	writeProvRun(t, "legacy2", legacy)

	out := runDiff(t, "legacy", "legacy2")
	if !strings.Contains(out, "legacy: mode "+modeUnknown) {
		t.Errorf("legacy run should show unknown mode:\n%s", out)
	}
	for _, bad := range []string{"⚠", "not directly comparable"} {
		if strings.Contains(out, bad) {
			t.Errorf("two unknown-mode runs must not warn (%q):\n%s", bad, out)
		}
	}
}

func TestDiffLegacyVsContainerDoesNotFail(t *testing.T) {
	chdirTemp(t)
	writeProvRun(t, "legacy", provRun{
		summary: Summary{GitHash: "old", AgentScores: map[string]float64{"a1": 0.4}},
		raw:     map[string]string{"a1.json": `{"agent":"a1","git_hash":"old","cases":[]}`},
	})
	cs := provSummary("container", map[string]float64{"a1": 0.5})
	cs.Containment = map[string]Containment{"a1": *trustContainment()}
	writeProvRun(t, "container", provRun{summary: cs})

	out := runDiff(t, "legacy", "container")
	if !strings.Contains(out, "legacy: mode "+modeUnknown) || !strings.Contains(out, "container: mode container") {
		t.Errorf("modes not shown:\n%s", out)
	}
	if strings.Contains(out, "not directly comparable") {
		t.Errorf("unknown mode must not assert incomparability:\n%s", out)
	}
	if !strings.Contains(out, "execution mode of at least one run is unknown") {
		t.Errorf("expected unknown-mode note:\n%s", out)
	}
}

func TestDeriveMode(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		agents  []*bool
		want    string
	}{
		{"summary wins", "container", []*bool{boolPtr(false)}, "container"},
		{"agents all native", "", []*bool{boolPtr(false), boolPtr(false)}, "native"},
		{"agents all container", "", []*bool{boolPtr(true)}, "container"},
		{"agents disagree", "", []*bool{boolPtr(true), boolPtr(false)}, modeUnknown},
		{"one agent legacy", "", []*bool{boolPtr(true), nil}, modeUnknown},
		{"no agents", "", nil, modeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveMode(tt.summary, tt.agents); got != tt.want {
				t.Errorf("deriveMode = %q, want %q", got, tt.want)
			}
		})
	}
}

// A summary without sandbox falls back to the agent files' sandbox bool.
func TestDiffModeFallsBackToAgentFiles(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	mk := func(sandbox bool) provRun {
		return provRun{
			summary: Summary{GitHash: "x", AgentScores: scores},
			agents:  map[string]AgentResult{"a1": {Agent: "a1", Sandbox: boolPtr(sandbox), Containment: containmentIf(sandbox)}},
		}
	}
	writeProvRun(t, "n", mk(false))
	writeProvRun(t, "c", mk(true))

	out := runDiff(t, "n", "c")
	for _, want := range []string{"n: mode native", "c: mode container", "sandbox: native → container", "not directly comparable"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Containment recorded only in the agent file is still compared.
	if !strings.Contains(out, "a1 containment.record: none →") {
		t.Errorf("agent-file containment not compared:\n%s", out)
	}
}

func containmentIf(container bool) *Containment {
	if container {
		return trustContainment()
	}
	return nil
}

// Malformed agent files are skipped; the diff still succeeds.
func TestDiffMalformedAgentFileDoesNotFail(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	r := provRun{
		summary: provSummary("native", scores),
		raw:     map[string]string{"a1.json": `{not json`},
	}
	writeProvRun(t, "r1", r)
	writeProvRun(t, "r2", r)
	out := runDiff(t, "r1", "r2")
	if !strings.Contains(out, "r1: mode native") {
		t.Errorf("mode should still come from summary:\n%s", out)
	}
}

// diffTo must write only to the supplied writer.
func TestDiffToWritesOnlyToWriter(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	writeProvRun(t, "r1", provRun{summary: provSummary("native", scores)})
	writeProvRun(t, "r2", provRun{summary: provSummary("native", scores)})

	origStdout := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	var buf bytes.Buffer
	diffErr := diffTo(&buf, "r1", "r2")
	os.Stdout = origStdout
	_ = wr.Close()
	leaked, _ := readAll(rd)
	_ = rd.Close()

	if diffErr != nil {
		t.Fatal(diffErr)
	}
	if leaked != "" {
		t.Errorf("diffTo wrote to stdout: %q", leaked)
	}
	if !strings.Contains(buf.String(), "Eval Diff: r1 → r2") {
		t.Errorf("writer missing output:\n%s", buf.String())
	}
}

func readAll(f *os.File) (string, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(f)
	return b.String(), err
}

// Diff itself still writes to stdout and succeeds.
func TestDiffStillWritesToStdout(t *testing.T) {
	chdirTemp(t)
	scores := map[string]float64{"a1": 0.5}
	writeProvRun(t, "r1", provRun{summary: provSummary("native", scores)})
	writeProvRun(t, "r2", provRun{summary: provSummary("container", scores)})

	origStdout := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	diffErr := Diff("r1", "r2")
	os.Stdout = origStdout
	_ = wr.Close()
	out, _ := readAll(rd)
	_ = rd.Close()

	if diffErr != nil {
		t.Fatal(diffErr)
	}
	if !strings.Contains(out, "Run Provenance:") || !strings.Contains(out, "not directly comparable") {
		t.Errorf("Diff stdout missing provenance block:\n%s", out)
	}
}

// The committed fixtures keep their documented shapes: legacy-native predates
// every provenance and mode field; native-pinned has provenance but no
// sandbox fields.
func TestRunFixturesShapes(t *testing.T) {
	chdirTemp(t)
	copyRunFixture(t, "legacy-native", "legacy")
	copyRunFixture(t, "native-pinned", "pinned")
	dir := filepath.Join(".kairon", "evals", "results")

	var ls, ps Summary
	readJSON(t, filepath.Join(dir, "legacy", "summary.json"), &ls)
	readJSON(t, filepath.Join(dir, "pinned", "summary.json"), &ps)
	if ls.JudgeModel != "" || ls.Agents != nil || ls.Sandbox != "" || ls.Containment != nil {
		t.Errorf("legacy summary carries post-#294 fields: %+v", ls)
	}
	if ps.JudgeModel == "" || ps.Agents["architect"].PromptSHA256 == "" || ps.Sandbox != "" || ps.Containment != nil {
		t.Errorf("native-pinned summary shape wrong: %+v", ps)
	}

	var la, pa AgentResult
	readJSON(t, filepath.Join(dir, "legacy", "architect.json"), &la)
	readJSON(t, filepath.Join(dir, "pinned", "architect.json"), &pa)
	if la.AgentModel != "" || la.PromptSHA256 != "" || la.Sandbox != nil || la.Containment != nil {
		t.Errorf("legacy agent file carries post-#294 fields: %+v", la)
	}
	if pa.AgentModel == "" || pa.JudgeModel == "" || pa.PromptSHA256 == "" || pa.Sandbox != nil || pa.Containment != nil {
		t.Errorf("native-pinned agent file shape wrong: %+v", pa)
	}

	// Skipped criteria count as 0 in the shared totals: legacy 6/12 (its skipped
	// criterion keeps its max in the denominator), pinned 7/8.
	if s, m := agentScoreTotals(la); s != 6 || m != 12 {
		t.Errorf("legacy totals = %v/%v, want 6/12", s, m)
	}
	if s, m := agentScoreTotals(pa); s != 7 || m != 8 {
		t.Errorf("native-pinned totals = %v/%v, want 7/8", s, m)
	}
}

// Legacy fixture vs a container run: unknown mode, no error, no false claim
// of incomparability.
func TestDiffLegacyFixtureVsContainer(t *testing.T) {
	chdirTemp(t)
	copyRunFixture(t, "legacy-native", "legacy")
	cs := provSummary("container", map[string]float64{"architect": 0.8})
	cs.Containment = map[string]Containment{"architect": *trustContainment("read")}
	writeProvRun(t, "container", provRun{summary: cs})

	out := runDiff(t, "legacy", "container")
	for _, want := range []string{
		"legacy: mode " + modeUnknown,
		"container: mode container",
		"execution mode of at least one run is unknown",
		"architect containment.record: none →",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "not directly comparable") || strings.Contains(out, "⚠") {
		t.Errorf("unknown mode must not assert incomparability:\n%s", out)
	}
}

// Legacy vs native-pinned: both predate mode tracking; the post-#294
// provenance the legacy run lacks is listed as a difference, never an error.
func TestDiffLegacyFixtureVsNativePinned(t *testing.T) {
	chdirTemp(t)
	copyRunFixture(t, "legacy-native", "legacy")
	copyRunFixture(t, "native-pinned", "pinned")

	out := runDiff(t, "legacy", "pinned")
	for _, want := range []string{
		"legacy: mode " + modeUnknown,
		"pinned: mode " + modeUnknown,
		"judge_model: " + notRecorded + " → claude-sonnet-5.5",
		"architect agent_model: " + notRecorded + " → claude-sonnet-5.5",
		"architect prompt_sha256: " + notRecorded + " → 1111",
		"architect: 0.750 → 0.875",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "⚠") || strings.Contains(out, "sandbox:") {
		t.Errorf("two unknown-mode runs must not report a mode difference:\n%s", out)
	}
}

// Legacy vs a run that recorded its mode: unknown on one side only.
func TestDiffLegacyFixtureVsRecordedNative(t *testing.T) {
	chdirTemp(t)
	copyRunFixture(t, "legacy-native", "legacy")
	writeProvRun(t, "native", provRun{summary: provSummary("native", map[string]float64{"architect": 0.8})})

	out := runDiff(t, "legacy", "native")
	if !strings.Contains(out, "legacy: mode "+modeUnknown) || !strings.Contains(out, "native: mode native") {
		t.Errorf("modes not shown:\n%s", out)
	}
	if strings.Contains(out, "not directly comparable") {
		t.Errorf("unknown mode must not assert incomparability:\n%s", out)
	}
}

// Same-mode (here: identical) fixture runs report no differences at all.
func TestDiffFixtureSameRunNoDifferences(t *testing.T) {
	for _, fixture := range []string{"legacy-native", "native-pinned"} {
		t.Run(fixture, func(t *testing.T) {
			chdirTemp(t)
			copyRunFixture(t, fixture, "r1")
			copyRunFixture(t, fixture, "r2")
			out := runDiff(t, "r1", "r2")
			if !strings.Contains(out, "No provenance differences.") {
				t.Errorf("expected no differences:\n%s", out)
			}
			for _, bad := range []string{"⚠", "not directly comparable", "sandbox:", "Note:"} {
				if strings.Contains(out, bad) {
					t.Errorf("same-run diff must not contain %q:\n%s", bad, out)
				}
			}
		})
	}
}
