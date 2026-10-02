---
name: validator-conventions
description: Structured templates and patterns for strict criterion-by-criterion validation. Enforces exact specification compliance and provides copy-paste report templates.
---

# Validator Conventions

Comprehensive templates, patterns, and examples for performing strict criterion-by-criterion validation of implementations against issue specifications.

## Structured Validation Report Template

**Copy this template for every validation**. Fill in each section completely before reporting results.

```markdown
# Validation Report: Issue #[NUMBER]

**Issue Title**: [Title]
**Repository**: [owner/repo]
**Validation Date**: [ISO timestamp]
**Validator**: [agent name/version]

---

## Phase 1: Acceptance Criteria Extraction

**Source**: Issue #[number] (retrieved via `gh issue view [number] --json body --repo [repo]`)

### Criterion 1: [Clear, specific description]
- **Type**: [Feature/Behavior/Implementation/Test/Documentation]
- **Specified Approach**: [If issue specifies HOW to implement, note it here. e.g., "use lipgloss.Place()"]
- **Source**: [Quote from issue or section reference]

### Criterion 2: [Clear, specific description]
- **Type**: [Feature/Behavior/Implementation/Test/Documentation]
- **Specified Approach**: [None / specific function/library/method]
- **Source**: [Quote from issue or section reference]

### Criterion 3: [Clear, specific description]
- **Type**: [Feature/Behavior/Implementation/Test/Documentation]
- **Specified Approach**: [None / specific function/library/method]
- **Source**: [Quote from issue or section reference]

[Continue for ALL criteria - typically 5-15 criteria per issue]

**Total Criteria Extracted**: [N]

---

## Phase 2: Individual Criterion Verification

### Criterion 1: [Repeat description from Phase 1]
- **Status**: ✅ PASS / ❌ FAIL
- **Evidence**: [Describe what was checked - file inspected, command run, text searched]
- **Location**: [File path and line numbers if applicable]
- **Finding**: [What was found in the code/output]
- **Verification Method**: [Code inspection / Test execution / Command run / Text search]

**Reasoning**: [1-2 sentences explaining why this criterion passed or failed]

### Criterion 2: [Repeat description from Phase 1]
- **Status**: ✅ PASS / ❌ FAIL
- **Evidence**: [Describe what was checked]
- **Location**: [File path and line numbers]
- **Finding**: [What was found]
- **Verification Method**: [Method used]

**Reasoning**: [Explanation]

### Criterion 3: [Repeat description from Phase 1]
- **Status**: ✅ PASS / ❌ FAIL
- **Evidence**: [Describe what was checked]
- **Location**: [File path and line numbers]
- **Finding**: [What was found]
- **Verification Method**: [Method used]

**Reasoning**: [Explanation]

[Continue for ALL criteria - every criterion must have explicit pass/fail status]

---

## Overall Validation Result

- **Status**: ✅ PASS / ❌ FAIL
- **Criteria Passed**: [X] of [N]
- **Failed Criteria**: [List criterion numbers, e.g., "2, 5, 7" or "None"]

### Summary
[2-3 sentence verdict stating whether implementation meets ALL acceptance criteria. If failed, list the specific unmet criteria.]

### Failed Criteria Details
[If any criteria failed, list them here with brief explanation of what's missing/wrong]

1. **Criterion [N]**: [Brief description of failure]
2. **Criterion [N]**: [Brief description of failure]

### Recommendations
[If failed, provide specific guidance for builder on what needs to be fixed]

---

## Quality Assurance Results

[Document results of running project QA tools - formatting, linting, tests]

### Formatting
- **Command**: [e.g., `task fmt:check`]
- **Result**: ✅ PASS / ❌ FAIL
- **Output**: [Relevant output if failed]

### Linting
- **Command**: [e.g., `task lint`]
- **Result**: ✅ PASS / ❌ FAIL
- **Output**: [Relevant output if failed]

### Tests
- **Command**: [e.g., `task test`]
- **Result**: ✅ PASS / ❌ FAIL
- **Coverage**: [If applicable]
- **Output**: [Relevant output if failed]

---

## Verification Completed
**Timestamp**: [ISO timestamp]
```

