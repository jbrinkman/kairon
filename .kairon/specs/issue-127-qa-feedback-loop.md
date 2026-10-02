# Design Specification: QA Feedback Loop with Language-Agnostic Quality Checks

**Issue:** #127 - Implement QA feedback loop with language-agnostic quality checks  
**Closes:** #127

## Solution Approach

Implement a comprehensive quality assurance system that prevents CI failures by catching issues before PR creation. The solution introduces:

1. **Quality Discovery Pattern**: Language-agnostic approach to discover project-specific QA tools from CI configs and project files
2. **Builder QA Integration**: Enhance builder agent to run quality checks as part of implementation workflow
3. **Validator QA Verification**: Strengthen validator agent to independently verify all quality gates pass
4. **Feedback Loop Protocol**: Create structured communication between validator and builder for iterative fixes
5. **Krew-Lead QA Orchestration**: Modify workflow to enforce quality gates before PR creation
6. **Quality Tracking**: Add QA loop iteration tracking to prevent infinite cycles

This approach maintains language-agnostic flexibility while ensuring consistent quality enforcement across all projects.

## Relevant Files

### Agent Configuration Files (Primary Changes)
- `.kiro/agents/builder-prompt.md` - Add comprehensive QA guidance and discovery patterns
- `.kiro/agents/validator-prompt.md` - Add QA verification steps and feedback generation
- `.kiro/agents/krew-lead-prompt.md` - Add QA feedback loop orchestration

### Sentinel File Format Changes
- `.kiro-krew/artifacts/builder-*.md` - Enhanced format including QA check results
- `.kiro-krew/artifacts/validator-*.md` - Enhanced format including detailed feedback for failures

### Quality Discovery Reference Files
- `.github/workflows/ci.yml` - CI workflow patterns to discover (no changes)
- `Taskfile.yml` - Build tool patterns to discover (no changes)  
- `package.json`, `Makefile`, `go.mod`, etc. - Language-specific patterns to discover (no changes)

## Team Orchestration

### Sequential Agent Enhancement
This implementation requires coordinated updates across three agents:

1. **Builder Agent Enhancement** (Task 1-2): Update to include QA discovery and execution
2. **Validator Agent Enhancement** (Task 3-4): Update to verify QA and provide structured feedback
3. **Krew-Lead Workflow Enhancement** (Task 5): Implement feedback loop orchestration

### Quality Gate Enforcement
The new workflow ensures:
- Builder runs QA checks before completion
- Validator independently verifies all checks pass
- Failed validation triggers builder re-execution with specific feedback
- PR creation only occurs after validator approval
- Configurable retry limits prevent infinite cycles

## Step-by-Step Task Breakdown

### Task 1: Enhance Builder QA Discovery
**Objective:** Add language-agnostic quality tool discovery to builder agent

**Files:** `.kiro/agents/builder-prompt.md`

**Changes Required:**
- Add "Quality Discovery" section with systematic approach to find QA tools
- Include CI/CD file patterns: `.github/workflows/*.yml`, `.gitlab-ci.yml`, `Jenkinsfile`
- Include build tool patterns: `package.json` (scripts), `Taskfile.yml`, `Makefile`, `tox.ini`
- Include language-specific patterns: `go.mod` (gofmt, go vet), `pyproject.toml` (black, flake8), etc.
- Add instructions to examine CI steps and map to local commands
- Require quality tool discovery before implementation

**Acceptance Criteria:**
- [ ] Builder discovers QA tools by examining CI configuration files
- [ ] Builder identifies build tool scripts that include quality checks
- [ ] Builder recognizes language-specific conventions and patterns
- [ ] Discovery approach works without hardcoding specific tool names
- [ ] Builder documents discovered QA commands in sentinel file

### Task 2: Integrate QA Execution in Builder Workflow
**Objective:** Require builder to run and pass all discovered quality checks

**Files:** `.kiro/agents/builder-prompt.md`

**Changes Required:**
- Update workflow section to include "Quality Assurance" step before completion
- Require execution of all discovered QA commands (formatting, linting, testing)
- Add error handling for QA failures with specific fix guidance
- Update sentinel file format to include QA check results and status
- Add hard rule: ALL tests must pass if tests exist in the project

**Acceptance Criteria:**
- [ ] Builder runs formatting checks (gofmt, black, prettier, etc.) automatically
- [ ] Builder runs linting checks (go vet, eslint, flake8, etc.) automatically  
- [ ] Builder runs all available tests and requires 100% pass rate
- [ ] Builder provides specific error messages for each failing QA check
- [ ] Sentinel file documents QA commands run and their results

### Task 3: Enhance Validator QA Verification
**Objective:** Add comprehensive QA verification and structured feedback generation

**Files:** `.kiro/agents/validator-prompt.md`

**Changes Required:**
- Add "Quality Verification" section mirroring builder's discovery process
- Require validator to independently run the same QA checks as builder
- Add structured feedback format for QA failures with actionable fixes
- Update sentinel file to include detailed failure analysis and recommendations
- Maintain read-only approach by using test runners, linters in check mode

**Acceptance Criteria:**
- [ ] Validator discovers and runs the same QA tools as builder
- [ ] Validator reports PASS/FAIL status for each quality check independently
- [ ] Validator provides structured feedback with specific failing checks and fixes
- [ ] Sentinel file includes actionable remediation steps for builder
- [ ] Validator maintains read-only principle (no file modifications)

