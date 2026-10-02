# Design Specification: Fix Update Status Error When No Releases Exist

**Issue**: #131  
**Title**: Fix: Update status shows error instead of hiding when no releases exist  
**Closes**: #131

## Problem Analysis

The update status section shows a semver validation error instead of being hidden when no GitHub releases exist. The root cause is that when GitHub API returns an empty or invalid `TagName`, the code reaches semver validation logic which fails, showing "Unable to compare versions (non-semver format)" error instead of treating it as no releases.

### Current Behavior
- Shows error: "Unable to compare versions (non-semver format)" 
- Displays "Current: 0.6.0, Latest: " (empty latest version)
- Appears in both about dialog and console activity feed

### Expected Behavior  
- Completely hide update status section when no releases exist
- No error messages or empty spacing in about dialog
- No update status activity in console mode

## Solution Approach

The issue occurs in the `updateCheckMsg` handler in `internal/tui/tui.go`. Currently, the code only handles explicit `ErrNoReleases` errors, but the GitHub API can return a release object with an empty/invalid `TagName` field instead of an error.

### Key Insight
The fix requires detecting when `msg.release.TagName` is empty or invalid and treating it the same as `ErrNoReleases`.

## Relevant Files

- **internal/tui/tui.go** - Main update logic in `updateCheckMsg` case (lines ~211-270)
- **internal/github/client.go** - Release fetching via GitHub API 
- **internal/tui/about.go** - About dialog content management
- **internal/tui/commands.go** - Update check command and message types

## Team Orchestration

This is a focused bug fix that can be implemented by a single developer. No coordination with external teams required.

## Step-by-Step Task Breakdown

### Task 1: Implement Empty TagName Detection
**File**: `internal/tui/tui.go`  
**Location**: `updateCheckMsg` case handler (around line 232)

**Changes**:
1. Add validation for empty/invalid `TagName` after the current `ErrNoReleases` check
2. When `msg.release != nil` but `msg.release.TagName` is empty or whitespace-only, treat as no releases
3. Apply same logic as `ErrNoReleases` case - hide status section completely

**Implementation**:
```go
case updateCheckMsg:
    updateLines := []string{}
    if msg.err != nil {
        // Check if error is ErrNoReleases - hide update status section entirely
        if errors.Is(msg.err, github.ErrNoReleases) {
            if m.activeOverlay == overlayAbout {
                m.aboutDialog.UpdateStatusLine([]string{})
                m.overlayContent.content = append(m.aboutDialog.GetFullContent(), "", "Press ESC to close")
            }
            return m, nil
        }
        // Other errors - show error message as before
        // ... existing error handling
    } else {
        // NEW: Check for empty/invalid TagName - treat as no releases
        if msg.release == nil || strings.TrimSpace(msg.release.TagName) == "" {
            if m.activeOverlay == overlayAbout {
                m.aboutDialog.UpdateStatusLine([]string{})
                m.overlayContent.content = append(m.aboutDialog.GetFullContent(), "", "Press ESC to close")
            }
            return m, nil
        }
        
        // Existing version comparison logic continues...
        current := version.Version
        latest := msg.release.TagName
        // ... rest of existing logic unchanged
    }
```

**Acceptance Criteria**:
- Empty `TagName` values are detected and handled
- Same behavior as `ErrNoReleases` - complete hiding of update status
- No activity lines added to console mode
- About dialog flows naturally without empty spacing

### Task 2: Verify No Side Effects
**File**: `internal/tui/tui.go`  
**Action**: Review and verify existing behavior is preserved

**Verification Points**:
- Valid releases still show update status correctly
- Development builds still show "Development build" status  
- Other error conditions (network errors, rate limits) still show appropriate error messages
- Semver validation errors for malformed but non-empty tags still display properly

**Acceptance Criteria**:
- All existing update status behaviors work unchanged
- Only empty/whitespace TagName values are treated as no releases

## Validation Commands

### Test Valid Release
```bash
# Test with a repository that has releases
# Should show normal update status
./kiro-krew
# Run: about
```

### Test No Releases
```bash 
# Test with repository that has no releases or empty TagName
# Should hide update status section completely
./kiro-krew  
# Run: about
# Verify no "Update Status" section appears
```

### Test Error Conditions
```bash
# Test with network error or rate limit
# Should show appropriate error message (not hidden)
# Can be simulated by temporarily blocking GitHub API access
```

### Manual Testing
1. Open about dialog when no releases exist - verify no update status section
2. Check console activity feed - verify no update status messages
3. Test with valid releases - verify normal behavior  
4. Test with network errors - verify error messages still appear

## Notes

- This is a minimal fix targeting only the specific issue
- No changes to GitHub API client needed - issue is in the response handling
- No changes to about dialog structure needed - existing hide mechanism works
- Maintains backward compatibility with all existing behaviors