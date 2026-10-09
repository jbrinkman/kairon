//go:build !unix

package eval

import "os/exec"

// setCheckProcessGroup is a no-op where process groups are unavailable; the
// default cancellation kills only the direct child.
func setCheckProcessGroup(cmd *exec.Cmd) {}

func killCheckGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
