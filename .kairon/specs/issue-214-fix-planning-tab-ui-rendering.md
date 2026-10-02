# Fix Planning Tab UI Rendering - Show Existing Footer, Remove Input Borders

**Issue:** #214  
**Closes:** #214

## Problem Analysis

The Planning tab has critical rendering issues affecting user experience:

1. **Footer Not Rendering**: The existing footer implementation is not being displayed in Planning tabs, despite the unified rendering system being in place
2. **Complex Input Styling**: The input area uses complex bordered textarea styling instead of the simple terminal prompt style `[planner] >` that matches kiro-cli aesthetics
3. **Excessive UI Clutter**: Multiple borders and visual elements take up excessive screen real estate

### Root Cause Investigation

Based on codebase analysis, the issues stem from:

1. **Footer Rendering**: The footer system is correctly implemented in `internal/tui/footer.go` and integrated via `renderTabContentWithFooter()` in `internal/tui/tui.go`, but Planning tabs may be miscalculating available height
2. **Input Area Design**: Planning tab uses `charm.land/bubbles/v2/textarea` with complex styling instead of a simple terminal-style prompt
3. **Visual Inconsistency**: Planning tab styling doesn't match the minimal kiro-cli aesthetic

## Solution Approach

**Terminal-Style Input & Footer Fix**: Transform the Planning tab to use a simple terminal prompt style and ensure the existing footer renders correctly.

### Core Principles

1. **Use Existing Footer**: Leverage the already-implemented footer system - do not create new footer code
2. **Terminal Prompt Style**: Replace textarea with simple `[planner] >` prompt matching kiro-cli aesthetics
3. **Minimal Visual Clutter**: Remove unnecessary borders and complex styling elements
4. **Preserve Functionality**: Maintain all existing planning agent logic and ACP communication

## Relevant Files

### Primary Implementation Files
- **`internal/tui/planning_tab.go`** - Modify input rendering and height calculations
- **`internal/tui/styles.go`** - Update planning input styles to use simple terminal aesthetics

### Footer System Files (DO NOT MODIFY - use existing implementation)
- **`internal/tui/footer.go`** - Already correctly implemented
- **`internal/tui/tui.go`** - Already has unified rendering system

### Validation Files
- **`internal/tui/tui.go`** - Verify footer appears in Planning tabs
- Theme files in `.kiro-krew/themes/` - Ensure styling works across themes

## Team Orchestration

This is a focused UI rendering fix that can be implemented as coordinated tasks:

1. **Input Style Simplification** (Core Task) - Replace textarea with terminal prompt style
2. **Height Calculation Fix** (Parallel Task) - Ensure Planning tab accounts for footer correctly
3. **Visual Cleanup** (Dependent Task) - Remove excessive borders and styling
4. **Validation** (Final Task) - Verify footer renders and styling matches kiro-cli

Tasks 1 and 2 can run in parallel. Task 3 depends on both. Task 4 validates the complete solution.

## Step-by-Step Task Breakdown

### Task 1: Implement Terminal-Style Input Prompt
**File**: `internal/tui/planning_tab.go`
**Acceptance Criteria**:
- Replace `charm.land/bubbles/v2/textarea` with simple text input that renders as `[planner] >`
- Remove complex bordered input area styling
- Input accepts text and sends on Enter key
- Input prompt matches kiro-cli terminal aesthetic: clean, minimal, no borders
- Multi-line input support can be simplified or removed for cleaner UX
- Input area takes up minimal vertical space (1-2 lines maximum)
- Preserve all existing message sending functionality to ACP
**Dependencies**: None

### Task 2: Fix Height Calculations for Footer Rendering
**File**: `internal/tui/planning_tab.go` (Resize and View methods)
**Acceptance Criteria**:
- Ensure Planning tab `Resize()` method properly accounts for footer space
- Verify `View()` method uses height calculations that leave room for footer
- Planning tab content height = total available height minus footer height minus input height
- Footer space calculations must match the main tab behavior in `internal/tui/tui.go`
- No modifications to existing footer system in `internal/tui/footer.go`
- Footer renders correctly below Planning tab content via existing `renderTabContentWithFooter()` method
**Dependencies**: None (can run parallel with Task 1)

### Task 3: Remove Visual Clutter and Excessive Borders
**File**: `internal/tui/styles.go` and `internal/tui/planning_tab.go`
**Acceptance Criteria**:
- Remove borders from input area styling
- Remove unnecessary separators within the Planning tab content
- Simplify `GetPlanningInputStyle()` to return minimal styling
- Update `renderInputArea()` to use clean terminal prompt style
- Remove padding and margin that creates excessive spacing
- Keep separator between message area and input area minimal (single line)
- Overall Planning tab visual design matches kiro-cli minimal aesthetic
**Dependencies**: Task 1 (must complete input style changes first)

