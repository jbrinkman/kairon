# Design Specification: Fix ANSI Escape Sequence Rendering Artifacts in Autocomplete Menu and Dialog Overlays

**Issue**: #244  
**Title**: Fix ANSI escape sequence rendering artifacts in autocomplete menu and dialog overlays  
**Closes**: #244

## Problem Statement

The autocomplete dropdown menu displays extraneous characters to the right of each menu row, including fragments like "5m", "m", and ";255;255m". These are ANSI escape sequence fragments from the footer's status row that are being improperly sliced and rendered.

The same bug exists in the dialog overlay rendering code (`layerOverlay`), though it doesn't manifest visibly in practice because dialog overlays use fixed widths that typically cover underlying styled text completely. Both functions must be fixed to prevent future rendering issues.

## Root Cause Analysis

### Code Location
`internal/tui/tui.go` - Functions added in commit dbbc9a3:
- `layerMenuOverlay()` - Line ~910 (positions autocomplete menu at bottom-left)
- `layerOverlay()` - Line ~853 (centers dialog overlays)

### The Bug Pattern (Present in Both Functions)

```go
// In layerMenuOverlay (line ~927):
overlayWidth := lipgloss.Width(overlayLine)  // Returns VISUAL width (strips ANSI codes)
// ...
if overlayWidth < len(baseLine) {
    afterOverlay = baseLine[overlayWidth:]   // BUG: Uses visual width as BYTE index
}

// In layerOverlay (line ~895):
overlayWidth := lipgloss.Width(overlayLine)  // Returns VISUAL width (strips ANSI codes)
// ...
afterStart := startCol + overlayWidth        // Calculate byte offset using visual width
if afterStart < len(baseLine) {
    afterOverlay = baseLine[afterStart:]     // BUG: Uses visual width as BYTE index
}
```

### Why It Fails

1. `lipgloss.Width()` returns the **visual width** of a string (excluding ANSI escape sequences)
2. This visual width is then used as a **byte offset** to slice `baseLine`
3. When `baseLine` contains ANSI codes (from styled status text), the byte length exceeds the visual width
4. Slicing at the visual width position cuts through the middle of ANSI escape sequences
5. The remaining fragments (like "5m" from color codes, "m" from time units in escape sequences) become visible

### Example Failure Scenario

```
baseLine = "theme: \x1b[3mhigh-contrast\x1b[0m" (20 characters visual, but more bytes with ANSI codes)
overlayWidth = 15 (visual width of menu item)
baseLine[15:] slices into the middle of the ANSI sequence, leaving fragments like ";255m" visible
```

### Why `layerOverlay` Doesn't Show the Bug

- Dialog overlays have fixed widths set via `m.styles.OverlayBorder.Width(m.overlayWidth - 4)`
- They are centered and typically wide enough to completely cover underlying styled text
- The bug exists but doesn't manifest because there's no partial overlap with ANSI sequences

### Why `layerMenuOverlay` Shows the Bug

- Menu items have variable widths based on command length
- Menu is positioned at bottom-left, not centered
- Short menu items (like "exit", "plan") don't fully cover the status row, causing partial ANSI code slicing

## Solution Approach

Use ANSI-aware string manipulation from the `github.com/charmbracelet/x/ansi` package (already available in dependencies as v0.11.7) to correctly calculate byte offsets when slicing strings containing ANSI escape sequences.

### Available ANSI Package Functions

From `github.com/charmbracelet/x/ansi`:
- `ansi.Truncate(s string, w int, tail string)` - Truncates string to visual width, preserving ANSI codes
- `ansi.StringWidth(s string)` - Returns visual width (same as lipgloss.Width)
- The package provides ANSI-aware string operations that preserve escape sequences

### Implementation Strategy

Replace byte-offset slicing with ANSI-aware truncation that:
1. Understands ANSI escape sequences as zero-width formatting
2. Calculates correct byte positions for visual character boundaries
3. Preserves complete ANSI sequences in the remaining string

## Relevant Files

- `internal/tui/tui.go` - Contains both buggy functions
  - `layerMenuOverlay()` at line ~910
  - `layerOverlay()` at line ~853
  
- `internal/tui/autocomplete.go` - Renders the menu overlay (calls layerMenuOverlay via View())

## Team Orchestration

This is a focused bug fix with no cross-component dependencies. Implementation sequence:

### Task 1: Fix layerMenuOverlay Function
**Acceptance Criteria**:
- Import `github.com/charmbracelet/x/ansi` package
- Replace `baseLine[overlayWidth:]` with ANSI-aware slicing logic
- Menu rows display without ANSI escape sequence fragments
- Status row styling remains intact
**Dependencies**: None

