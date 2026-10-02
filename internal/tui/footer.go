package tui

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/session"
)

// FooterManager manages the two-row footer display system
type FooterManager struct {
	styles            *Styles
	config            *config.Config
	contextTracker    *ContextTracker
	autocompleteInput *AutocompleteInput
	tabManager        *TabManager
	width             int
	height            int
}

// FooterContent represents the structured content for the footer
type FooterContent struct {
	InputRow  string // Row 1: Command entry area
	StatusRow string // Row 2: Contextual information
}

// NewFooterManager creates a new footer manager
func NewFooterManager(styles *Styles, config *config.Config, autocompleteInput *AutocompleteInput, tabManager *TabManager) *FooterManager {
	return &FooterManager{
		styles:            styles,
		config:            config,
		contextTracker:    NewContextTracker(),
		autocompleteInput: autocompleteInput,
		tabManager:        tabManager,
	}
}

// Resize updates the footer dimensions
func (fm *FooterManager) Resize(width, height int) {
	fm.width = width
	fm.height = height
}

// GetContextTracker returns the context tracker for external access
func (fm *FooterManager) GetContextTracker() *ContextTracker {
	return fm.contextTracker
}

// RenderFooter renders the complete two-row footer
func (fm *FooterManager) RenderFooter(activeTabType TabType) FooterContent {
	// Row 1: Command entry area (same as existing prompt functionality)
	inputRow := fm.renderInputRow()

	// Row 2: Contextual information based on tab type
	statusRow := fm.renderStatusRow(activeTabType)

	return FooterContent{
		InputRow:  inputRow,
		StatusRow: statusRow,
	}
}

// renderInputRow creates the command entry row
func (fm *FooterManager) renderInputRow() string {
	if fm.autocompleteInput == nil {
		return ""
	}

	// Use full width for the prompt input
	promptWidth := fm.width
	if promptWidth < 1 {
		promptWidth = 1
	}

	promptInput := fm.autocompleteInput.View()
	return fm.styles.Prompt.Width(promptWidth).Render(promptInput)
}

// renderStatusRow creates the contextual information row based on tab type
func (fm *FooterManager) renderStatusRow(activeTabType TabType) string {
	// Prefer tab manager as source of truth for active tab type when available
	if fm.tabManager != nil {
		if activeTab := fm.tabManager.GetActiveTab(); activeTab != nil {
			activeTabType = activeTab.Type()
		}
	}

	// Base information shown on all tabs
	baseInfo := fm.renderBaseInfo()

	// Enhanced information for planning tabs
	if activeTabType == TabTypePlanning {
		planningInfo := fm.renderPlanningInfo()
		planningStatusInfo := fm.renderPlanningStatusInfo()
		planningACPInfo := fm.renderPlanningACPInfo()

		// Combine all planning information with priority ordering
		var planningInfoParts []string

		// Priority 1: Session status (most important)
		if planningStatusInfo != "" {
			planningInfoParts = append(planningInfoParts, planningStatusInfo)
		}

		// Priority 2: ACP connection and model info
		if planningACPInfo != "" {
			planningInfoParts = append(planningInfoParts, planningACPInfo)
		}

		// Priority 3: Context and directory info
		if planningInfo != "" {
			planningInfoParts = append(planningInfoParts, planningInfo)
		}

		if len(planningInfoParts) > 0 {
			combinedPlanningInfo := strings.Join(planningInfoParts, " | ")
			return fm.joinStatusInfo(baseInfo, combinedPlanningInfo)
		}
	}

	return baseInfo
}

// renderBaseInfo renders the base information shown on all tabs
func (fm *FooterManager) renderBaseInfo() string {
	return fmt.Sprintf("theme: %s", fm.config.Theme)
}

