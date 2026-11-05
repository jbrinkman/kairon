//go:build linux

package agent

import (
	"os"
	"strconv"
	"strings"
)

// processStart returns an opaque identity for the process: its start time in
// clock ticks since boot (field 22 of /proc/<pid>/stat). A PID the OS hands to
// a new process gets a different value.
func processStart(pid int) (string, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", false
	}
	// The command name (field 2) is in parentheses and may contain spaces, so
	// count fields from the last ')'.
	s := string(data)
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return "", false
	}
	fields := strings.Fields(s[end+1:])
	// fields[0] is field 3 (state), so field 22 is fields[19].
	if len(fields) < 20 {
		return "", false
	}
	return fields[19], true
}
