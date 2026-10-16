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
	"regexp"
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
	// PromptFile is the candidate prompt path (--prompt-file) the hash was
	// computed with; empty when the live prompt was used.
	PromptFile string
	// ResourcesPresent lists existing resource entries exactly as written in
	// the config, in config order. Never nil.
	ResourcesPresent []string
	// SubAgents holds the provenance of every agent the dispatching agent
	// trusts, transitively (the dispatcher itself excluded), keyed by name.
	// Nil when the agent trusts no agents.
	SubAgents map[string]SubAgentProvenance
	// WorkflowSHA256 is the lowercase hex SHA-256 over the dispatching agent
	// and its sub-agents (see workflowSHA256). Empty when SubAgents is nil.
	WorkflowSHA256 string
}

// agentConfigFile is the subset of a kiro agent config read for provenance.
type agentConfigFile struct {
	Model     string            `json:"model"`
	Prompt    string            `json:"prompt"`
	Resources []json.RawMessage `json:"resources"`
	// AllowedTools is the agent's own tool allowlist; it is the default
	// tool-trust set for container runs (see resolveTrustSet).
	AllowedTools []string `json:"allowedTools"`
	// ToolsSettings.Subagent.TrustedAgents lists the agents this agent may
	// dispatch. A non-list value fails to decode (an error, not ignored).
	ToolsSettings struct {
		Subagent struct {
			TrustedAgents []string `json:"trustedAgents"`
		} `json:"subagent"`
	} `json:"toolsSettings"`
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
	return resolveAgentProvenanceWith(agent, ignoreOverlay, cfg.candidate)
}

