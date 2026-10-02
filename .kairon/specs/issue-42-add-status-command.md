# Design Specification: Add status command to TUI

**Issue:** #42  
**Title:** Add status command to TUI  
**Created:** 2026-06-19  

Closes #42

## Solution Approach

The status command already exists and is fully implemented in the TUI. The current implementation provides comprehensive agent status information including:

- Active agents with issue numbers, titles, status, and elapsed time
- Tab management information
- Interactive navigation options
- Clean table formatting with proper truncation

**No changes are required** - this is a documentation-only specification as the feature is already complete.

## Current Implementation Analysis

The `handleStatus()` function in `internal/tui/commands.go` already provides:

1. **Agent Information Display**
   - Lists all running agents with issue numbers and titles
   - Shows elapsed time since agent start
   - Displays agent status (running/completed/failed)
   - Proper sorting by issue number for deterministic ordering

2. **Table Formatting**
   - Clean, readable table format with proper column alignment
   - Title truncation based on available overlay width
   - "No agents running" message when appropriate
   - Separate sections for running and stopped agents

3. **Interactive Features**
   - Numbered list for running agents (1-9)
   - Press number keys to open agent views
   - Tab navigation information and shortcuts

4. **Responsive Design**
   - Dynamic width calculation for overlay content
   - Proper title truncation to prevent layout issues
   - Scales to terminal size

## Relevant Files

- `internal/tui/commands.go` - Contains fully implemented `handleStatus()` function (lines 59-147)
- `internal/agent/manager.go` - Provides agent data structure and List() method
- `internal/tui/tui.go` - Model structure with agent tracking

## Team Orchestration

No coordination required - feature is complete and working as specified.

## Step-by-Step Task Breakdown

**Task 1: Verification** ✅ COMPLETE
- [x] Verify status command exists and is registered
- [x] Confirm agent information display works
- [x] Validate table formatting and truncation
- [x] Test "No agents running" state

**Task 2: Documentation** ✅ COMPLETE  
- [x] Status command is documented in README.md
- [x] Command appears in help output
- [x] Usage examples available

## Validation Commands

The following commands can verify the implementation:

```bash
# Start kiro-krew and test status command
./kiro-krew
# In REPL: type "status" and press enter

# Verify help shows status command
# In REPL: type "help" and press enter

# Test with running agents (requires labeled GitHub issue)
# In REPL: "watch start" then "status" after agents spawn
```

## Implementation Status

✅ **FEATURE COMPLETE** - No implementation needed.

The status command fully meets all acceptance criteria:
- [x] `status` command shows all active agents  
- [x] Output includes issue number, title (truncated), status, and elapsed time
- [x] Command shows "No agents running" when none are active
- [x] Table formatting is clean and readable

The current implementation exceeds the basic requirements by providing:
- Interactive agent selection via number keys
- Tab management information
- Proper responsive design
- Comprehensive status information
