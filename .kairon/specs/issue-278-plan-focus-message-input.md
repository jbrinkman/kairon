# Design Specification: Plan Command Focus Management

**Issue:** #278  
**Title:** plan [description] should focus message input with cursor at end, not footer command line  
**Closes:** #278

## Executive Summary

When a user executes `plan [description]` to create a new ACP-based planning tab, the description is pre-populated into the message input field, but keyboard focus remains on the footer command line. This forces users to press Tab before they can continue typing or editing their planning request. This specification outlines the solution to automatically move focus to the message input with the cursor positioned at the end of the pre-filled text.

## Problem Statement

### Current Behavior
1. User types: `plan implement user authentication`
2. New planning tab opens with "implement user authentication" in the message input
3. Focus stays on footer command line
4. User must press Tab to move focus to message input before they can edit or submit

### Expected Behavior
1. User types: `plan implement user authentication`
2. New planning tab opens with "implement user authentication" in the message input
3. Focus automatically moves to message input with cursor positioned at the end
4. User can immediately continue typing, edit, or press Enter to submit

### Root Cause

The issue lies in the `handlePlan` function in `internal/tui/commands.go`. Currently:

1. A new planning tab is created via `CreateAndAddPlanningTab`
2. The description is added as a user message to the tab
3. `switchActiveTab` is called, which defaults to `FocusTargetFooter` for new tabs (no pre-existing focus state)
4. The focus state is not pre-seeded before the tab switch

The focus management system in Kairon uses a centralized `tabFocusStates` map (in `tui.go`) that tracks which input field should have focus for each tab. When switching to a tab, `switchActiveTab` checks this map and restores the previous focus state, defaulting to footer focus if no state exists.

## Solution Approach

### High-Level Strategy

Follow the implementation pattern from the Flowdra PR #41, which successfully solved a similar focus management issue. The solution leverages the existing focus restoration mechanism in `switchActiveTab` without requiring modifications to that function.

**Key Insight:** Pre-seed the focus state in `tabFocusStates` before calling `switchActiveTab`, so the existing restoration logic applies the desired focus target.

### Implementation Pattern

```go
// In handlePlan, after creating planningTab but before switchActiveTab:
m.tabFocusStates[planningTab.ID()] = FocusTargetMessage

// After adding description message (if provided):
planningTab.textinput.CursorEnd() // Position cursor at end of pre-filled text
```

This approach:
- Uses the existing focus management infrastructure
- Requires no changes to `switchActiveTab` or focus restoration logic
- Preserves existing behavior for other entry paths (Ctrl+Alt+P hotkey, tab switching)
- Minimal code changes confined to `handlePlan`

## Codebase Analysis

### Relevant Files and Components

#### 1. **internal/tui/commands.go** (Primary Implementation)
- **Function:** `handlePlan(description string)` (lines 219-284)
  - Creates planning tab via `CreateAndAddPlanningTab`
  - Sets tab title based on description
  - Adds initial messages to the tab
  - Calls `switchActiveTab` to activate the new tab
  - **Changes needed:** Pre-seed focus state, position cursor

#### 2. **internal/tui/tui.go** (Focus Management Infrastructure)
- **Field:** `tabFocusStates map[string]FocusTarget` (line ~90)
  - Centralized map tracking focus state per tab ID
- **Function:** `switchActiveTab(index int)` (lines 1150-1200)
  - Captures focus state from current tab
  - Restores focus state for newly active tab
  - Defaults to `FocusTargetFooter` if no state exists
  - **No changes needed** - works correctly with pre-seeded state

#### 3. **internal/tui/planning_tab.go** (Planning Tab Component)
- **Type:** `PlanningTab` struct (lines 35-65)
  - Contains `textinput textinput.Model` field
  - Tracks `focusTarget FocusTarget`
- **Method:** `AddMessage(role, content string)` (line 440)
  - Appends message to conversation history
  - Updates viewport content
  - Saves session state
- **Method:** `RestoreFocusState(target FocusTarget)` (lines 1050-1065)
  - Applies focus state to the tab's text input
  - Called by `switchActiveTab`
- **Field Access:** `textinput` is public, allowing direct cursor manipulation

#### 4. **internal/tui/focus_state.go** (Focus State Types)
- **Type:** `FocusTarget` string enum
- **Constants:** `FocusTargetFooter`, `FocusTargetMessage`
- **No changes needed**

### Focus Management Flow

