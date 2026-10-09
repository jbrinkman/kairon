//go:build unix

package eval

import (
	"os/exec"
	"syscall"
)

// setCheckProcessGroup runs cmd in its own process group and makes context
// cancellation kill the whole group, so `sh -c "sleep 60"` children cannot
// outlive a timeout.
func setCheckProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killCheckGroup(cmd) }
}

// killCheckGroup SIGKILLs cmd's process group (pgid == pid with Setpgid).
func killCheckGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
