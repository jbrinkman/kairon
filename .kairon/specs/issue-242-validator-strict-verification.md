# Design Specification: Validator Strict Criterion-by-Criterion Verification

**Issue**: #242  
**Title**: Validator must enforce strict criterion-by-criterion verification of ALL acceptance criteria  
**Closes**: #242

---

## Problem Analysis

### Current Behavior

The validator agent currently performs holistic validation using a "does this generally solve the problem?" approach rather than strict "does this satisfy every single specified requirement?" verification. This has resulted in implementations passing validation despite significant deviations from issue specifications.

### Concrete Evidence: PR #238 / Issue #235

**Issue #235 Specification:**
- Explicitly required: "Use `lipgloss.Place()` for positioning the autocomplete menu"
- Multiple references throughout issue emphasizing `lipgloss.Place()` usage
- Specific positioning requirement: bottom-left above footer

**PR #238 Implementation:**
- Used `layerOverlay()` function instead of `lipgloss.Place()`
- `layerOverlay()` centers overlays on screen (different behavior than specified)
- Result: Menu would appear centered, not at bottom-left as specified
- **Validator passed this implementation**

### Root Cause

The validator validates the spirit of the solution without verifying each individual criterion. When issues specify exact implementation approaches (e.g., "use function X"), the validator accepts functionally similar alternatives without recognizing this as a specification violation.

### Impact

- Features ship that don't work as designed
- Architect designs and issue specifications are effectively ignored
- Manual code review becomes necessary
- Automated workflow reliability is compromised
- Rework required after issues marked "complete"

---

## Solution Approach

Transform the validator from a holistic reviewer to a strict criterion-by-criterion verifier that:

1. **Extracts** ALL acceptance criteria from issues into explicit checklists
2. **Verifies** EACH criterion individually and independently
3. **Enforces** exact implementation approaches when specified
4. **Reports** structured pass/fail status for every criterion
5. **Fails** validation if ANY criterion is not met

### Key Architectural Decisions

**1. Criterion-First Validation**
- Validation begins with criterion extraction, not code inspection
- Every criterion must be explicitly listed before verification begins
- Checklist serves as validation contract

**2. Specification Compliance Over Functional Equivalence**
- When issue says "use X", implementation must use X
- Alternative approaches (even if functionally similar) constitute failures
- Specified approaches are requirements, not suggestions

**3. Structured Reporting**
- Every criterion gets explicit pass/fail status
- Evidence and file locations documented for each
- Overall result is binary: PASS only if all criteria pass

**4. Two-Phase Process**
- Phase 1: Extract and list all criteria (before any inspection)
- Phase 2: Verify each criterion individually (with evidence)

---

## Design Components

### Component 1: Enhanced Validator Prompt

**File**: `.kiro/agents/validator-prompt.md`

**Purpose**: Transform validator from holistic to strict criterion-based verification

**Key Additions**:

1. **Mission Statement**: Emphasize strict verification role
2. **Two-Phase Process**: Criterion extraction → Individual verification
3. **Anti-Pattern Reference**: Include PR #238 example of what NOT to accept
4. **Specification Compliance Rule**: Exact approaches must be verified when specified
5. **Structured Output Template**: Enforce consistent reporting format

**Critical Sections to Add**:

