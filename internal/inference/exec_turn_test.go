package inference

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestKiroCLIAgentCommand_Resume pins the turn contract at the kiro-cli
// boundary: Turn 0 produces exactly the historical arguments, Turn > 0 adds
// --resume directly after --no-interactive and changes nothing else.
func TestKiroCLIAgentCommand_Resume(t *testing.T) {
	tests := []struct {
		name  string
		trust *ToolTrust
		model string
		// base is the historical (Turn 0) argv; the Turn > 0 argv is base with
		// --resume inserted after --no-interactive.
		base        []string
		baseCommand string
	}{
		{
			name:        "trust-all",
			base:        []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools"},
			baseCommand: "kiro-cli chat --agent builder --no-interactive --trust-all-tools",
		},
		{
			name:        "trust-all with model",
			model:       "claude-x",
			base:        []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools", "--model", "claude-x"},
			baseCommand: "kiro-cli chat --agent builder --no-interactive --trust-all-tools --model claude-x",
		},
		{
			name:        "trust-tools",
			trust:       NewToolTrust([]string{"read", "write"}),
			base:        []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools=fs_read,fs_write"},
			baseCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=fs_read,fs_write",
		},
		{
			name:        "trust-tools with model",
			trust:       NewToolTrust([]string{"read"}),
			model:       "claude-x",
			base:        []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools=fs_read", "--model", "claude-x"},
			baseCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=fs_read --model claude-x",
		},
		{
			name:        "empty restricted set",
			trust:       NewToolTrust(nil),
			base:        []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools="},
			baseCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Agent: "builder", Model: tt.model, ToolTrust: tt.trust, Prompt: "p"}

			// Turn 0 (the zero value) is byte-identical to the historical output.
			args, command := KiroCLIAgentCommand(req)
			if !reflect.DeepEqual(args, tt.base) {
				t.Errorf("Turn 0 args = %q, want %q", args, tt.base)
			}
			if command != tt.baseCommand {
				t.Errorf("Turn 0 command = %q, want %q", command, tt.baseCommand)
			}

			// Every Turn > 0 resumes: --resume directly after --no-interactive.
			wantArgs := append(append(append([]string{}, tt.base[:4]...), "--resume"), tt.base[4:]...)
			wantCommand := strings.Replace(tt.baseCommand, "--no-interactive", "--no-interactive --resume", 1)
			for _, turn := range []int{1, 2, 7} {
				req.Turn = turn
				args, command := KiroCLIAgentCommand(req)
				if !reflect.DeepEqual(args, wantArgs) {
					t.Errorf("Turn %d args = %q, want %q", turn, args, wantArgs)
				}
				if command != wantCommand {
					t.Errorf("Turn %d command = %q, want %q", turn, command, wantCommand)
				}
			}
		})
	}
}

// TestKiroCLIAgentCommand_ResumeNeverTouchesPrompt: the prompt carries only
// that turn's message and is never part of the arguments.
func TestKiroCLIAgentCommand_ResumeNeverTouchesPrompt(t *testing.T) {
	args, command := KiroCLIAgentCommand(Request{Agent: "a", Turn: 2, Prompt: "SECRET-PROMPT"})
	if strings.Contains(strings.Join(args, " "), "SECRET-PROMPT") || strings.Contains(command, "SECRET-PROMPT") {
		t.Errorf("prompt leaked into args=%v command=%q", args, command)
	}
}

// TestKiroCLI_TurnResumeNative drives the native backend with a fake kiro-cli:
// Turn 0 has no --resume, Turn 1 has it, and stdin is only that turn's message.
func TestKiroCLI_TurnResumeNative(t *testing.T) {
	rec := installFakeKiro(t, "")
	b := newKiro(t)

	resp, err := b.Invoke(context.Background(), Request{Role: RoleAgent, Agent: "a", Prompt: "first message", Turn: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(readRec(t, rec, "args")), "chat --agent a --no-interactive --trust-all-tools"; got != want {
		t.Errorf("turn 0 argv = %q, want %q", got, want)
	}
	if strings.Contains(resp.Command, "--resume") {
		t.Errorf("turn 0 Command = %q, must not resume", resp.Command)
	}
	if got := readRec(t, rec, "stdin"); got != "first message" {
		t.Errorf("turn 0 stdin = %q", got)
	}

	resp, err = b.Invoke(context.Background(), Request{Role: RoleAgent, Agent: "a", Prompt: "second message", Turn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(readRec(t, rec, "args")), "chat --agent a --no-interactive --resume --trust-all-tools"; got != want {
		t.Errorf("turn 1 argv = %q, want %q", got, want)
	}
	if want := "kiro-cli chat --agent a --no-interactive --resume --trust-all-tools"; resp.Command != want {
		t.Errorf("turn 1 Command = %q, want %q", resp.Command, want)
	}
	if got := readRec(t, rec, "stdin"); got != "second message" {
		t.Errorf("turn 1 stdin = %q, want only that turn's message", got)
	}
}

// TestStub_TurnSelectionAndCommandsPerTurn is a characterisation test: the stub
// answers with Turns[req.Turn] and runs that turn's commands (in the same
// WorkDir, so later turns see earlier turns' files) for Turn 0, 1 and 2.
func TestStub_TurnSelectionAndCommandsPerTurn(t *testing.T) {
	dir := t.TempDir()
	script := &StubScript{Turns: []StubTurn{
		{Response: "ONE", Commands: []string{"echo one > turn1.txt"}},
		{Response: "TWO", Commands: []string{"test -f turn1.txt && echo two > turn2.txt"}},
		{Response: "THREE", Commands: []string{"test -f turn2.txt && echo three > turn3.txt"}},
	}}
	stub := newTestStub(t)

	for turn, want := range []string{"ONE", "TWO", "THREE"} {
		resp, err := stub.Invoke(context.Background(), Request{
			Role: RoleAgent, Agent: "a", Prompt: "msg", WorkDir: dir, Stub: script, Turn: turn,
		})
		if err != nil {
			t.Fatalf("turn %d: %v", turn, err)
		}
		if resp.Text != want {
			t.Errorf("turn %d Text = %q, want %q", turn, resp.Text, want)
		}
		file := filepath.Join(dir, "turn"+string(rune('1'+turn))+".txt")
		if _, err := os.Stat(file); err != nil {
			t.Errorf("turn %d did not run its commands: %v", turn, err)
		}
	}

	// A turn beyond the script is an error naming the index.
	_, err := stub.Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "a", WorkDir: dir, Stub: script, Turn: 3,
	})
	if err == nil || !strings.Contains(err.Error(), "stub.turns[3]") {
		t.Errorf("turn 3 error = %v, want mention of stub.turns[3]", err)
	}
}
