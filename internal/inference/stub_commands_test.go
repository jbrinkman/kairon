//go:build !windows

package inference

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stubReq(workDir string, timeout time.Duration, cmds ...string) Request {
	return Request{
		Role:    RoleAgent,
		Agent:   "a",
		Prompt:  "p",
		WorkDir: workDir,
		Timeout: timeout,
		Stub:    &StubScript{Turns: []StubTurn{{Response: "scripted", Commands: cmds}}},
	}
}

func TestStub_CommandsCreateFileInWorkDir(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), stubReq(dir, 0, "echo hi > marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "scripted" {
		t.Errorf("Text = %q, want scripted response", resp.Text)
	}
	got, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil {
		t.Fatalf("marker.txt not created in WorkDir: %v", err)
	}
	if string(got) != "hi\n" {
		t.Errorf("marker.txt = %q, want %q", got, "hi\n")
	}
}

func TestStub_CommandsRunInOrder(t *testing.T) {
	dir := t.TempDir()
	_, err := newTestStub(t).Invoke(context.Background(),
		stubReq(dir, 0, "echo one > f", "echo two >> f"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f"))
	if string(got) != "one\ntwo\n" {
		t.Errorf("f = %q", got)
	}
}

func TestStub_CommandsEmptyWorkDirRunsNothing(t *testing.T) {
	// Run from a scratch cwd so a (wrongly) executed command would be visible.
	scratch := t.TempDir()
	t.Chdir(scratch)

	resp, err := newTestStub(t).Invoke(context.Background(), stubReq("", 0, "echo hi > marker.txt"))
	if err == nil {
		t.Fatal("expected error for commands with empty WorkDir")
	}
	if !strings.Contains(err.Error(), "WorkDir") {
		t.Errorf("unclear error: %v", err)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty", resp.Text)
	}
	if _, statErr := os.Stat(filepath.Join(scratch, "marker.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("command ran despite empty WorkDir (stat err = %v)", statErr)
	}
}

func TestStub_NoCommandsIgnoresEmptyWorkDir(t *testing.T) {
	resp, err := newTestStub(t).Invoke(context.Background(), stubReq("", 0))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "scripted" {
		t.Errorf("Text = %q", resp.Text)
	}
}

func TestStub_CommandFailureReturnsStderrAndNoText(t *testing.T) {
	dir := t.TempDir()
	const cmd = "echo boom >&2; exit 7"
	resp, err := newTestStub(t).Invoke(context.Background(), stubReq(dir, 0, cmd))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), cmd) || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should contain the command and its stderr, got: %v", err)
	}
	if errors.Is(err, ErrTimeout) {
		t.Error("a failing command must not be reported as a timeout")
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty", resp.Text)
	}
	if resp.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", resp.ExitCode)
	}
	if !strings.Contains(resp.Stderr, "boom") {
		t.Errorf("Stderr = %q", resp.Stderr)
	}
}

func TestStub_FailedCommandStopsLaterCommands(t *testing.T) {
	dir := t.TempDir()
	_, err := newTestStub(t).Invoke(context.Background(),
		stubReq(dir, 0, "exit 1", "echo late > late.txt"))
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "late.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("later command ran after a failure (stat err = %v)", statErr)
	}
}

func TestStub_CommandTimeoutKillsProcessGroupPromptly(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	resp, err := newTestStub(t).Invoke(context.Background(), stubReq(dir, time.Second, "sleep 3"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false; err = %v", err)
	}
	if err.Error() != "stub timeout after 1s" {
		t.Errorf("error = %q, want %q", err.Error(), "stub timeout after 1s")
	}
	if elapsed >= 2*time.Second {
		t.Errorf("timeout took %v, want < 2s (child of sh not killed?)", elapsed)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty", resp.Text)
	}
}

func TestStub_ParentCancelIsNotATimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := newTestStub(t).Invoke(ctx, stubReq(t.TempDir(), 30*time.Second, "sleep 3"))
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrTimeout) {
		t.Errorf("cancellation reported as timeout: %v", err)
	}
	if time.Since(start) >= 2*time.Second {
		t.Errorf("cancel took %v", time.Since(start))
	}
}

func TestRequestAndStubTurn_JSONRoundTripKeepsWorkDirAndCommands(t *testing.T) {
	in := Request{
		Role:    RoleAgent,
		Agent:   "a",
		WorkDir: "/workspace",
		Stub: &StubScript{Turns: []StubTurn{{
			Response: "r",
			Commands: []string{"echo hi > marker.txt", "true"},
		}}},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["WorkDir"]) != `"/workspace"` {
		t.Errorf(`JSON key "WorkDir" = %s`, raw["WorkDir"])
	}
	var stub struct {
		Turns []struct {
			Commands []string `json:"commands"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw["Stub"], &stub); err != nil {
		t.Fatal(err)
	}
	if len(stub.Turns) != 1 || !reflect.DeepEqual(stub.Turns[0].Commands, []string{"echo hi > marker.txt", "true"}) {
		t.Errorf(`Stub JSON lacks "commands" key: %s`, raw["Stub"])
	}

	var out Request
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

func TestStubTurn_CommandsOmittedWhenEmpty(t *testing.T) {
	data, _ := json.Marshal(StubTurn{Response: "r"})
	if strings.Contains(string(data), "commands") {
		t.Errorf("empty Commands should be omitted: %s", data)
	}
}