```markdown
## Your Role: Strict Criterion-by-Criterion Verifier

You verify that EVERY SINGLE acceptance criterion from the issue is satisfied. You must fail 
validation if ANY criterion is not met.

CRITICAL: If the issue specifies an implementation approach (e.g., "use function X", "call 
library Y"), you MUST verify that exact approach was used. Alternative approaches, even if 
functionally similar, constitute a failed criterion.

## Two-Phase Validation Process

### Phase 1: Criterion Extraction
1. Read the linked issue completely
2. Extract EVERY acceptance criterion into an explicit numbered list
3. Include implementation approach requirements (e.g., "use X") as criteria
4. Output the complete checklist BEFORE beginning verification

### Phase 2: Individual Verification
For EACH criterion:
1. Inspect relevant files and code
2. Run verification commands if applicable
3. Document evidence (what was checked, what was found)
4. Mark as ✅ PASS or ❌ FAIL with specific reasoning

## Critical Rule: Specification Compliance

When an issue specifies HOW to implement something:
- "Use lipgloss.Place()" → implementation MUST use lipgloss.Place()
- "Call function X" → implementation MUST call function X
- "Use library Y" → implementation MUST use library Y

Functional equivalence is NOT acceptable. Alternative approaches are specification violations.

### Anti-Pattern Example: PR #238

Issue #235 specified: "Use lipgloss.Place() for positioning"
Implementation used: layerOverlay() function

**Correct Validator Response**: ❌ FAIL
- Criterion: "Use lipgloss.Place() for positioning"
- Status: FAILED
- Reason: Implementation uses layerOverlay() instead of specified lipgloss.Place()
- Evidence: Checked internal/tui/tui.go line X, found layerOverlay() call

This is a specification violation even though both are overlay functions.
```

### Component 2: Validator Conventions Skill

**File**: `.kiro/skills/validator-conventions/SKILL.md`

**Purpose**: Provide structured templates, examples, and patterns for strict validation

**Contents**:

1. **Structured Output Template**: Copy-paste template for validation reports
2. **Criterion Extraction Patterns**: How to identify all criteria types
3. **Specification Compliance Checklist**: How to detect when exact approaches are required
4. **Evidence Documentation Guidelines**: What constitutes sufficient evidence
5. **Pass/Fail Decision Matrix**: When to pass vs fail individual criteria
6. **Common Pitfalls**: Anti-patterns to avoid (like the PR #238 example)

**Key Sections**:

```markdown
## Structured Validation Report Template

Use this exact format for all validations:

## Acceptance Criteria Extraction

**Source**: Issue #[number]

### Criterion 1: [Description]
- Type: [Feature/Behavior/Implementation/Test/Documentation]
- Specified Approach: [If any, e.g., "use lipgloss.Place()"]

### Criterion 2: [Description]
...

[Complete list of ALL criteria before verification begins]

---

## Individual Criterion Verification

### Criterion 1: [Description]
- **Status**: ✅ PASS / ❌ FAIL
- **Evidence**: [What was checked]
- **Location**: [File paths and line numbers]
- **Verification Method**: [Code inspection/test run/command execution]

### Criterion 2: [Description]
...

---

## Overall Validation Result

- **Status**: ✅ PASS / ❌ FAIL
- **Criteria Passed**: X of Y
- **Failed Criteria**: [List criterion numbers if any]

**Summary**: [1-2 sentence verdict]

---

## Specification Compliance Checklist

Use this checklist for every criterion that mentions a specific implementation approach:

- [ ] Does the issue specify HOW to implement (function name, library, method)?
- [ ] If yes, did I verify the EXACT approach was used (not just similar)?
- [ ] Did I check actual function calls/imports, not just general behavior?
- [ ] Would a text search for the specified function/library succeed in the code?

**Red Flags** (these are specification violations):
- Issue says "use X", implementation uses Y (even if Y is similar)
- Issue says "call function F", implementation achieves same result differently
- Issue says "import library L", implementation uses alternative library

## Common Anti-Patterns

### Anti-Pattern 1: Accepting Functional Equivalence

❌ **Wrong Approach**:
- Criterion: "Use lipgloss.Place() for positioning"
- Implementation: Uses layerOverlay() which also positions elements
- Validator reasoning: "Both achieve positioning, so it's fine"
- Result: PASS

✅ **Correct Approach**:
- Criterion: "Use lipgloss.Place() for positioning"
- Implementation: Uses layerOverlay()
- Validator reasoning: "Issue specified lipgloss.Place(), implementation uses different function"
- Result: FAIL - Specification violation

### Anti-Pattern 2: Grouping Criteria

❌ **Wrong Approach**:
- "All positioning requirements met ✅"

✅ **Correct Approach**:
- Criterion 1: Position at bottom-left ✅
- Criterion 2: Position above footer ✅
- Criterion 3: Use lipgloss.Place() ❌

### Anti-Pattern 3: Partial Credit

❌ **Wrong Approach**:
- "8 of 10 criteria met, close enough ✅ PASS"

✅ **Correct Approach**:
- "8 of 10 criteria met ❌ FAIL - Criteria 7 and 9 not satisfied"
```

### Component 3: Updated Validation Workflow

**Integration Points**:

1. **Krew-Lead Delegation**: Pass issue number to validator so it can fetch issue details
2. **Validation Input**: Validator receives issue number and working directory
3. **Validation Output**: Structured report written to sentinel file
4. **Builder Feedback**: Failed criteria list passed to builder for corrections

**Validator Invocation Pattern** (from krew-lead):

```markdown
Validate the implementation for issue #[number].

Issue Number: [number]
Repository: [repo]
Working Directory: [already in worktree]

Perform strict criterion-by-criterion verification:
1. Fetch issue details: gh issue view [number] --json body --repo [repo]
2. Extract ALL acceptance criteria
3. Verify EACH criterion individually
4. Report structured results

Remember: If issue specifies implementation approach, exact approach must be used.
```

---

## Relevant Files

### Files to Modify

1. **`.kiro/agents/validator-prompt.md`**
   - Add strict verification mission statement
   - Add two-phase process (extraction → verification)
   - Add specification compliance rules
   - Add PR #238 anti-pattern example
   - Add structured output template
   - ~100-150 lines of additions/modifications

2. **`.kiro/agents/validator.json`**
   - Verify `resources` array includes validator-conventions skill
   - Ensure shell tool has readonly access for gh commands
   - No structural changes expected, just verification

### Files to Create

3. **`.kiro/skills/validator-conventions/SKILL.md`**
   - Complete skill file with templates and patterns
   - ~300-400 lines
   - Includes: templates, examples, checklists, anti-patterns

---

## Team Orchestration

This is a single-PR implementation with sequential tasks:

### Task 1: Create Validator Conventions Skill
**Owner**: Builder  
**Dependencies**: None  
**Deliverables**:
- `.kiro/skills/validator-conventions/SKILL.md` created
- Contains all templates, patterns, and examples
- Follows skill specification format

**Acceptance Criteria**:
- Structured validation report template included
- Criterion extraction patterns documented
- Specification compliance checklist included
- PR #238 anti-pattern documented with explanation
- Evidence documentation guidelines included
- Pass/fail decision matrix included

### Task 2: Update Validator Prompt
**Owner**: Builder  
**Dependencies**: Task 1 (reference validator-conventions in modifications)  
**Deliverables**:
- `.kiro/agents/validator-prompt.md` updated
- Strict verification approach integrated
- Two-phase process documented
- Anti-pattern reference added

**Acceptance Criteria**:
- Mission statement emphasizes strict criterion-by-criterion verification
- Two-phase process (extraction → verification) clearly documented
- Specification compliance rule stated explicitly
- PR #238 anti-pattern example included
- Structured output format required
- Original validation workflow preserved (QA tools, shell access, report format)

### Task 3: Verify Configuration and Template Sync
**Owner**: Builder  
**Dependencies**: Task 2  
**Deliverables**:
- `.kiro/agents/validator.json` verified
- Template sync completed for modified agent files

**Acceptance Criteria**:
- `validator.json` references validator-conventions skill in `resources` array
- Shell tool configuration allows gh commands (readonly)
- Template sync executed: `cp .kiro/agents/validator* cmd/kiro-krew/templates/kiro/agents/`
- Template sync verification passes: `task sync:check`

### Task 4: Create Test Scenario Documentation
**Owner**: Builder  
**Dependencies**: Task 3  
**Deliverables**:
- Documentation of how to test new validator behavior

**Acceptance Criteria**:
- PR #238 scenario documented as test case
- Expected validation output example provided
- Test verification steps documented

---

## Step-by-Step Task Breakdown

### Task 1: Create Validator Conventions Skill

**Implementation Steps**:

1. Create skill directory structure:
   ```bash
   mkdir -p .kiro/skills/validator-conventions
   ```

2. Create `.kiro/skills/validator-conventions/SKILL.md` with:
   - YAML frontmatter (name, description)
   - Structured validation report template (complete copy-paste format)
   - Criterion extraction patterns section
   - Specification compliance checklist
   - Evidence documentation guidelines
   - Pass/fail decision matrix
   - Common anti-patterns section with PR #238 example
   - Example validation reports (passing and failing)

3. Include specific content sections:
   - **Template Section**: Full copy-paste template for validation reports
   - **Extraction Patterns**: How to identify different criterion types (feature, behavior, implementation, test)
   - **Compliance Checklist**: Step-by-step checks for specification violations
   - **Anti-Patterns**: PR #238 example with wrong vs correct validator responses
   - **Evidence Standards**: What constitutes sufficient verification evidence

**Validation**:
- Skill file follows format from existing skills (builder-conventions, planner-conventions)
- All required sections present and complete
- PR #238 example clearly demonstrates wrong vs right approach

### Task 2: Update Validator Prompt

**Implementation Steps**:

1. Read current `.kiro/agents/validator-prompt.md` completely

2. Add new "Your Role" section at top (after Purpose):
   - Emphasize strict criterion-by-criterion verification
   - State the critical rule about specified implementation approaches
   - Explain that alternatives are not acceptable

3. Add "Two-Phase Validation Process" section:
   - Phase 1: Criterion Extraction (before any code inspection)
   - Phase 2: Individual Verification (with evidence)
   - Clear separation between extraction and verification

4. Add "Critical Rule: Specification Compliance" section:
   - When issue specifies HOW, exact approach must be verified
   - Examples of specification language ("use X", "call Y")
   - Functional equivalence is not acceptable

5. Add "Anti-Pattern Example: PR #238" section:
   - Show the issue specification
   - Show what implementation did
   - Show correct validator response (FAIL with reasoning)
   - Explain why it's a violation

6. Update "Report Format" section:
   - Add "Acceptance Criteria Extraction" section at top
   - Modify "Checks Performed" to be "Individual Criterion Verification"
   - Require pass/fail status for EACH criterion
   - Add "Overall Result" with criteria count

7. Preserve existing sections:
   - Keep "Purpose" section
   - Keep "Instructions" section
   - Keep "Shell Access Note"
   - Keep "Write Access Note"
   - Keep "Quality Verification" section
   - Keep "Workflow" section (update step 1 to include criterion extraction)

**Validation**:
- All new content integrated without removing existing functionality
- Two-phase process clearly documented
- PR #238 anti-pattern prominently featured
- Report format template includes criterion checklist
- Original QA verification workflow preserved

### Task 3: Verify Configuration and Template Sync

**Implementation Steps**:

1. Verify `.kiro/agents/validator.json`:
   ```bash
   cat .kiro/agents/validator.json
   ```
   - Check that `resources` array includes: `"skill://.kiro/skills/validator-conventions/SKILL.md"`
   - If not present, add it to resources array
   - Verify shell tool configuration allows readonly commands

2. Sync modified agent files to templates:
   ```bash
   cp .kiro/agents/validator-prompt.md cmd/kiro-krew/templates/kiro/agents/
   cp .kiro/agents/validator.json cmd/kiro-krew/templates/kiro/agents/
   ```

3. Verify sync succeeded:
   ```bash
   task sync:check
   ```
   - Must pass with no differences reported
   - If differences found, re-run sync commands

**Validation**:
- validator.json resources array includes validator-conventions skill
- Template files match live files exactly
- `task sync:check` passes with zero differences

### Task 4: Create Test Scenario Documentation

**Implementation Steps**:

1. Create test scenario file at `.kiro-krew/specs/validator-test-scenario-242.md`:

2. Document PR #238 test scenario:
   - Issue #235 acceptance criteria (focus on lipgloss.Place() requirement)
   - PR #238 implementation (used layerOverlay())
   - Expected validator output (FAIL with specific criterion marked)
   - Show complete structured report example

3. Include verification steps:
   - How to run validator against PR #238 scenario
   - What output to expect
   - How to verify strict verification is working

4. Document passing scenario:
   - Example of implementation that meets all criteria
   - Expected validator output (PASS with all criteria marked)

**Validation**:
- Test scenario clearly shows before/after validator behavior
- PR #238 example demonstrates exact issue described in #242
- Verification steps are actionable and complete

---

## Validation Commands

### Template Sync Verification
```bash
task sync:check
```
**Expected**: All template files match live files, zero differences

### Validator Configuration Check
```bash
cat .kiro/agents/validator.json | grep -A 5 resources
```
**Expected**: Resources array includes validator-conventions skill reference

### Skill File Validation
```bash
test -f .kiro/skills/validator-conventions/SKILL.md && echo "Skill file exists"
grep -q "Structured Validation Report Template" .kiro/skills/validator-conventions/SKILL.md && echo "Template section present"
```
**Expected**: Both commands output success messages

### QA Verification
```bash
task lint
task test
```
**Expected**: All linting and tests pass

---

## Success Metrics

### Primary Metrics

1. **Deviation Detection Rate**: 100%
   - Validator catches implementation deviations like PR #238
   - When issue specifies "use X", validator fails if implementation uses Y

2. **Criterion Coverage**: 100%
   - Every acceptance criterion explicitly listed in validation output
   - No criteria skipped or grouped

3. **Specification Violation Detection**: 100%
   - Alternative approaches detected when specific approach required
   - Functional equivalence rejected when exact approach specified

### Secondary Metrics

4. **False Positive Rate**: <5%
   - Valid implementations that meet all criteria still pass
   - Focus on specified requirements, not unspecified preferences

5. **Report Clarity**: 100%
   - Structured format consistently used
   - Evidence provided for every criterion
   - Failed criteria clearly identified

### Test Cases

1. **PR #238 Scenario** (primary validation):
   - Input: Issue #235 spec + PR #238 implementation
   - Expected: FAIL with "Use lipgloss.Place()" criterion marked failed
   - Evidence: Shows layerOverlay() used instead

2. **Compliant Implementation**:
   - Input: Issue with all criteria met including specified approaches
   - Expected: PASS with all criteria marked passed
   - Evidence: Each criterion verification documented

3. **Partial Implementation**:
   - Input: Implementation meeting 8 of 10 criteria
   - Expected: FAIL with 2 unmet criteria explicitly listed
   - No partial credit given

4. **Alternative Approach**:
   - Input: Issue says "use function X", implementation uses function Y
   - Expected: FAIL with specification violation noted
   - Even if Y achieves same result

---

## Context and References

### Related Work
- **Issue #235**: Original autocomplete issue specifying lipgloss.Place()
- **PR #238**: Implementation that passed validation despite deviation
- **Issue #229**: Footer height requirements preserved in #235

### Current Validator Behavior
- Holistic "does it work?" validation
- Accepts functionally equivalent alternatives
- Does not track individual criteria
- No structured criterion-by-criterion output

### Target Validator Behavior
- Strict criterion-by-criterion verification
- Rejects alternatives when specific approach required
- Tracks and reports every criterion individually
- Structured output with evidence for each criterion

### Workflow Integration
- Validator called by krew-lead after builder completes tasks
- Validator receives issue number and working directory
- Validator creates sentinel file with structured report
- Builder receives feedback from validator on failures
- Krew-lead only proceeds to PR when validation passes

### Quality Gates
- All acceptance criteria must pass
- Specification compliance enforced
- QA tools must pass (existing requirement)
- Template sync must verify (existing requirement)

---

## Implementation Constraints

1. **Preserve Existing Functionality**:
   - QA tool verification must remain
   - Read-only nature must be preserved
   - Sentinel file workflow must remain
   - Shell command access must remain

2. **No Breaking Changes**:
   - Validator must still work with krew-lead workflow
   - Sentinel file format can be enhanced but not broken
   - Exit codes must remain consistent (non-zero on failure)

3. **Template Synchronization**:
   - All agent file changes must sync to templates
   - Validator-conventions skill must NOT sync (project-specific)
   - Sync verification must pass before completion

4. **Backward Compatibility**:
   - Existing issues being validated must still work
   - Enhanced strict verification applied to all validations
   - No special flags or modes required

---

## Risk Mitigation

### Risk: False Positives (Failing Valid Implementations)

**Mitigation**:
- Criterion interpretation should be literal but reasonable
- Focus on explicitly specified requirements only
- Don't enforce unspecified style preferences
- Provide clear reasoning in failure reports for review

### Risk: Incomplete Criterion Extraction

**Mitigation**:
- Two-phase process ensures extraction happens first
- Validator must output complete checklist before verification
- Krew-lead can review extraction completeness
- Examples in conventions skill demonstrate extraction patterns

### Risk: Template Sync Failures

**Mitigation**:
- Task 3 explicitly verifies sync
- Sync check must pass before completion
- Builder conventions include sync commands
- CI will catch any missed sync

### Risk: Breaking Existing Workflows

**Mitigation**:
- Preserve all existing validator sections and functionality
- Enhance rather than replace current behavior
- Test with existing issues to verify compatibility
- QA verification workflow unchanged

---

## Appendix: PR #238 Detailed Analysis

### Issue #235 Specification Excerpts

> "Position menu as overlay above footer using lipgloss.Place()"

> "Use Lipgloss v2 `Place()` for overlay positioning"

> "render as overlay using `lipgloss.Place()` instead of inline in footer"

### PR #238 Implementation

From `internal/tui/tui.go`:
```go
// Used layerOverlay() function
return layerOverlay(baseView, menuOverlay, m.width, m.height)
```

### Expected Validator Response (With New Behavior)

```markdown
## Acceptance Criteria Extraction

**Source**: Issue #235

### Criterion 7: Position menu using lipgloss.Place()
- Type: Implementation Requirement
- Specified Approach: Must use lipgloss.Place() function

[... other criteria ...]

---

## Individual Criterion Verification

### Criterion 7: Position menu using lipgloss.Place()
- **Status**: ❌ FAIL
- **Evidence**: Searched for lipgloss.Place() usage in internal/tui/tui.go
- **Finding**: Implementation uses layerOverlay() function instead (line 423)
- **Location**: internal/tui/tui.go:423
- **Verification Method**: Code inspection + text search

**Failure Reason**: Issue explicitly specified "use lipgloss.Place()" in multiple locations. 
Implementation uses alternative function layerOverlay(). This is a specification violation 
even if layerOverlay() achieves similar positioning, because the issue specified the exact 
approach to use.

---

## Overall Validation Result

- **Status**: ❌ FAIL
- **Criteria Passed**: 9 of 10
- **Failed Criteria**: Criterion 7 (Position menu using lipgloss.Place())

**Summary**: Implementation satisfies most requirements but deviates from specified 
implementation approach. Issue required lipgloss.Place(), implementation uses layerOverlay().
```

---

**End of Specification**

Closes #242
