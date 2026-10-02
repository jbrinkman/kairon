# Design Specification: Enhanced Planner Agent with Deep Root Cause Analysis

**Issue**: #189 - Enhance planner agent with deep root cause analysis to prevent symptom-only fixes

**Closes**: #189

## Solution Approach

Transform the planner agent from surface-level issue creation to a root cause analysis engine that performs deep investigation before drafting GitHub issues. The solution introduces experimental worktree capabilities for validation while maintaining strict write restrictions on the main branch.

Key architectural changes:
1. **Tool Expansion**: Add `write` tool access exclusively for planning worktree operations  
2. **Gate 1 Enhancement**: Expand analysis requirements with mandatory root cause investigation
3. **Worktree Integration**: Add planning worktree lifecycle management for testing hypotheses
4. **Decision Framework**: Present root cause vs symptom options when they differ

## Relevant Files

### Core Configuration Files
- `.kiro/agents/planner.json` - Agent manifest requiring tool updates
- `.kiro/agents/planner-prompt.md` - Gate 1 expansion with root cause analysis steps

### New Infrastructure Files
- `.kiro-krew/scripts/planning-worktree-create.sh` - Planning-specific worktree creation
- `.kiro-krew/scripts/planning-worktree-cleanup.sh` - Mandatory cleanup script
- `.kiro/skills/planner-conventions/SKILL.md` - Analysis methodology documentation

### Validation Files
- `.kiro-krew/scripts/validate-planner-tools.sh` - Verify tool restrictions are maintained

## Team Orchestration

**Single Agent Enhancement**: This is primarily a planner agent enhancement that doesn't require coordination with other agents (architect, builder, validator). The planner remains isolated in its planning role while gaining investigation capabilities.

**Worktree Isolation**: Planning worktrees operate completely separately from issue processing worktrees to avoid conflicts. Planning uses `planning-<timestamp>` naming convention vs issue processing's `issue-<number>-<pid>`.

**Tool Restriction Enforcement**: Validate that write access is limited to planning worktrees only through script-based checks.

## Step-by-Step Task Breakdown

### Task 1: Update Planner Agent Manifest
**Acceptance Criteria**:
- Add `write` tool to planner.json tools array
- Add `write` tool to allowedTools array  
- Maintain existing tools: read, shell, web_search, web_fetch
- Preserve existing model and description settings

### Task 2: Create Planning Worktree Scripts
**Acceptance Criteria**:
- `planning-worktree-create.sh` creates planning worktrees with unique naming
- Script prevents conflicts with issue processing worktrees
- Returns worktree path for planner use
- Handles cleanup of stale planning branches
- `planning-worktree-cleanup.sh` removes planning worktrees completely
- Both scripts include logging and error handling

### Task 3: Expand Gate 1 with Root Cause Analysis
**Acceptance Criteria**:
- Insert new analysis steps between "Understand the Request" and "Collaborate"
- Mandate code tracing from error points back to source
- Require hypothesis testing through planning worktrees
- Add multi-layer investigation requirements
- Preserve existing Gate 1/2/3 structure
- Maintain restriction language on main branch modifications

### Task 4: Add Root Cause vs Symptom Decision Framework  
**Acceptance Criteria**:
- When root cause differs from symptom, present both options clearly
- Use structured a/b/c format: "Address symptom as described" vs "Address discovered root cause"  
- Maintain user decision authority
- Preserve existing draft review and label confirmation gates
- Add analysis methodology explanation requirement

### Task 5: Create Analysis Methodology Skill
**Acceptance Criteria**:
- Document step-by-step root cause analysis process
- Include code tracing techniques
- Define hypothesis validation methods
- Provide planning worktree usage guidelines
- Reference as skill resource in planner.json

### Task 6: Add Tool Restriction Validation
**Acceptance Criteria**:
- Script validates write operations only occur in planning worktrees
- Prevents accidental main branch modifications
- Can be run as verification step
- Reports violations clearly

### Task 7: Implement Mandatory Worktree Cleanup
**Acceptance Criteria**:
- Planning worktrees are removed after issue creation
- No artifacts remain in main branch after planning
- Cleanup occurs regardless of issue creation success/failure
- Add cleanup verification to validation script

## Validation Commands

### Test Enhanced Analysis Capabilities
```bash
# Verify planner can create and use planning worktrees
cd /path/to/test/repo
kiro-cli chat --agent planner "Analyze this mock bug report and create an issue"
# Should see planning worktree creation, analysis, and cleanup

# Validate tool restrictions  
.kiro-krew/scripts/validate-planner-tools.sh
```

### Test Root Cause Detection
```bash
# Test with symptom-only description
kiro-cli chat --agent planner "Create issue for '/workspace: read-only file system' error"
# Should investigate code, find host filesystem operations, present both options

# Verify worktree cleanup
ls .worktrees/ | grep planning
# Should show no planning worktrees after completion
```

### Regression Testing
```bash
# Ensure existing workflow preserved
kiro-cli chat --agent planner "Create simple feature request"
# Should follow normal Gate 1/2/3 flow for non-investigative cases

# Validate write restrictions maintained
# Planner should still refuse to modify project files directly
```

### Integration Validation
```bash
# Test full kiro-krew pipeline with enhanced planner
kiro-krew
# Create test issue requiring deep analysis through planner
# Verify issue quality improvement in subsequent architect/builder work
```

## Implementation Notes

**Security**: Write tool access must be strictly limited to planning worktree operations. Any attempt to modify main branch files should fail with clear error messages.

**Cleanup Guarantee**: Planning worktrees must be cleaned up even if the planner process crashes or is interrupted. Consider using trap handlers in scripts.

**Backward Compatibility**: Enhanced analysis should not disrupt simple issue creation workflows. Users creating straightforward feature requests should not face unnecessary investigation steps.

**Performance**: Planning worktree operations should be fast and lightweight. Avoid heavy build operations or extensive file copying during analysis.