---

## Criterion Extraction Patterns

### Types of Acceptance Criteria

#### 1. Feature Criteria
**Pattern**: "Add [feature]", "Implement [functionality]", "Create [component]"

**Example from Issue**:
> "Add autocomplete menu for commands"

**Extraction**:
```markdown
### Criterion: Autocomplete menu functionality
- **Type**: Feature
- **Specified Approach**: None
- **Source**: "Add autocomplete menu for commands"
```

#### 2. Behavior Criteria
**Pattern**: "When [condition], then [behavior]", "Should [action] when [condition]"

**Example from Issue**:
> "Menu should close when user presses Escape"

**Extraction**:
```markdown
### Criterion: Escape key closes menu
- **Type**: Behavior
- **Specified Approach**: None
- **Source**: "Menu should close when user presses Escape"
```

#### 3. Implementation Criteria (CRITICAL)
**Pattern**: "Use [function]", "Call [method]", "Import [library]", "Implement using [approach]"

**Example from Issue**:
> "Position menu as overlay using lipgloss.Place()"

**Extraction**:
```markdown
### Criterion: Use lipgloss.Place() for menu positioning
- **Type**: Implementation
- **Specified Approach**: lipgloss.Place() function
- **Source**: "Position menu as overlay using lipgloss.Place()"
```

**CRITICAL**: Implementation criteria specify HOW to implement. These are MANDATORY requirements, not suggestions.

#### 4. Test Criteria
**Pattern**: "Add tests for [functionality]", "Ensure test coverage", "Write unit tests"

**Example from Issue**:
> "Add unit tests for command matching logic"

**Extraction**:
```markdown
### Criterion: Unit tests for command matching
- **Type**: Test
- **Specified Approach**: None (unit tests)
- **Source**: "Add unit tests for command matching logic"
```

#### 5. Documentation Criteria
**Pattern**: "Document [feature]", "Update README", "Add comments"

**Example from Issue**:
> "Update README with autocomplete usage examples"

**Extraction**:
```markdown
### Criterion: README includes autocomplete examples
- **Type**: Documentation
- **Specified Approach**: None
- **Source**: "Update README with autocomplete usage examples"
```

### Extraction Best Practices

1. **Read the entire issue** before extracting criteria
2. **Extract every requirement** - don't group or summarize
3. **Quote the source** for each criterion
4. **Identify implementation requirements** explicitly
5. **Number criteria sequentially** for easy reference
6. **Complete extraction BEFORE verification** - this is Phase 1

---

## Specification Compliance Checklist

Use this checklist for EVERY criterion that mentions a specific implementation approach.

### Step 1: Identify Implementation Requirements

**Questions to ask**:
- [ ] Does the issue specify a function name to use?
- [ ] Does the issue specify a library or package to import?
- [ ] Does the issue specify a method or API to call?
- [ ] Does the issue specify an approach or pattern to follow?

**If YES to any**: This is an **Implementation Criterion** requiring exact compliance.

### Step 2: Verify Exact Approach Used

**For function requirements**:
```bash
# Search for exact function usage in relevant files
grep -rn "functionName" --include="*.go" .

# Example: Verify lipgloss.Place() is used
grep -rn "lipgloss.Place" --include="*.go" internal/
```

**For library/package requirements**:
```bash
# Check imports in relevant files
grep -n "import" file.go | grep "libraryName"

# Example: Verify lipgloss v2 is imported
grep -n "import" internal/tui/tui.go | grep "lipgloss"
```

**For method/API requirements**:
```bash
# Search for method calls
grep -rn ".MethodName(" --include="*.ext" .

# Example: Verify specific API endpoint is called
grep -rn "api.endpoint" --include="*.go" internal/
```

### Step 3: Evaluate Compliance

**PASS Criteria**:
- ✅ Exact function/method/library specified in issue is present in code
- ✅ Located in expected files/components
- ✅ Used in the context described by the issue

**FAIL Criteria**:
- ❌ Different function/method/library used instead
- ❌ Specified approach is absent from implementation
- ❌ Alternative approach used (even if functionally similar)

