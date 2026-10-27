package plannertest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
)

var seq atomic.Int64

// ReplyCommand returns a real process that prints reply on standard output,
// standing in for one kiro-cli run. The reply is written to a file in dir so no
// quoting reaches the shell.
func ReplyCommand(dir, reply string) *exec.Cmd {
	path := filepath.Join(dir, fmt.Sprintf("reply-%d.txt", seq.Add(1)))
	if err := os.WriteFile(path, []byte(reply), 0o644); err != nil {
		panic(fmt.Sprintf("plannertest: write reply: %v", err))
	}
	return exec.Command("sh", "-c", `cat "$1"`, "sh", path)
}

// BlockingCommand is ReplyCommand that waits until the file release exists
// before printing, so a test can hold a run open.
func BlockingCommand(dir, reply, release string) *exec.Cmd {
	path := filepath.Join(dir, fmt.Sprintf("reply-%d.txt", seq.Add(1)))
	if err := os.WriteFile(path, []byte(reply), 0o644); err != nil {
		panic(fmt.Sprintf("plannertest: write reply: %v", err))
	}
	return exec.Command("sh", "-c", `while [ ! -e "$1" ]; do sleep 0.02; done; cat "$2"`, "sh", release, path)
}

// FailingCommand is a process that writes msg to standard error and exits 3,
// as kiro-cli does when it is not logged in.
func FailingCommand(msg string) *exec.Cmd {
	return exec.Command("sh", "-c", `echo "$1" >&2; exit 3`, "sh", msg)
}
