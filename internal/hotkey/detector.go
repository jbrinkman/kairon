package hotkey

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// HotkeyTriggeredMsg represents a hotkey event for Bubble Tea
type HotkeyTriggeredMsg struct{}

// HotkeyErrorMsg represents a hotkey error event
type HotkeyErrorMsg struct {
	Err error
}

// IsKaironContext validates that the current process is running in a kairon terminal
func IsKaironContext() bool {
	return os.Getenv("KAIRON_WATCHER_PID") != ""
}

// IsCtrlOptionP checks if the key sequence matches Ctrl+Option+P
func IsCtrlOptionP(msg tea.KeyPressMsg) bool {
	return msg.String() == "ctrl+alt+p"
}

// HandleKeyMsg processes key messages and returns hotkey events when appropriate
func HandleKeyMsg(msg tea.KeyPressMsg) tea.Cmd {
	if IsCtrlOptionP(msg) {
		if !IsKaironContext() {
			return func() tea.Msg {
				return HotkeyErrorMsg{
					Err: fmt.Errorf("hotkey toggle not available outside kairon context"),
				}
			}
		}

		return func() tea.Msg {
			return HotkeyTriggeredMsg{}
		}
	}
	return nil
}
