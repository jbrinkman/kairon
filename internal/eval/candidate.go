package eval

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// candidatePrompt is a --prompt-file candidate: replacement content for an
// agent's prompt file, injected only into per-case workspaces. Nothing under
// the repository's .kiro/agents is ever written.
type candidatePrompt struct {
	// Path is the candidate path as given on the command line, slash-
	// normalised and not made absolute, so recorded results are
	// machine-independent.
	Path string
	// Content is the candidate prompt bytes.
	Content []byte
	// Target is where the content lands, relative to a staged .kiro dir
	// (e.g. "agents/a-prompt.md"). It is resolved once from the agent
	// config's relative file:// prompt reference.
	Target string
}

// validateCandidateOptions rejects --prompt-file combinations that cannot
// work, before anything else happens. Without a candidate it is a no-op.
// --list, --perf and --cleanup run no cases through case workspaces, so a
// candidate would be silently ignored.
func validateCandidateOptions(agent string, opts RunOptions) error {
	if opts.PromptFile == "" {
		return nil
	}
	if agent == "" {
		return errors.New("--prompt-file: an agent is required (usage: kairon eval <agent> --prompt-file <path>)")
	}
	for _, c := range []struct {
		on   bool
		flag string
	}{
		{opts.List, "--list"},
		{opts.Perf, "--perf"},
		{opts.Cleanup, "--cleanup"},
	} {
		if c.on {
			return fmt.Errorf("--prompt-file cannot be combined with %s", c.flag)
		}
	}
	return nil
}

// loadCandidatePrompt reads and validates the candidate for agent. The file
// must exist, be a regular non-empty file, and the agent's config (overlay
// first, then .kiro/agents) must reference its prompt with a relative
// file:// path that stays inside the .kiro tree, since that is the only
// shape where a workspace copy is what the agent reads.
func loadCandidatePrompt(agent, path string) (*candidatePrompt, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--prompt-file: %s is not a regular file", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file: reading %s: %w", path, err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, fmt.Errorf("--prompt-file: %s is empty", path)
	}

	confPath, err := locateAgentConfig(agent, false)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file: %w", err)
	}
	conf, _, err := readAgentConfig(confPath)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file: %w", err)
	}
	target, err := candidateTarget(conf.Prompt)
	if err != nil {
		return nil, fmt.Errorf("--prompt-file: agent %q (%s): %w", agent, confPath, err)
	}

	return &candidatePrompt{
		Path:    filepath.ToSlash(path),
		Content: content,
		Target:  target,
	}, nil
}

// candidateTarget maps an agent config "prompt" value to the staged path
// relative to the .kiro dir (agent configs live in <kiro>/agents).
func candidateTarget(prompt string) (string, error) {
	ref, ok := strings.CutPrefix(prompt, "file://")
	if !ok {
		return "", errors.New(`prompt must be a relative file:// reference (e.g. "file://./name-prompt.md") to use --prompt-file; inline or missing prompts cannot be replaced`)
	}
	if ref == "" || filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") {
		return "", fmt.Errorf("prompt reference %q must be a relative file:// path to use --prompt-file", prompt)
	}
	target := filepath.Clean(filepath.Join("agents", filepath.FromSlash(ref)))
	if !withinDir(target) {
		return "", fmt.Errorf("prompt reference %q escapes the .kiro tree; --prompt-file needs a file:// path inside .kiro", prompt)
	}
	if target == "agents" {
		return "", fmt.Errorf("prompt reference %q does not name a file", prompt)
	}
	return target, nil
}

// withinDir reports whether the cleaned relative path rel stays inside the
// directory it is relative to (and is not that directory itself).
func withinDir(rel string) bool {
	if rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stage writes the candidate into the staged .kiro dir at Target. The target
// is removed first so a pre-existing entry (fixture file, symlink) is never
// written through; the candidate always wins over a fixture-provided file.
// It must run before setPermissions makes the workspace read-only. A nil
// candidate is a no-op.
func (c *candidatePrompt) stage(kiroDir string) error {
	if c == nil {
		return nil
	}
	if !withinDir(filepath.Clean(c.Target)) {
		return fmt.Errorf("candidate target %q is not inside the .kiro dir", c.Target)
	}
	dst := filepath.Join(kiroDir, c.Target)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("staging candidate prompt: %w", err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("staging candidate prompt: removing %s: %w", dst, err)
	}
	if err := os.WriteFile(dst, c.Content, 0o644); err != nil {
		return fmt.Errorf("staging candidate prompt: %w", err)
	}
	return nil
}
