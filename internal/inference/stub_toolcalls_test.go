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
)

func toolCallReq(workDir string, trust *ToolTrust, commands []string, calls ...StubToolCall) Request {
	return Request{
		Role:      RoleAgent,
		Agent:     "a",
		Prompt:    "p",
		WorkDir:   workDir,
		ToolTrust: trust,
		Stub:      &StubScript{Turns: []StubTurn{{Response: "scripted", Commands: commands, ToolCalls: calls}}},
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestStub_ToolCallTrustedRunsInWorkDir(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir,
		NewToolTrust([]string{"write"}), nil,
		StubToolCall{Tool: "fs_write", Command: "echo x > tool-marker.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "scripted" {
		t.Errorf("Text = %q", resp.Text)
	}
	if len(resp.ToolDenials) != 0 {
		t.Errorf("ToolDenials = %+v, want none", resp.ToolDenials)
	}
	got, err := os.ReadFile(filepath.Join(dir, "tool-marker.txt"))
	if err != nil {
		t.Fatalf("trusted tool call did not run in WorkDir: %v", err)
	}
	if string(got) != "x\n" {
		t.Errorf("tool-marker.txt = %q", got)
	}
}

func TestStub_ToolCallDeniedIsSkippedAndRecorded(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir,
		NewToolTrust([]string{"read"}), nil,
		StubToolCall{Tool: "fs_write", Command: "echo x > tool-marker.txt"}))
	if err != nil {
		t.Fatalf("a denial must not fail the turn: %v", err)
	}
	if resp.Text != "scripted" {
		t.Errorf("Text = %q, want the turn response despite the denial", resp.Text)
	}
	if exists(filepath.Join(dir, "tool-marker.txt")) {
		t.Error("denied tool call's command ran")
	}
	want := []ToolDenial{{Tool: "fs_write", Command: "echo x > tool-marker.txt", Reason: "tool not trusted"}}
	if !reflect.DeepEqual(resp.ToolDenials, want) {
		t.Errorf("ToolDenials = %+v, want %+v", resp.ToolDenials, want)
	}
}

func TestStub_ToolCallAliasIsNormalizedBeforeGate(t *testing.T) {
	dir := t.TempDir()
	// "write" alias in the script, trust expressed as canonical fs_write.
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir,
		NewToolTrust([]string{"fs_write"}), nil,
		StubToolCall{Tool: "write", Command: "touch a"},
		StubToolCall{Tool: "shell", Command: "touch b"}))
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "a")) {
		t.Error("alias 'write' should match trusted fs_write")
	}
	if exists(filepath.Join(dir, "b")) {
		t.Error("'shell' (execute_bash) is not trusted and must not run")
	}
	if len(resp.ToolDenials) != 1 || resp.ToolDenials[0].Tool != "execute_bash" {
		t.Errorf("ToolDenials = %+v, want one execute_bash denial (normalized name)", resp.ToolDenials)
	}
}

func TestStub_ToolCallNilTrustIsUnrestricted(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, nil, nil,
		StubToolCall{Tool: "fs_write", Command: "touch w"},
		StubToolCall{Tool: "execute_bash", Command: "touch e"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"w", "e"} {
		if !exists(filepath.Join(dir, f)) {
			t.Errorf("%s not created with nil ToolTrust", f)
		}
	}
	if len(resp.ToolDenials) != 0 {
		t.Errorf("ToolDenials = %+v, want none", resp.ToolDenials)
	}
}

func TestStub_ToolCallEmptyTrustDeniesEverything(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, NewToolTrust(nil), nil,
		StubToolCall{Tool: "fs_read", Command: "touch r"},
		StubToolCall{Tool: "fs_write", Command: "touch w"}))
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "r")) || exists(filepath.Join(dir, "w")) {
		t.Error("a tool call ran under a trust-nothing set")
	}
	if len(resp.ToolDenials) != 2 {
		t.Fatalf("ToolDenials = %+v, want 2", resp.ToolDenials)
	}
	if resp.ToolDenials[0].Tool != "fs_read" || resp.ToolDenials[1].Tool != "fs_write" {
		t.Errorf("denials not in call order: %+v", resp.ToolDenials)
	}
}

func TestStub_CommandsRemainUngatedUnderRestrictedTrust(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, NewToolTrust(nil),
		[]string{"echo env > env.txt"},
		StubToolCall{Tool: "fs_write", Command: "echo x > denied.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "env.txt")) {
		t.Error("Commands must run regardless of ToolTrust")
	}
	if exists(filepath.Join(dir, "denied.txt")) {
		t.Error("denied tool call ran")
	}
	if len(resp.ToolDenials) != 1 {
		t.Errorf("ToolDenials = %+v, want 1", resp.ToolDenials)
	}
}

