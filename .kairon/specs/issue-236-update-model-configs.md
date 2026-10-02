# Design Specification: Update Planner, Builder, and Architect Agents to Claude Sonnet 4.5

**Issue**: #236  
**Title**: Update planner, builder, and architect agents to use claude-sonnet-4.5  
**Repository**: jbrinkman/kiro-krew  
**Closes**: #236

## Solution Approach

This is a straightforward configuration update task that requires updating the `model` field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"` in six JSON configuration files. The task maintains strict synchronization between live agent configurations (used by running agents) and template configurations (used during project initialization).

The update ensures planner, builder, and architect agents use the latest Claude Sonnet 4.5 model for improved performance and capabilities while preserving all existing functionality and configuration structure.

## Relevant Files

### Files to Modify:
1. `.kiro/agents/planner.json` - Live planner agent configuration
2. `.kiro/agents/builder.json` - Live builder agent configuration
3. `.kiro/agents/architect.json` - Live architect agent configuration
4. `cmd/kiro-krew/templates/kiro/agents/planner.json` - Template planner configuration
5. `cmd/kiro-krew/templates/kiro/agents/builder.json` - Template builder configuration
6. `cmd/kiro-krew/templates/kiro/agents/architect.json` - Template architect configuration

### Configuration Structure Analysis:
All six files follow identical JSON structure with only the model field requiring updates:
- Planner configs: Complex configuration with tools, allowedTools, toolsSettings
- Builder configs: Configuration with basic tools and welcomeMessage
- Architect configs: Configuration with read, write, and shell tools
- All other fields remain unchanged to preserve functionality

## Team Orchestration

This task involves simple, independent JSON file updates with no interdependencies. All six configuration updates can be executed in parallel as they operate on separate files with no shared state or dependencies.

**Parallel Execution**: All tasks can run simultaneously since each modifies a distinct file.

## Step-by-Step Task Breakdown

### Task 1: Update Live Agent Configurations
**Acceptance Criteria**:
- Update `.kiro/agents/planner.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Update `.kiro/agents/builder.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Update `.kiro/agents/architect.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Preserve all other configuration fields exactly as they are
- Maintain valid JSON formatting
**Dependencies**: None (can run in parallel with Task 2)

### Task 2: Update Template Configurations
**Acceptance Criteria**:
- Update `cmd/kiro-krew/templates/kiro/agents/planner.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Update `cmd/kiro-krew/templates/kiro/agents/builder.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Update `cmd/kiro-krew/templates/kiro/agents/architect.json` model field from `"claude-sonnet-4"` to `"claude-sonnet-4.5"`
- Preserve all other configuration fields exactly as they are
- Maintain valid JSON formatting
**Dependencies**: None (can run in parallel with Task 1)

### Task 3: Validate Configuration Integrity
**Acceptance Criteria**:
- Verify all six JSON files have valid syntax
- Confirm live and template configurations remain synchronized
- Verify no other fields were inadvertently changed
- Ensure agent functionality is preserved
**Dependencies**: Task 1, Task 2

## Validation Commands

### JSON Syntax Validation:
```bash
# Validate JSON syntax for all modified files
jq . .kiro/agents/planner.json > /dev/null && echo "planner.json: valid"
jq . .kiro/agents/builder.json > /dev/null && echo "builder.json: valid"
jq . .kiro/agents/architect.json > /dev/null && echo "architect.json: valid"
jq . cmd/kiro-krew/templates/kiro/agents/planner.json > /dev/null && echo "template planner.json: valid"
jq . cmd/kiro-krew/templates/kiro/agents/builder.json > /dev/null && echo "template builder.json: valid"
jq . cmd/kiro-krew/templates/kiro/agents/architect.json > /dev/null && echo "template architect.json: valid"
```

### Model Field Verification:
```bash
# Confirm model field updates
echo "Live planner model: $(jq -r '.model' .kiro/agents/planner.json)"
echo "Live builder model: $(jq -r '.model' .kiro/agents/builder.json)"
echo "Live architect model: $(jq -r '.model' .kiro/agents/architect.json)"
echo "Template planner model: $(jq -r '.model' cmd/kiro-krew/templates/kiro/agents/planner.json)"
echo "Template builder model: $(jq -r '.model' cmd/kiro-krew/templates/kiro/agents/builder.json)"
echo "Template architect model: $(jq -r '.model' cmd/kiro-krew/templates/kiro/agents/architect.json)"
```

### Configuration Synchronization Check:
```bash
# Verify live and template configs remain synchronized (excluding model field)
diff <(jq 'del(.model)' .kiro/agents/planner.json) <(jq 'del(.model)' cmd/kiro-krew/templates/kiro/agents/planner.json)
diff <(jq 'del(.model)' .kiro/agents/builder.json) <(jq 'del(.model)' cmd/kiro-krew/templates/kiro/agents/builder.json)
diff <(jq 'del(.model)' .kiro/agents/architect.json) <(jq 'del(.model)' cmd/kiro-krew/templates/kiro/agents/architect.json)
```

### Expected Validation Results:
- All JSON files should pass syntax validation
- All model fields should show `claude-sonnet-4.5`
- Configuration sync checks should show no differences (empty diff output)
- No other configuration fields should be modified

## Implementation Notes

- This is a low-risk configuration update with no breaking changes
- The change only affects which AI model is used for agent responses
- All existing agent capabilities, tools, and behaviors are preserved
- Template synchronization ensures new kiro-krew project initializations use the updated model
- Changes take effect immediately for new agent spawns; existing running agents continue with their current configuration until restarted