package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// isTemplateCommand checks if a command is a template (contains placeholders like <...>)
func isTemplateCommand(command string) bool {
	return strings.Contains(command, "<") && strings.Contains(command, ">")
}

// extractTemplatePrefix extracts the prefix before the placeholder for cursor positioning
func extractTemplatePrefix(command string) string {
	idx := strings.Index(command, "<")
	if idx == -1 {
		return command
	}
	return strings.TrimSpace(command[:idx])
}

// AutocompleteInput wraps textinput.Model with built-in autocomplete functionality
type AutocompleteInput struct {
	textinput textinput.Model
	registry  *CommandRegistry
	styles    *Styles
}

// NewAutocompleteInput creates a new autocomplete input component using built-in suggestions
func NewAutocompleteInput(registry *CommandRegistry, styles *Styles) *AutocompleteInput {
	ti := textinput.New()
	ti.Prompt = "kairon> "

	// Configure solid cursor (non-blinking)
	currentStyles := ti.Styles()
	currentStyles.Cursor.Blink = false
	ti.SetStyles(currentStyles)

	// Enable built-in suggestions
	ti.ShowSuggestions = true

	ti.Focus()

	input := &AutocompleteInput{
		textinput: ti,
		registry:  registry,
		styles:    styles,
	}

	// Initialize suggestions
	input.updateSuggestions()

	return input
}

// Focused returns whether the input currently has focus
func (a *AutocompleteInput) Focused() bool {
	return a.textinput.Focused()
}

// SetFocus sets the focus state without triggering focus commands
func (a *AutocompleteInput) SetFocus(focused bool) {
	if focused {
		a.textinput.Focus()
	} else {
		a.textinput.Blur()
	}
}

// Focus gives focus to the underlying textinput
func (a *AutocompleteInput) Focus() tea.Cmd {
	return a.textinput.Focus()
}

// Blur removes focus from the underlying textinput
func (a *AutocompleteInput) Blur() {
	a.textinput.Blur()
}

// Value returns the current input value
func (a *AutocompleteInput) Value() string {
	return a.textinput.Value()
}

// SetValue sets the input value and updates autocomplete state
func (a *AutocompleteInput) SetValue(value string) {
	a.textinput.SetValue(value)
	a.updateSuggestions()
}

// Update handles key events for the autocomplete input
func (a *AutocompleteInput) Update(msg tea.Msg) (*AutocompleteInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)
	}

	// Update underlying textinput for other messages
	var cmd tea.Cmd
	a.textinput, cmd = a.textinput.Update(msg)
	return a, cmd
}

// handleKeyMsg processes key events with template command support
func (a *AutocompleteInput) handleKeyMsg(msg tea.KeyMsg) (*AutocompleteInput, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Handle template commands before normal enter processing
		current := a.textinput.CurrentSuggestion()
		if current != "" && isTemplateCommand(current) {
			// Template commands position cursor without executing
			a.textinput.SetValue(extractTemplatePrefix(current))
			a.textinput.CursorEnd()
			a.updateSuggestions()
			return a, nil
		}
		// Accept the current suggestion before execution (matches previous behavior)
		if current != "" {
			a.textinput.SetValue(current)
			a.textinput.CursorEnd()
		}
		// Fall through to normal textinput handling for execution
	}

	// Handle all other input normally and update suggestions.
	// Note: suggestions auto-hide when input doesn't match any commands
	// (built-in ShowSuggestions behavior) — no explicit dismiss needed.
	var cmd tea.Cmd
	a.textinput, cmd = a.textinput.Update(msg)

	// Update suggestions after any text change
	a.updateSuggestions()

	return a, cmd
}

// updateSuggestions updates the textinput's built-in suggestions based on current input
func (a *AutocompleteInput) updateSuggestions() {
	input := a.textinput.Value()

	if input == "" {
		// Built-in textinput clears matchedSuggestions when value is empty,
		// so no suggestions will display regardless of what we set here.
		a.textinput.SetSuggestions([]string{})
		return
	}

	// Use flattened commands for atomic compound command matching
	suggestions := a.registry.GetFlattenedMatches(input)
	a.textinput.SetSuggestions(suggestions)
}

// View renders the autocomplete input using built-in textinput rendering
func (a *AutocompleteInput) View() string {
	return a.textinput.View()
}

// HasMatchedSuggestions returns whether there are currently matched suggestions visible
func (a *AutocompleteInput) HasMatchedSuggestions() bool {
	return len(a.textinput.MatchedSuggestions()) > 0
}

// RenderSuggestionsMenu returns a styled autocomplete dropdown menu
func (a *AutocompleteInput) RenderSuggestionsMenu() string {
	if !a.HasMatchedSuggestions() {
		return ""
	}

	suggestions := a.textinput.MatchedSuggestions()
	currentIndex := a.textinput.CurrentSuggestionIndex()

	var menuItems []string
	maxItems := 10
	startIdx := 0

	// Implement sliding window to keep current selection visible
	if len(suggestions) > maxItems {
		if currentIndex >= maxItems {
			// Slide window to keep selection in view
			startIdx = currentIndex - maxItems + 1
			if startIdx > len(suggestions)-maxItems {
				startIdx = len(suggestions) - maxItems
			}
		}
		maxItems = len(suggestions) - startIdx
		if maxItems > 10 {
			maxItems = 10
		}
	} else {
		maxItems = len(suggestions)
	}

	selectedStyle := a.styles.AutocompleteSelected.Padding(0, 1)
	defaultStyle := lipgloss.NewStyle().Padding(0, 1)

	for i := 0; i < maxItems; i++ {
		actualIdx := startIdx + i
		var style lipgloss.Style
		if actualIdx == currentIndex {
			style = selectedStyle
		} else {
			style = defaultStyle
		}
		menuItems = append(menuItems, style.Render(suggestions[actualIdx]))
	}

	menuBox := strings.Join(menuItems, "\n")
	return a.styles.AutocompleteDropdown.Render(menuBox)
}

// IsValidCommand checks if current input is a valid command
func (a *AutocompleteInput) IsValidCommand() bool {
	return a.registry.IsValidCommand(a.textinput.Value())
}