func TestStub_CommandsRunBeforeToolCalls(t *testing.T) {
	dir := t.TempDir()
	_, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, nil,
		[]string{"echo cmd > f"},
		StubToolCall{Tool: "fs_write", Command: "echo tool >> f"}))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f"))
	if string(got) != "cmd\ntool\n" {
		t.Errorf("f = %q, want commands before tool calls", got)
	}
}

func TestStub_DeniedToolCallNeedsNoWorkDir(t *testing.T) {
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq("", NewToolTrust(nil), nil,
		StubToolCall{Tool: "fs_write", Command: "touch nope"}))
	if err != nil {
		t.Fatalf("a denied call runs nothing, so it must not require WorkDir: %v", err)
	}
	if len(resp.ToolDenials) != 1 {
		t.Errorf("ToolDenials = %+v", resp.ToolDenials)
	}
}

func TestStub_TrustedToolCallWithoutWorkDirRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := newTestStub(t).Invoke(context.Background(), toolCallReq("", nil, nil,
		StubToolCall{Tool: "fs_write", Command: "touch nope"}))
	if err == nil || !strings.Contains(err.Error(), "WorkDir") {
		t.Fatalf("expected WorkDir error, got %v", err)
	}
	if exists("nope") {
		t.Error("tool call ran in the process working directory")
	}
}

func TestStub_FailedToolCallReturnsError(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, NewToolTrust([]string{"write"}), nil,
		StubToolCall{Tool: "fs_read", Command: "touch skipped"},
		StubToolCall{Tool: "fs_write", Command: "echo boom >&2; exit 4"}))
	if err == nil {
		t.Fatal("expected error from failing trusted tool call")
	}
	if errors.Is(err, ErrTimeout) {
		t.Errorf("not a timeout: %v", err)
	}
	if resp.ExitCode != 4 || !strings.Contains(resp.Stderr, "boom") {
		t.Errorf("resp = %+v", resp)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty on failure", resp.Text)
	}
	// The denial recorded before the failure is still reported.
	if len(resp.ToolDenials) != 1 || resp.ToolDenials[0].Tool != "fs_read" {
		t.Errorf("ToolDenials = %+v", resp.ToolDenials)
	}
}

// TestStub_UnreachedDenialsAfterFailureAreNotRecorded verifies that a denied
// tool call positioned AFTER a failing (allowed) call is not reported: the turn
// halts at the failure, so execution never reaches the later call and its
// denial must not pad the failed-call record. The inverse (a denial BEFORE the
// failure is kept) is covered by TestStub_FailedToolCallReturnsError.
func TestStub_UnreachedDenialsAfterFailureAreNotRecorded(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, NewToolTrust([]string{"write"}), nil,
		StubToolCall{Tool: "fs_write", Command: "echo boom >&2; exit 7"}, // allowed, fails -> halts
		StubToolCall{Tool: "fs_read", Command: "touch unreached"},        // denied, but never reached
	))
	if err == nil {
		t.Fatal("expected error from failing trusted tool call")
	}
	if resp.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7; resp = %+v", resp.ExitCode, resp)
	}
	if len(resp.ToolDenials) != 0 {
		t.Errorf("ToolDenials = %+v, want none: the fs_read denial is after the failure and was never reached", resp.ToolDenials)
	}
}

// TestStub_EnvCommandFailureRecordsNoDenials verifies that when an environment
// command fails (before any tool call), no tool-call denials are recorded —
// execution never reached the gating stage.
func TestStub_EnvCommandFailureRecordsNoDenials(t *testing.T) {
	dir := t.TempDir()
	resp, err := newTestStub(t).Invoke(context.Background(), toolCallReq(dir, NewToolTrust(nil),
		[]string{"exit 5"},
		StubToolCall{Tool: "fs_write", Command: "touch nope"}))
	if err == nil {
		t.Fatal("expected error from failing environment command")
	}
	if resp.ExitCode != 5 {
		t.Errorf("ExitCode = %d, want 5", resp.ExitCode)
	}
	if len(resp.ToolDenials) != 0 {
		t.Errorf("ToolDenials = %+v, want none: no tool call was reached", resp.ToolDenials)
	}
}

func TestStubTurn_ToolCallsOmittedWhenEmpty(t *testing.T) {
	if got := marshalString(t, StubTurn{Response: "r"}); strings.Contains(got, "tool_calls") {
		t.Errorf("empty ToolCalls should be omitted: %s", got)
	}
	got := marshalString(t, StubTurn{Response: "r", ToolCalls: []StubToolCall{{Tool: "fs_read", Command: "c"}}})
	if !strings.Contains(got, `"tool_calls":[{"tool":"fs_read","command":"c"}]`) {
		t.Errorf("tool_calls JSON = %s", got)
	}
}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