### Task 2: Fix layerOverlay Function
**Acceptance Criteria**:
- Apply identical ANSI-aware slicing fix to layerOverlay
- Dialog overlays continue to render correctly (no regression)
- Function handles ANSI codes correctly for future edge cases
**Dependencies**: Task 1 (use same approach)

### Task 3: Verify Across All Themes
**Acceptance Criteria**:
- Test with default theme
- Test with high-contrast theme
- Test with light theme
- No artifacts appear in any theme configuration
**Dependencies**: Task 1, Task 2

### Task 4: Manual Terminal Verification
**Acceptance Criteria**:
- Start kiro-krew in actual terminal
- Type partial commands to trigger autocomplete menu
- Navigate through all menu items
- Verify no extraneous characters appear
- Verify status row remains properly styled
**Dependencies**: Task 1, Task 2, Task 3

## Step-by-Step Task Breakdown

### Task 1: Fix layerMenuOverlay Function

**Location**: `internal/tui/tui.go` line ~910

**Current buggy code**:
```go
func (m model) layerMenuOverlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate start row: above the 3-line footer
	startRow := len(baseLines) - 3 - len(overlayLines)
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			afterOverlay := ""
			// BUG IS HERE: Similar to layerOverlay, slice the remainder of the base line
			if overlayWidth < len(baseLine) {
				afterOverlay = baseLine[overlayWidth:]  // ← CUTS THROUGH ANSI CODES
			}

			result[targetRow] = overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}
```

**Fixed implementation**:
```go
func (m model) layerMenuOverlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate start row: above the 3-line footer
	startRow := len(baseLines) - 3 - len(overlayLines)
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			afterOverlay := ""
			// FIX: Use ANSI-aware truncation to find the correct byte offset
			// Truncate to overlayWidth visual characters, then get the remainder
			if overlayWidth < lipgloss.Width(baseLine) {
				// Strip the first overlayWidth visual characters, preserving ANSI codes
				afterOverlay = ansi.Truncate(baseLine, lipgloss.Width(baseLine)-overlayWidth, "")
				// Actually, we need to skip the first overlayWidth characters
				// Better approach: truncate from the start, measure bytes, then slice
				truncated := ansi.Truncate(baseLine, overlayWidth, "")
				// Count bytes in truncated to find where to slice
				afterOverlay = strings.TrimPrefix(baseLine, truncated)
			}

			result[targetRow] = overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}
```

**Alternative simpler approach using rune iteration**:
```go
func (m model) layerMenuOverlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate start row: above the 3-line footer
	startRow := len(baseLines) - 3 - len(overlayLines)
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			// FIX: Use ansi.Truncate to remove the first overlayWidth visual characters
			// This preserves ANSI codes and gives us the correct remainder
			afterOverlay := ""
			baseWidth := lipgloss.Width(baseLine)
			if overlayWidth < baseWidth {
				// Get everything after the first overlayWidth visual characters
				// Strategy: Use ansi package to handle this properly
				remainder := skipVisualChars(baseLine, overlayWidth)
				afterOverlay = remainder
			}

			result[targetRow] = overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}

// skipVisualChars returns the substring starting after 'n' visual characters,
// preserving ANSI escape sequences
func skipVisualChars(s string, n int) string {
	if n <= 0 {
		return s
	}
	
	// Use ansi.Truncate to get first n visual chars, then find remainder
	prefix := ansi.Truncate(s, n, "")
	// Find where prefix ends in the original string by comparing bytes
	// This is complex due to ANSI codes...
	
	// Better: iterate through string tracking visual width
	visualCount := 0
	inEscape := false
	bytePos := 0
	
	for bytePos < len(s) {
		if visualCount >= n {
			break
		}
		
		// Check for ANSI escape sequence start
		if s[bytePos] == '\x1b' && bytePos+1 < len(s) && s[bytePos+1] == '[' {
			inEscape = true
			bytePos++
			continue
		}
		
		if inEscape {
			bytePos++
			// Look for escape sequence end (letter)
			if (s[bytePos-1] >= 'A' && s[bytePos-1] <= 'Z') || (s[bytePos-1] >= 'a' && s[bytePos-1] <= 'z') {
				inEscape = false
			}
			continue
		}
		
		// Regular character - advance visual count
		_, size := utf8.DecodeRuneInString(s[bytePos:])
		bytePos += size
		visualCount++
	}
	
	return s[bytePos:]
}
```

**BEST APPROACH - Using ansi package utilities**:

After reviewing the ansi package more carefully, the cleanest solution is:

