package eval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// changedPath is one path that differs from the base revision.
type changedPath struct {
	Path string // slash-separated, repo-relative
	Kind string // added, modified, deleted, untracked
}

const checkGitTimeout = 60 * time.Second

// gitArgs prefixes args with options that stop repository configuration (which
// the agent controls) from running commands or touching the index.
func gitArgs(args ...string) []string {
	return append([]string{
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.pager=cat",
		"-c", "diff.external=",
		"-c", "core.quotePath=false",
	}, args...)
}

func runGit(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", gitArgs(args...)...)
	cmd.Dir = dir
	cmd.Env = hermeticGitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil {
			msg = "timed out"
		}
		return nil, fmt.Errorf("git %s failed: %v: %s", args[0], err, shorten(msg, 300))
	}
	return stdout.Bytes(), nil
}

// changedPaths lists tracked changes of the working tree versus base (staged
// and unstaged, so a change the agent committed is still seen) plus untracked
// files. Paths under .eval/ are dropped. dir must be the repository root.
func changedPaths(dir, base string) ([]changedPath, error) {
	if base == "" {
		base = "HEAD"
	}
	if strings.HasPrefix(base, "-") || strings.ContainsAny(base, "\x00\n") {
		return nil, fmt.Errorf("invalid base revision %q", base)
	}

	top, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	if !sameDir(strings.TrimSpace(string(top)), dir) {
		return nil, fmt.Errorf("%s is not a git repository root (found %s)", dir, strings.TrimSpace(string(top)))
	}

	diff, err := runGit(dir, "diff", "--name-status", "--no-renames", "--no-ext-diff", "--no-textconv", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	var out []changedPath
	fields := strings.Split(strings.TrimSuffix(string(diff), "\x00"), "\x00")
	if len(fields) == 1 && fields[0] == "" {
		fields = nil
	}
	if len(fields)%2 != 0 {
		return nil, errors.New("git diff: unexpected output")
	}
	for i := 0; i < len(fields); i += 2 {
		kind := "modified"
		switch fields[i][:1] {
		case "A":
			kind = "added"
		case "D":
			kind = "deleted"
		}
		out = append(out, changedPath{Path: fields[i+1], Kind: kind})
	}

	others, err := runGit(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(string(others), "\x00") {
		if p != "" {
			out = append(out, changedPath{Path: p, Kind: "untracked"})
		}
	}

	kept := out[:0]
	for _, c := range out {
		if c.Path == ".eval" || strings.HasPrefix(c.Path, ".eval/") {
			continue
		}
		kept = append(kept, c)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	return kept, nil
}

// sameDir reports whether a and b name the same directory (symlinks resolved).
func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return false
	}
	ia, err1 := os.Stat(ra)
	ib, err2 := os.Stat(rb)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}

const maxReportedChanges = 20

func evalChangedFiles(c Check, in CheckInput) (bool, string) {
	paths, err := changedPaths(in.Dir, in.Base)
	if err != nil {
		return false, err.Error()
	}
	var bad []string
	for _, p := range paths {
		allowed := false
		for _, g := range c.globs {
			if g.match(p.Path) {
				allowed = true
				break
			}
		}
		if !allowed {
			bad = append(bad, p.Kind+" "+p.Path)
		}
	}
	if len(bad) == 0 {
		return true, ""
	}
	shown := bad
	if len(shown) > maxReportedChanges {
		shown = append(append([]string{}, bad[:maxReportedChanges]...), fmt.Sprintf("… and %d more", len(bad)-maxReportedChanges))
	}
	return false, fmt.Sprintf("changes outside allow [%s]: %s", strings.Join(c.Allow, " "), strings.Join(shown, "; "))
}
