package eval

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// modeUnknown is the execution mode shown for a run that predates
// sandbox-mode tracking (no summary.sandbox and no agent-file sandbox bool).
const modeUnknown = "unknown (predates sandbox-mode tracking)"

// notRecorded is shown for a provenance value a run does not carry.
const notRecorded = "(not recorded)"

// agentProv is the provenance of one agent in one run.
type agentProv struct {
	model       string
	sha         string
	promptFile  string // candidate prompt path (--prompt-file); empty when not a candidate run
	containment *Containment
}

// runInfo is the provenance of one run as read for eval diff. Every field is
// optional: a run written before provenance tracking simply has empty values.
type runInfo struct {
	mode       string
	judgeModel string
	agents     map[string]agentProv
}

// knownMode reports whether the run's mode was recorded.
func (r runInfo) knownMode() bool { return r.mode != modeUnknown }

// loadRunInfo gathers a run's provenance from its summary and from the agent
// files in dir. It never fails: missing, old or malformed inputs yield empty
// values (a malformed agent file is skipped, as elsewhere in diff).
func loadRunInfo(summary Summary, dir string) runInfo {
	info := runInfo{
		judgeModel: summary.JudgeModel,
		agents:     map[string]agentProv{},
	}

	for name, p := range summary.Agents {
		ap := info.agents[name]
		ap.model, ap.sha, ap.promptFile = p.AgentModel, p.PromptSHA256, p.PromptFile
		info.agents[name] = ap
	}
	for name, c := range summary.Containment {
		ap := info.agents[name]
		c := c
		ap.containment = &c
		info.agents[name] = ap
	}

	// Agent files fill what the summary lacks (older runs, single-case runs)
	// and supply the mode fallback.
	var modes []*bool
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "summary.json" {
			continue
		}
		res, err := loadAgentResult(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		ap := info.agents[name]
		if ap.model == "" {
			ap.model = res.AgentModel
		}
		if ap.sha == "" {
			ap.sha = res.PromptSHA256
		}
		if ap.promptFile == "" {
			ap.promptFile = res.PromptFile
		}
		if ap.containment == nil && res.Containment != nil {
			c := *res.Containment
			ap.containment = &c
		}
		info.agents[name] = ap
		if info.judgeModel == "" {
			info.judgeModel = res.JudgeModel
		}
		modes = append(modes, res.Sandbox)
	}

	info.mode = deriveMode(summary.Sandbox, modes)
	return info
}

// deriveMode resolves the execution mode: summary.sandbox when set, else the
// agent files' sandbox bool provided every file records it and they all
// agree, else modeUnknown.
func deriveMode(summaryMode string, agentModes []*bool) string {
	if summaryMode != "" {
		return summaryMode
	}
	if len(agentModes) == 0 {
		return modeUnknown
	}
	for _, m := range agentModes {
		if m == nil || *m != *agentModes[0] {
			return modeUnknown
		}
	}
	return string(runModeOf(*agentModes[0]))
}

// printProvenance writes the "Run Provenance" block: each run's mode, then
// every difference in sandbox, containment, judge_model, agent_model,
// prompt_sha256 and prompt_file. It only reports; it never fails the diff.
func printProvenance(w io.Writer, runA, runB string, a, b runInfo) {
	fmt.Fprintf(w, "\nRun Provenance:\n")
	fmt.Fprintf(w, "  %s: mode %s\n", runA, a.mode)
	fmt.Fprintf(w, "  %s: mode %s\n", runB, b.mode)

	differences := 0
	diff := func(format string, args ...any) {
		differences++
		fmt.Fprintf(w, "  "+format+"\n", args...)
	}
	show := func(s string) string {
		if s == "" {
			return notRecorded
		}
		return s
	}

	modesDiffer := a.mode != b.mode
	if modesDiffer {
		diff("sandbox: %s → %s", a.mode, b.mode)
	}
	if a.judgeModel != b.judgeModel {
		diff("judge_model: %s → %s", show(a.judgeModel), show(b.judgeModel))
	}

	for _, agent := range mergeAgentNames(a.agents, b.agents) {
		pa, pb := a.agents[agent], b.agents[agent]
		for _, line := range containmentDiff(pa.containment, pb.containment) {
			diff("%s containment.%s", agent, line)
		}
		if pa.model != pb.model {
			diff("%s agent_model: %s → %s", agent, show(pa.model), show(pb.model))
		}
		if pa.sha != pb.sha {
			diff("%s prompt_sha256: %s → %s", agent, show(pa.sha), show(pb.sha))
		}
		if pa.promptFile != pb.promptFile {
			diff("%s prompt_file: %s → %s", agent, show(pa.promptFile), show(pb.promptFile))
		}
	}

	if differences == 0 {
		fmt.Fprintf(w, "  No provenance differences.\n")
	}

	switch {
	case modesDiffer && a.knownMode() && b.knownMode():
		fmt.Fprintf(w, "  ⚠ Runs used different execution modes (%s vs %s); scores are not directly comparable.\n", a.mode, b.mode)
	case modesDiffer:
		fmt.Fprintf(w, "  Note: the execution mode of at least one run is unknown, so comparability cannot be verified.\n")
	}
}

// containmentDiff describes how containment a differs from b, one line per
// differing field. A missing containment (native or old run) is "none".
func containmentDiff(a, b *Containment) []string {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		return []string{fmt.Sprintf("record: none → %s", describeContainment(*b))}
	case b == nil:
		return []string{fmt.Sprintf("record: %s → none", describeContainment(*a))}
	}
	var lines []string
	if ta, tb := trustString(a.ToolTrust), trustString(b.ToolTrust); ta != tb {
		lines = append(lines, fmt.Sprintf("tool_trust: %s → %s", ta, tb))
	}
	if a.FakeGH != b.FakeGH {
		lines = append(lines, fmt.Sprintf("fake_gh: %t → %t", a.FakeGH, b.FakeGH))
	}
	if a.ReadOnlyFS != b.ReadOnlyFS {
		lines = append(lines, fmt.Sprintf("read_only_fs: %t → %t", a.ReadOnlyFS, b.ReadOnlyFS))
	}
	if a.Network != b.Network {
		lines = append(lines, fmt.Sprintf("network: %s → %s", a.Network, b.Network))
	}
	return lines
}

func describeContainment(c Containment) string {
	return fmt.Sprintf("tool_trust=%s fake_gh=%t read_only_fs=%t network=%s",
		trustString(c.ToolTrust), c.FakeGH, c.ReadOnlyFS, c.Network)
}

// trustString renders a trust set order-insensitively.
func trustString(names []string) string {
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	return "[" + strings.Join(sorted, ",") + "]"
}

// mergeAgentNames returns the sorted union of agent names.
func mergeAgentNames(a, b map[string]agentProv) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range []map[string]agentProv{a, b} {
		for n := range m {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	return names
}