// resolveAgentProvenanceWith is resolveAgentProvenance with an explicit
// candidate prompt. A candidate for another agent is ignored.
//
// Provenance is computed from the same bytes a case workspace gets, never from
// the repository working directory: it stages a throwaway baseline .kiro (no
// fixture files, so the project's tracked files without *-conventions override
// hooks, the evals-dir agents overlay unless ignoreOverlay, and the candidate
// prompt over both) and hashes that. The directory is always removed. The
// candidate is therefore hashed from the very bytes staged for the agent,
// including through a resource that aliases the prompt file.
func resolveAgentProvenanceWith(agent string, ignoreOverlay bool, cand *candidatePrompt) (agentProvenance, error) {
	if cand != nil && cand.Agent != agent {
		cand = nil
	}

	path, err := locateAgentConfig(agent, ignoreOverlay)
	if err != nil {
		return agentProvenance{}, err
	}

	root, err := os.MkdirTemp("", "kairon-eval-provenance-*")
	if err != nil {
		return agentProvenance{}, fmt.Errorf("creating baseline workspace for provenance: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	if err := stageKiroInto(filepath.Join(root, kiroDirName), nil, ignoreOverlay, cand); err != nil {
		return agentProvenance{}, err
	}
	prov, err := resolveProvenanceInRoot(root, agent, cand)
	if err != nil {
		return agentProvenance{}, err
	}
	// The staged config is a copy; report the source the config came from.
	prov.ConfigPath = path
	return prov, nil
}

// resolveProvenanceInRoot computes the provenance of agent from a staged
// directory root that holds a .kiro/. The config is
// <root>/.kiro/agents/<agent>.json, a file:// prompt is relative to the
// config's directory, and resources resolve against root (absolute paths stay
// absolute), so what is hashed is exactly what an agent running in root sees.
//
// A candidate for another agent is ignored. A candidate for this agent is
// validated (an inline, absolute or escaping prompt is an error) and recorded
// as PromptFile; its bytes are not injected here because the caller has
// already staged them over the prompt file inside root.
//
// ConfigPath is the staged config path; callers that know the source path
// override it.
//
// The agents the config trusts (toolsSettings.subagent.trustedAgents,
// transitively) are resolved from the same root and recorded in SubAgents,
// and WorkflowSHA256 is computed over the whole workflow. The dispatching
// agent's own PromptSHA256 is computed first from its own files only, so a
// sub-agent edit never changes it. A candidate applies to the dispatching
// agent only.
func resolveProvenanceInRoot(root, agent string, cand *candidatePrompt) (agentProvenance, error) {
	prov, conf, err := resolveSingleProvenanceInRoot(root, agent, cand)
	if err != nil {
		return agentProvenance{}, err
	}
	subs, err := collectSubAgents(root, agent, conf.ToolsSettings.Subagent.TrustedAgents)
	if err != nil {
		return agentProvenance{}, err
	}
	if len(subs) > 0 {
		prov.SubAgents = subs
		prov.WorkflowSHA256 = workflowSHA256(agent, prov.PromptSHA256, subs)
	}
	return prov, nil
}

// subAgentNameRE is the shape of a trusted agent name: it becomes a file name
// under <root>/.kiro/agents, so it may not contain a path separator or start
// with a dot.
var subAgentNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// collectSubAgents resolves the transitive closure of trusted (the names the
// dispatching agent trusts) from root. Each agent is resolved once (cycle
// safe) and the dispatching agent is never listed. A trusted agent without a
// staged config, or with an unsafe name, is an error: a workflow hash over a
// missing step would be misleading. Returns nil when there are none.
func collectSubAgents(root, dispatcher string, trusted []string) (map[string]SubAgentProvenance, error) {
	var subs map[string]SubAgentProvenance
	visited := map[string]bool{dispatcher: true}
	queue := append([]string(nil), trusted...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if !subAgentNameRE.MatchString(name) {
			return nil, fmt.Errorf("agent %q: unsafe trusted agent name %q in toolsSettings.subagent.trustedAgents (allowed: letters, digits, '.', '_', '-', not starting with a symbol)", dispatcher, name)
		}
		if visited[name] {
			continue
		}
		visited[name] = true

		p, conf, err := resolveSingleProvenanceInRoot(root, name, nil)
		if err != nil {
			return nil, fmt.Errorf("sub-agent %q trusted by agent %q: %w", name, dispatcher, err)
		}
		if subs == nil {
			subs = map[string]SubAgentProvenance{}
		}
		subs[name] = SubAgentProvenance{
			AgentModel:       p.Model,
			PromptSHA256:     p.PromptSHA256,
			ResourcesPresent: p.ResourcesPresent,
		}
		queue = append(queue, conf.ToolsSettings.Subagent.TrustedAgents...)
	}
	return subs, nil
}

// workflowSHA256 is the lowercase hex SHA-256 over length-framed parts: for the
// dispatching agent first and then every sub-agent sorted by name, a
// "workflow-agent" part (the agent name) and a "prompt-sha256" part (that
// agent's prompt_sha256 hex). Names are hashed so swapping prompts between
// steps changes the hash; paths are never hashed.
func workflowSHA256(dispatcher, dispatcherSHA string, subs map[string]SubAgentProvenance) string {
	names := make([]string, 0, len(subs))
	for n := range subs {
		names = append(names, n)
	}
	sort.Strings(names)

	h := sha256.New()
	writeHashPart(h, "workflow-agent", []byte(dispatcher))
	writeHashPart(h, "prompt-sha256", []byte(dispatcherSHA))
	for _, n := range names {
		writeHashPart(h, "workflow-agent", []byte(n))
		writeHashPart(h, "prompt-sha256", []byte(subs[n].PromptSHA256))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// resolveSingleProvenanceInRoot computes the provenance of one agent from its
// own config, prompt and resources only (no sub-agents), and returns the
// parsed config so callers can read the agents it trusts.
func resolveSingleProvenanceInRoot(root, agent string, cand *candidatePrompt) (agentProvenance, agentConfigFile, error) {
	if cand != nil && cand.Agent != agent {
		cand = nil
	}

	path := filepath.Join(root, kiroDirName, "agents", agent+".json")
	configBytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return agentProvenance{}, agentConfigFile{}, fmt.Errorf("agent config for %q not found in the staged %s (%s); a project config must be tracked by git (git add -N)", agent, kiroDirName, path)
		}
		return agentProvenance{}, agentConfigFile{}, fmt.Errorf("reading agent config %s: %w", path, err)
	}
	var conf agentConfigFile
	if err := json.Unmarshal(configBytes, &conf); err != nil {
		return agentProvenance{}, agentConfigFile{}, fmt.Errorf("parsing agent config %s: %w", path, err)
	}

	h := sha256.New()
	writeHashPart(h, "config", configBytes)

	// Prompt: a file:// reference is read (and required); an inline prompt is
	// already covered by the config bytes.
	if cand != nil {
		if _, err := candidatePromptDest(path, conf.Prompt); err != nil {
			return agentProvenance{}, agentConfigFile{}, err
		}
	}
	if ref, ok := strings.CutPrefix(conf.Prompt, "file://"); ok {
		promptPath := ref
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(filepath.Dir(path), promptPath)
		}
		promptBytes, err := os.ReadFile(promptPath)
		if err != nil {
			return agentProvenance{}, agentConfigFile{}, fmt.Errorf("reading prompt file %s referenced by %s: %w", promptPath, path, err)
		}
		writeHashPart(h, "prompt", promptBytes)
	}

	present := []string{}
	for _, raw := range conf.Resources {
		var entry string
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue // non-string (object) entries are ignored
		}
		files, err := resolveResourceFilesIn(root, entry)
		if err != nil {
			return agentProvenance{}, agentConfigFile{}, fmt.Errorf("resource %q in %s: %w", entry, path, err)
		}
		if len(files) == 0 {
			continue // missing resources are normal (e.g. *-conventions overrides)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return agentProvenance{}, agentConfigFile{}, fmt.Errorf("resource %q in %s: %w", entry, path, err)
			}
			writeHashPart(h, "resource", b)
		}
		present = append(present, entry)
	}

	prov := agentProvenance{
		Model:            strings.TrimSpace(conf.Model),
		ConfigPath:       path,
		PromptSHA256:     hex.EncodeToString(h.Sum(nil)),
		ResourcesPresent: present,
	}
	if cand != nil {
		prov.PromptFile = cand.Path
	}
	return prov, conf, nil
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