```
User Command: "plan [description]"
    ↓
handlePlan() in commands.go
    ↓
CreateAndAddPlanningTab() → creates PlanningTab
    ↓
[NEW] m.tabFocusStates[planningTab.ID()] = FocusTargetMessage
    ↓
AddMessage("user", description) → if description provided
    ↓
[NEW] planningTab.textinput.CursorEnd() → if description provided
    ↓
switchActiveTab(newTabIndex)
    ↓
    └→ Captures current tab focus state
    └→ Sets active tab to planning tab
    └→ Retrieves m.tabFocusStates[planningTab.ID()] → FocusTargetMessage
    └→ Calls planningTab.RestoreFocusState(FocusTargetMessage)
        └→ Focuses textinput, blurs footer
```

### Textinput API Reference

From the `charm.land/bubbles/v2/textinput` package:

- **`CursorEnd()`**: Moves cursor to end of input value
  - Implementation: `m.SetCursor(len(m.value))`
- **`SetCursor(pos int)`**: Moves cursor to specific position
  - Clamps position to valid range `[0, len(value)]`

Both methods are available on the `textinput.Model` and are safe to call directly.

## Implementation Plan

### Step-by-Step Task Breakdown

#### Task 1: Implement Focus Pre-Seeding in handlePlan
**Agent:** builder  
**File:** `internal/tui/commands.go`

**Changes:**
1. After `CreateAndAddPlanningTab` succeeds (line ~235), add:
   ```go
   // Pre-seed focus state to target message input when tab opens
   m.tabFocusStates[planningTab.ID()] = FocusTargetMessage
   ```

2. After `AddMessage("user", description)` (if description provided, line ~273), add:
   ```go
   // Position cursor at end of pre-filled description
   planningTab.textinput.CursorEnd()
   ```

**Acceptance Criteria:**
- Focus state is set to `FocusTargetMessage` immediately after planning tab creation
- Cursor is positioned at end of description text when description is provided
- Empty `plan` command behavior sets focus state to message input
- Code compiles without errors
- No modifications to `switchActiveTab` or other focus management functions

**Validation Commands:**
```bash
go build ./internal/tui
go vet ./internal/tui
```

#### Task 2: Add Integration Tests for Focus Behavior
**Agent:** builder  
**File:** `internal/tui/plan_focus_test.go` (new file)

**Test Cases to Implement:**

**T1: `plan [description]` focuses message input**
```go
func TestPlanWithDescriptionFocusesMessageInput(t *testing.T)
```
- Setup: Console tab active, footer focused
- Action: Execute `plan implement feature X` through `model.Update`
- Assert:
  - New planning tab created and active
  - `m.tabFocusStates[planningTab.ID()]` equals `FocusTargetMessage`
  - Planning tab's message input contains "implement feature X"
  - Footer input is not focused (`m.input.IsFocused()` is false)
  - Planning tab's textinput is focused

**T2: Empty `plan` command focuses message input**
```go
func TestPlanEmptyFocusesMessageInput(t *testing.T)
```
- Setup: Console tab active
- Action: Execute `plan` through `model.Update`
- Assert:
  - New planning tab created and active
  - `m.tabFocusStates[planningTab.ID()]` equals `FocusTargetMessage`
  - Focus is on message input

**T3: Cursor positioned at end of description**
```go
func TestPlanDescriptionCursorAtEnd(t *testing.T)
```
- Setup: Console tab active
- Action: Execute `plan hello world` through `model.Update`
- Assert:
  - Message input value contains "hello world"
  - Cursor position is at end (position equals length of text)

**T4: Tab toggle works after initial focus**
```go
func TestTabToggleAfterPlanFocus(t *testing.T)
```
- Setup: Planning tab created via `plan [description]`, message input focused
- Action: Send Tab key through `model.Update`
- Assert:
  - Focus moves to footer command line
  - `m.tabFocusStates[planningTab.ID()]` equals `FocusTargetFooter`
  - Subsequent Tab returns focus to message input

**T5: Other planning tab entry paths unchanged**
```go
func TestPlanningTabSwitchPreservesDefaultFocus(t *testing.T)
```
- Setup: Planning tab already exists (created earlier), switch away
- Action: Switch back to planning tab
- Assert:
  - Focus state is restored from `m.tabFocusStates`
  - If no prior override, behavior matches existing logic

**T6: `plan classic` behavior unchanged**
```go
func TestPlanClassicUnchanged(t *testing.T)
```
- Action: Execute `plan classic [description]`
- Assert: Subprocess-based planner launches (verify no TUI focus changes)