### Step 4: Document Evidence

**For PASS**:
```markdown
- **Status**: ✅ PASS
- **Evidence**: Searched for lipgloss.Place() in internal/tui/tui.go
- **Finding**: lipgloss.Place() called on line 423
- **Location**: internal/tui/tui.go:423
```

**For FAIL**:
```markdown
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() in internal/tui/tui.go
- **Finding**: lipgloss.Place() not found. Implementation uses layerOverlay() instead (line 425)
- **Location**: internal/tui/tui.go:425
- **Reasoning**: Issue explicitly required lipgloss.Place(). Alternative function used.
```

---

## Pass/Fail Decision Matrix

### Decision Rules

| Situation | Decision | Reasoning |
|-----------|----------|-----------|
| All criteria met, all tests pass | ✅ PASS | Complete compliance |
| N-1 criteria met, 1 criterion failed | ❌ FAIL | Any unmet criterion = failure |
| All functional criteria met, implementation approach different | ❌ FAIL | Specification violation |
| Implementation works but tests fail | ❌ FAIL | QA failure = overall failure |
| All criteria met but formatting fails | ❌ FAIL | QA failure = overall failure |
| Issue ambiguous, implementation reasonable | ✅ PASS* | Document assumption in report |
| Alternative approach clearly superior but not specified | ❌ FAIL | Must match spec (note in report) |

**\*Ambiguity Handling**: If criterion is genuinely ambiguous, pass with documentation. But explicit specifications (e.g., "use function X") are never ambiguous.

### Individual Criterion Decisions

#### Feature Criteria
**PASS if**:
- Feature exists in implementation
- Feature is accessible to users as described
- Feature behavior matches description

**FAIL if**:
- Feature is missing
- Feature exists but not accessible as described
- Feature behavior differs from specification

#### Behavior Criteria
**PASS if**:
- Specified behavior occurs under specified conditions
- Edge cases handled as described
- Error handling matches specification

**FAIL if**:
- Behavior does not occur as specified
- Different behavior occurs instead
- Behavior occurs under different conditions

#### Implementation Criteria (CRITICAL)
**PASS if**:
- EXACT function/method/library specified is used
- Used in the context/location described
- Implementation matches specification precisely

**FAIL if**:
- Different function/method/library used
- Specified approach not present in code
- Alternative approach used (regardless of functionality)

**NEVER accept**:
- "Functionally equivalent" alternatives
- "Better" approaches not specified in issue
- "Similar" functions or methods

#### Test Criteria
**PASS if**:
- Tests exist for specified functionality
- Tests cover edge cases if mentioned
- All tests pass

**FAIL if**:
- Tests missing for specified functionality
- Tests exist but fail
- Test coverage below specified threshold

#### Documentation Criteria
**PASS if**:
- Documentation exists in specified location
- Documentation covers specified topics
- Documentation is accurate and complete

**FAIL if**:
- Documentation missing
- Documentation incomplete
- Documentation location incorrect

---

## Evidence Documentation Guidelines

### What Constitutes Sufficient Evidence?

#### For Code Verification
**Minimum Requirements**:
- File path where criterion was checked
- Line numbers if applicable
- Exact code snippet or function name found
- Method used to verify (grep, visual inspection, etc.)

**Good Example**:
```markdown
- **Evidence**: Inspected internal/tui/tui.go for lipgloss.Place() usage
- **Location**: internal/tui/tui.go:423-427
- **Finding**: Found lipgloss.Place() call positioning menu overlay
- **Verification Method**: Visual code inspection + grep search
```

**Insufficient Example**:
```markdown
- **Evidence**: Checked the code
- **Finding**: Looks good
```

#### For Test Execution
**Minimum Requirements**:
- Command executed
- Exit code
- Summary of results (pass/fail counts)
- Any error output if failed

**Good Example**:
```markdown
- **Evidence**: Executed `go test ./internal/tui/...`
- **Result**: PASS (12/12 tests passed)
- **Verification Method**: Test execution
- **Output**: All autocomplete menu tests passed, including edge cases
```

