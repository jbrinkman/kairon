# Design Specification: Workaround for Empty Subagent Responses with Artifact Checking

**Issue:** #77  
**Closes:** #77  
**Created:** 2026-06-09  
**Author:** Architect Agent  

## Problem Statement

Due to a bug in Kiro CLI, subagents occasionally return empty responses even when they have successfully completed their work and created the expected artifacts. This causes the krew-lead agent to incorrectly treat successful work as failures and trigger unnecessary retry escalation through the four-stage retry process.

## Solution Approach

### High-Level Strategy
1. **Agent Configuration Enhancement**: Add `sentinelFile` field to each agent JSON configuration file
2. **Sentinel File Detection Logic**: Implement sentinel file checking in krew-lead retry logic to detect successful completion despite empty responses  
3. **Temporary Workaround Implementation**: Clearly mark all workaround code for future removal when the Kiro CLI bug is fixed
4. **Preserve Existing Logic**: Maintain all current retry escalation behavior for legitimate failures

### Architectural Decisions
- **Sentinel File Approach**: Each agent writes a well-known file (`.kiro-krew/artifacts/<agent>-<issue>.md`) on completion, avoiding false positives from pre-existing files
- **Non-Breaking Implementation**: Workaround activates only for empty responses, preserving normal failure handling
- **Clear Separation**: All workaround code is isolated and marked with clear comments for easy removal

## Relevant Files

### Files to Modify
- `.kiro/agents/architect.json` - Add sentinelFile for design specifications
- `.kiro/agents/builder.json` - Add sentinelFile for implementation artifacts  
- `.kiro/agents/validator.json` - Add sentinelFile for validation reports, add write permission
- `.kiro/agents/documenter.json` - Add sentinelFile for documentation files
- `.kiro/agents/krew-lead-prompt.md` - Implement sentinel file checking logic in retry stages
- `.kiro/agents/validator-prompt.md` - Note write permission scoped to sentinel file

### Files Not Modified
- `.kiro/agents/planner.json` - Planner is interactive and not part of automated krew workflow
- `.kiro/agents/krew-lead.json` - No configuration changes needed

## Agent Configuration Updates

### Sentinel File by Agent

Each agent declares a `sentinelFile` path pattern. The agent writes this file upon successful completion with a summary of work performed. The `<issue>` placeholder is replaced at runtime with the issue number.

**Architect Agent**
```json
{
  "name": "architect",
  "sentinelFile": ".kiro-krew/artifacts/architect-<issue>.md",
  "prompt": "file://./architect-prompt.md",
  "tools": ["read", "write", "shell"],
  "allowedTools": ["read", "write", "shell"],
  "model": "claude-sonnet-4"
}
```

**Builder Agent**
```json
{
  "name": "builder",
  "sentinelFile": ".kiro-krew/artifacts/builder-<issue>.md",
  "description": "Focused engineering agent that executes ONE task at a time. Builds, implements, creates.",
  "prompt": "file://./builder-prompt.md",
  "tools": ["read", "write", "shell"],
  "allowedTools": ["read", "write", "shell"],
  "model": "claude-sonnet-4",
  "welcomeMessage": "Builder ready. What task should I execute?"
}
```

**Validator Agent**
```json
{
  "name": "validator",
  "sentinelFile": ".kiro-krew/artifacts/validator-<issue>.md",
  "description": "Read-only validation agent that verifies task completion against acceptance criteria.",
  "prompt": "file://./validator-prompt.md",
  "tools": ["read", "write", "shell"],
  "allowedTools": ["read", "write", "shell"],
  "toolsSettings": {
    "shell": {
      "autoAllowReadonly": true
    }
  },
  "model": "claude-sonnet-4",
  "welcomeMessage": "Validator ready. What should I verify?"
}
```
Note: Validator gains `write` permission scoped to creating its sentinel file only.

**Documenter Agent**
```json
{
  "name": "documenter",
  "sentinelFile": ".kiro-krew/artifacts/documenter-<issue>.md",
  "description": "Generates documentation for completed and validated features.",
  "prompt": "file://./documenter-prompt.md",
  "tools": ["read", "write"],
  "allowedTools": ["read", "write"],
  "model": "claude-sonnet-4",
  "welcomeMessage": "Documenter ready. What should I document?"
}
```

## Krew-Lead Logic Enhancement

### Sentinel File Checking Implementation

Add the following logic to krew-lead-prompt.md after the existing retry policy section:

```markdown
### TEMPORARY WORKAROUND: Empty Response Sentinel File Detection
<!-- TODO: Remove this section when Kiro CLI empty response bug is fixed -->

When a subagent returns an empty response, before escalating to the next retry stage:

1. **Check for Sentinel File**: Read the agent's `sentinelFile` from its JSON config and check existence:
   ```bash
   test -f .kiro-krew/artifacts/<agent>-<issue>.md
   ```
2. **Handle Detection Results**:
   - **If sentinel file exists**: Read its contents for context, log incident with "empty-response-workaround" label, and continue workflow normally
   - **If sentinel file missing**: Proceed with normal retry escalation

Each subagent writes a sentinel file at `.kiro-krew/artifacts/<agent>-<issue>.md` upon successful completion, including a summary of work performed.
```

