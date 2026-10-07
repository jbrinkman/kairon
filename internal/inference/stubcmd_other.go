//go:build !unix

package inference

import "os/exec"

// setProcessGroup is a no-op where process groups are not available; the
// default exec.CommandContext cancellation kills only the direct child.
func setProcessGroup(cmd *exec.Cmd) {}