```go
import (
	"github.com/charmbracelet/x/ansi"
)

func (m model) layerMenuOverlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	startRow := len(baseLines) - 3 - len(overlayLines)
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			// FIX: Truncate baseLine to keep only characters after overlayWidth position
			// First get total visual width
			baseWidth := lipgloss.Width(baseLine)
			
			afterOverlay := ""
			if overlayWidth < baseWidth {
				// Calculate how many visual characters to keep (everything after overlay)
				keepWidth := baseWidth - overlayWidth
				// Use ansi to strip the visual prefix and keep the suffix
				afterOverlay = ansi.Truncate(baseLine, baseWidth, "")
				afterOverlay = ansi.Truncate(afterOverlay[len(ansi.Truncate(baseLine, overlayWidth, "")):], keepWidth, "")
			}

			result[targetRow] = overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}
```

Actually, the cleanest approach is to use a helper function that skips visual characters:

```go
import (
	"unicode/utf8"
	"github.com/charmbracelet/x/ansi"
)

// skipVisualWidth returns the portion of s after the first n visual characters,
// preserving ANSI escape sequences
func skipVisualWidth(s string, n int) string {
	if n <= 0 {
		return s
	}
	
	var (
		visualCount int
		bytePos     int
		inEscape    bool
	)
	
	for bytePos < len(s) && visualCount < n {
		if s[bytePos] == '\x1b' {
			// Start of ANSI escape sequence
			inEscape = true
			bytePos++
			continue
		}
		
		if inEscape {
			if (s[bytePos] >= 'A' && s[bytePos] <= 'Z') || 
			   (s[bytePos] >= 'a' && s[bytePos] <= 'z') {
				// End of ANSI escape sequence
				inEscape = false
			}
			bytePos++
			continue
		}
		
		// Regular character - count it and advance
		_, size := utf8.DecodeRuneInString(s[bytePos:])
		bytePos += size
		visualCount++
	}
	
	if bytePos >= len(s) {
		return ""
	}
	return s[bytePos:]
}

func (m model) layerMenuOverlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	startRow := len(baseLines) - 3 - len(overlayLines)
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			// FIX: Use ANSI-aware skip function to get remainder after overlay
			afterOverlay := skipVisualWidth(baseLine, overlayWidth)

			result[targetRow] = overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}
```

### Task 2: Fix layerOverlay Function

**Location**: `internal/tui/tui.go` line ~853

Apply the same fix pattern using the `skipVisualWidth` helper:

```go
func (m model) layerOverlay(base, overlay string) string {
	// Center overlay on base view
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > overlayW {
			overlayW = w
		}
	}

	startRow := (m.height - len(overlayLines)) / 2
	startCol := (m.width - overlayW) / 2
	// Ensure overlay stays within bounds
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}
	if startRow+len(overlayLines) > len(baseLines) {
		startRow = len(baseLines) - len(overlayLines)
		if startRow < 0 {
			startRow = 0
		}
	}

	// Create result with same length as base
	result := make([]string, len(baseLines))
	copy(result, baseLines)

	// Overlay the content
	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			overlayWidth := lipgloss.Width(overlayLine)

			// Pad base line if needed
			if len(baseLine) < startCol {
				baseLine += strings.Repeat(" ", startCol-len(baseLine))
			}

			// Calculate portions of base line
			beforeOverlay := ""
			if startCol > 0 && len(baseLine) > 0 {
				// FIX: Use ANSI-aware truncation for beforeOverlay
				beforeOverlay = ansi.Truncate(baseLine, startCol, "")
			}

			// FIX: Use ANSI-aware skip for afterOverlay
			afterOverlay := skipVisualWidth(baseLine, startCol+overlayWidth)

			result[targetRow] = beforeOverlay + overlayLine + afterOverlay
		}
	}

	return strings.Join(result, "\n")
}
```

### Task 3: Add Required Import

Add the import at the top of `internal/tui/tui.go`:

```go
import (
	// ... existing imports ...
	"unicode/utf8"
	"github.com/charmbracelet/x/ansi"
)
```

## Validation Commands

### Build and Test
```bash
# Build the project
go build ./cmd/kiro-krew

# Run all existing tests
go test ./internal/tui/... -v

# Run with coverage
go test ./internal/tui/... -cover
```

### Manual Testing
```bash
# Start kiro-krew
./kiro-krew

# In the REPL:
# 1. Type partial commands to trigger autocomplete (e.g., "wa", "st", "pl")
# 2. Use up/down arrows to navigate menu
# 3. Verify no extraneous characters appear to the right of menu items
# 4. Verify status row below menu remains properly styled
# 5. Try all theme configurations:

theme default
theme high-contrast  
theme light

# For each theme, test autocomplete menu again
```

