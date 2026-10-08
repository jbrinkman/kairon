package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jbrinkman/kairon/internal/config"
)

// agentProvenance is the resolved identity of an agent under test: the model
// its config names, a hash over everything that shapes its prompt, and which
// of its declared resources actually exist.
type agentProvenance struct {
	// Model is the "model" from the agent config (trimmed). The effective
	// model is decided by the caller (evals.agent_model may override it).
	Model string
	// ConfigPath is the agent config file that was read.
	ConfigPath string
	// PromptSHA256 is the lowercase hex SHA-256 described on hashParts.
	PromptSHA256 string
	// ResourcesPresent lists existing resource entries exactly as written in
	// the config, in config order. Never nil.
	ResourcesPresent []string
}

// agentConfigFile is the subset of a kiro agent config read for provenance.
type agentConfigFile struct {
	Model     string            `json:"model"`
	Prompt    string            `json:"prompt"`
	Resources []json.RawMessage `json:"resources"`
	// AllowedTools is the agent's own tool allowlist; it is the default
	// tool-trust set for container runs (see resolveTrustSet).
	AllowedTools []string `json:"allowedTools"`
}

// locateAgentConfig returns the path of the agent config: <evals-dir>/agents/
// <agent>.json when it exists (and ignoreOverlay is false), else
// .kiro/agents/<agent>.json. A missing config is an error listing both paths.
func locateAgentConfig(agent string, ignoreOverlay bool) (string, error) {
	repoPath := filepath.Join(".kiro", "agents", agent+".json")
	overlayPath := filepath.Join(evalsPath("agents"), agent+".json")

	if !ignoreOverlay && fileExists(overlayPath) {
		return overlayPath, nil
	}
	if !fileExists(repoPath) {
		return "", fmt.Errorf("agent config for %q not found: tried %s and %s", agent, overlayPath, repoPath)
	}
	return repoPath, nil
}

// resolveAgentProvenance locates the agent config and computes its provenance.
//
// The config is <evals-dir>/agents/<agent>.json when that exists, else
// .kiro/agents/<agent>.json relative to the working directory. When
// ignoreOverlay is true only the latter is considered: a kiro-cli container
// cannot see the evals-dir overlay.
func resolveAgentProvenance(agent string, ignoreOverlay bool) (agentProvenance, error) {
	path, err := locateAgentConfig(agent, ignoreOverlay)
	if err != nil {
		return agentProvenance{}, err
	}

	configBytes, err := os.ReadFile(path)
	if err != nil {
		return agentProvenance{}, fmt.Errorf("reading agent config %s: %w", path, err)
	}
	var conf agentConfigFile
	if err := json.Unmarshal(configBytes, &conf); err != nil {
		return agentProvenance{}, fmt.Errorf("parsing agent config %s: %w", path, err)
	}

	h := sha256.New()
	writeHashPart(h, "config", configBytes)

	// Prompt: a file:// reference is read (and required); an inline prompt is
	// already covered by the config bytes.
	if ref, ok := strings.CutPrefix(conf.Prompt, "file://"); ok {
		promptPath := ref
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(filepath.Dir(path), promptPath)
		}
		promptBytes, err := os.ReadFile(promptPath)
		if err != nil {
			return agentProvenance{}, fmt.Errorf("reading prompt file %s referenced by %s: %w", promptPath, path, err)
		}
		writeHashPart(h, "prompt", promptBytes)
	}

	present := []string{}
	for _, raw := range conf.Resources {
		var entry string
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue // non-string (object) entries are ignored
		}
		files, err := resolveResourceFiles(entry)
		if err != nil {
			return agentProvenance{}, fmt.Errorf("resource %q in %s: %w", entry, path, err)
		}
		if len(files) == 0 {
			continue // missing resources are normal (e.g. *-conventions overrides)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return agentProvenance{}, fmt.Errorf("resource %q in %s: %w", entry, path, err)
			}
			writeHashPart(h, "resource", b)
		}
		present = append(present, entry)
	}

	return agentProvenance{
		Model:            strings.TrimSpace(conf.Model),
		ConfigPath:       path,
		PromptSHA256:     hex.EncodeToString(h.Sum(nil)),
		ResourcesPresent: present,
	}, nil
}

// hashWriter is the subset of hash.Hash used for framing.
type hashWriter interface{ Write(p []byte) (int, error) }

// writeHashPart writes one length-framed part: <kind>\x00<decimal length>\x00<bytes>.
// Framing keeps the concatenation unambiguous. File paths are deliberately not
// hashed so the hash is machine-independent.
func writeHashPart(h hashWriter, kind string, data []byte) {
	_, _ = h.Write([]byte(kind + "\x00" + strconv.Itoa(len(data)) + "\x00"))
	_, _ = h.Write(data)
}

// resolveResourceFiles returns the regular files a resource entry refers to.
// Only file:// and skill:// (or bare) paths are local; other schemes yield no
// files. Relative paths resolve against the working directory. A missing file
// yields an empty result and no error; other stat errors are returned.
func resolveResourceFiles(entry string) ([]string, error) {
	p := entry
	switch {
	case strings.HasPrefix(p, "file://"):
		p = strings.TrimPrefix(p, "file://")
	case strings.HasPrefix(p, "skill://"):
		p = strings.TrimPrefix(p, "skill://")
	case strings.Contains(p, "://"):
		return nil, nil
	}
	if p == "" {
		return nil, nil
	}

	if strings.ContainsAny(p, "*?[") {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		var files []string
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, err
			}
			if info.Mode().IsRegular() {
				files = append(files, m)
			}
		}
		return files, nil
	}

	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	return []string{p}, nil
}