#### For Behavior Verification
**Minimum Requirements**:
- How behavior was tested (manual/automated)
- Conditions under which behavior was verified
- Observed result
- Expected result from issue

**Good Example**:
```markdown
- **Evidence**: Tested Escape key behavior manually in development build
- **Conditions**: Menu open with 3 suggestions displayed
- **Result**: Pressing Escape closed menu and returned to normal mode
- **Expected**: "Menu should close when user presses Escape" (from issue)
- **Verification Method**: Manual testing
```

### Evidence Collection Commands

```bash
# Search for function usage
grep -rn "functionName" --include="*.go" .

# Check imports
grep "import" file.go | grep "package"

# Run tests
go test ./...
task test

# Check formatting
task fmt:check
go fmt -l .

# Run linting
task lint
go vet ./...

# Verify file exists
test -f path/to/file && echo "EXISTS" || echo "MISSING"

# Count occurrences
grep -c "pattern" file.go
```

---

## Common Anti-Patterns

### Anti-Pattern 1: Accepting Functional Equivalence

**❌ WRONG APPROACH**:

**Issue Criterion**:
> "Use lipgloss.Place() for positioning the autocomplete menu"

**Implementation**:
```go
// Uses layerOverlay() instead
return layerOverlay(baseView, menuOverlay, m.width, m.height)
```

**Validator Response** (INCORRECT):
```markdown
### Criterion: Menu positioning
- **Status**: ✅ PASS
- **Finding**: Menu is correctly positioned using overlay function
- **Reasoning**: layerOverlay() achieves the positioning requirement
```

**Why This Is Wrong**: Issue specified the EXACT function to use. Alternative approaches, even if functionally similar, violate the specification.

---

**✅ CORRECT APPROACH**:

**Validator Response**:
```markdown
### Criterion: Use lipgloss.Place() for positioning
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() in internal/tui/tui.go
- **Finding**: lipgloss.Place() not found. Implementation uses layerOverlay() on line 423
- **Location**: internal/tui/tui.go:423
- **Verification Method**: Code inspection + grep search
- **Reasoning**: Issue explicitly specified "use lipgloss.Place()" in multiple locations. Implementation uses different function (layerOverlay()). This is a specification violation even though both functions perform overlay positioning.
```

---

### Anti-Pattern 2: Grouping Criteria

**❌ WRONG APPROACH**:

```markdown
### Positioning Requirements
- **Status**: ✅ PASS
- **Finding**: All positioning requirements met
```

**Why This Is Wrong**: Multiple criteria grouped together make it impossible to identify which specific requirement failed. Each criterion must be evaluated individually.

---

**✅ CORRECT APPROACH**:

```markdown
### Criterion 7: Menu positioned at bottom-left
- **Status**: ✅ PASS
- **Evidence**: Inspected positioning logic in render function
- **Finding**: Menu positioned at bottom-left of viewport
- **Location**: internal/tui/tui.go:418-420

### Criterion 8: Menu positioned above footer
- **Status**: ✅ PASS
- **Evidence**: Checked vertical positioning calculation
- **Finding**: Menu Y position calculated as (height - footerHeight - menuHeight)
- **Location**: internal/tui/tui.go:419

### Criterion 9: Use lipgloss.Place() for positioning
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() usage
- **Finding**: Implementation uses layerOverlay() instead
- **Location**: internal/tui/tui.go:423
```

---

### Anti-Pattern 3: Partial Credit

**❌ WRONG APPROACH**:

```markdown
## Overall Validation Result
- **Status**: ✅ PASS
- **Criteria Passed**: 8 of 10
- **Summary**: Most requirements met, implementation is acceptable
```

**Why This Is Wrong**: Validation is binary. If ANY criterion fails, overall validation fails. There is no "mostly passing."

---

**✅ CORRECT APPROACH**:

```markdown
## Overall Validation Result
- **Status**: ❌ FAIL
- **Criteria Passed**: 8 of 10
- **Failed Criteria**: 7, 9

### Summary
Implementation meets 8 of 10 acceptance criteria but fails overall validation due to 2 unmet requirements.

### Failed Criteria Details
1. **Criterion 7**: Menu does not close on Escape key - no key handler found
2. **Criterion 9**: Implementation uses layerOverlay() instead of specified lipgloss.Place()

### Recommendations
- Add Escape key handler to close menu (see issue requirement #3)
- Refactor positioning logic to use lipgloss.Place() as specified in issue
```

---

### Anti-Pattern 4: Ignoring QA Failures

**❌ WRONG APPROACH**:

```markdown
## Overall Validation Result
- **Status**: ✅ PASS
- **Summary**: All acceptance criteria met

[QA section shows tests failed but overall status is PASS]
```

**Why This Is Wrong**: QA failures (tests, linting, formatting) are part of validation. Even if all functional criteria pass, QA failures mean overall FAIL.

---

**✅ CORRECT APPROACH**:

```markdown
## Overall Validation Result
- **Status**: ❌ FAIL
- **Criteria Passed**: 10 of 10
- **QA Status**: FAIL (tests failing)

### Summary
All 10 acceptance criteria met but validation fails due to QA failures. Tests must pass before implementation can be accepted.

### QA Failures
- 2 of 15 tests failing in autocomplete_test.go
- Tests: TestMenuCloseOnEscape, TestMenuItemSelection

### Recommendations
- Fix failing tests in internal/tui/autocomplete_test.go
- Ensure all edge cases covered by test suite
```

---

## Real-World Example: PR #238 Analysis

This is the EXACT scenario that Issue #242 addresses - an implementation that should have failed validation but passed.

### Issue #235 Specification

**Original Issue Requirements**:
> "Add autocomplete menu for command input"
> "Position menu as overlay above footer using lipgloss.Place()"
> "Use Lipgloss v2 Place() for overlay positioning"
> "Render as overlay using lipgloss.Place() instead of inline in footer"

**Acceptance Criteria Extracted** (what validator SHOULD have done):

```markdown
### Criterion 1: Add autocomplete menu functionality
- **Type**: Feature
- **Specified Approach**: None

### Criterion 2: Position menu above footer
- **Type**: Behavior
- **Specified Approach**: None

### Criterion 3: Render as overlay (not inline)
- **Type**: Behavior
- **Specified Approach**: None

### Criterion 4: Use lipgloss.Place() for overlay positioning
- **Type**: Implementation
- **Specified Approach**: lipgloss.Place() function (explicitly required)
```

### PR #238 Implementation

**What Was Actually Implemented**:
```go
// internal/tui/tui.go
func (m Model) View() string {
    baseView := m.renderBase()
    
    if m.showAutocomplete {
        menuOverlay := m.renderAutocompleteMenu()
        // Used layerOverlay() instead of lipgloss.Place()
        return layerOverlay(baseView, menuOverlay, m.width, m.height)
    }
    
    return baseView
}

func layerOverlay(base, overlay string, width, height int) string {
    // Centers overlay on screen - different behavior than specified
    // ...
}
```

### What Validator Did (INCORRECT)

**Actual Validator Response**:
```markdown
## Validation Result: ✅ PASS

All acceptance criteria met:
- Autocomplete menu added ✅
- Menu positioned above footer ✅
- Menu rendered as overlay ✅
- Positioning logic implemented ✅

Implementation successfully adds autocomplete functionality.
```

**Why This Was Wrong**: Validator accepted "positioning logic implemented" without verifying the EXACT approach specified (lipgloss.Place()). This is the holistic "does it work?" approach that Issue #242 aims to fix.

### What Validator SHOULD Have Done (CORRECT)

**Correct Validator Response**:

