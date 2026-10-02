# Synchronize Template Versions with Live Configurations

**Issue**: #178  
**Title**: Synchronize template versions with live configurations across themes, agents, evals, and fixtures  
**Type**: Feature Enhancement  

Closes #178

## Solution Approach

Create a systematic synchronization process that treats live versions (`.kiro-krew/`, `.kiro/`) as the authoritative source and updates all template files to match. The approach involves:

1. **Comparison Script**: Build a systematic tool to identify differences between template and live versions
2. **Automated Synchronization**: Update templates to match live versions exactly
3. **Missing Fixtures**: Add eval fixtures that are missing from templates
4. **Verification System**: Ensure `kiro-krew init` produces identical structure to live setup

## Relevant Files

### Template Files (Source)
- `./cmd/kiro-krew/templates/kiro-krew/themes/*.yaml` - Theme configurations
- `./cmd/kiro-krew/templates/kiro/agents/*.json` - Agent configurations  
- `./cmd/kiro-krew/templates/kiro/agents/*-prompt.md` - Agent prompt files
- `./cmd/kiro-krew/templates/kiro-krew/evals/rubrics/*.yaml` - Evaluation rubrics
- `./cmd/kiro-krew/templates/kiro-krew/evals/cases/` - Evaluation test cases
- `./cmd/kiro-krew/templates/kiro-krew/evals/fixtures/` - Missing directory to create

### Live Files (Target/Authority)
- `./.kiro-krew/themes/*.yaml` - Live theme configurations
- `./.kiro/agents/*.json` - Live agent configurations
- `./.kiro/agents/*-prompt.md` - Live agent prompts
- `./.kiro-krew/evals/rubrics/*.yaml` - Live evaluation rubrics  
- `./.kiro-krew/evals/cases/` - Live evaluation cases
- `./.kiro-krew/evals/fixtures/` - Live fixtures to copy

### Implementation Files
- `./internal/templates/extract.go` - Template extraction logic
- `./cmd/kiro-krew/main.go` - Embedded templates
- New comparison/sync script

## Team Orchestration

This is a single-agent implementation task with clear coordination:

1. **Builder Agent**: Implements all synchronization logic and file updates
2. **Validator Agent**: Verifies that post-sync `kiro-krew init` produces correct results
3. **No dependencies**: Can be completed independently without cross-team coordination

## Step-by-Step Task Breakdown

### Task 1: Create Comparison Script
**Acceptance Criteria:**
- Script systematically compares template vs live file structures
- Identifies missing files, differing content, and structural differences
- Outputs structured report of synchronization needed
- Treats live versions as authoritative source
- Handles both `.kiro-krew/` and `.kiro/` directory synchronization

### Task 2: Synchronize Theme Configurations
**Acceptance Criteria:**
- Update all theme YAML files in `./cmd/kiro-krew/templates/kiro-krew/themes/`
- Add missing fields: `agent_success`, `agent_fail` color properties
- Ensure exact match with live versions in `./.kiro-krew/themes/`
- Preserve all existing color configurations while adding new ones

### Task 3: Synchronize Agent Configurations
**Acceptance Criteria:**
- Update all agent JSON configs to match `./.kiro/agents/*.json`
- Update all agent prompt files to match `./.kiro/agents/*-prompt.md` 
- Include QA feedback processing, quality verification sections in prompts
- Ensure builder and validator prompts include latest workflow improvements

### Task 4: Synchronize Evaluation Framework
**Acceptance Criteria:**
- Update all rubric YAML files to match `./.kiro-krew/evals/rubrics/`
- Sync evaluation test cases from `./.kiro-krew/evals/cases/`
- Add enhanced criteria descriptions and deterministic flags
- Maintain evaluation framework structure and scoring systems

### Task 5: Add Missing Fixtures
**Acceptance Criteria:**
- Create `./cmd/kiro-krew/templates/kiro-krew/evals/fixtures/` directory
- Copy all fixture files from `./.kiro-krew/evals/fixtures/`:
  - `codebase-context.md`
  - `status-command-issue.md` 
  - `sample-spec.md`
- Ensure fixtures are included in embedded template resources

### Task 6: Verify Template Extraction
**Acceptance Criteria:**
- Test `kiro-krew init` in fresh directory produces identical structure to live setup
- Compare generated `.kiro-krew/` and `.kiro/` directories with current live versions
- Ensure all files, directories, and content match exactly
- Verify fixtures directory and files are properly created

### Task 7: Update Embedded Resources
**Acceptance Criteria:**
- Ensure Go embed directive includes new fixtures directory
- Verify template extraction logic handles fixtures correctly  
- Test that `kiro-krew update` overwrites with latest templates
- Maintain backward compatibility for existing installations

## Validation Commands

```bash
# Create test directory and verify init
mkdir /tmp/kiro-krew-test
cd /tmp/kiro-krew-test
kiro-krew init

# Compare structures 
diff -r .kiro-krew /path/to/live/.kiro-krew --exclude=specs --exclude=retries --exclude=artifacts
diff -r .kiro /path/to/live/.kiro --exclude=skills

# Verify specific missing elements
ls .kiro-krew/evals/fixtures/
grep "agent_success\|agent_fail" .kiro-krew/themes/*.yaml  
grep "Quality Verification" .kiro/agents/validator-prompt.md

# Test update command
kiro-krew update
echo $? # Should be 0

# Verify fixtures are embedded
go run cmd/kiro-krew/main.go init 2>&1 | grep fixtures
```

## Technical Notes

- Live versions in `.kiro-krew/` and `.kiro/` are the authoritative source - never modify them
- Template directory structure must be preserved to maintain `//go:embed` compatibility
- The `extract.go` logic already handles path mapping from templates to live directories
- Config.yaml is protected from overwrite - this protection should remain
- All fixtures should be included in the embed directive scope

## Risk Mitigation

- Back up current template state before modifications
- Test init/update commands in isolated environments
- Verify embed compilation succeeds with new directory structure
- Ensure no breaking changes to existing `kiro-krew init` behavior