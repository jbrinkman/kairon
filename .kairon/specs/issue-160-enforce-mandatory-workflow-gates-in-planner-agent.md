# Design Specification: Enforce Mandatory Workflow Gates in Planner Agent

**Issue:** #160  
**Repository:** jbrinkman/kiro-krew  
Closes #160

## Solution Approach

The planner agent currently has insufficient workflow enforcement, allowing it to skip critical user review gates and create potentially low-quality issues. This creates cascading problems where automated systems (kiro-krew) implement poor specifications.

The solution implements two mandatory, non-bypassable gates:

1. **Draft Review Gate** - Planner MUST show complete draft issue and wait for explicit user approval
2. **Label Confirmation Gate** - After draft approval, planner MUST separately ask about kiro-krew labeling

These gates are enforced through prompt modifications that make the workflow steps absolute requirements with explicit validation checks.

## Relevant Files

### Files to Modify:
- `.kiro/agents/planner-prompt.md` - Add mandatory workflow enforcement
- `.kiro/skills/plan-with-krew/SKILL.md` - Update skill to match new workflow

### Files to Reference:
- `internal/session/planner.go` - Understanding planner process integration
- `internal/tui/commands.go` - How planning sessions are initiated

## Team Orchestration

This is a prompt engineering change that requires no coordination with other components. The planner agent operates independently, and the workflow changes are self-contained within the agent's instructions.

## Step-by-Step Task Breakdown

### Task 1: Update Planner Agent Prompt
**Acceptance Criteria:**
- [ ] Remove current workflow section and replace with mandatory gate enforcement
- [ ] Add explicit draft review requirement before any `gh issue create` command
- [ ] Add explicit label confirmation requirement as separate step after draft approval
- [ ] Include validation language that these gates cannot be skipped under any circumstances
- [ ] Maintain existing restrictions on shell commands and implementation tasks

### Task 2: Update Plan-with-Krew Skill
**Acceptance Criteria:**
- [ ] Modify skill description to reflect mandatory gates
- [ ] Update behavior section to enforce draft review and label confirmation steps
- [ ] Ensure consistency with planner agent prompt changes
- [ ] Remove any language suggesting gates can be bypassed for "urgent" work

### Task 3: Test Workflow Enforcement
**Acceptance Criteria:**
- [ ] Test planner agent in interactive mode to verify draft review gate
- [ ] Test that planner waits for explicit approval before creating issues
- [ ] Test that planner asks separately about labeling after draft approval
- [ ] Verify that agent refuses to skip gates regardless of request clarity

## Validation Commands

```bash
# Test planner agent workflow
kiro-cli chat --agent planner "Create an issue for adding user authentication"

# Test plan-with-krew skill
kiro-cli chat "@plan-with-krew Add user authentication system"

# Verify agent configurations are valid
kiro-cli validate .kiro/agents/planner.json

# Test interactive planning session
kiro-krew
> plan Add user authentication
```

The validation should confirm:
1. Agent shows draft issue and waits for confirmation
2. After approval, agent asks separately about kiro-krew labeling  
3. No issue is created without explicit user approval of draft
4. Gates cannot be bypassed with any request phrasing

## Implementation Notes

- Focus on prompt engineering rather than code changes
- Use absolute language ("MUST", "NEVER") to prevent interpretation flexibility
- Add explicit validation steps that check for user confirmation before proceeding
- Ensure backward compatibility with existing planner agent usage patterns