// fileExists reports whether path exists (any stat success).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// agentPin is the pinned identity of one agent for the run.
type agentPin struct {
	// Model is the effective model: evals.agent_model when set, else the agent config's model.
	Model string
	// Provenance is the resolved agent config identity.
	Provenance agentProvenance
}

// runPins is the pre-flight result kept in cfg.pins. It is written once by
// pinRun before any case starts and only read during the run. All methods
// are safe on a nil receiver (unpinned).
type runPins struct {
	Judge  string
	Agents map[string]agentPin
	// TrustOverrides is evals.trust_tools: agent name -> tool names trusted
	// in container runs, taking precedence over the agent's allowedTools.
	TrustOverrides map[string][]string
}

// trustOverride returns the evals.trust_tools entry for agent, if any. An
// entry present with no tools (trust nothing) reports ok.
func (p *runPins) trustOverride(agent string) ([]string, bool) {
	if p == nil {
		return nil, false
	}
	tools, ok := p.TrustOverrides[agent]
	return tools, ok
}

func (p *runPins) judgeModel() string {
	if p == nil {
		return ""
	}
	return p.Judge
}

func (p *runPins) agentModel(agent string) string {
	if p == nil {
		return ""
	}
	return p.Agents[agent].Model
}

func (p *runPins) agentPromptSHA(agent string) string {
	if p == nil {
		return ""
	}
	return p.Agents[agent].Provenance.PromptSHA256
}

// pinOf returns the pin for agent, if the run is pinned and covers it.
func (p *runPins) pinOf(agent string) (agentPin, bool) {
	if p == nil {
		return agentPin{}, false
	}
	pin, ok := p.Agents[agent]
	return pin, ok
}

// applyTo fills the provenance fields of r from the pins (no-op if unpinned
// or the agent is not covered). ResourcesPresent is always non-nil when set.
func (p *runPins) applyTo(r *AgentResult) {
	pin, ok := p.pinOf(r.Agent)
	if !ok {
		return
	}
	r.AgentModel = pin.Model
	r.JudgeModel = p.Judge
	r.PromptSHA256 = pin.Provenance.PromptSHA256
	r.ResourcesPresent = append([]string{}, pin.Provenance.ResourcesPresent...)
}

// agentsInScope lists the agents a run covers. An explicit agent wins; a
// --perf run without an agent covers none; otherwise every rubric's agent.
// Unreadable rubrics yield no agents so that the downstream, more specific
// error ("rubrics directory not found", ...) is what the user sees.
func agentsInScope(agent string, opts RunOptions) []string {
	if agent != "" {
		return []string{agent}
	}
	if opts.Perf {
		return nil
	}
	rubrics, err := loadRubrics("")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var agents []string
	for _, r := range rubrics {
		if r.Agent != "" && !seen[r.Agent] {
			seen[r.Agent] = true
			agents = append(agents, r.Agent)
		}
	}
	return agents
}

// pinRun is the pre-flight for a run. Before any case starts it resolves and
// validates the judge model and every in-scope agent's effective model against
// evals.allowed_models, and computes each agent's prompt provenance. All
// violations are reported together. On success it sets cfg.pins.
//
// evals.agent_model is honoured on every path, including --sandbox (the model
// reaches the agent as --model). ignoreOverlay, when true, considers only
// .kiro/agents and skips the <evals-dir>/agents overlay; the runner passes
// false because the overlay is always visible to the agent (staged into the
// per-case workspace .kiro, which the sandbox bind-mounts).
func pinRun(agent string, opts RunOptions, ignoreOverlay bool) error {
	ev, err := config.LoadEvals()
	if err != nil {
		return fmt.Errorf("❌ cannot pin eval models: %w", err)
	}

	var violations []string
	if err := ev.CheckModel(ev.JudgeModel); err != nil {
		violations = append(violations, fmt.Sprintf("evals.judge_model: %v", err))
	}

	pins := &runPins{Judge: ev.JudgeModel, Agents: map[string]agentPin{}, TrustOverrides: ev.TrustTools}
	for _, name := range agentsInScope(agent, opts) {
		prov, err := resolveAgentProvenance(name, ignoreOverlay)
		if err != nil {
			violations = append(violations, fmt.Sprintf("agent %q: %v", name, err))
			continue
		}
		model, source := prov.Model, prov.ConfigPath+` "model"`
		if ev.AgentModel != "" {
			model, source = ev.AgentModel, "evals.agent_model"
		}
		if err := ev.CheckModel(model); err != nil {
			violations = append(violations, fmt.Sprintf("agent %q (model from %s): %v", name, source, err))
			continue
		}
		pins.Agents[name] = agentPin{Model: model, Provenance: prov}
	}

	if len(violations) > 0 {
		return fmt.Errorf("❌ eval model pinning refused the run before any case started:\n  - %s\nEdit the evals block in .kairon/config.yaml (agent_model, judge_model, allowed_models)",
			strings.Join(violations, "\n  - "))
	}

	cfg.pins = pins
	names := make([]string, 0, len(pins.Agents))
	for n := range pins.Agents {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := []string{"judge=" + pins.Judge}
	for _, n := range names {
		sha := pins.Agents[n].Provenance.PromptSHA256
		parts = append(parts, fmt.Sprintf("%s=%s (prompt sha256 %s…)", n, pins.Agents[n].Model, sha[:8]))
	}
	fmt.Printf("🔒 Models: %s\n", strings.Join(parts, ", "))
	return nil
}
