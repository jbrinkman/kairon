# Validator Test Scenario for Issue #242

**Purpose**: Demonstrate and test the strict criterion-by-criterion verification behavior implemented in issue #242.

**Date**: 2026-07-14

---

## Overview

This document provides concrete test scenarios to verify that the validator now enforces strict criterion-by-criterion verification, particularly catching specification deviations like the PR #238 case.

---

## Test Scenario 1: PR #238 - Specification Deviation (FAIL Case)

### Background

This is the real-world case that motivated issue #242. The validator previously passed an implementation that deviated from the specified approach.

### Issue #235 Acceptance Criteria (Relevant Excerpt)

From Issue #235:

1. ✅ Display autocomplete menu below input field
2. ✅ Menu appears only when typing command prefix
3. ✅ Menu shows matching commands dynamically
4. ✅ Arrow keys navigate menu items
5. ✅ Enter/Tab select highlighted item
6. ✅ Escape dismisses menu
7. **❌ Position menu using `lipgloss.Place()` for overlay rendering** ← KEY CRITERION
8. ✅ Menu positioned above footer (respecting footer height from #229)
9. ✅ Menu width fits terminal width
10. ✅ Tests added for autocomplete behavior

**Key Specification Language**:
- "Position menu as overlay above footer using lipgloss.Place()"
- "Use Lipgloss v2 `Place()` for overlay positioning"
- "render as overlay using `lipgloss.Place()` instead of inline in footer"

### PR #238 Implementation

**What was implemented**:
```go
// File: internal/tui/tui.go, line ~423
func (m model) View() string {
    baseView := m.renderBaseView()
    menuOverlay := m.renderAutocompleteMenu()
    
    // ❌ Used layerOverlay() instead of lipgloss.Place()
    return layerOverlay(baseView, menuOverlay, m.width, m.height)
}

// layerOverlay() is a custom function that centers overlays on screen
func layerOverlay(base, overlay string, width, height int) string {
    // ... centers overlay on screen (different positioning than specified)
}
```

**Deviation**: Implementation used `layerOverlay()` function instead of `lipgloss.Place()`.

**Why this matters**:
- `layerOverlay()` centers overlays on screen
- `lipgloss.Place()` allows precise positioning (bottom-left above footer)
- Different behavior → specification violation
- Even though both are overlay functions, issue explicitly required `lipgloss.Place()`

### Expected Validator Output (NEW Behavior)

```markdown
# Validation Report: Issue #235 Implementation

**Issue**: #235
**PR**: #238
**Date**: 2026-07-14

---

## Acceptance Criteria Extraction

**Source**: Issue #235 (fetched via gh issue view)

### Criterion 1: Display autocomplete menu below input field
- Type: Feature Requirement
- Specified Approach: None

### Criterion 2: Menu appears only when typing command prefix
- Type: Behavior Requirement
- Specified Approach: None

### Criterion 3: Menu shows matching commands dynamically
- Type: Behavior Requirement
- Specified Approach: None

### Criterion 4: Arrow keys navigate menu items
- Type: Behavior Requirement
- Specified Approach: None

### Criterion 5: Enter/Tab select highlighted item
- Type: Behavior Requirement
- Specified Approach: None

### Criterion 6: Escape dismisses menu
- Type: Behavior Requirement
- Specified Approach: None

### Criterion 7: Position menu using lipgloss.Place()
- Type: Implementation Requirement
- **Specified Approach**: MUST use lipgloss.Place() function

### Criterion 8: Menu positioned above footer
- Type: Positioning Requirement
- Specified Approach: Respect footer height from issue #229

### Criterion 9: Menu width fits terminal width
- Type: Layout Requirement
- Specified Approach: None

### Criterion 10: Tests added for autocomplete behavior
- Type: Test Requirement
- Specified Approach: None

---

## Individual Criterion Verification

### Criterion 1: Display autocomplete menu below input field
- **Status**: ✅ PASS
- **Evidence**: Reviewed internal/tui/tui.go, menu rendering code present
- **Location**: internal/tui/tui.go:380-410
- **Verification Method**: Code inspection

### Criterion 2: Menu appears only when typing command prefix
- **Status**: ✅ PASS
- **Evidence**: Conditional rendering based on m.showAutocomplete flag
- **Location**: internal/tui/tui.go:385
- **Verification Method**: Code inspection

### Criterion 3: Menu shows matching commands dynamically
- **Status**: ✅ PASS
- **Evidence**: filterCommands() function filters based on input
- **Location**: internal/tui/autocomplete.go:45-60
- **Verification Method**: Code inspection

### Criterion 4: Arrow keys navigate menu items
- **Status**: ✅ PASS
- **Evidence**: Up/Down key handling updates selectedItem index
- **Location**: internal/tui/tui.go:120-135
- **Verification Method**: Code inspection

### Criterion 5: Enter/Tab select highlighted item
- **Status**: ✅ PASS
- **Evidence**: Enter and Tab key cases insert selected command
- **Location**: internal/tui/tui.go:140-150
- **Verification Method**: Code inspection

### Criterion 6: Escape dismisses menu
- **Status**: ✅ PASS
- **Evidence**: Escape key sets m.showAutocomplete = false
- **Location**: internal/tui/tui.go:155
- **Verification Method**: Code inspection

### Criterion 7: Position menu using lipgloss.Place()
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() usage in internal/tui/ directory
- **Finding**: Implementation uses layerOverlay() function instead
- **Location**: internal/tui/tui.go:423
- **Verification Method**: Code inspection + grep search
  ```bash
  grep -r "lipgloss.Place" internal/tui/
  # No results found
  
  grep -r "layerOverlay" internal/tui/
  # internal/tui/tui.go:423: return layerOverlay(baseView, menuOverlay, m.width, m.height)
  ```

**Failure Reason**: Issue #235 explicitly specified "use lipgloss.Place()" in multiple locations throughout the issue description. The implementation uses a custom `layerOverlay()` function instead. This constitutes a specification violation.

While `layerOverlay()` may achieve overlay rendering, it is not the specified approach. The issue required `lipgloss.Place()` specifically, making this a failed criterion regardless of functional similarity.

**Impact**: `layerOverlay()` centers overlays on screen, whereas `lipgloss.Place()` allows precise bottom-left positioning as specified. The implementation will not position the menu correctly as designed.

### Criterion 8: Menu positioned above footer
- **Status**: ✅ PASS
- **Evidence**: Positioning calculations account for footer height
- **Location**: internal/tui/tui.go:390-395
- **Verification Method**: Code inspection

### Criterion 9: Menu width fits terminal width
- **Status**: ✅ PASS
- **Evidence**: Menu width set to m.width (terminal width)
- **Location**: internal/tui/tui.go:392
- **Verification Method**: Code inspection

### Criterion 10: Tests added for autocomplete behavior
- **Status**: ✅ PASS
- **Evidence**: Test file internal/tui/autocomplete_test.go exists with test cases
- **Location**: internal/tui/autocomplete_test.go
- **Verification Method**: File inspection + go test execution
  ```bash
  go test ./internal/tui/... -v -run TestAutocomplete
  # PASS TestAutocompleteFiltering
  # PASS TestAutocompleteNavigation
  # PASS TestAutocompleteSelection
  ```

---

## Overall Validation Result

- **Status**: ❌ FAIL
- **Criteria Passed**: 9 of 10
- **Failed Criteria**: Criterion 7 (Position menu using lipgloss.Place())

**Summary**: Implementation satisfies 9 of 10 acceptance criteria but fails strict specification compliance. Criterion 7 explicitly required using `lipgloss.Place()` for positioning, but the implementation uses a custom `layerOverlay()` function. This is a specification violation that must be corrected.

**Required Action**: Replace `layerOverlay()` usage with `lipgloss.Place()` as specified in issue #235.

---

## Quality Verification

[Standard QA checks would follow here...]
```

### Verification Steps for This Test Case

1. **Prepare test environment**:
   ```bash
   # Check out PR #238 branch (or recreate scenario)
   git fetch origin pull/238/head:test-pr-238
   git checkout test-pr-238
   ```

2. **Run validator with new behavior**:
   ```bash
   cd .worktrees/test-issue-235/
   kiro-cli chat --agent validator --no-interactive --trust-all-tools \
     "Validate implementation for issue #235 in current directory"
   ```

3. **Check validation output**:
   ```bash
   cat .kiro-krew/artifacts/validator-235.md
   ```

4. **Expected result**:
   - Validation status: ❌ FAIL
   - Criterion 7 marked as FAILED
   - Specific reason: "Implementation uses layerOverlay() instead of specified lipgloss.Place()"
   - Evidence section shows grep search results
   - Overall result: FAIL with 9/10 criteria passed

5. **Verify validator exit code**:
   ```bash
   echo $?  # Should be non-zero (failure)
   ```

---

## Test Scenario 2: Compliant Implementation (PASS Case)

### Setup

Create a test implementation that satisfies ALL criteria including the specified `lipgloss.Place()` approach.

### Mock Issue Specification

**Issue #999 - Test Issue**:

Acceptance Criteria:
1. Display welcome message on startup
2. Message should use blue color
3. **Use lipgloss.NewStyle() for styling** ← Specified approach
4. Message centered on screen
5. Tests verify message display

### Compliant Implementation

```go
// File: internal/tui/welcome.go
import (
    "github.com/charmbracelet/lipgloss"
)

func renderWelcome() string {
    // ✅ Uses lipgloss.NewStyle() as specified
    style := lipgloss.NewStyle().
        Foreground(lipgloss.Color("12")).  // Blue color
        Bold(true)
    
    message := "Welcome to Kiro Krew!"
    styledMessage := style.Render(message)
    
    // Center on screen
    return lipgloss.Place(
        80, 24,
        lipgloss.Center, lipgloss.Center,
        styledMessage,
    )
}
```

### Expected Validator Output (NEW Behavior)

```markdown
# Validation Report: Issue #999 Implementation

**Issue**: #999
**Date**: 2026-07-14

---

## Acceptance Criteria Extraction

**Source**: Issue #999

### Criterion 1: Display welcome message on startup
- Type: Feature Requirement
- Specified Approach: None

### Criterion 2: Message should use blue color
- Type: Visual Requirement
- Specified Approach: None (color specified, not method)

### Criterion 3: Use lipgloss.NewStyle() for styling
- Type: Implementation Requirement
- **Specified Approach**: MUST use lipgloss.NewStyle() function

### Criterion 4: Message centered on screen
- Type: Layout Requirement
- Specified Approach: None

### Criterion 5: Tests verify message display
- Type: Test Requirement
- Specified Approach: None

---

## Individual Criterion Verification

### Criterion 1: Display welcome message on startup
- **Status**: ✅ PASS
- **Evidence**: renderWelcome() called in Init() function
- **Location**: internal/tui/tui.go:25, internal/tui/welcome.go:10-25
- **Verification Method**: Code inspection

### Criterion 2: Message should use blue color
- **Status**: ✅ PASS
- **Evidence**: Foreground color set to "12" (blue)
- **Location**: internal/tui/welcome.go:14
- **Verification Method**: Code inspection

### Criterion 3: Use lipgloss.NewStyle() for styling
- **Status**: ✅ PASS
- **Evidence**: Found lipgloss.NewStyle() usage in welcome.go
- **Location**: internal/tui/welcome.go:13
- **Verification Method**: Code inspection + text search
  ```bash
  grep "lipgloss.NewStyle()" internal/tui/welcome.go
  # internal/tui/welcome.go:13:    style := lipgloss.NewStyle().
  ```

### Criterion 4: Message centered on screen
- **Status**: ✅ PASS
- **Evidence**: lipgloss.Place() with Center, Center parameters
- **Location**: internal/tui/welcome.go:20-24
- **Verification Method**: Code inspection

### Criterion 5: Tests verify message display
- **Status**: ✅ PASS
- **Evidence**: Test file internal/tui/welcome_test.go exists
- **Location**: internal/tui/welcome_test.go
- **Verification Method**: Test execution
  ```bash
  go test ./internal/tui/... -v -run TestWelcome
  # PASS TestWelcomeMessage
  # PASS TestWelcomeColor
  ```

---

## Overall Validation Result

- **Status**: ✅ PASS
- **Criteria Passed**: 5 of 5
- **Failed Criteria**: None

**Summary**: Implementation satisfies all acceptance criteria with full specification compliance. All requirements met, including the specified use of lipgloss.NewStyle() for styling.

---

## Quality Verification

✅ All formatting checks passed
✅ All linting checks passed
✅ All tests passed (5/5)
```

### Verification Steps for This Test Case

1. **Create test implementation**:
   ```bash
   # Create files matching the compliant implementation above
   ```

2. **Run validator**:
   ```bash
   kiro-cli chat --agent validator --no-interactive --trust-all-tools \
     "Validate implementation for issue #999 in current directory"
   ```

3. **Check validation output**:
   ```bash
   cat .kiro-krew/artifacts/validator-999.md
   ```

4. **Expected result**:
   - Validation status: ✅ PASS
   - All 5 criteria marked PASS
   - Criterion 3 shows evidence of lipgloss.NewStyle() usage
   - Overall result: PASS with 5/5 criteria passed

5. **Verify validator exit code**:
   ```bash
   echo $?  # Should be 0 (success)
   ```

---

## Test Scenario 3: Alternative Approach Detection (FAIL Case)

### Mock Issue

**Issue #1000**:

Acceptance Criteria:
1. Parse command arguments
2. **Use `flag.Parse()` from standard library** ← Specified approach
3. Support --help flag
4. Support --version flag

### Non-Compliant Implementation

```go
// ❌ Uses third-party library instead of flag.Parse()
import "github.com/spf13/cobra"

func main() {
    rootCmd := &cobra.Command{  // ❌ Not using flag.Parse()
        Use: "kiro-krew",
    }
    rootCmd.Execute()
}
```

### Expected Validator Output

```markdown
### Criterion 2: Use flag.Parse() from standard library
- **Status**: ❌ FAIL
- **Evidence**: Searched for flag.Parse() in main.go
- **Finding**: Implementation uses cobra library instead
- **Location**: main.go:15-20
- **Verification Method**: Code inspection + imports check

**Failure Reason**: Issue explicitly specified "Use flag.Parse() from standard library". Implementation uses third-party cobra library for argument parsing. This is a specification violation even though cobra provides argument parsing functionality, because the issue specified the exact approach to use.

---

## Overall Validation Result

- **Status**: ❌ FAIL
- **Criteria Passed**: 3 of 4
- **Failed Criteria**: Criterion 2 (Use flag.Parse() from standard library)
```

---

## Validation Behavior Comparison

### OLD Validator Behavior (Before Issue #242)

**PR #238 Case**:
```
✅ PASS - Implementation provides autocomplete overlay functionality

Checks:
- Autocomplete menu displays ✓
- Arrow key navigation works ✓
- Selection functionality works ✓
- Positioned above footer ✓

All major features working as expected.
```

**Problem**: Accepted alternative approach, missed specification deviation.

### NEW Validator Behavior (After Issue #242)

**PR #238 Case**:
```
❌ FAIL - Specification violation detected

Criteria Status: 9/10 passed

Failed Criteria:
- Criterion 7: Position menu using lipgloss.Place() ❌
  Reason: Implementation uses layerOverlay() instead of specified lipgloss.Place()
  Evidence: grep search shows no lipgloss.Place() usage
  Location: internal/tui/tui.go:423

Required Action: Replace layerOverlay() with lipgloss.Place() as specified.
```

**Improvement**: Catches specification deviation, enforces exact approach requirement.

---

## How to Test the New Validator

### Quick Test Script

```bash
#!/bin/bash
# test-validator-242.sh - Test new validator behavior

echo "Testing new strict validator behavior..."

# Test 1: Create failing scenario (alternative approach)
echo "Test 1: Alternative approach detection..."
cat > test_impl.go << 'EOF'
package main
// Issue said: Use lipgloss.Place()
// Implementation uses: layerOverlay()
func render() string {
    return layerOverlay(base, overlay)  // ❌ Should fail
}
EOF

# Run validator
kiro-cli chat --agent validator --no-interactive --trust-all-tools \
  "Validate: Issue requires lipgloss.Place(). Check test_impl.go"

if [ $? -ne 0 ]; then
    echo "✅ Test 1 PASSED: Validator correctly failed alternative approach"
else
    echo "❌ Test 1 FAILED: Validator should have failed"
fi

# Test 2: Create passing scenario (exact approach)
echo "Test 2: Compliant implementation..."
cat > test_impl.go << 'EOF'
package main
import "github.com/charmbracelet/lipgloss"
// Issue said: Use lipgloss.Place()
// Implementation uses: lipgloss.Place() ✅
func render() string {
    return lipgloss.Place(80, 24, lipgloss.Center, lipgloss.Center, content)
}
EOF

# Run validator
kiro-cli chat --agent validator --no-interactive --trust-all-tools \
  "Validate: Issue requires lipgloss.Place(). Check test_impl.go"

if [ $? -eq 0 ]; then
    echo "✅ Test 2 PASSED: Validator correctly passed compliant implementation"
else
    echo "❌ Test 2 FAILED: Validator should have passed"
fi

# Cleanup
rm test_impl.go
```

### Manual Testing Steps

1. **Set up test environment**:
   ```bash
   mkdir -p /tmp/validator-test-242
   cd /tmp/validator-test-242
   git init
   ```

2. **Create test issue** (mock or real GitHub issue):
   ```bash
   gh issue create --title "Test Validator Strict Verification" \
     --body "Acceptance Criteria:
   1. Display message
   2. **Use fmt.Println() for output** <- Specified approach
   3. Message includes timestamp"
   ```

3. **Implement with alternative approach**:
   ```go
   // main.go - Using alternative approach
   package main
   import "log"
   func main() {
       log.Println("Hello")  // ❌ Using log instead of fmt
   }
   ```

4. **Run validator**:
   ```bash
   kiro-cli chat --agent validator --no-interactive --trust-all-tools \
     "Validate implementation for issue #[number]"
   ```

5. **Verify failure detection**:
   ```bash
   cat .kiro-krew/artifacts/validator-*.md
   # Should show FAIL for criterion 2
   # Should note log.Println used instead of fmt.Println
   ```

6. **Fix implementation**:
   ```go
   // main.go - Using specified approach
   package main
   import "fmt"
   func main() {
       fmt.Println("Hello")  // ✅ Using fmt as specified
   }
   ```

7. **Run validator again**:
   ```bash
   kiro-cli chat --agent validator --no-interactive --trust-all-tools \
     "Validate implementation for issue #[number]"
   ```

8. **Verify pass**:
   ```bash
   cat .kiro-krew/artifacts/validator-*.md
   # Should show PASS with all criteria met
   ```

---

## Expected Impact

### Detection Rate

**Before Issue #242**:
- Specification deviations: ~0% detection (holistic validation)
- Alternative approaches: Accepted if functionally similar

**After Issue #242**:
- Specification deviations: ~100% detection (criterion-by-criterion)
- Alternative approaches: Rejected when specific approach required

### Report Quality

**Before**:
```
✅ Implementation looks good. Features working as expected.
```

**After**:
```
## Criteria Status: 9/10 ✅ | 1/10 ❌

Failed Criteria:
- Criterion 7: Use lipgloss.Place() ❌
  Evidence: Found layerOverlay() at line 423
  Required: Replace with lipgloss.Place()
```

### Developer Experience

**Before**:
- Unclear why implementation was rejected in code review
- Manual detection of specification mismatches
- Rework after "completion"

**After**:
- Clear criterion-by-criterion feedback
- Specific evidence and locations provided
- Exact deviations identified automatically
- Know exactly what to fix

---

## Success Criteria for Issue #242

To verify issue #242 was successfully implemented, the validator should:

1. ✅ **Extract ALL criteria** before verification begins
2. ✅ **Verify each criterion individually** with evidence
3. ✅ **Detect specification deviations** like PR #238
4. ✅ **Reject alternative approaches** when exact approach specified
5. ✅ **Provide structured reports** with pass/fail per criterion
6. ✅ **Fail validation** if ANY criterion not met (no partial credit)
7. ✅ **Pass validation** only when ALL criteria met exactly

**Test with PR #238 scenario** to confirm:
- Criterion "Use lipgloss.Place()" is extracted
- Implementation using layerOverlay() is detected
- Validation fails with specific reason
- Evidence shows what was checked and found

---

**End of Test Scenario Documentation**
