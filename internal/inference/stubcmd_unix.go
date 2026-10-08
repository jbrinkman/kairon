//go:build unix

package inference

import (
	"os/exec"
	"syscall"
)

// setProcessGroup runs cmd in its own process group and makes context
// cancellation kill the whole group. Without this, killing only the `sh -c`
// parent leaves children such as `sleep 3` alive and holding the output
// pipes, so Wait would block until they exit on their own.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid addresses the process group (pgid == pid with Setpgid).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