**Acceptance Criteria:**
- All 6 test cases pass
- Tests exercise full input routing through `model.Update`
- Tests verify focus state in `tabFocusStates` map
- Tests check actual focus state of textinput and footer input components
- Code coverage includes both description and empty plan command paths

**Validation Commands:**
```bash
go test ./internal/tui -run TestPlan -v
task test
```

#### Task 3: Validate Complete Implementation
**Agent:** validator  
**File:** All modified files

**Validation Activities:**
1. Run full test suite: `task test`
2. Run linters: `task fmt:check`
3. Build verification: `go build ./...`
4. Vet analysis: `go vet ./...`
5. Manual testing scenario:
   - Start Kairon in console mode
   - Execute `plan test description`
   - Verify: focus is on message input, cursor at end
   - Type additional text immediately
   - Verify: text appends correctly to pre-filled description
   - Press Tab
   - Verify: focus moves to footer
   - Press Tab again
   - Verify: focus returns to message input
   - Execute `plan` (no description)
   - Verify: focus is on message input, empty input field
   - Press Ctrl+Alt+P to create planning tab via hotkey
   - Verify: focus behavior matches existing (footer focused)

**Acceptance Criteria:**
- All unit tests pass (including new tests from Task 2)
- All integration tests pass
- Linting passes with no warnings
- Build succeeds without errors
- Manual testing confirms all acceptance criteria from issue #278 (AC1-AC6)
- No regressions in existing focus behavior

**Validation Commands:**
```bash
task test
task fmt:check
go build ./...
go vet ./...
```

## Team Orchestration

### Task Dependencies

- **Task 1** (Implementation) has no dependencies - can start immediately
- **Task 2** (Tests) depends on Task 1 - requires implementation to test against
- **Task 3** (Validation) depends on Tasks 1 and 2 - validates complete solution

### Parallel Execution Opportunities

None - tasks must execute sequentially due to dependencies.

### Agent Assignments

- **builder**: Executes Tasks 1 and 2 (implementation and test authoring)
- **validator**: Executes Task 3 (verification and quality assurance)

## Acceptance Criteria Mapping

### Issue AC1: Focus targets message input on `plan [description]`
- **Satisfied by:** Task 1 implementation + Task 2 test T1

### Issue AC2: Cursor positioned at end of pre-filled description
- **Satisfied by:** Task 1 implementation (CursorEnd call) + Task 2 test T3

### Issue AC3: Empty `plan` command behavior unchanged
- **Satisfied by:** Task 1 implementation (focus pre-seeding) + Task 2 test T2

### Issue AC4: Other planning tab entry paths unchanged
- **Satisfied by:** No changes to switchActiveTab + Task 2 test T5

### Issue AC5: Tab toggle still works after initial focus
- **Satisfied by:** No changes to focus transfer logic + Task 2 test T4

### Issue AC6: `plan classic` behavior unchanged
- **Satisfied by:** No changes to handlePlanClassic + Task 2 test T6

## Testing Strategy

### Unit Tests
- Focus state pre-seeding logic
- Cursor positioning after description added
- Empty vs. non-empty description handling

### Integration Tests
- Full command execution through `model.Update`
- Focus state coordination between footer and message input
- Tab switching and focus restoration
- Tab key toggle behavior

### Manual Testing
- End-to-end user workflow verification
- Visual confirmation of cursor position
- Tab navigation UX verification

## Constraints and Considerations

### Constraints from Issue
- All changes confined to `internal/tui/` directory
- Must not modify `switchActiveTab` focus restoration logic
- Must preserve existing focus behavior for Ctrl+Alt+P hotkey toggle
- Must preserve existing behavior for tab switching
- No changes to `plan classic` / `handlePlanSubprocess` paths
- All tests, linting, and build must pass

### Design Decisions

**Why pre-seed focus state instead of modifying switchActiveTab?**
- Follows proven pattern from Flowdra PR #41
- Leverages existing focus restoration infrastructure
- Minimizes code changes and complexity
- Preserves existing behavior for other entry paths
- No risk of breaking tab switching or focus restoration

**Why use textinput.CursorEnd() instead of calculating position?**
- Built-in method handles edge cases correctly
- Automatically clamps to valid range
- More maintainable and readable
- Consistent with bubbles API conventions

