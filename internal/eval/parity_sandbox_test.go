package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

// parityRun is one completed eval run read back from its result directory.
type parityRun struct {
	name    string // result directory name under <evals>/results
	agent   AgentResult
	summary Summary
}

// runParity runs agent with the stub backend, natively or with --sandbox, in
// evalsDir and reads back <agent>.json and summary.json.
func runParity(t *testing.T, evalsDir, agent string, useSandbox bool) parityRun {
	t.Helper()
	resultsDir := filepath.Join(evalsDir, "results")
	before := snapshotDirs(t, resultsDir)

	if err := RunWithOptions(agent, "", RunOptions{
		Backend:   "stub",
		EvalsDir:  evalsDir,
		Sandbox:   useSandbox,
		NoSandbox: !useSandbox,
	}); err != nil {
		t.Fatalf("%s run (sandbox=%v) failed: %v", agent, useSandbox, err)
	}
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("%s run (sandbox=%v) created %d result dirs, want 1: %v", agent, useSandbox, len(added), added)
	}

	run := parityRun{name: added[0]}
	run.agent = readSelfTestResult(t, filepath.Join(resultsDir, run.name, agent+".json"))
	readJSON(t, filepath.Join(resultsDir, run.name, "summary.json"), &run.summary)

	// Result directories are named to the second; make sure the next run in
	// this shared results dir cannot reuse this name.
	time.Sleep(1100 * time.Millisecond)
	return run
}

// provenanceOf is the provenance block shared by <agent>.json and summary.json
// as one comparable value.
type provenanceOf struct {
	AgentModel       string
	JudgeModel       string
	PromptSHA256     string
	ResourcesPresent []string
}

// TestProvenanceParitySandbox runs selftest and selftest-fail natively and
// under --sandbox (stub backend inside the container) and requires:
//   - identical provenance in <agent>.json, summary.json and every calls[] record;
//   - summary sandbox native without containment vs container with it;
//   - an identical whole Summary once sandbox and containment are zeroed;
//   - equal per-case score totals and threshold outcome;
//   - a successful diffTo that reports the execution-mode difference.
//
// Gated like TestSelftestSandbox: it needs KAIRON_EVAL_SANDBOX_SELFTEST=1 and a
// reachable Podman or Docker daemon, and otherwise skips (a skip is not a pass)
// without starting anything.
func TestProvenanceParitySandbox(t *testing.T) {
	if os.Getenv(sandboxSelftestEnv) != "1" {
		t.Skipf("provenance parity sandbox test is opt-in: set %s=1 or run `task eval:selftest:sandbox` (needs Podman or Docker)", sandboxSelftestEnv)
	}
	if err := sandbox.EnsureContainerDaemon(); err != nil {
		t.Skipf("no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the provenance parity test: %v", err)
	}

	t.Cleanup(resetConfig)
	t.Chdir(repoRoot(t))
	evalsDir := copyEvalsFromRepoRoot(t)

	for _, agent := range []string{"selftest", "selftest-fail"} {
		t.Run(agent, func(t *testing.T) {
			native := runParity(t, evalsDir, agent, false)
			container := runParity(t, evalsDir, agent, true)

			t.Run("provenance", func(t *testing.T) { assertProvenanceParity(t, native, container) })
			t.Run("mode and containment", func(t *testing.T) { assertModeAndContainment(t, evalsDir, agent, native, container) })
			t.Run("summary", func(t *testing.T) { assertSummaryParity(t, native, container) })
			t.Run("scoring", func(t *testing.T) { assertScoringParity(t, evalsDir, agent, native, container) })
			t.Run("diff", func(t *testing.T) { assertDiffReportsModeDifference(t, evalsDir, native, container) })
		})
	}
}