### Task 4: Validation and Testing
**Files**: Manual verification and existing test files
**Acceptance Criteria**:
- Footer displays correctly in Planning tabs (theme, status, context info)
- Input prompt displays as `[planner] >` with clean terminal styling
- No complex borders or excessive visual elements in Planning tab
- All Planning tab functionality preserved (message sending, ACP communication, session management)
- Styling works across all themes (default, light, high-contrast)
- Planning tab state indicators work correctly in footer
- Input area is minimal and doesn't consume excessive screen space
- Message display area maximized for conversation history
**Dependencies**: Tasks 1, 2, 3

## Validation Commands

```bash
# Build and test the application
go build ./cmd/kiro-krew
./kiro-krew

# Test Planning tab functionality
# In REPL: press Ctrl+Alt+P to switch to planning mode
# Verify:
# 1. Footer displays at bottom with theme/status information
# 2. Input shows as "[planner] >" without borders
# 3. Enter key sends messages
# 4. No excessive visual clutter

# Test across themes
# In REPL: theme light, theme default, theme high-contrast
# Verify styling works in all themes

# Run existing tests
go test ./internal/tui/... -v
```

## Implementation Notes

### Current Problematic Input Pattern (in planning_tab.go)
```go
// CURRENT COMPLEX TEXTAREA APPROACH - REPLACE THIS:
ta := textarea.New()
ta.Placeholder = "Type your message here..."
ta.ShowLineNumbers = false
ta.CharLimit = 4000
ta.SetWidth(80)
ta.SetHeight(3)

// Complex container styling with borders and padding
inputStyle.Width(containerWidth).Height(containerHeight).Render(input)
```

### Target Simple Terminal Prompt Pattern
```go
// REPLACE WITH SIMPLE TERMINAL STYLE:
// Use a simple text input that renders as "[planner] >" 
// Minimal height (1 line), no borders, clean prompt style
// Match kiro-cli aesthetic: simple, minimal, functional
```

### Footer Integration Pattern (DO NOT MODIFY - already correct)
```go
// EXISTING CORRECT PATTERN in tui.go:
tabContent := activeTab.View()
content = m.renderTabContentWithFooter(tabContent, activeTab.Type())

// renderTabContentWithFooter() method already correctly:
return tabContent + "\n" + footerWithDropdown
```

### Height Calculation Fix Pattern
```go
// ENSURE Planning tab accounts for footer like main tab does:
// In planning_tab.go Resize() method:
// height parameter should already exclude footer (handled by tab manager)
// But verify calculations don't double-subtract footer space

// In View() method:
// Use full available height for message area + minimal input area
// Let unified rendering system add footer below
```

## Critical Implementation Constraints

### MUST NOT MODIFY (use existing implementations):
- **`internal/tui/footer.go`** - Footer system is correctly implemented
- **`internal/tui/tui.go`** `renderTabContentWithFooter()` method - Unified rendering works
- Planning agent logic or ACP communication in `internal/session/` or `internal/acp/`
- Tab management system in `internal/tui/tab_manager.go`

### MUST PRESERVE:
- All existing Planning tab functionality (message sending, session management, state tracking)
- ACP communication and streaming responses
- Tab lifecycle management (create, close, state persistence)
- Context tracking and session restoration

### MUST CHANGE:
- Input area styling to use simple terminal prompt `[planner] >`
- Height calculations to properly accommodate existing footer
- Visual styling to remove borders and excessive padding

## Expected Visual Transformation

### Before (Current Issues):
```
┌─────────────────────────────────────────┐
│ Message history...                      │
│                                         │
└─────────────────────────────────────────┘
────────────────────────────────────────────
┌─────────────────────────────────────────┐
│ ┌─────────────────────────────────────┐ │
│ │ Type your message here...           │ │
│ │                                     │ │
│ └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
[NO FOOTER VISIBLE]
```

### After (Target Design):
```
Message history...
More conversation...
Assistant response...

────────────────────────────────────────────
[planner] > _

────────────────────────────────────────────  
kiro-krew > help
theme: default | status: ready (3 msgs) | model: claude-sonnet-4
```

## Risk Mitigation

### Potential Risks:
- Input functionality breaks during textarea replacement
- Footer still doesn't appear due to height miscalculations
- Styling changes affect other tab types

### Mitigation Strategies:
- Implement input changes incrementally, testing at each step
- Use existing tab patterns (main tab) as reference for footer integration
- Test across all tab types to ensure no regressions
- Validate with all themes to ensure consistent behavior

The solution focuses on minimal, targeted changes to achieve the clean terminal aesthetic while leveraging the existing, correctly-implemented footer system.