**Why add tests in a new file instead of existing test files?**
- Focused test organization for feature-specific validation
- Easier to review and maintain
- Clear mapping to issue requirements
- Avoids bloating existing test files

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-focus-preseeding"
    agent: "builder"
    description: "Implement focus pre-seeding in handlePlan to target message input when creating planning tabs"
    dependencies: []
    acceptance_criteria:
      - "Focus state set to FocusTargetMessage immediately after planning tab creation"
      - "Cursor positioned at end of description text when description provided"
      - "Empty plan command sets focus state to message input"
      - "Code compiles without errors"
      - "No modifications to switchActiveTab or other focus management functions"
    validation_commands:
      - "go build ./internal/tui"
      - "go vet ./internal/tui"

  - id: "add-integration-tests"
    agent: "builder"
    description: "Add comprehensive integration tests for plan command focus behavior in new test file"
    dependencies: ["implement-focus-preseeding"]
    acceptance_criteria:
      - "Test T1: plan [description] focuses message input"
      - "Test T2: empty plan command focuses message input"
      - "Test T3: cursor positioned at end of description"
      - "Test T4: Tab toggle works after initial focus"
      - "Test T5: other planning tab entry paths unchanged"
      - "Test T6: plan classic behavior unchanged"
      - "All tests exercise full input routing through model.Update"
      - "Tests verify focus state in tabFocusStates map"
      - "Tests check actual focus state of textinput and footer components"
    validation_commands:
      - "go test ./internal/tui -run TestPlan -v"
      - "go test ./internal/tui"

  - id: "validate-complete-implementation"
    agent: "validator"
    description: "Verify complete implementation meets all acceptance criteria and passes all quality checks"
    dependencies: ["implement-focus-preseeding", "add-integration-tests"]
    acceptance_criteria:
      - "All unit tests pass including new tests"
      - "All integration tests pass"
      - "Linting passes with no warnings"
      - "Build succeeds without errors"
      - "All issue acceptance criteria AC1-AC6 satisfied"
      - "No regressions in existing focus behavior"
      - "Manual testing confirms expected user experience"
    validation_commands:
      - "task test"
      - "task fmt:check"
      - "go build ./..."
      - "go vet ./..."
```

## Validation Commands

### Build Verification
```bash
go build ./internal/tui
go build ./cmd/kairon
go build ./...
```

### Test Execution
```bash
# Run new focus tests
go test ./internal/tui -run TestPlan -v

# Run all TUI tests
go test ./internal/tui -v

# Run full test suite
task test
```

### Code Quality
```bash
# Format checking
task fmt:check

# Linting
go vet ./...
```

### Manual Testing Script
```bash
# 1. Start Kairon
./kairon

# 2. Test plan with description
kairon> plan implement user authentication
# Expected: Focus on message input, cursor at end

# 3. Type immediately
[continue typing in message input]
# Expected: Text appends to "implement user authentication"

# 4. Test Tab toggle
[Press Tab]
# Expected: Focus moves to footer
[Press Tab again]
# Expected: Focus returns to message input

# 5. Test empty plan
kairon> plan
# Expected: Focus on message input, empty field

# 6. Test hotkey entry (Ctrl+Alt+P)
[Press Ctrl+Alt+P from console]
# Expected: Default footer focus (existing behavior)
```

## Out of Scope

The following are explicitly out of scope for this issue:

- Changes to focus behavior on tabs other than planning tabs
- Modifications to the Ctrl+Alt+P hotkey toggle initial focus state
- Changes to `switchActiveTab` or general focus management architecture
- Any changes outside `internal/tui/` directory
- Changes to `plan classic` subprocess-based planning
- Changes to tab switching or focus restoration logic
- Additional focus management features not specified in AC1-AC6

## Risk Assessment

### Low Risk
- **Focus pre-seeding approach:** Proven pattern from Flowdra PR #41
- **Minimal code changes:** Two lines in `handlePlan`
- **No architectural changes:** Uses existing infrastructure
- **Comprehensive testing:** Six integration tests cover all paths

### Mitigation Strategies
- **Regression prevention:** Tests validate existing behavior preserved
- **Manual testing:** End-to-end UX verification before PR
- **Code review:** Focus on focus state coordination and cursor positioning

## Success Criteria

The implementation will be considered successful when:

1. All six acceptance criteria (AC1-AC6) from issue #278 are satisfied
2. All six testing requirements (T1-T6) pass
3. All existing tests continue to pass (no regressions)
4. Code quality checks pass (linting, vetting, formatting)
5. Manual testing confirms expected user experience
6. Code changes are confined to `internal/tui/` as specified
7. Single pull request delivers complete, working solution

---

**Specification Author:** architect  
**Date:** 2026-10-03  
**Version:** 1.0