## Step-by-Step Task Breakdown

### Task 1: Update Agent Configuration Files
**Acceptance Criteria:**
- All agent JSON files contain `sentinelFile` field with path `.kiro-krew/artifacts/<agent>-<issue>.md`
- Validator gains `write` permission scoped to sentinel file
- JSON syntax is valid and parseable

**Validation Commands:**
```bash
# Validate JSON syntax
jq . .kiro/agents/architect.json
jq . .kiro/agents/builder.json  
jq . .kiro/agents/validator.json
jq . .kiro/agents/documenter.json

# Verify sentinelFile field exists
jq '.sentinelFile' .kiro/agents/architect.json
```

### Task 2: Implement Sentinel File Checking Logic
**Acceptance Criteria:**
- Krew-lead prompt contains sentinel file checking logic
- Workaround code is clearly marked as temporary
- Logic only activates for empty responses
- Existing retry escalation preserved for legitimate failures

**Validation Commands:**
```bash
# Verify krew-lead prompt contains workaround section
grep -A 10 "TEMPORARY WORKAROUND" .kiro/agents/krew-lead-prompt.md

# Test sentinel file detection
mkdir -p .kiro-krew/artifacts
touch .kiro-krew/artifacts/architect-77.md
test -f .kiro-krew/artifacts/architect-77.md && echo "Sentinel detection works"
rm .kiro-krew/artifacts/architect-77.md
```

### Task 3: Test Workaround Implementation  
**Acceptance Criteria:**
- Workaround correctly identifies when sentinel file exists
- Normal retry behavior preserved for actual failures
- Incident logging captures empty response events
- No breaking changes to existing workflow

**Validation Commands:**
```bash
# Create test sentinel and verify detection
mkdir -p .kiro-krew/artifacts
echo "# Test" > .kiro-krew/artifacts/builder-77.md
test -f .kiro-krew/artifacts/builder-77.md && echo "Test sentinel detected"

# Clean up test
rm .kiro-krew/artifacts/builder-77.md
```

## Team Orchestration

### Implementation Sequence
1. **Builder Agent**: Update all agent JSON configuration files with sentinelFile fields
2. **Builder Agent**: Implement sentinel file checking logic in krew-lead-prompt.md  
3. **Validator Agent**: Verify all configurations are valid and logic is correctly implemented
4. **Builder Agent**: Create unit tests or validation scripts to verify workaround behavior

### Communication Protocol
- All workaround code must include comments referencing issue #77
- Mark all sections with "TEMPORARY WORKAROUND" for easy identification
- Document sentinel file paths clearly in agent configurations

## Validation Commands

### Pre-Implementation Validation
```bash
# Verify current agent configurations are valid
for agent in architect builder validator documenter; do
  jq . ".kiro/agents/$agent.json" > /dev/null && echo "$agent.json is valid"
done

# Check krew-lead prompt exists
test -f .kiro/agents/krew-lead-prompt.md && echo "krew-lead-prompt.md exists"
```

### Post-Implementation Validation  
```bash
# Verify sentinelFile fields are present
for agent in architect builder validator documenter; do
  jq -e '.sentinelFile' ".kiro/agents/$agent.json" && echo "$agent has sentinelFile"
done

# Test sentinel file detection
mkdir -p .kiro-krew/artifacts
echo "# Test summary" > .kiro-krew/artifacts/architect-99.md
test -f .kiro-krew/artifacts/architect-99.md && echo "✓ Sentinel detection works"

# Cleanup
rm -f .kiro-krew/artifacts/architect-99.md
```

### Runtime Validation
```bash
# Verify workaround logic is present in krew-lead
grep -q "TEMPORARY WORKAROUND" .kiro/agents/krew-lead-prompt.md && echo "✓ Workaround implemented"

# Check for proper marking of temporary code
grep -q "TODO: Remove.*Kiro CLI.*bug" .kiro/agents/krew-lead-prompt.md && echo "✓ Removal instructions present"
```

## Risk Assessment

### Low Risk
- Agent JSON configuration changes (easily reversible)
- Adding sentinel file path (non-breaking addition)

### Medium Risk  
- Modifying krew-lead retry logic (could affect failure handling)
- Adding write permission to validator (scoped to sentinel file only)

### Mitigation Strategies
- Preserve all existing retry logic as fallback
- Sentinel file path is unique per agent+issue, avoiding false positives
- Clear marking of workaround code for easy removal
- Comprehensive validation before deployment

## Future Removal Plan

When the Kiro CLI empty response bug is fixed:

1. **Remove Workaround Code**: Delete all sections marked "TEMPORARY WORKAROUND" from krew-lead-prompt.md
2. **Clean Agent Configs**: Remove `sentinelFile` fields from all agent JSON files  
3. **Revert Validator Permissions**: Remove `write` from validator tools if no longer needed
4. **Update Documentation**: Remove references to empty response handling from workflow documentation
5. **Test Cleanup**: Verify normal retry escalation still works correctly

Search pattern for removal:
```bash
grep -r "TEMPORARY WORKAROUND\|TODO.*Kiro CLI.*bug\|sentinelFile" .kiro/
```

