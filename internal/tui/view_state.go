package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/agent"
)

// ViewType represents the different views in the TUI
type ViewType int

const (
	ViewConsole ViewType = iota
	ViewAgentOutput
)

// ViewState holds the state for each view type
type ViewState struct {
	// Console view state
	consoleScrollPos int

	// Agent output view state
	outputScrollPos int
	outputViewState *OutputView
}

// ViewManager manages view transitions and state preservation
type ViewManager struct {
	currentView ViewType
	state       ViewState
	width       int
	height      int
}

// NewViewManager creates a new view manager
func NewViewManager() *ViewManager {
	return &ViewManager{
		currentView: ViewConsole,
		state: ViewState{
			consoleScrollPos: 0,
			outputScrollPos:  0,
		},
	}
}

// CurrentView returns the current active view
func (vm *ViewManager) CurrentView() ViewType {
	return vm.currentView
}

// ToggleView switches between console and agent output views
func (vm *ViewManager) ToggleView() {
	if vm.currentView == ViewConsole {
		vm.currentView = ViewAgentOutput
	} else {
		vm.currentView = ViewConsole
	}
}

// SetView explicitly sets the current view
func (vm *ViewManager) SetView(view ViewType) {
	vm.currentView = view
}

// Update handles view manager updates and forwards to appropriate view
func (vm *ViewManager) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		vm.width = msg.Width
		vm.height = msg.Height

		// Update output view if initialized
		if vm.state.outputViewState != nil {
			vm.state.outputViewState.Resize(msg.Width, msg.Height)
		}
	}

	// Forward messages to active view's state
	if vm.currentView == ViewAgentOutput && vm.state.outputViewState != nil {
		_, cmd := vm.state.outputViewState.Update(msg)
		return cmd
	}

	return nil
}

// InitOutputView initializes the agent output view with manager and styles
func (vm *ViewManager) InitOutputView(manager *agent.Manager, styles *Styles) {
	vm.state.outputViewState = NewOutputView(manager, styles)
	if vm.width > 0 && vm.height > 0 {
		vm.state.outputViewState.Resize(vm.width, vm.height)
	}
}

// UpdateOutputViewStyles updates output view styles.
func (vm *ViewManager) UpdateOutputViewStyles(styles *Styles) {
	if vm.state.outputViewState != nil {
		vm.state.outputViewState.SetStyles(styles)
	}
}

// RenderCurrentView renders the active view
func (vm *ViewManager) RenderCurrentView(baseView string) string {
	if vm.currentView == ViewAgentOutput && vm.state.outputViewState != nil {
		return vm.state.outputViewState.View()
	}
	return baseView
}

// PreserveConsoleScroll saves console scroll position
func (vm *ViewManager) PreserveConsoleScroll(pos int) {
	vm.state.consoleScrollPos = pos
}

// GetConsoleScroll returns saved console scroll position
func (vm *ViewManager) GetConsoleScroll() int {
	return vm.state.consoleScrollPos
}