```markdown
# Validation Report: Issue #235

## Phase 1: Acceptance Criteria Extraction

### Criterion 1: Add autocomplete menu functionality
- **Type**: Feature
- **Specified Approach**: None
- **Source**: "Add autocomplete menu for command input"

### Criterion 2: Position menu above footer
- **Type**: Behavior
- **Specified Approach**: None
- **Source**: "Position menu as overlay above footer"

### Criterion 3: Render as overlay (not inline in footer)
- **Type**: Behavior
- **Specified Approach**: None
- **Source**: "render as overlay using lipgloss.Place() instead of inline in footer"

### Criterion 4: Use lipgloss.Place() for overlay positioning
- **Type**: Implementation
- **Specified Approach**: lipgloss.Place() function
- **Source**: "using lipgloss.Place()" (mentioned 3x in issue)

**Total Criteria Extracted**: 4

---

## Phase 2: Individual Criterion Verification

### Criterion 1: Add autocomplete menu functionality
- **Status**: ✅ PASS
- **Evidence**: Inspected internal/tui/autocomplete.go
- **Location**: internal/tui/autocomplete.go:15-89
- **Finding**: Complete autocomplete implementation with command matching
- **Verification Method**: Code inspection

### Criterion 2: Position menu above footer
- **Status**: ✅ PASS
- **Evidence**: Checked positioning calculation in View() method
- **Location**: internal/tui/tui.go:156
- **Finding**: Menu Y position calculated to place above footer
- **Verification Method**: Code inspection

### Criterion 3: Render as overlay (not inline in footer)
- **Status**: ✅ PASS
- **Evidence**: Inspected rendering logic in View() method
- **Location**: internal/tui/tui.go:154-158
- **Finding**: Menu rendered as separate overlay, not embedded in footer string
- **Verification Method**: Code inspection

### Criterion 4: Use lipgloss.Place() for overlay positioning
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() usage in tui package
  ```bash
  grep -rn "lipgloss.Place" internal/tui/
  # No results found
  ```
- **Location**: internal/tui/tui.go:157
- **Finding**: Implementation uses layerOverlay() function instead of lipgloss.Place()
  ```go
  return layerOverlay(baseView, menuOverlay, m.width, m.height)
  ```
- **Verification Method**: Code inspection + grep search

**Reasoning**: Issue explicitly specified "use lipgloss.Place()" for overlay positioning in 3 separate locations. This is an implementation requirement, not a suggestion. Implementation uses different function (layerOverlay()) instead. While layerOverlay() also performs overlay positioning, using an alternative approach when specific function is required constitutes a specification violation.

**Additional Context**: layerOverlay() centers overlays on screen, while lipgloss.Place() allows precise positioning control. These have different behaviors, so this is both a specification violation AND a potential functional issue.

---

## Overall Validation Result

- **Status**: ❌ FAIL
- **Criteria Passed**: 3 of 4
- **Failed Criteria**: Criterion 4 (Use lipgloss.Place() for positioning)

### Summary
Implementation meets 3 of 4 acceptance criteria but fails overall validation. Criterion 4 (implementation requirement to use lipgloss.Place()) not satisfied.

### Failed Criteria Details
1. **Criterion 4**: Implementation uses layerOverlay() instead of specified lipgloss.Place(). Issue explicitly required lipgloss.Place() function in multiple locations, making this an implementation specification that must be followed exactly.

### Recommendations
Refactor overlay positioning to use lipgloss.Place() as specified:
1. Remove layerOverlay() function call on line 157
2. Import lipgloss v2 if not already present
3. Replace with lipgloss.Place() call with appropriate positioning parameters
4. Verify positioning behavior matches requirements (above footer, bottom-left)

**Reference**: See issue #235 for multiple mentions of required lipgloss.Place() usage
```

### Impact Analysis

**With Incorrect Validation** (what actually happened):
- PR #238 merged with specification violation
- Implementation used different function than specified
- Potential behavior differences (centering vs precise positioning)
- Manual code review needed to catch the issue
- Rework required after "completion"

**With Correct Validation** (what should have happened):
- Validation fails with clear explanation
- Builder receives specific feedback: "Replace layerOverlay() with lipgloss.Place()"
- Builder fixes the issue before PR creation
- Implementation matches specification exactly
- No manual intervention needed

---

## Quick Reference Checklist

Use this checklist for every validation:

### Before Starting
- [ ] Retrieved issue via `gh issue view [number] --json body --repo [repo]`
- [ ] Read entire issue completely
- [ ] Identified repository context and working directory