func assertProvenanceParity(t *testing.T, native, container parityRun) {
	t.Helper()
	agentProv := func(r AgentResult) provenanceOf {
		return provenanceOf{r.AgentModel, r.JudgeModel, r.PromptSHA256, r.ResourcesPresent}
	}
	sumProv := func(s Summary) provenanceOf {
		return provenanceOf{s.AgentModel, s.JudgeModel, s.PromptSHA256, s.ResourcesPresent}
	}

	// <agent>.json
	np, cp := agentProv(native.agent), agentProv(container.agent)
	if np.AgentModel == "" || np.JudgeModel == "" || np.PromptSHA256 == "" {
		t.Fatalf("native run is not pinned, nothing to compare: %+v", np)
	}
	if !reflect.DeepEqual(np, cp) {
		t.Errorf("<agent>.json provenance differs\n native: %+v\ncontainer: %+v", np, cp)
	}

	// summary.json: top-level fields and the per-agent map.
	if ns, cs := sumProv(native.summary), sumProv(container.summary); !reflect.DeepEqual(ns, cs) {
		t.Errorf("summary.json provenance differs\n native: %+v\ncontainer: %+v", ns, cs)
	}
	if !reflect.DeepEqual(native.summary.Agents, container.summary.Agents) {
		t.Errorf("summary.json agents differ\n native: %+v\ncontainer: %+v", native.summary.Agents, container.summary.Agents)
	}
	// The result file and the summary agree within each run.
	for name, r := range map[string]parityRun{"native": native, "container": container} {
		if got := sumProv(r.summary); !reflect.DeepEqual(got, agentProv(r.agent)) {
			t.Errorf("%s: summary.json provenance %+v != <agent>.json provenance %+v", name, got, agentProv(r.agent))
		}
	}

	// Every calls[] record.
	if len(native.agent.Cases) != len(container.agent.Cases) {
		t.Fatalf("native has %d cases, container has %d", len(native.agent.Cases), len(container.agent.Cases))
	}
	containerCases := map[string]CaseResult{}
	for _, c := range container.agent.Cases {
		containerCases[c.CaseName] = c
	}
	for _, nc := range native.agent.Cases {
		cc, ok := containerCases[nc.CaseName]
		if !ok {
			t.Errorf("case %q missing from the container run", nc.CaseName)
			continue
		}
		if len(nc.Calls) != len(cc.Calls) {
			t.Errorf("case %s: native has %d calls, container has %d", nc.CaseName, len(nc.Calls), len(cc.Calls))
			continue
		}
		for i := range nc.Calls {
			n, c := nc.Calls[i], cc.Calls[i]
			if n.Role != c.Role || n.Agent != c.Agent || n.Criterion != c.Criterion ||
				n.Model != c.Model || n.PromptSHA256 != c.PromptSHA256 {
				t.Errorf("case %s call %d provenance differs\n native: %+v\ncontainer: %+v", nc.CaseName, i, n, c)
			}
			if n.Role == "agent" && n.PromptSHA256 != np.PromptSHA256 {
				t.Errorf("case %s call %d: prompt_sha256 %q != run prompt_sha256 %q", nc.CaseName, i, n.PromptSHA256, np.PromptSHA256)
			}
		}
	}
}

func assertModeAndContainment(t *testing.T, evalsDir, agent string, native, container parityRun) {
	t.Helper()
	// Resolve the trust set against the same evals dir the runs used.
	if err := configure(RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatal(err)
	}
	if native.agent.Sandbox == nil || *native.agent.Sandbox || native.agent.Containment != nil {
		t.Errorf("native %s.json: sandbox=%v containment=%v, want false/nil", agent, native.agent.Sandbox, native.agent.Containment)
	}
	if container.agent.Sandbox == nil || !*container.agent.Sandbox {
		t.Errorf("container %s.json: sandbox=%v, want true", agent, container.agent.Sandbox)
	}

	if native.summary.Sandbox != string(RunModeNative) || native.summary.Containment != nil {
		t.Errorf("native summary: sandbox=%q containment=%v, want %q/nil", native.summary.Sandbox, native.summary.Containment, RunModeNative)
	}
	if container.summary.Sandbox != string(RunModeContainer) {
		t.Errorf("container summary sandbox = %q, want %q", container.summary.Sandbox, RunModeContainer)
	}

	// The containment block: tool_trust equals the trust set the agent calls ran with.
	trust, err := resolveTrustSet(agent)
	if err != nil {
		t.Fatal(err)
	}
	want := Containment{ToolTrust: trust.Names(), FakeGH: true, ReadOnlyFS: true, Network: "unrestricted"}
	got, ok := container.summary.Containment[agent]
	if !ok {
		t.Fatalf("container summary has no containment entry for %s: %v", agent, container.summary.Containment)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("summary containment[%s] = %+v, want %+v", agent, got, want)
	}
	if container.agent.Containment == nil || !reflect.DeepEqual(*container.agent.Containment, want) {
		t.Errorf("%s.json containment = %+v, want %+v", agent, container.agent.Containment, want)
	}
	for _, c := range container.agent.Cases {
		for _, call := range c.Calls {
			if call.Role != "agent" || call.TrustedTools == nil {
				continue
			}
			if !reflect.DeepEqual(append([]string{}, *call.TrustedTools...), append([]string{}, want.ToolTrust...)) {
				t.Errorf("case %s: call trusted_tools %v != containment.tool_trust %v", c.CaseName, *call.TrustedTools, want.ToolTrust)
			}
		}
	}
}

