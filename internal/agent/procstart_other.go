//go:build !darwin && !linux

package agent

// processStart reports no identity on this platform; PID markers fall back to
// a liveness check.
func processStart(int) (string, bool) { return "", false }