// resolveResourceFiles returns the regular files a resource entry refers to,
// resolving relative paths against the working directory. See
// resolveResourceFilesIn.
func resolveResourceFiles(entry string) ([]string, error) {
	return resolveResourceFilesIn("", entry)
}

// resolveResourceFilesIn returns the regular files a resource entry refers to.
// Only file:// and skill:// (or bare) paths are local; other schemes yield no
// files. Relative paths resolve against root (the working directory when root
// is ""); absolute paths stay absolute. Returned paths include root. A missing
// file yields an empty result and no error; other stat errors are returned.
func resolveResourceFilesIn(root, entry string) ([]string, error) {
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
	globRoot, rel := "", p
	if root != "" && !filepath.IsAbs(p) {
		globRoot = root
		p = filepath.Join(root, p)
	}

	if strings.ContainsAny(p, "*?[") {
		pattern := p
		if globRoot != "" {
			// Escape only the literal root prefix so the staged root is matched
			// verbatim while the entry keeps its own glob metacharacters.
			pattern = filepath.Join(escapeGlobMeta(globRoot), rel)
		}
		matches, err := filepath.Glob(pattern)
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

// escapeGlobMeta backslash-escapes filepath.Glob metacharacters in a literal
// directory so a staged root such as "/tmp/a[1]" is matched verbatim. Windows
// has no glob escape, so the path is returned unchanged there.
func escapeGlobMeta(dir string) string {
	if filepath.Separator == '\\' || !strings.ContainsAny(dir, `*?[\`) {
		return dir
	}
	var b strings.Builder
	for _, r := range dir {
		if strings.ContainsRune(`*?[\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
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
	r.PromptFile = pin.Provenance.PromptFile
	r.ResourcesPresent = append([]string{}, pin.Provenance.ResourcesPresent...)
	r.SubAgents = copySubAgents(pin.Provenance.SubAgents)
	r.WorkflowSHA256 = pin.Provenance.WorkflowSHA256
}

// copySubAgents deep-copies m (nil for an empty map, so a plain agent's JSON
// omits sub_agents).
func copySubAgents(m map[string]SubAgentProvenance) map[string]SubAgentProvenance {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]SubAgentProvenance, len(m))
	for n, sa := range m {
		sa.ResourcesPresent = append([]string{}, sa.ResourcesPresent...)
		out[n] = sa
	}
	return out
}

// subAgentsLine renders the pin-line detail for a workflow agent: each
// sub-agent's model and short prompt hash (sorted by name) and the workflow
// hash. Empty for an agent without sub-agents.
func subAgentsLine(prov agentProvenance) string {
	if len(prov.SubAgents) == 0 {
		return ""
	}
	names := make([]string, 0, len(prov.SubAgents))
	for n := range prov.SubAgents {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		sa := prov.SubAgents[n]
		parts = append(parts, fmt.Sprintf("%s=%s (prompt sha256 %s…)", n, sa.AgentModel, shortHash(sa.PromptSHA256)))
	}
	return fmt.Sprintf("sub-agents: %s ; workflow sha256 %s…", strings.Join(parts, ", "), shortHash(prov.WorkflowSHA256))
}

// shortHash is the first 8 characters of a hex hash (all of it if shorter).
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
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
	for _, n := range names {
		if line := subAgentsLine(pins.Agents[n].Provenance); line != "" {
			fmt.Printf("   %s: %s\n", n, line)
		}
	}
	return nil
}
