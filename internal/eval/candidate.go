package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// candidatePrompt is a prompt file supplied with --prompt-file that is
// evaluated in place of the agent's live prompt. It is never written into the
// repository: it only affects the prompt provenance hash and (elsewhere) the
// per-case workspaces.
type candidatePrompt struct {
	// Agent is the agent the candidate was supplied for.
	Agent string
	// Path is the path as given on the command line, normalised with
	// filepath.Clean and filepath.ToSlash. It is what gets recorded as
	// prompt_file.
	Path string
	// Content is the candidate file's bytes.
	Content []byte
}

// loadCandidatePrompt validates opts.PromptFile and reads it. It returns
// (nil, nil) when no candidate was requested. It performs no side effects, so
// it can run before configure and before any state is changed.
//
// A candidate requires an agent, cannot be combined with --list, --perf or
// --cleanup (those modes never stage a workspace, so the flag would be
// silently ignored), and must name a readable, non-empty regular file.
func loadCandidatePrompt(agent string, opts RunOptions) (*candidatePrompt, error) {
	if opts.PromptFile == "" {
		return nil, nil
	}
	if agent == "" {
		return nil, errors.New("--prompt-file: an agent is required (usage: kairon eval <agent> --prompt-file <path>)")
	}
	for _, c := range []struct {
		on   bool
		flag string
	}{{opts.List, "--list"}, {opts.Perf, "--perf"}, {opts.Cleanup, "--cleanup"}} {
		if c.on {
			return nil, fmt.Errorf("--prompt-file cannot be combined with %s", c.flag)
		}
	}

	info, err := os.Stat(opts.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file %s: %w", opts.PromptFile, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("--prompt-file %s: is a directory, want a file", opts.PromptFile)
	}
	content, err := os.ReadFile(opts.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file %s: %w", opts.PromptFile, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("--prompt-file %s: file is empty", opts.PromptFile)
	}
	return &candidatePrompt{
		Agent:   agent,
		Path:    filepath.ToSlash(filepath.Clean(opts.PromptFile)),
		Content: content,
	}, nil
}

// candidatePromptDest resolves where a candidate prompt lands for an agent
// whose config declares promptRef (the config's "prompt" value). The result is
// slash-separated and relative to the staged .kiro directory, e.g.
// "agents/a-prompt.md".
//
// A prompt ref is resolved relative to the config's directory, which is
// .kiro/agents in a case workspace whichever of <evals-dir>/agents or
// .kiro/agents the config came from. It is an error if the prompt is inline
// (no file:// prefix, nothing to substitute), absolute (it would not resolve
// inside a workspace or container), or escapes .kiro/ once cleaned.
func candidatePromptDest(configPath, promptRef string) (string, error) {
	ref, ok := strings.CutPrefix(promptRef, "file://")
	if !ok {
		return "", fmt.Errorf("--prompt-file: agent config %s has an inline prompt; only a file:// prompt can be replaced by a candidate", configPath)
	}
	if ref == "" {
		return "", fmt.Errorf("--prompt-file: agent config %s has an empty file:// prompt reference", configPath)
	}
	if filepath.IsAbs(ref) || path.IsAbs(filepath.ToSlash(ref)) {
		return "", fmt.Errorf("--prompt-file: agent config %s references an absolute prompt path %q; only a path relative to the config can be replaced by a candidate", configPath, ref)
	}
	// The join base is the literal "agents" segment because both the project
	// .kiro/agents config and the <evals-dir>/agents overlay are staged into
	// <workspace>/.kiro/agents/. resolveAgentProvenanceWith (hash) and
	// stageCandidatePrompt (stage) both resolve the destination through this
	// helper, so the hashed and staged paths cannot diverge; a future change
	// to the staged .kiro layout must keep that invariant.
	dest := path.Join("agents", filepath.ToSlash(ref))
	if dest == ".." || strings.HasPrefix(dest, "../") {
		return "", fmt.Errorf("--prompt-file: prompt reference %q in %s escapes .kiro/; only a prompt inside .kiro/ can be replaced by a candidate", ref, configPath)
	}
	if dest == "agents" || dest == "." {
		return "", fmt.Errorf("--prompt-file: prompt reference %q in %s does not name a file", ref, configPath)
	}
	return dest, nil
}

// stageCandidatePrompt writes the candidate's bytes over the prompt file the
// agent config references, inside the case workspace's staged .kiro directory.
// Only the workspace copy is written: the live prompt and the evals-dir copy
// are never touched.
//
// It must run after the project .kiro copy and the evals-dir overlay (so the
// candidate wins over both and over any fixture-provided file at the same
// path) and before setPermissions (so the file ends up read-only like the rest
// of .kiro). The config is located exactly as provenance locates it, and the
// destination comes from the same candidatePromptDest helper, so the staged
// file and the hashed prompt cannot disagree.
//
// Any existing entry at the destination is removed first, so a symlink or
// hard link there is replaced rather than written through.
func stageCandidatePrompt(w *caseWorkspace, cand *candidatePrompt) error {
	if cand == nil {
		return nil
	}
	configPath, err := locateAgentConfig(cand.Agent, false)
	if err != nil {
		return err
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading agent config %s: %w", configPath, err)
	}
	var conf agentConfigFile
	if err := json.Unmarshal(configBytes, &conf); err != nil {
		return fmt.Errorf("parsing agent config %s: %w", configPath, err)
	}
	rel, err := candidatePromptDest(configPath, conf.Prompt)
	if err != nil {
		return err
	}
	dest := filepath.Join(w.KiroDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("staging candidate prompt: %w", err)
	}
	if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("staging candidate prompt: replacing %s: %w", dest, err)
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("staging candidate prompt: %w", err)
	}
	_, werr := f.Write(cand.Content)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("staging candidate prompt: writing %s: %w", dest, werr)
	}
	return os.Chmod(dest, 0o644) // not subject to umask
}
