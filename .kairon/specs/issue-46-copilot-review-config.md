# Design Specification: Add Copilot Review Configuration

**Issue:** #46 - Add configuration option to automatically request Copilot reviews on PRs

Closes #46

## Solution Approach

Add a new boolean configuration field `enable_copilot_review` to the existing configuration system that automatically requests GitHub Copilot reviews on PRs created by kiro-krew agents. The feature will integrate into the existing PR creation workflow in step 8 of the krew-lead agent process.

### Architectural Decisions

1. **Configuration Integration**: Add the new field to the existing `Config` struct in `internal/config/config.go` with proper YAML tag
2. **Template Update**: Update the configuration template to include the new field with default value `true`
3. **PR Workflow Enhancement**: Modify the krew-lead agent prompt to conditionally execute Copilot review request after PR creation
4. **Error Handling**: Implement graceful error handling where Copilot review failures don't break the overall workflow

## Relevant Files

### Files to Modify
- `internal/config/config.go` - Add `EnableCopilotReview` field to Config struct
- `cmd/kiro-krew/templates/kiro-krew/config.yaml` - Add configuration field with default value
- `.kiro/agents/krew-lead-prompt.md` - Update workflow to include conditional Copilot review step
- `cmd/kiro-krew/templates/kiro/agents/krew-lead-prompt.md` - Update template version of workflow

### Files Referenced (No Changes)
- `internal/github/client.go` - Existing GitHub CLI integration patterns for reference
- `internal/agent/manager.go` - Existing configuration usage patterns for reference

## Team Orchestration

This is a single-component change that affects:
1. **Configuration Team**: Update config struct and template
2. **Workflow Team**: Update agent prompts with new conditional step
3. **Testing Team**: Validate configuration loading and workflow integration

The changes are independent and can be implemented sequentially without coordination dependencies.

## Step-by-Step Task Breakdown

### Task 1: Update Configuration Structure
**Acceptance Criteria:**
- Add `EnableCopilotReview bool` field to Config struct with YAML tag `enable_copilot_review`
- Ensure proper default value handling in Load() function
- Maintain backward compatibility with existing config files

**Implementation:**
- Modify `internal/config/config.go` to add the new field
- No changes to Load() function needed as Go zero-value (false) works, but we want default true

### Task 2: Update Configuration Template
**Acceptance Criteria:**
- Add `enable_copilot_review: true` to the config template
- Include inline comment explaining the feature
- Maintain existing template structure and formatting

**Implementation:**
- Modify `cmd/kiro-krew/templates/kiro-krew/config.yaml`

### Task 3: Update Krew-Lead Agent Workflow
**Acceptance Criteria:**
- Add conditional step after PR creation to request Copilot review
- Use `gh pr edit --add-reviewer @copilot` command when enabled
- Handle errors gracefully without failing the entire workflow
- Update both active and template versions of the prompt

**Implementation:**
- Modify `.kiro/agents/krew-lead-prompt.md` (active version)
- Modify `cmd/kiro-krew/templates/kiro/agents/krew-lead-prompt.md` (template version)
- Insert new step between "Create PR" and "Label Done" steps

### Task 4: Configuration Loading Enhancement
**Acceptance Criteria:**
- Ensure `enable_copilot_review` defaults to `true` when not specified
- Maintain zero-impact on existing installations
- Validate config loading works with new field

**Implementation:**
- Update default value assignment in Load() function to set EnableCopilotReview: true

## Validation Commands

### Configuration Validation
```bash
# Test config loading with new field
go run cmd/kiro-krew/main.go init --help  # Should not error
cat .kiro-krew/config.yaml | grep enable_copilot_review  # Should show: enable_copilot_review: true
```

### Workflow Integration Validation
```bash
# Verify agent prompt includes new step
grep -A5 -B5 "copilot" .kiro/agents/krew-lead-prompt.md
grep -A5 -B5 "copilot" cmd/kiro-krew/templates/kiro/agents/krew-lead-prompt.md
```

### End-to-End Validation
```bash
# Create test config with enable_copilot_review: false
echo "enable_copilot_review: false" >> test-config.yaml
# Verify that when disabled, no copilot review is requested
# This requires manual testing with actual PR creation workflow
```

### Error Handling Validation
```bash
# Test graceful failure when @copilot is not available
# This requires testing in environment without Copilot access
gh pr edit --add-reviewer @copilot 2>&1 | grep -q "error" || echo "Command succeeded"
```

## Implementation Notes

### Configuration Pattern
The implementation follows the existing configuration pattern where:
- Config struct uses proper YAML tags for field mapping
- Default values are set in the Load() function
- New fields are additive and don't break existing configs

### Workflow Integration Pattern
The enhancement follows the existing step-based workflow where:
- Each step has clear success/failure handling
- Shell commands use `gh` CLI consistently
- Errors in non-critical steps don't halt the workflow

### Error Handling Strategy
Copilot review request failures should be logged but not fail the workflow because:
- PR creation is the primary objective
- Copilot review is an enhancement feature
- Repository might not have Copilot access enabled
- User can manually add reviewer if automated request fails