// renderPlanningInfo renders context usage and directory information for planning tabs
func (fm *FooterManager) renderPlanningInfo() string {
	// Only show context info if context tracker is active
	if !fm.contextTracker.IsActive() {
		return ""
	}

	planningContext := fm.contextTracker.GetPlanningContext()
	if planningContext == nil {
		return ""
	}

	var parts []string

	// Context usage with visual indicator
	if contextUsage := fm.contextTracker.FormatContextUsage(); contextUsage != "" {
		parts = append(parts, contextUsage)

		// Add usage warning as separate segment for clean visual separation
		used, total := fm.contextTracker.GetUsage()
		if total > 0 {
			percentage := (used * 100) / total
			if percentage > 90 {
				parts = append(parts, "🔥 critical usage")
			} else if percentage > 75 {
				parts = append(parts, "⚠️ high usage")
			}
		}
	}

	// Directory information with improved formatting
	if planningContext.Directory != "" {
		dirInfo := fmt.Sprintf("📁 %s", planningContext.Directory)
		parts = append(parts, dirInfo)
	}

	return strings.Join(parts, " | ")
}

// renderPlanningStatusInfo renders the status of the active planning tab
func (fm *FooterManager) renderPlanningStatusInfo() string {
	if fm.tabManager == nil {
		return ""
	}

	activeTab := fm.tabManager.GetActiveTab()
	if activeTab == nil || activeTab.Type() != TabTypePlanning {
		return ""
	}

	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		return ""
	}

	state := planningTab.GetState()

	// Format status with appropriate visual indicator
	var statusText string
	switch state {
	case session.PlanningStateIdle:
		statusText = "status: ready"
	case session.PlanningStateActive:
		statusText = "status: ● processing"
	case session.PlanningStateCompleted:
		statusText = "status: ✓ completed"
	case session.PlanningStateFailed:
		statusText = "status: ✗ failed"
	case session.PlanningStateReadOnly:
		statusText = "status: 🔒 read-only"
	default:
		statusText = "status: ready"
	}

	// Add message count if there are messages
	messageCount := planningTab.GetMessageCount()
	if messageCount > 0 {
		plural := ""
		if messageCount != 1 {
			plural = "s"
		}
		statusText += fmt.Sprintf(" (%d msg%s)", messageCount, plural)
	}

	return statusText
}

// renderPlanningACPInfo renders ACP connection and model information for planning tabs
func (fm *FooterManager) renderPlanningACPInfo() string {
	var parts []string

	// Get model information from context tracker if available
	if fm.contextTracker.IsActive() {
		if planningContext := fm.contextTracker.GetPlanningContext(); planningContext != nil {
			if planningContext.Model != "" {
				parts = append(parts, fmt.Sprintf("model: %s", planningContext.Model))
			}
		}
	}

	// Get active planning tab connection status
	if fm.tabManager != nil {
		if activeTab := fm.tabManager.GetActiveTab(); activeTab != nil && activeTab.Type() == TabTypePlanning {
			if planningTab, ok := activeTab.(*PlanningTab); ok {
				// Add processing status indicator
				if planningTab.IsActive() {
					parts = append(parts, "● active")
				}
			}
		}
	}

	return strings.Join(parts, " | ")
}

// joinStatusInfo combines base and additional status information
func (fm *FooterManager) joinStatusInfo(baseInfo, additionalInfo string) string {
	if baseInfo == "" {
		return additionalInfo
	}
	if additionalInfo == "" {
		return baseInfo
	}

	return fmt.Sprintf("%s | %s", baseInfo, additionalInfo)
}

// RenderWithSeparator renders the footer with separator line above it
func (fm *FooterManager) RenderWithSeparator(activeTabType TabType) string {
	footer := fm.RenderFooter(activeTabType)

	// Create separator line
	separator := fm.styles.Separator.Render(strings.Repeat("─", fm.width))

	// Combine separator with footer rows
	var result []string
	result = append(result, separator)
	result = append(result, footer.InputRow)

	// Only add status row if it has content
	if strings.TrimSpace(footer.StatusRow) != "" {
		// Render and trim any trailing newlines to prevent extra blank lines
		renderedStatusRow := strings.TrimRight(fm.styles.ThemeLabel.Render(footer.StatusRow), "\n")
		result = append(result, renderedStatusRow)
	}

	return strings.Join(result, "\n")
}

// GetFooterHeight returns the total height occupied by the footer
func (fm *FooterManager) GetFooterHeight() int {
	// Base height: separator (1) + input row (1) + status row (1) = 3
	return 3
}
