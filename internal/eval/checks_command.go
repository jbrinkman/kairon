package eval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// maxCommandOutput is how much combined output (the tail) is retained.
	maxCommandOutput = 64 << 10
	// commandDetailTail is how much output goes into a failure Detail.
	commandDetailTail = 512
	// commandWaitDelay bounds the wait for pipes held open by leftover children.
	commandWaitDelay = time.Second
)

// commandOutcome is the result of running a check command.
type commandOutcome struct {
	ExitCode int    // valid when Err == nil
	Output   string // tail of combined stdout+stderr
	Err      error  // start failure, timeout or signal death
}

// runCheckCommand runs `sh -c run` in dir. It is a variable so a container
// runner (or a test) can replace it.
var runCheckCommand = defaultRunCheckCommand

// tailBuffer keeps only the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > 2*b.max {
		n := copy(b.buf, b.buf[len(b.buf)-b.max:])
		b.buf = b.buf[:n]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	if len(b.buf) > b.max {
		return string(b.buf[len(b.buf)-b.max:])
	}
	return string(b.buf)
}

// checkCommandEnv is the process environment minus GitHub credentials.
func checkCommandEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if blockedContainerEnv[name] {
			continue
		}
		env = append(env, kv)
	}
	return env
}

func defaultRunCheckCommand(dir, run string) commandOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), checkCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", run)
	cmd.Dir = dir
	cmd.Env = checkCommandEnv()
	cmd.Stdin = nil // /dev/null
	cmd.WaitDelay = commandWaitDelay
	setCheckProcessGroup(cmd)
	out := &tailBuffer{max: maxCommandOutput}
	cmd.Stdout, cmd.Stderr = out, out

	err := cmd.Run()
	// Whatever the command left running must not outlive the check (it could
	// touch the workspace after inject restore or the next check).
	killCheckGroup(cmd)

	res := commandOutcome{Output: out.String()}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Err = fmt.Errorf("timed out after %s", checkCommandTimeout)
	case err == nil:
	case errors.As(err, &exitErr):
		if code := exitErr.ExitCode(); code >= 0 {
			res.ExitCode = code
		} else {
			res.Err = fmt.Errorf("terminated by signal (%s)", exitErr.String())
		}
	case errors.Is(err, exec.ErrWaitDelay):
		// Exited normally but a leftover child held the pipes; exit status is 0.
	default:
		res.Err = fmt.Errorf("could not start command: %w", err)
	}
	return res
}

func evalCommand(c Check, dir string) (bool, string) {
	abs, err := absDir(dir)
	if err != nil {
		return false, err.Error()
	}
	var inj *injection
	if len(c.Inject) > 0 {
		if inj, err = injectFiles(abs, c.injectDir, c.Inject); err != nil {
			return false, err.Error()
		}
	}
	res := runCheckCommand(abs, c.Run)
	var restoreErr error
	if inj != nil {
		restoreErr = inj.restore()
	}

	passed, detail := true, ""
	switch {
	case res.Err != nil:
		passed, detail = false, withTail(res.Err.Error(), res.Output)
	case res.ExitCode != c.expectedExit():
		passed, detail = false, withTail(fmt.Sprintf("exit status %d (want %d)", res.ExitCode, c.expectedExit()), res.Output)
	}
	if restoreErr != nil {
		msg := "could not restore workspace after inject: " + restoreErr.Error()
		if detail != "" {
			msg = detail + "; " + msg
		}
		return false, msg
	}
	return passed, detail
}

// withTail appends the last commandDetailTail bytes of output to msg.
func withTail(msg, output string) string {
	output = strings.TrimSpace(strings.ToValidUTF8(output, "?"))
	if output == "" {
		return msg
	}
	if len(output) > commandDetailTail {
		output = "…" + output[len(output)-commandDetailTail:]
	}
	return msg + ": " + output
}
