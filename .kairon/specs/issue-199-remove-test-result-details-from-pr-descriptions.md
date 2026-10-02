# Design Specification: Remove Test Result Details from PR Descriptions

**Issue**: #199  
**Status**: Architecture Complete  
**Date**: 2026-07-08  

Closes #199

## Problem Analysis

### Current State
The krew-lead agent includes detailed test results in GitHub PR descriptions via step 8, which requires the PR body to include "How it was tested or validated". This creates bloated PR descriptions that make code review difficult, as seen in recent PRs like #191, #190, and others.

### Root Cause
The PR description template in `.kiro/agents/krew-lead-prompt.md` at step 8 explicitly requires:
- "How it was tested or validated" as one of the body bullet points

This causes krew-lead to include verbose test output (hundreds of lines) that duplicates what CI already provides.

### Quality Gates Analysis
The system maintains robust quality verification through multiple layers:
1. **Validator Agent**: Read-only verification that blocks PR creation if tests fail
2. **CI Pipeline**: Automated checks (format, lint, test, build) on every PR
3. **QA Tool Discovery**: Projects have standardized QA commands (`task fmt:check`, `task lint`, `task test`)

## Solution Approach

### Strategy
**Surgical Template Modification**: Remove only the test results requirement while preserving all quality gates and maintaining existing PR description structure.

### Design Principles
1. **Minimal Change**: Only modify the PR body template, no functional changes
2. **Quality Preservation**: All existing validation and CI checks remain identical
3. **Structure Maintenance**: PR descriptions retain their professional format

## Relevant Files

### Primary Target
- **`.kiro/agents/krew-lead-prompt.md`** - Contains the PR body template at step 8

### Validation Files (Reference Only)
- **`.kiro/agents/validator-prompt.md`** - Confirms validator blocks PRs when tests fail
- **`.github/workflows/ci.yml`** - Shows CI provides test status independently
- **`Taskfile.yml`** - Documents project QA commands

## Team Orchestration

### Single Agent Approach
This is a simple template modification that requires only:
- **Builder Agent**: Make the precise text change to the PR template

### No Multi-Agent Coordination Required
- Architect produces this spec
- Builder implements the change
- Validator verifies the modification (but doesn't need to understand the full workflow)

## Step-by-Step Task Breakdown

### Task 1: Modify PR Body Template
**Owner**: Builder Agent  
**File**: `.kiro/agents/krew-lead-prompt.md`  
**Action**: Update step 8 PR body template

**Current Template (lines ~36-40)**:
```markdown
8. **Create PR**: Create a well-formed PR with a detailed description. Use `gh pr create --repo <repo> --head spec/<worktree-name> --title "<issue-title>" --body "<body>"` where the body includes:
   - A summary of what was changed and why
   - List of key files modified/created
   - How it was tested or validated
   - `Closes #<number>` at the end
```

**Required Change**:
```markdown
8. **Create PR**: Create a well-formed PR with a detailed description. Use `gh pr create --repo <repo> --head spec/<worktree-name> --title "<issue-title>" --body "<body>"` where the body includes:
   - A summary of what was changed and why
   - List of key files modified/created
   - `Closes #<number>` at the end
```

**Acceptance Criteria**:
- [ ] Remove the bullet point "How it was tested or validated" from step 8
- [ ] Preserve all other bullet points in exact order
- [ ] Maintain identical formatting and structure
- [ ] No other changes to the file

### Task 2: Verify No Functional Changes
**Owner**: Validator Agent  
**Action**: Confirm the change is purely textual

**Acceptance Criteria**:
- [ ] Only one line removed from krew-lead-prompt.md
- [ ] No changes to validator agent behavior
- [ ] No changes to CI configuration
- [ ] No changes to quality verification process
- [ ] No other agent configurations modified

## Validation Commands

### File Verification
```bash
# Verify the specific change
git diff HEAD -- .kiro/agents/krew-lead-prompt.md

# Confirm only one file changed
git diff --name-only HEAD

# Verify line count change
wc -l .kiro/agents/krew-lead-prompt.md
```

### Quality Gates Verification
```bash
# Standard QA checks (same as CI)
task fmt:check
task lint
task test
task build
```

### Functional Verification
```bash
# Confirm validator agent still has quality verification
grep -n "Quality Verification" .kiro/agents/validator-prompt.md

# Confirm CI still runs tests
grep -n "Test" .github/workflows/ci.yml
```

## Impact Assessment

### What Changes
- **PR Descriptions**: Will no longer include verbose test output details
- **Review Experience**: Cleaner, more focused PR descriptions for reviewers

### What Stays The Same
- **Quality Gates**: Validator still blocks PRs when tests fail
- **CI Validation**: GitHub Actions still runs all quality checks
- **Test Execution**: All tests still run during validation phase
- **PR Creation**: Only happens after validator confirms tests pass
- **Error Handling**: Failed tests still prevent PR creation
- **Other PR Content**: Summary, files modified, and issue reference preserved

### Risk Assessment
- **Risk Level**: Very Low
- **Rationale**: This is a cosmetic change to PR templates with no functional impact
- **Mitigation**: All quality gates remain active and enforced

## Success Criteria

### Primary Goals
1. **Clean PR Descriptions**: New PRs don't include test result details
2. **Quality Preservation**: All existing quality gates remain functional
3. **Minimal Change**: Only the PR template is modified

### Verification Approach
1. **Template Verification**: Confirm bullet point removal
2. **Quality Check**: Run full CI suite to ensure no regressions  
3. **Behavioral Testing**: Validator agent still blocks on test failures
4. **Integration Testing**: Complete krew-lead workflow still functions correctly

## Architecture Notes

### Current Workflow (Unchanged)
```
Issue → Architect → Builder → Validator (QA Verification) → Krew-Lead (PR Creation)
                                    ↓
                                Tests Pass? → Yes: Create PR
                                           → No: Block & Report
```

### PR Description Structure (After Change)
```markdown
## Summary
[What was changed and why]

## Key Files Modified/Created
- file1.go - [description]
- file2.md - [description]

Closes #[number]
```

### Quality Verification (Unchanged)
- Validator runs: `task fmt:check`, `task lint`, `task test`, `task build`
- CI runs identical checks on every PR
- Test failures prevent PR creation at validator level