### Visual Verification Checklist
- [ ] No "5m", "m", ";255;255m" or similar fragments visible
- [ ] Menu items display cleanly without trailing artifacts
- [ ] Status row styling intact (theme name, model info, etc.)
- [ ] Works with short commands (exit, plan, help)
- [ ] Works with long commands (watch start, theme high-contrast)
- [ ] Dialog overlays (help, about, status) still render correctly
- [ ] All three themes display correctly

## Acceptance Criteria

1. ✅ **No Extraneous Characters**: Menu rows display without trailing ANSI escape sequence fragments
2. ✅ **All Themes**: Fix works correctly across all theme configurations (default, high-contrast, light)
3. ✅ **All Menu Items**: No artifacts appear for any menu item, regardless of length
4. ✅ **Status Row Intact**: The footer status row below the menu remains properly styled
5. ✅ **Dialog Overlays Unchanged**: Dialog overlays continue to render correctly (no regression)
6. ✅ **Both Functions Fixed**: Both `layerMenuOverlay()` and `layerOverlay()` handle ANSI codes correctly
7. ✅ **Existing Tests Pass**: All existing tests continue to pass
8. ✅ **Visual Verification**: Manual testing confirms clean rendering in actual terminal

## Constraints

- Must preserve existing menu and dialog overlay positioning and layout behavior
- Must handle ANSI escape sequences correctly when calculating byte offsets
- Must maintain performance (no significant rendering slowdown)
- Should not change the visual appearance or behavior of dialog overlays
- Use existing dependency `github.com/charmbracelet/x/ansi v0.11.7` (already in go.mod)

## Implementation Notes

### Key Insight

The core issue is confusing **visual width** (what the user sees) with **byte length** (what slicing operates on). ANSI escape sequences have zero visual width but occupy bytes in the string.

### Solution Pattern

1. Add `skipVisualWidth` helper function that iterates through string byte-by-byte
2. Track when inside ANSI escape sequences (starts with `\x1b[`, ends with letter)
3. Only count visual characters when not inside escape sequences
4. Return substring starting at the correct byte position

### ANSI Escape Sequence Structure

```
\x1b[<parameters>m
^    ^           ^
|    |           |
|    |           +-- End marker (letter)
|    +-------------- Parameters (numbers, semicolons)
+------------------- Escape sequence start
```

Common patterns:
- `\x1b[0m` - Reset all attributes
- `\x1b[3m` - Italic
- `\x1b[38;2;R;G;Bm` - 24-bit foreground color (explains ";255;255m" fragments)

### Performance Considerations

The `skipVisualWidth` function adds minimal overhead:
- Single pass through string (O(n) where n is string length)
- No allocations except final substring
- Only called for lines that have overlays (typically 1-10 lines per frame)
- Significantly better than creating intermediate allocations

## Dependencies

**Required Package** (already in dependencies):
- `github.com/charmbracelet/x/ansi v0.11.7`

**Standard Library**:
- `unicode/utf8` - For proper rune handling in skipVisualWidth

## Testing Strategy

### Unit Tests (Nice to Have)
While not required for this bug fix, consider adding tests for `skipVisualWidth`:

```go
func TestSkipVisualWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		skip     int
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "hello world",
			skip:     6,
			expected: "world",
		},
		{
			name:     "ANSI codes in middle",
			input:    "hello \x1b[3mworld\x1b[0m",
			skip:     6,
			expected: "\x1b[3mworld\x1b[0m",
		},
		{
			name:     "ANSI codes at start",
			input:    "\x1b[3mhello world\x1b[0m",
			skip:     6,
			expected: "world\x1b[0m",
		},
		{
			name:     "skip beyond length",
			input:    "short",
			skip:     10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipVisualWidth(tt.input, tt.skip)
			if result != tt.expected {
				t.Errorf("skipVisualWidth(%q, %d) = %q, want %q", 
					tt.input, tt.skip, result, tt.expected)
			}
		})
	}
}
```

### Integration Tests
Existing tests in `internal/tui/*_test.go` should continue to pass without modification.

## Risk Assessment

**Low Risk** - This is a focused bug fix that:
- Only affects string slicing logic in two functions
- Uses well-tested standard library UTF-8 handling
- Preserves all existing behavior except fixing the bug
- Has clear visual verification criteria

## Summary

This specification provides a complete solution to fix ANSI escape sequence rendering artifacts by replacing naive byte-offset slicing with ANSI-aware string manipulation. The fix applies to both `layerMenuOverlay` and `layerOverlay` functions, ensuring consistent behavior across all overlay types and preventing future rendering issues.