func assertSummaryParity(t *testing.T, native, container parityRun) {
	t.Helper()
	n, c := native.summary, container.summary
	// Only the fields that are meant to differ are cleared; everything else,
	// including agent_scores, total cost and any threshold fields (E7), is
	// compared as part of the whole value.
	n.Sandbox, n.Containment = "", nil
	c.Sandbox, c.Containment = "", nil
	if !reflect.DeepEqual(n, c) {
		t.Errorf("summaries differ beyond sandbox/containment\n native: %+v\ncontainer: %+v", n, c)
	}
}

func assertScoringParity(t *testing.T, evalsDir, agent string, native, container parityRun) {
	t.Helper()
	if err := configure(RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatal(err)
	}
	cases, err := loadCases(agent)
	if err != nil {
		t.Fatal(err)
	}
	thresholds := map[string]float64{}
	for _, tc := range cases {
		thresholds[tc.Name] = getThreshold(tc)
	}

	index := func(r AgentResult) map[string]CaseResult {
		m := map[string]CaseResult{}
		for _, c := range r.Cases {
			m[c.CaseName] = c
		}
		return m
	}
	nc, cc := index(native.agent), index(container.agent)
	if len(nc) == 0 {
		t.Fatal("native run produced no cases")
	}
	if len(nc) != len(cc) {
		t.Fatalf("native has %d cases, container has %d", len(nc), len(cc))
	}

	// passed mirrors printCaseResult's threshold rule.
	passed := func(c CaseResult) bool {
		score, max := caseTotals(c)
		if max == 0 {
			return false
		}
		return float64(score)/float64(max)*100 >= thresholds[c.CaseName]
	}
	for name, n := range nc {
		c, ok := cc[name]
		if !ok {
			t.Errorf("case %q missing from the container run", name)
			continue
		}
		if _, ok := thresholds[name]; !ok {
			t.Errorf("case %s has no loaded test case, so no threshold", name)
		}
		ns, nm := caseTotals(n)
		cs, cm := caseTotals(c)
		if ns != cs || nm != cm {
			t.Errorf("case %s: caseTotals native %d/%d, container %d/%d", name, ns, nm, cs, cm)
		}
		if np, cp := passed(n), passed(c); np != cp {
			t.Errorf("case %s: threshold outcome native=%t container=%t", name, np, cp)
		}
	}

	ns, nm := agentScoreTotals(native.agent)
	cs, cm := agentScoreTotals(container.agent)
	if ns != cs || nm != cm {
		t.Errorf("agentScoreTotals native %v/%v, container %v/%v", ns, nm, cs, cm)
	}
	if !reflect.DeepEqual(native.summary.AgentScores, container.summary.AgentScores) {
		t.Errorf("agent_scores native %v, container %v", native.summary.AgentScores, container.summary.AgentScores)
	}
}

func assertDiffReportsModeDifference(t *testing.T, evalsDir string, native, container parityRun) {
	t.Helper()
	if err := configure(RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := diffTo(&buf, native.name, container.name); err != nil {
		t.Fatalf("diffTo(%s, %s): %v", native.name, container.name, err)
	}
	out := buf.String()
	for _, want := range []string{
		native.name + ": mode native",
		container.name + ": mode container",
		"sandbox: native → container",
		"not directly comparable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output missing %q:\n%s", want, out)
		}
	}
}
