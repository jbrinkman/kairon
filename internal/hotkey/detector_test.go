package hotkey

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHandleKeyMsg(t *testing.T) {
	const errText = "hotkey toggle not available outside kairon context"

	ctrlAltP := tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl | tea.ModAlt}

	tests := []struct {
		name       string
		key        tea.KeyPressMsg
		inContext  bool
		wantTrig   bool   // cmd yields HotkeyTriggeredMsg
		wantErrMsg string // cmd yields HotkeyErrorMsg with this text
	}{
		{name: "ctrl+alt+p in kairon context", key: ctrlAltP, inContext: true, wantTrig: true},
		{name: "ctrl+alt+p outside kairon context", key: ctrlAltP, inContext: false, wantErrMsg: errText},
		{name: "ctrl+c in kairon context", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, inContext: true},
		{name: "ctrl+c outside kairon context", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, inContext: false},
		{name: "ctrl+p in kairon context", key: tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, inContext: true},
		{name: "ctrl+p outside kairon context", key: tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, inContext: false},
		{name: "alt+p in kairon context", key: tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt}, inContext: true},
		{name: "alt+p outside kairon context", key: tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt}, inContext: false},
		{name: "plain p in kairon context", key: tea.KeyPressMsg{Code: 'p', Text: "p"}, inContext: true},
		{name: "plain p outside kairon context", key: tea.KeyPressMsg{Code: 'p', Text: "p"}, inContext: false},
		{name: "empty key in kairon context", key: tea.KeyPressMsg{}, inContext: true},
		{name: "empty key outside kairon context", key: tea.KeyPressMsg{}, inContext: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv registers cleanup that restores the original value.
			if tt.inContext {
				t.Setenv("KAIRON_WATCHER_PID", "12345")
			} else {
				t.Setenv("KAIRON_WATCHER_PID", "")
				if err := os.Unsetenv("KAIRON_WATCHER_PID"); err != nil {
					t.Fatalf("unset KAIRON_WATCHER_PID: %v", err)
				}
			}

			cmd := HandleKeyMsg(tt.key)

			if !tt.wantTrig && tt.wantErrMsg == "" {
				if cmd != nil {
					t.Fatalf("HandleKeyMsg(%q) inContext=%v: expected nil cmd, got non-nil", tt.key.String(), tt.inContext)
				}
				return
			}

			if cmd == nil {
				t.Fatalf("HandleKeyMsg(%q) inContext=%v: expected non-nil cmd, got nil", tt.key.String(), tt.inContext)
			}

			switch got := cmd().(type) {
			case HotkeyTriggeredMsg:
				if !tt.wantTrig {
					t.Errorf("HandleKeyMsg(%q) inContext=%v: got HotkeyTriggeredMsg, want error %q", tt.key.String(), tt.inContext, tt.wantErrMsg)
				}
			case HotkeyErrorMsg:
				if tt.wantTrig {
					t.Errorf("HandleKeyMsg(%q) inContext=%v: got HotkeyErrorMsg, want HotkeyTriggeredMsg", tt.key.String(), tt.inContext)
					return
				}
				if got.Err == nil {
					t.Fatalf("HandleKeyMsg(%q) inContext=%v: HotkeyErrorMsg has nil Err", tt.key.String(), tt.inContext)
				}
				if got.Err.Error() != tt.wantErrMsg {
					t.Errorf("HandleKeyMsg(%q) inContext=%v: error = %q, want %q", tt.key.String(), tt.inContext, got.Err.Error(), tt.wantErrMsg)
				}
			default:
				t.Errorf("HandleKeyMsg(%q) inContext=%v: cmd yielded unexpected message %T", tt.key.String(), tt.inContext, got)
			}
		})
	}
}
