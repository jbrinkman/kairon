//go:build darwin

package agent

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// processStart returns an opaque identity for the process: its start time. A
// PID the OS hands to a new process gets a different value.
func processStart(pid int) (string, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) {
		return "", false
	}
	t := kp.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", t.Sec, t.Usec), true
}
