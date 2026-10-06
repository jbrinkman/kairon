package inference

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ansiRegex matches all CSI (Control Sequence Introducer) escape sequences.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRegex.ReplaceAllString(s, "") }

// kiroCLIBackend shells out to the kiro-cli binary.
type kiroCLIBackend struct{}

func newKiroCLI() *kiroCLIBackend { return &kiroCLIBackend{} }

func (*kiroCLIBackend) Name() string { return NameKiroCLI }

// Available reports whether kiro-cli is on PATH.
func (*kiroCLIBackend) Available() error {
	_, err := exec.LookPath("kiro-cli")
	return err
}

// StartupProbe times `kiro-cli --version`.
func (*kiroCLIBackend) StartupProbe() time.Duration {
	start := time.Now()
	cmd := exec.Command("kiro-cli", "--version")
	_ = cmd.Run()
	return time.Since(start)
}

func (b *kiroCLIBackend) Invoke(ctx context.Context, req Request) (Response, error) {
	switch req.Role {
	case RoleAgent:
		return b.invokeAgent(ctx, req)
	case RoleJudge:
		return b.invokeJudge(ctx, req)
	default:
		return Response{}, fmt.Errorf("kiro-cli backend: unsupported role %q", req.Role)
	}
}

// invokeAgent runs `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools`
// with the prompt on stdin. stdout/stderr are captured separately and ANSI
// sequences are stripped from the returned text.
func (*kiroCLIBackend) invokeAgent(parent context.Context, req Request) (Response, error) {
	timeout := timeoutOrDefault(req.Timeout)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kiro-cli", "chat", "--agent", req.Agent, "--no-interactive", "--trust-all-tools")
	cmd.Stdin = strings.NewReader(req.Prompt)

	resp := Response{
		Command: fmt.Sprintf("kiro-cli chat --agent %s --no-interactive --trust-all-tools", req.Agent),
	}

	cleanup, err := applyAgentConfigOverlay(cmd, req.AgentConfigDir, req.Agent)
	if err != nil {
		return resp, fmt.Errorf("kiro-cli invocation failed: %w", err)
	}
	defer cleanup()

	output := &strings.Builder{}
	errOutput := &strings.Builder{}
	cmd.Stdout = output
	cmd.Stderr = errOutput

	start := time.Now()
	runErr := cmd.Run()
	resp.Duration = time.Since(start)
	resp.Stderr = errOutput.String()

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		resp.ExitCode = exitErr.ExitCode()
	}

	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return resp, &invokeError{
				msg:  fmt.Sprintf("kiro-cli timeout after %v", timeout),
				errs: []error{ErrTimeout, runErr},
			}
		}
		return resp, fmt.Errorf("kiro-cli invocation failed: %w", runErr)
	}

	resp.Text = stripANSI(output.String())
	resp.Usage = EstimateUsage(req.Prompt, resp.Text)
	return resp, nil
}

// invokeJudge runs `kiro-cli chat --no-interactive` with the prompt on stdin
// using cmd.Output() semantics. The raw (un-stripped) stdout is returned.
func (*kiroCLIBackend) invokeJudge(parent context.Context, req Request) (Response, error) {
	timeout := timeoutOrDefault(req.Timeout)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kiro-cli", "chat", "--no-interactive")
	cmd.Stdin = strings.NewReader(req.Prompt)

	resp := Response{Command: "kiro-cli chat --no-interactive"}

	start := time.Now()
	out, runErr := cmd.Output()
	resp.Duration = time.Since(start)

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		resp.ExitCode = exitErr.ExitCode()
		resp.Stderr = string(exitErr.Stderr)
	}

	if runErr != nil {
		errs := []error{runErr}
		if ctx.Err() == context.DeadlineExceeded {
			errs = append([]error{ErrTimeout}, errs...)
		}
		return resp, &invokeError{
			msg:  fmt.Sprintf("kiro-cli chat failed: %v", runErr),
			errs: errs,
		}
	}

	resp.Text = string(out)
	resp.Usage = EstimateUsage(req.Prompt, resp.Text)
	return resp, nil
}

// applyAgentConfigOverlay makes agent configs in dir visible to kiro-cli.
//
// kiro-cli only discovers local agents under <cwd>/.kiro/agents/, so when dir
// contains <agent>.json the whole directory is copied into a temporary
// <tmp>/.kiro/agents/ and the command runs with cmd.Dir=<tmp> (so relative
// file:// prompt references keep resolving). The returned function removes the
// temporary directory. When dir is empty or lacks <agent>.json nothing is
// changed and cmd.Dir stays unset. The repo's own .kiro/agents is never touched.
func applyAgentConfigOverlay(cmd *exec.Cmd, dir, agent string) (func(), error) {
	noop := func() {}
	if dir == "" || agent == "" {
		return noop, nil
	}
	if _, err := os.Stat(filepath.Join(dir, agent+".json")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return noop, nil
		}
		return noop, fmt.Errorf("checking agent config %s: %w", filepath.Join(dir, agent+".json"), err)
	}

	tmp, err := os.MkdirTemp("", "kairon-inference-*")
	if err != nil {
		return noop, fmt.Errorf("creating agent config overlay: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	dst := filepath.Join(tmp, ".kiro", "agents")
	if err := copyDir(dir, dst); err != nil {
		cleanup()
		return noop, fmt.Errorf("staging agent config overlay: %w", err)
	}
	cmd.Dir = tmp
	return cleanup, nil
}

// copyDir recursively copies src into dst, following symlinks for file content.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