### Phase 1: Extraction
- [ ] Extracted ALL acceptance criteria into numbered list
- [ ] Identified type for each criterion (Feature/Behavior/Implementation/Test/Doc)
- [ ] Noted specified approaches for implementation criteria
- [ ] Quoted source from issue for each criterion
- [ ] Completed extraction BEFORE beginning verification

### Phase 2: Verification
- [ ] Verified EACH criterion individually (not grouped)
- [ ] Documented evidence for each criterion
- [ ] Provided file locations and line numbers
- [ ] Searched for exact functions/methods when specified in issue
- [ ] Ran relevant tests and QA tools
- [ ] Marked each criterion explicitly as PASS or FAIL

### Specification Compliance
- [ ] For each "use X" requirement, verified X is actually used (not alternative)
- [ ] Checked that alternatives were NOT accepted as equivalent
- [ ] Applied strict compliance for implementation criteria

### Reporting
- [ ] Used structured template format
- [ ] Overall status is FAIL if ANY criterion failed
- [ ] Overall status is FAIL if ANY QA check failed
- [ ] Listed all failed criteria with specific details
- [ ] Provided actionable recommendations for failures

### Final Check
- [ ] Every criterion has explicit ✅ PASS or ❌ FAIL
- [ ] No grouped criteria ("all requirements met")
- [ ] No partial credit given
- [ ] Evidence documented for all verifications
- [ ] Report uses structured template format

---

## Validation Workflow Integration

### Input from Krew-Lead

Expected information when validator is invoked:

```markdown
Validate implementation for issue #[number]

**Issue Number**: [number]
**Repository**: [owner/repo]
**Working Directory**: [already in worktree - no cd needed]

Perform strict criterion-by-criterion verification following validator-conventions skill.
```

### Fetch Issue Details

```bash
# Retrieve issue body and acceptance criteria
gh issue view [number] --json body --repo [owner/repo]

# Example
gh issue view 235 --json body --repo jbrinkman/kairon
```

### Create Sentinel File

Write your validation report to the sentinel file whose path is defined by the
**sentinel-protocol** skill (the single source of truth for sentinel naming) —
do NOT hard-code a path here. In brief:

- **Plan task** (you were given a task `id`): use the task-scoped form
  `.kairon/artifacts/<agent>-<issue-number>-<task-id>.md`
  (e.g. `.kairon/artifacts/validator-235-verify-api.md`). The lead waits on
  exactly this path and will NOT accept the task-less fallback for a plan task.
- **Legacy / no-plan run** (no task `id`): use the task-less form
  `.kairon/artifacts/validator-<issue-number>.md`
  (e.g. `.kairon/artifacts/validator-235.md`).

See `skill://.kiro/skills/sentinel-protocol/SKILL.md` for the authoritative
rules, including the write/read contract.

### Report to Krew-Lead

**On PASS**:
- Write sentinel file with ✅ PASS status
- Exit with code 0
- Krew-lead proceeds to PR creation

**On FAIL**:
- Write sentinel file with ❌ FAIL status and specific failures
- Exit with non-zero code
- Krew-lead delegates back to builder with feedback

---

## Summary

This skill provides everything needed for strict criterion-by-criterion validation:

1. **Structured Template**: Copy-paste format for consistent validation reports
2. **Extraction Patterns**: How to identify and extract ALL acceptance criteria
3. **Compliance Checklist**: Step-by-step verification for specification requirements
4. **Decision Matrix**: Clear rules for pass/fail decisions
5. **Evidence Guidelines**: What constitutes sufficient verification evidence
6. **Anti-Patterns**: Examples of what NOT to do (with PR #238 as primary example)
7. **Real-World Example**: Complete walkthrough of PR #238 validation (wrong vs right)

**Core Principle**: When issue specifies HOW to implement (e.g., "use function X"), that becomes a mandatory implementation requirement. Alternative approaches, even if functionally equivalent, constitute specification violations and must fail validation.

**Remember**: Validation is binary. ALL criteria must pass for overall PASS. Any failed criterion = overall FAIL.