### Task 4: Implement QA Feedback Protocol  
**Objective:** Define structured communication format between validator and builder

**Files:** `.kiro/agents/validator-prompt.md`, `.kiro/agents/builder-prompt.md`

**Changes Required:**
- Define standard QA feedback format in validator sentinel file
- Include specific failing commands, error output, and suggested remediation
- Update builder to parse validator feedback on subsequent attempts
- Add guidance for builder to address specific QA failures from validator feedback
- Ensure feedback loop maintains context across retry attempts

**Acceptance Criteria:**
- [ ] Validator feedback includes failing command name and full error output
- [ ] Validator feedback provides specific file names and line numbers where available
- [ ] Builder can parse validator feedback and focus on specific failing checks
- [ ] Feedback format is consistent and machine-readable for builder processing
- [ ] Builder acknowledges validator feedback in subsequent attempt reports

### Task 5: Implement Krew-Lead QA Orchestration
**Objective:** Add QA feedback loop workflow to krew-lead orchestration

**Files:** `.kiro/agents/krew-lead-prompt.md`

**Changes Required:**
- Insert "Quality Assurance Loop" step between builder completion and PR creation
- Implement builder-validator feedback cycle with configurable retry limits (default: 3 attempts)
- Add QA loop iteration tracking in attempt system
- Parse validator feedback and re-delegate to builder with specific instructions
- Only proceed to PR creation when validator reports all QA checks passing
- Track QA iterations separately from general retry attempts

**Acceptance Criteria:**
- [ ] Krew-lead checks validator QA status before PR creation
- [ ] Failed QA triggers builder re-delegation with validator feedback
- [ ] QA loop has separate retry limit from general agent failures  
- [ ] QA iteration count is tracked and logged
- [ ] PR creation blocked until validator reports PASS status
- [ ] Infinite loop prevention via configurable QA retry limits

### Task 6: Update Sentinel File Formats
**Objective:** Enhance sentinel files to support QA reporting and feedback

**Files:** Builder and validator sentinel file formats

**Changes Required:**
- Add QA section to builder sentinel with discovered commands and results
- Add feedback section to validator sentinel with specific failure details
- Include structured data for machine parsing by krew-lead
- Maintain backward compatibility with existing sentinel file readers
- Add QA iteration tracking fields

**Acceptance Criteria:**
- [ ] Builder sentinel includes "QA Commands Discovered" and "QA Results" sections
- [ ] Validator sentinel includes "QA Verification" and "Feedback" sections  
- [ ] QA results include command name, exit code, and output for each check
- [ ] Feedback section is structured for machine parsing by krew-lead
- [ ] Sentinel files remain human-readable and debuggable

## Validation Commands

### Quality Discovery Verification
```bash
# Test builder QA discovery in Go project
cd test-project && task lint && task fmt:check && task test

# Verify discovered commands match CI configuration  
grep -r "task\|go\|test" .github/workflows/ci.yml

# Test in different language projects
cd node-project && npm run lint && npm test
cd python-project && python -m black --check . && python -m pytest
```

### QA Loop Integration Test
```bash
# Create intentionally failing code (formatting issue)
echo "package main;func main(){ }" > cmd/test/broken.go

# Run builder and verify QA catches the issue
kiro-cli chat --agent builder --no-interactive "Fix formatting issues"

# Verify validator catches and provides feedback
kiro-cli chat --agent validator --no-interactive "Verify QA compliance"

# Check sentinel files include QA results
cat .kiro-krew/artifacts/builder-*.md | grep -A5 "QA Results"
cat .kiro-krew/artifacts/validator-*.md | grep -A5 "QA Verification"
```

### End-to-End QA Workflow Test
```bash
# Start with fresh worktree containing intentional QA failures
git checkout -b qa-test-branch

# Simulate krew-lead workflow with QA loop
kiro-cli chat --agent krew-lead --no-interactive "Process issue with QA failures"

# Verify QA loop prevents PR creation until fixes applied
git log --oneline | grep -v "PR created" # Should show multiple QA fix attempts

# Verify final PR creation only after QA passes
gh pr list --head qa-test-branch | grep "QA.*PASS"
```

### Language-Agnostic Pattern Test
```bash
# Test discovery in multiple project types
for dir in go-project node-project python-project; do
  cd $dir
  echo "Testing QA discovery in $dir..."
  kiro-cli chat --agent builder --no-interactive "Discover QA tools"
  echo "Discovered tools: $(grep 'QA Commands' .kiro-krew/artifacts/builder-*.md)"
done

# Verify no hardcoded tool names in agent prompts
grep -i "gofmt\|eslint\|black\|pytest" .kiro/agents/*-prompt.md && echo "FAIL: Hardcoded tools found"
```

### Retry Loop Prevention Test
```bash
# Create unfixable QA failure scenario
echo "syntax error that cannot be fixed automatically" > broken.txt

# Run QA loop and verify it halts after retry limit  
kiro-cli chat --agent krew-lead --no-interactive "Process broken issue"

# Check incident report creation for QA failures
test -f .kiro-krew/specs/incidents/qa-loop-incident.md && echo "PASS: Incident report created"

# Verify issue labeled as failed after retry exhaustion
gh issue view <issue-number> --json labels | grep "kiro-krew-failed"
```