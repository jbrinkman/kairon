// Package kaironbin puts the running kairon binary on the PATH of every agent
// process Kairon starts.
//
// Agent prompts call kairon subcommands (kairon scope check, kairon plan
// parse). An agent must reach the binary that started it, not whatever kairon
// happens to be installed, and must reach it even when no kairon is installed
// at all (a watcher run from ./dist/kairon). Kairon therefore keeps a
// directory it owns, .kairon/bin under the project root, holding a single
// kairon symlink to the running binary. Install refreshes that link when the
// watcher and TUI start; Apply prepends the directory to an agent's PATH and
// sets KAIRON_BIN to the binary's path.
//
// Like every other .kairon path in Kairon, the directory is resolved against
// the working directory, which is the project root for the watcher and TUI.
package kaironbin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar names the variable that carries the running binary's resolved path to
// agent processes.
const EnvVar = "KAIRON_BIN"

// linkName is the only entry Install creates in the bin directory.
const linkName = "kairon"

// Executable returns the running binary's path with symlinks resolved.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the running kairon binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolving the running kairon binary %s: %w", exe, err)
	}
	return resolved, nil
}

// Dir returns the absolute path of the Kairon bin directory, .kairon/bin under
// the working directory. Agents run in worktrees, so the PATH entry must be
// absolute.
func Dir() (string, error) {
	dir, err := filepath.Abs(filepath.Join(".kairon", "bin"))
	if err != nil {
		return "", fmt.Errorf("resolving the kairon bin directory: %w", err)
	}
	return dir, nil
}

// Install creates the bin directory if needed and points its kairon symlink at
// the running binary, replacing any link an older binary left. The new link is
// created under a temporary name and renamed into place, so an agent starting
// at the same moment sees either the old link or the new one, never neither.
func Install() error {
	exe, err := Executable()
	if err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating the kairon bin directory: %w", err)
	}
	link := filepath.Join(dir, linkName)
	tmp := fmt.Sprintf("%s.tmp-%d", link, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(exe, tmp); err != nil {
		return fmt.Errorf("linking %s to the running binary: %w", link, err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("linking %s to the running binary: %w", link, err)
	}
	return nil
}

// Apply returns a copy of env with the bin directory as the first PATH entry
// and KAIRON_BIN set to the running binary. Any existing PATH is kept after
// the bin directory; any existing KAIRON_BIN is replaced.
//
// If the binary or directory cannot be resolved, env is returned unchanged.
// Startup has already run Install, which resolves both and fails loudly, so in
// practice this cannot happen once the watcher or TUI is running.
func Apply(env []string) []string {
	exe, err := Executable()
	if err != nil {
		return env
	}
	dir, err := Dir()
	if err != nil {
		return env
	}
	out := make([]string, 0, len(env)+2)
	path := dir
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			if v != "" {
				path = dir + string(os.PathListSeparator) + v
			}
			continue
		}
		if strings.HasPrefix(kv, EnvVar+"=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PATH="+path, EnvVar+"="+exe)
}

// Environ is Apply(os.Environ()): the environment for an agent process.
func Environ() []string {
	return Apply(os.Environ())
}
