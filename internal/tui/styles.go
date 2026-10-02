package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/jbrinkman/kairon/internal/config"
)

// getColorOrFallback returns the primary color if not empty, otherwise returns fallback
func getColorOrFallback(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

type Styles struct {
	Prompt    lipgloss.Style
	Activity  lipgloss.Style
	Separator lipgloss.Style
	Success   lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style

	// Agent status styles
	AgentSuccess lipgloss.Style
	AgentFail    lipgloss.Style

	// Tab header styles
	TabActive        lipgloss.Style
	TabInactive      lipgloss.Style
	TabInactiveHover lipgloss.Style
	TabClose         lipgloss.Style

	// Overlay styles
	OverlayBorder     lipgloss.Style
	OverlayTitle      lipgloss.Style
	OverlayContent    lipgloss.Style
	OverlayBackground lipgloss.Style
	ThemeLabel        lipgloss.Style

	// Autocomplete styles
	AutocompleteGhost    lipgloss.Style
	AutocompleteDropdown lipgloss.Style
	AutocompleteSelected lipgloss.Style
	AutocompleteError    lipgloss.Style

	// Planning tab styles
	PlanningUser               lipgloss.Style
	PlanningAssistant          lipgloss.Style
	PlanningInputActive        lipgloss.Style
	PlanningInputInactive      lipgloss.Style
	PlanningError              lipgloss.Style
	PlanningStreamingIndicator lipgloss.Style
	PlanningPrompt             lipgloss.Style
}

func NewStyles(theme *config.Theme) *Styles {
	return &Styles{
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Prompt)),
		Activity:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Activity)),
		Separator: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Separator)),
		Success:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Success)),
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Warning)),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Error)),

		// Agent status styles with fallbacks for backwards compatibility
		AgentSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color(getColorOrFallback(theme.Colors.AgentSuccess, theme.Colors.Success))),
		AgentFail:    lipgloss.NewStyle().Foreground(lipgloss.Color(getColorOrFallback(theme.Colors.AgentFail, theme.Colors.Error))),

		// Tab header styles
		TabActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Primary)).
			Background(lipgloss.Color(theme.Colors.Surface)).
			Bold(true).
			Padding(0, 1),
		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextMuted)).
			Padding(0, 1),
		TabInactiveHover: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextMuted)).
			Padding(0, 1).
			Underline(true),
		TabClose: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Warning)).
			Bold(true),

		OverlayBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(theme.Colors.Primary)).
			Background(lipgloss.Color(theme.Colors.Surface)).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Primary)).
			Bold(true),
		OverlayContent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextPrimary)).
			Background(lipgloss.Color(theme.Colors.Surface)),
		OverlayBackground: lipgloss.NewStyle().
			Background(lipgloss.Color(theme.Colors.Surface)),
		ThemeLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextMuted)).
			Italic(true),

		// Autocomplete styles
		AutocompleteGhost: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextMuted)),
		AutocompleteDropdown: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(theme.Colors.Primary)).
			Background(lipgloss.Color(theme.Colors.Surface)).
			Padding(0, 1),
		AutocompleteSelected: lipgloss.NewStyle().
			Background(lipgloss.Color(theme.Colors.Primary)).
			Foreground(lipgloss.Color(theme.Colors.Surface)),
		AutocompleteError: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Error)).
			Bold(true),

		// Planning tab styles - minimal terminal aesthetic with clean styling
		PlanningUser: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Primary)).
			Bold(true),
		PlanningAssistant: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextPrimary)),
		PlanningInputActive:   lipgloss.NewStyle(), // Minimal style - no borders or padding
		PlanningInputInactive: lipgloss.NewStyle(), // Minimal style - no borders or padding
		PlanningError: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Error)).
			Bold(true),
		PlanningStreamingIndicator: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Warning)).
			Bold(true),
		PlanningPrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Primary)).
			Bold(true),
	}
}

// Planning Tab Responsive Styling Methods

// GetPlanningInputStyle returns minimal input style - clean terminal aesthetic
func (s *Styles) GetPlanningInputStyle(focused bool, width int) lipgloss.Style {
	// Return minimal style regardless of focus or width - no borders, no padding, no backgrounds
	return lipgloss.NewStyle()
}

// GetPlanningMessageStyle returns minimal message style with clean spacing
func (s *Styles) GetPlanningMessageStyle(role string, width int) lipgloss.Style {
	if role == "user" {
		return s.PlanningUser
	}
	return s.PlanningAssistant
}
