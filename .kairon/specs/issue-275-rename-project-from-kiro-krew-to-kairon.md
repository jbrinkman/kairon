# Design Specification: Rename Project from Kiro-Krew to Kairon

**Issue**: #275  
**Title**: Rename project from Kiro-Krew to Kairon  
**Repository**: jbrinkman/kairon  
**Closes**: #275

## Problem Statement

The project name "Kiro-Krew" conflicts with an existing project called "KiroCrew". To avoid naming conflicts and potential confusion, we need to systematically rename the entire project to "Kairon" and update all references throughout the codebase (~126 files), documentation, configuration files, and directory structures.

## Solution Approach

This is a comprehensive, systematic rename operation affecting multiple layers of the project:

1. **Module and Import Path Migration**: Update Go module path from `github.com/jbrinkman/kiro-krew` to `github.com/jbrinkman/kairon` across all Go files
2. **Directory Structure Renaming**: Rename `.kiro-krew/` to `.kairon/` and `cmd/kiro-krew/` to `cmd/kairon/`
3. **Binary and Build System**: Update binary name, build scripts, and CI/CD workflows
4. **Documentation and Configuration**: Update all references in markdown, YAML, JSON, and shell scripts
5. **Embedded Templates**: Synchronize all template changes (critical for self-hosting)
6. **Historical Artifact Preservation**: Explicitly exclude historical directories from renaming

### Reference Work

The Flowdra PR (https://github.com/Bit-Quill/flowdra/pull/6) demonstrates a similar rename from kiro-krew to flowdra, affecting ~200+ files. Key patterns:
- Directory rename: `.kiro-krew/` → `.flowdra/`
- Binary rename: `kiro-krew` → `flowdra`
- Module path: `github.com/jbrinkman/kiro-krew` → `github.com/Bit-Quill/flowdra`
- Config label: `kiro-krew` → `flowdra`

### Case Sensitivity Convention

- **Prose/Titles**: Use "Kairon" (capitalized)
- **Technical Identifiers**: Use "kairon" (lowercase) for paths, binary names, labels, module paths

## Relevant Files

### Go Module and Source Files (~80 files)
- `go.mod` - Module path definition
- All `.go` files in `cmd/kairon/`, `internal/*/` with string literals or comments
- `internal/version/version.go` - Binary name references
- `internal/config/*.go` - Configuration path references
- `internal/watcher/*.go` - Label and path references
- `internal/agent/*.go` - Environment variable references
- `internal/tui/*.go` - UI text and path references
- `internal/session/*.go` - Worktree path references
- `internal/eval/*.go` - Evaluation path references
- `internal/incidents/*.go` - Incident logging paths
- `internal/hotkey/*.go` - Hotkey references
- `internal/templates/*.go` - Template extraction

### Build and CI/CD Configuration (~5 files)
- `Taskfile.yml` - Binary name, build paths, ldflags
- `.github/workflows/ci.yml` - Workflow references
- `.github/workflows/release.yml` - Release asset names
- `.releaserc.json` - Asset paths and labels
- `package.json` - Package name and repository URL

### Documentation (~10 files)
- `README.md` - Main documentation
- `CHANGELOG.md` - Header and structure (not historical entries)
- `CONTRIBUTING.md` - Project references
- `docs/evaluation.md` - Evaluation documentation
- `docs/unified-container-flow.md` - Container workflow docs
- `docs/hotkey-toggle.md` - Hotkey documentation
- `docs/agent-conventions.md` - Agent conventions
- `docs/eval-framework-analysis.md` - Eval framework docs
- `docs/performance-investigation.md` - Performance docs

### Configuration Files (~3 files)
- `.kairon/config.yaml` - Repository, label, and path settings
- Live configuration files in `.kairon/`

### Shell Scripts (~6 files)
- `.kairon/scripts/worktree-create.sh`
- `.kairon/scripts/worktree-merge.sh`
- `.kairon/scripts/planning-worktree-create.sh`
- `.kairon/scripts/planning-worktree-cleanup.sh`
- `scripts/template-sync-summary.sh`
- `scripts/compare-templates.go`
- `test_integration.sh`
- `validate_integration.sh`

### Agent Configuration (~12 files)
- `.kiro/agents/planner-prompt.md`
- `.kiro/agents/krew-lead-prompt.md`
- `.kiro/agents/architect-prompt.md`
- `.kiro/agents/builder-prompt.md`
- `.kiro/agents/validator-prompt.md`
- `.kiro/agents/documenter-prompt.md`

### Skills Documentation (~4 files)
- `.kiro/skills/builder-conventions/SKILL.md`
- `.kiro/skills/validator-conventions/SKILL.md`
- `.kiro/skills/planner-conventions/SKILL.md`
- `.kiro/skills/discover-qa-tools/SKILL.md`
- `.kiro/skills/sentinel-protocol/SKILL.md`

### Embedded Templates (~30+ files)
- `cmd/kairon/templates/kiro/agents/*.json`
- `cmd/kairon/templates/kiro/agents/*.md`
- `cmd/kairon/templates/kairon/config.yaml`
- `cmd/kairon/templates/kairon/scripts/*.sh`
- `cmd/kairon/templates/kairon/themes/*.yaml`
- `cmd/kairon/templates/kairon/evals/fixtures/*`
- `cmd/kairon/templates/kairon/evals/rubrics/*.yaml`
- `cmd/kairon/templates/kairon/evals/cases/**/*.yaml`
- `cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md`

### Directories to Rename
- `.kiro-krew/` → `.kairon/`
- `cmd/kiro-krew/` → `cmd/kairon/`
- `cmd/kairon/templates/kiro-krew/` → `cmd/kairon/templates/kairon/`

### Historical Artifacts to PRESERVE (no changes)
- `.kairon/logs/*` - Historical agent logs
- `.kairon/retries/*` - Historical retry counts
- `.kairon/specs/*` - Historical specifications
- `.kairon/artifacts/*` - Historical build artifacts
- `.kairon/evals/results/*` - Historical evaluation results

These files reflect the historical reality when they were generated under the "kiro-krew" name and must NOT be modified.

## Team Orchestration

This is a large-scale refactoring that must be executed systematically to avoid breaking the build or introducing inconsistencies. The work is organized into layers:

1. **Foundation Layer**: Module path and directory structure
2. **Source Code Layer**: Go source files and imports
3. **Build System Layer**: Build configuration and CI/CD
4. **Documentation Layer**: All documentation and configuration files
5. **Template Synchronization Layer**: Embedded templates (must happen AFTER live files)
6. **Validation Layer**: Comprehensive testing and verification

Each layer must be completed and validated before proceeding to the next. The builder agent will execute tasks sequentially, with validation gates at each step.

## Step-by-Step Task Breakdown

### Phase 1: Foundation - Module and Directory Structure

#### Task 1.1: Rename Directory Structures
**Agent**: builder  
**Description**: Rename the primary directory structures from kiro-krew to kairon.

**Actions**:
1. Rename `cmd/kiro-krew/` to `cmd/kairon/`
2. Rename `.kiro-krew/` to `.kairon/`
3. Within `cmd/kairon/templates/`, rename `kiro-krew/` to `kairon/`
4. Update `.gitignore` if it contains hardcoded `.kiro-krew/` references

**Acceptance Criteria**:
- Directory `cmd/kairon/` exists
- Directory `.kairon/` exists
- Directory `cmd/kairon/templates/kairon/` exists
- No `cmd/kiro-krew/` or `.kiro-krew/` directories remain (except in historical artifact paths)
- Historical artifact directories preserved: `.kairon/logs/`, `.kairon/retries/`, `.kairon/specs/`, `.kairon/artifacts/`, `.kairon/evals/results/`

**Validation Commands**:
```bash
test -d cmd/kairon
test -d .kairon
test -d cmd/kairon/templates/kairon
test ! -d cmd/kiro-krew
test -d .kairon/logs || echo "logs dir may not exist yet"
```

**Dependencies**: None

---

#### Task 1.2: Update Go Module Path
**Agent**: builder  
**Description**: Update the Go module path and all import statements from `github.com/jbrinkman/kiro-krew` to `github.com/jbrinkman/kairon`.

**Actions**:
1. Update `go.mod`: change module path to `github.com/jbrinkman/kairon`
2. Find and replace all import statements across all `.go` files
3. Update ldflags references in build scripts (Taskfile.yml)
4. Run `go mod tidy` to verify module integrity

**Acceptance Criteria**:
- `go.mod` declares module `github.com/jbrinkman/kairon`
- No Go files contain imports of `github.com/jbrinkman/kiro-krew`
- `go mod tidy` runs without errors
- All Go files compile without import errors

**Validation Commands**:
```bash
grep -q "module github.com/jbrinkman/kairon" go.mod
! grep -r "github.com/jbrinkman/kiro-krew" --include="*.go" . || exit 1
go mod tidy
go build ./...
```

**Dependencies**: Task 1.1 (directory rename must precede build path updates)

---

### Phase 2: Build System and Configuration

#### Task 2.1: Update Taskfile.yml
**Agent**: builder  
**Description**: Update all Taskfile.yml configuration to use "kairon" instead of "kiro-krew".

**Actions**:
1. Change `BINARY_NAME` from `kiro-krew` to `kairon`
2. Update ldflags module path from `github.com/jbrinkman/kiro-krew/internal/version` to `github.com/jbrinkman/kairon/internal/version`
3. Update build command paths from `./cmd/kiro-krew` to `./cmd/kairon`
4. Update sync:check task paths from `.kiro-krew/` to `.kairon/`
5. Update all asset path references in build:release tasks

**Acceptance Criteria**:
- `BINARY_NAME` variable set to `kairon`
- All ldflags reference `github.com/jbrinkman/kairon`
- Build paths point to `./cmd/kairon`
- Sync check paths reference `.kairon/`
- No references to `kiro-krew` remain in Taskfile.yml

**Validation Commands**:
```bash
grep -q "BINARY_NAME: kairon" Taskfile.yml
grep -q "github.com/jbrinkman/kairon/internal/version" Taskfile.yml
! grep "kiro-krew" Taskfile.yml || exit 1
```

**Dependencies**: Task 1.1, Task 1.2

---

#### Task 2.2: Update package.json and .releaserc.json
**Agent**: builder  
**Description**: Update npm package configuration and semantic-release configuration.

**Actions**:
1. Update `package.json`:
   - Change `name` from `kiro-krew` to `kairon`
   - Update repository URL from `jbrinkman/kiro-krew` to `jbrinkman/kairon`
2. Update `.releaserc.json`:
   - Change asset paths from `kiro-krew` to `kairon`
   - Update asset labels from "kiro-krew" to "kairon"

**Acceptance Criteria**:
- `package.json` has name "kairon"
- `package.json` repository URL is `https://github.com/jbrinkman/kairon.git`
- `.releaserc.json` asset paths reference `dist/release/kairon`
- `.releaserc.json` asset labels reference "kairon"
- No "kiro-krew" references remain in either file

**Validation Commands**:
```bash
grep -q '"name": "kairon"' package.json
grep -q "jbrinkman/kairon" package.json
grep -q "dist/release/kairon" .releaserc.json
! grep "kiro-krew" package.json || exit 1
! grep "kiro-krew" .releaserc.json || exit 1
```

**Dependencies**: None

---

#### Task 2.3: Update GitHub Actions Workflows
**Agent**: builder  
**Description**: Update CI/CD workflow files to reference "kairon".

**Actions**:
1. Update `.github/workflows/ci.yml`:
   - No binary name changes needed (uses task commands)
   - Verify all task commands are correct
2. Update `.github/workflows/release.yml`:
   - Update binary paths from `kiro-krew` to `kairon`
   - Update release asset names and labels
   - Update gh release create command asset references

**Acceptance Criteria**:
- `.github/workflows/release.yml` references `dist/release/kairon` binaries
- Release asset labels reference "kairon"
- No "kiro-krew" references in workflow files

**Validation Commands**:
```bash
grep -q "kairon" .github/workflows/release.yml
! grep "kiro-krew" .github/workflows/ci.yml || exit 1
! grep "kiro-krew" .github/workflows/release.yml || exit 1
```

**Dependencies**: Task 2.1

---

### Phase 3: Configuration Files

#### Task 3.1: Update .kairon/config.yaml
**Agent**: builder  
**Description**: Update the main configuration file with new repository name and paths.

**Actions**:
1. Update `repo` from `jbrinkman/kiro-krew` to `jbrinkman/kairon`
2. Update `label` from `kiro-krew` to `kairon`
3. Update `sessions_dir` from `.kiro-krew/sessions` to `.kairon/sessions`
4. Update `log_dir` from `.kiro-krew/logs` to `.kairon/logs`

**Acceptance Criteria**:
- `repo` field is `jbrinkman/kairon`
- `label` field is `kairon`
- `sessions_dir` is `.kairon/sessions`
- `log_dir` is `.kairon/logs`
- No "kiro-krew" references in config.yaml

**Validation Commands**:
```bash
grep -q "repo: jbrinkman/kairon" .kairon/config.yaml
grep -q "label: kairon" .kairon/config.yaml
grep -q "sessions_dir: .kairon/sessions" .kairon/config.yaml
! grep "kiro-krew" .kairon/config.yaml || exit 1
```

**Dependencies**: Task 1.1

---

#### Task 3.2: Update Template config.yaml
**Agent**: builder  
**Description**: Update the embedded template configuration file.

**Actions**:
1. Update `cmd/kairon/templates/kairon/config.yaml`:
   - Update `repo` placeholder pattern to reference "kairon"
   - Update `label` from `kiro-krew` to `kairon`
   - Update all path references from `.kiro-krew/` to `.kairon/`

**Acceptance Criteria**:
- Template config.yaml uses "kairon" label
- Template config.yaml uses `.kairon/` paths
- No "kiro-krew" references in template config

**Validation Commands**:
```bash
grep -q "label: kairon" cmd/kairon/templates/kairon/config.yaml
! grep "kiro-krew" cmd/kairon/templates/kairon/config.yaml || exit 1
```

**Dependencies**: Task 1.1, Task 3.1

---

### Phase 4: Source Code Updates

#### Task 4.1: Update All Go Source Files
**Agent**: builder  
**Description**: Update all string literals, comments, and constants in Go source files that reference "kiro-krew".

**Actions**:
1. Search all `.go` files for "kiro-krew", "kiro_krew", "KIRO_KREW"
2. Update environment variables: `KIRO_KREW_*` → `KAIRON_*`
3. Update string literals for paths: `.kiro-krew/` → `.kairon/`
4. Update binary name references: "kiro-krew" → "kairon"
5. Update comments and documentation strings
6. Key files to update:
   - `cmd/kairon/cmd/*.go` (all command files)
   - `internal/config/*.go`
   - `internal/watcher/*.go`
   - `internal/agent/*.go`
   - `internal/tui/*.go`
   - `internal/session/*.go`
   - `internal/eval/*.go`
   - `internal/incidents/*.go`
   - `internal/hotkey/*.go`
   - `internal/templates/*.go`

**Acceptance Criteria**:
- No Go files contain "kiro-krew" in string literals (except historical context)
- Environment variables use `KAIRON_` prefix
- Path references use `.kairon/`
- Binary name references use "kairon"
- Code compiles without errors

**Validation Commands**:
```bash
go build ./...
! grep -r "KIRO_KREW" --include="*.go" . || exit 1
! grep -r '\.kiro-krew/' --include="*.go" . || exit 1
```

**Dependencies**: Task 1.2

---

#### Task 4.2: Update Test Files
**Agent**: builder  
**Description**: Update all test files with kiro-krew references.

**Actions**:
1. Search all `*_test.go` files for "kiro-krew" references
2. Update test fixtures and mock data
3. Update test assertions for path and binary name expectations
4. Update test comments and documentation

**Acceptance Criteria**:
- Test files use "kairon" references
- Test fixtures use `.kairon/` paths
- All tests pass

**Validation Commands**:
```bash
go test ./...
! grep -r "kiro-krew" --include="*_test.go" . || exit 1
```

**Dependencies**: Task 4.1

---

### Phase 5: Shell Scripts and Tools

#### Task 5.1: Update .kairon/scripts/ Shell Scripts
**Agent**: builder  
**Description**: Update all shell scripts in the .kairon/scripts/ directory.

**Actions**:
1. Update `.kairon/scripts/worktree-create.sh`:
   - Update path references from `.kiro-krew/` to `.kairon/`
   - Update comments
2. Update `.kairon/scripts/worktree-merge.sh`:
   - Update path references
   - Update comments
3. Update `.kairon/scripts/planning-worktree-create.sh`:
   - Update path references
   - Update comments
4. Update `.kairon/scripts/planning-worktree-cleanup.sh`:
   - Update path references
   - Update comments

**Acceptance Criteria**:
- All scripts use `.kairon/` paths
- No "kiro-krew" references in script files
- Scripts maintain their executable permissions

**Validation Commands**:
```bash
! grep "kiro-krew" .kairon/scripts/*.sh || exit 1
test -x .kairon/scripts/worktree-create.sh
```

**Dependencies**: Task 1.1

---

#### Task 5.2: Update scripts/ Directory
**Agent**: builder  
**Description**: Update helper scripts in the scripts/ directory.

**Actions**:
1. Update `scripts/template-sync-summary.sh`:
   - Update path references from `.kiro-krew/` to `.kairon/`
   - Update references from `cmd/kiro-krew/` to `cmd/kairon/`
2. Update `scripts/compare-templates.go`:
   - Update import paths
   - Update path constants

**Acceptance Criteria**:
- Scripts use `.kairon/` and `cmd/kairon/` paths
- No "kiro-krew" references in scripts
- Scripts execute without errors

**Validation Commands**:
```bash
! grep "kiro-krew" scripts/*.sh scripts/*.go || exit 1
```

**Dependencies**: Task 1.1, Task 1.2

---

#### Task 5.3: Update Integration Test Scripts
**Agent**: builder  
**Description**: Update test_integration.sh and validate_integration.sh.

**Actions**:
1. Update `test_integration.sh`:
   - Update binary name from `kiro-krew` to `kairon`
   - Update path references from `.kiro-krew/` to `.kairon/`
   - Update environment variables from `KIRO_KREW_*` to `KAIRON_*`
2. Update `validate_integration.sh`:
   - Update similar references
   - Update test assertions

**Acceptance Criteria**:
- Scripts reference "kairon" binary
- Scripts use `.kairon/` paths
- Scripts use `KAIRON_*` environment variables
- No "kiro-krew" references remain

**Validation Commands**:
```bash
! grep "kiro-krew" test_integration.sh validate_integration.sh || exit 1
grep -q "kairon" test_integration.sh
```

**Dependencies**: Task 1.1

---

### Phase 6: Documentation

#### Task 6.1: Update README.md
**Agent**: builder  
**Description**: Update the main README with comprehensive kairon references.

**Actions**:
1. Update title from "Kiro Krew" to "Kairon"
2. Update all narrative text (e.g., "Kiro Krew watches..." → "Kairon watches...")
3. Update installation URLs from `kiro-krew` to `kairon`
4. Update command examples from `kiro-krew` to `kairon`
5. Update directory references from `.kiro-krew/` to `.kairon/`
6. Update repository references from `jbrinkman/kiro-krew` to `jbrinkman/kairon`
7. Update environment variable examples from `KIRO_KREW_*` to `KAIRON_*`
8. Update REPL prompt examples from `kiro-krew>` to `kairon>`

**Acceptance Criteria**:
- Title is "Kairon"
- All prose uses "Kairon" (capitalized)
- All technical references use "kairon" (lowercase)
- All paths use `.kairon/`
- All URLs reference `kairon`
- No "kiro-krew" references remain

**Validation Commands**:
```bash
grep -q "# Kairon" README.md
! grep "kiro-krew\|Kiro Krew\|Kiro-Krew" README.md || exit 1
grep -q ".kairon/" README.md
```

**Dependencies**: None

---

#### Task 6.2: Update docs/ Directory
**Agent**: builder  
**Description**: Update all documentation files in the docs/ directory.

**Actions**:
1. Update `docs/evaluation.md`:
   - Update project name references
   - Update path references to `.kairon/`
   - Update command examples to use `kairon`
2. Update `docs/unified-container-flow.md`:
   - Update project references
   - Update path references
3. Update `docs/hotkey-toggle.md`:
   - Update UI text references
   - Update command references
4. Update `docs/agent-conventions.md`:
   - Update path references
   - Update directory structure examples
5. Update `docs/eval-framework-analysis.md`:
   - Update project references
   - Update path references
6. Update `docs/performance-investigation.md`:
   - Update project references

**Acceptance Criteria**:
- All docs use "Kairon" for prose
- All docs use "kairon" for technical references
- All docs use `.kairon/` paths
- No "kiro-krew" references remain in docs

**Validation Commands**:
```bash
! grep -r "kiro-krew\|Kiro Krew\|Kiro-Krew" docs/ || exit 1
grep -q "Kairon" docs/evaluation.md
```

**Dependencies**: None

---

#### Task 6.3: Update CHANGELOG.md Header
**Agent**: builder  
**Description**: Update CHANGELOG structure (not historical entries).

**Actions**:
1. Update the header/title to reference "Kairon"
2. Update any structural elements (if present)
3. Do NOT modify historical changelog entries - they reflect historical reality

**Acceptance Criteria**:
- Header references "Kairon"
- Historical entries unchanged
- Structure updated

**Validation Commands**:
```bash
grep -q "Kairon" CHANGELOG.md
```

**Dependencies**: None

---

#### Task 6.4: Update CONTRIBUTING.md
**Agent**: builder  
**Description**: Update contributor documentation.

**Actions**:
1. Update project name references from "Kiro Krew" to "Kairon"
2. Update repository references from `jbrinkman/kiro-krew` to `jbrinkman/kairon`
3. Update command examples from `kiro-krew` to `kairon`
4. Update path references from `.kiro-krew/` to `.kairon/`

**Acceptance Criteria**:
- Uses "Kairon" for project name
- Uses `jbrinkman/kairon` for repository
- Uses `kairon` for commands
- Uses `.kairon/` for paths
- No "kiro-krew" references

**Validation Commands**:
```bash
! grep "kiro-krew\|Kiro Krew" CONTRIBUTING.md || exit 1
grep -q "Kairon" CONTRIBUTING.md
```

**Dependencies**: None

---

### Phase 7: Agent Configuration

#### Task 7.1: Update Agent Prompt Files
**Agent**: builder  
**Description**: Update all agent prompt markdown files in .kiro/agents/.

**Actions**:
1. Update `.kiro/agents/krew-lead-prompt.md`:
   - Update path references from `.kiro-krew/` to `.kairon/`
   - Update environment variable references from `KIRO_KREW_*` to `KAIRON_*`
   - Update command examples from `kiro-krew` to `kairon`
2. Update `.kiro/agents/architect-prompt.md`:
   - Update path references for spec creation
   - Update example paths
3. Update `.kiro/agents/builder-prompt.md`:
   - Update path references
   - Update project references
4. Update `.kiro/agents/validator-prompt.md`:
   - Update path references
   - Update validation examples
5. Update `.kiro/agents/documenter-prompt.md`:
   - Update path references
   - Update documentation examples
6. Update `.kiro/agents/planner-prompt.md`:
   - Update path references
   - Update project references

**Acceptance Criteria**:
- All prompts use `.kairon/` paths
- All prompts use `KAIRON_*` environment variables
- All prompts use "kairon" for commands
- No "kiro-krew" references in agent prompts

**Validation Commands**:
```bash
! grep "kiro-krew\|KIRO_KREW" .kiro/agents/*.md || exit 1
grep -q ".kairon/" .kiro/agents/krew-lead-prompt.md
```

**Dependencies**: Task 1.1

---

### Phase 8: Skills Documentation

#### Task 8.1: Update Skills Documentation
**Agent**: builder  
**Description**: Update all skill documentation files.

**Actions**:
1. Update `.kiro/skills/builder-conventions/SKILL.md`:
   - Update sync mappings table with `.kairon/` paths
   - Update sync commands with new paths
   - Update template paths from `cmd/kiro-krew/` to `cmd/kairon/`
2. Update `.kiro/skills/validator-conventions/SKILL.md`:
   - Update path references
   - Update validation examples
3. Update `.kiro/skills/planner-conventions/SKILL.md`:
   - Update path references
   - Update analysis examples
4. Update `.kiro/skills/discover-qa-tools/SKILL.md`:
   - Update path references for artifact storage
5. Update `.kiro/skills/sentinel-protocol/SKILL.md`:
   - Update path references for sentinel files

**Acceptance Criteria**:
- All skills use `.kairon/` paths
- All skills use `cmd/kairon/` template paths
- Sync mappings table updated
- No "kiro-krew" references in skills

**Validation Commands**:
```bash
! grep "kiro-krew" .kiro/skills/*/SKILL.md || exit 1
grep -q ".kairon/" .kiro/skills/builder-conventions/SKILL.md
grep -q "cmd/kairon/" .kiro/skills/builder-conventions/SKILL.md
```

**Dependencies**: Task 1.1

---

### Phase 9: Embedded Templates Synchronization

#### Task 9.1: Update Agent Template Files
**Agent**: builder  
**Description**: Update embedded agent templates in cmd/kairon/templates/kiro/agents/.

**Actions**:
1. Copy all updated agent prompt files from `.kiro/agents/*.md` to `cmd/kairon/templates/kiro/agents/`
2. Verify all agent JSON configs are in sync
3. Ensure no "kiro-krew" references in template agent files

**Acceptance Criteria**:
- Template agent files match live agent files
- No "kiro-krew" references in template agents
- All agent prompts use `.kairon/` paths

**Validation Commands**:
```bash
diff -r .kiro/agents/ cmd/kairon/templates/kiro/agents/
! grep "kiro-krew" cmd/kairon/templates/kiro/agents/*.md || exit 1
```

**Dependencies**: Task 7.1

---

#### Task 9.2: Update Script Templates
**Agent**: builder  
**Description**: Update embedded script templates in cmd/kairon/templates/kairon/scripts/.

**Actions**:
1. Copy all updated scripts from `.kairon/scripts/*.sh` to `cmd/kairon/templates/kairon/scripts/`
2. Verify no "kiro-krew" references in templates
3. Preserve executable permissions

**Acceptance Criteria**:
- Template scripts match live scripts
- No "kiro-krew" references in template scripts
- Scripts maintain executable permissions

**Validation Commands**:
```bash
diff -r .kairon/scripts/ cmd/kairon/templates/kairon/scripts/
! grep "kiro-krew" cmd/kairon/templates/kairon/scripts/*.sh || exit 1
```

**Dependencies**: Task 5.1

---

#### Task 9.3: Update Evaluation Templates
**Agent**: builder  
**Description**: Update embedded evaluation templates (fixtures, rubrics, cases).

**Actions**:
1. Update `cmd/kairon/templates/kairon/evals/fixtures/codebase-context.md`:
   - Update project references from "Kiro Krew" to "Kairon"
   - Update repository references
   - Update path references
2. Update all rubric files in `cmd/kairon/templates/kairon/evals/rubrics/*.yaml`:
   - Update any project references
   - Update path references
3. Update all case files in `cmd/kairon/templates/kairon/evals/cases/**/*.yaml`:
   - Update repository references
   - Update project name in descriptions
   - Update path references
   - Update command examples

**Acceptance Criteria**:
- All eval templates use "Kairon" for project name
- All eval templates use `jbrinkman/kairon` for repository
- All eval templates use `.kairon/` paths
- No "kiro-krew" references in eval templates

**Validation Commands**:
```bash
! grep "kiro-krew\|Kiro Krew" cmd/kairon/templates/kairon/evals/fixtures/*.md || exit 1
! grep "kiro-krew" cmd/kairon/templates/kairon/evals/rubrics/*.yaml || exit 1
! grep "kiro-krew\|jbrinkman/kiro-krew" cmd/kairon/templates/kairon/evals/cases/**/*.yaml || exit 1
```

**Dependencies**: None (templates only, not live eval data)

---

#### Task 9.4: Update Skills Template
**Agent**: builder  
**Description**: Update embedded sentinel-protocol skill template.

**Actions**:
1. Update `cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md`:
   - Update path references from `.kiro-krew/` to `.kairon/`
   - Update examples

**Acceptance Criteria**:
- Sentinel protocol template uses `.kairon/` paths
- No "kiro-krew" references in template

**Validation Commands**:
```bash
! grep "kiro-krew" cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md || exit 1
grep -q ".kairon/" cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md
```

**Dependencies**: Task 8.1

---

### Phase 10: Validation and Verification

#### Task 10.1: Comprehensive Build and Test Validation
**Agent**: validator  
**Description**: Execute comprehensive validation to ensure all changes are correct and the system builds and tests successfully.

**Validation Steps**:
1. Run `go mod tidy` to verify module integrity
2. Run `task build` to verify build succeeds
3. Run `task test` to verify all tests pass
4. Run `task fmt:check` to verify code formatting
5. Run `task sync:check` to verify template synchronization
6. Run `task lint` to verify code quality
7. Verify no "kiro-krew" references remain in source code (excluding historical artifacts)
8. Verify directory structure is correct
9. Verify binary name is "kairon"
10. Verify go.mod module path is correct

**Acceptance Criteria**:
- `go mod tidy` runs without errors
- `task build` produces `dist/kairon` binary
- `task test` passes all tests
- `task fmt:check` passes
- `task sync:check` passes
- `task lint` passes
- No "kiro-krew" in source files (excluding historical paths)
- Directory `.kairon/` exists
- Directory `cmd/kairon/` exists
- Module path is `github.com/jbrinkman/kairon`

**Validation Commands**:
```bash
go mod tidy
task build
task test
task fmt:check
task sync:check
task lint
test -f dist/kairon
test -d .kairon
test -d cmd/kairon
grep -q "module github.com/jbrinkman/kairon" go.mod
```

**Dependencies**: All previous tasks

---

## Validation Commands

Final verification commands to run after all tasks are complete:

```bash
# Module and build verification
go mod tidy
task build
task test
task fmt:check
task sync:check
task lint

# Directory structure verification
test -d .kairon
test -d cmd/kairon
test -d cmd/kairon/templates/kairon
test ! -d .kiro-krew
test ! -d cmd/kiro-krew

# Binary verification
test -f dist/kairon

# Module path verification
grep -q "module github.com/jbrinkman/kairon" go.mod

# String reference verification (should find no matches except in historical artifacts)
! find . -type f \( -name "*.go" -o -name "*.md" -o -name "*.yaml" -o -name "*.yml" -o -name "*.json" -o -name "*.sh" \) \
  -not -path "./.git/*" \
  -not -path "./.kairon/specs/*" \
  -not -path "./.kairon/logs/*" \
  -not -path "./.kairon/retries/*" \
  -not -path "./.kairon/artifacts/*" \
  -not -path "./.kairon/evals/results/*" \
  -exec grep -l "kiro-krew" {} \; | grep -q .

# Historical artifacts preserved
test -d .kairon/specs || echo "specs may not exist yet"
test -d .kairon/logs || echo "logs may not exist yet"
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "rename-directories"
    agent: "builder"
    description: "Rename directory structures from kiro-krew to kairon (cmd/kiro-krew/ → cmd/kairon/, .kiro-krew/ → .kairon/, templates/kiro-krew/ → templates/kairon/)"
    dependencies: []
    acceptance_criteria:
      - "Directory cmd/kairon/ exists"
      - "Directory .kairon/ exists"
      - "Directory cmd/kairon/templates/kairon/ exists"
      - "No cmd/kiro-krew/ directory remains"
      - "Historical artifact directories preserved in .kairon/"
    validation_commands:
      - "test -d cmd/kairon"
      - "test -d .kairon"
      - "test -d cmd/kairon/templates/kairon"
      - "test ! -d cmd/kiro-krew"

  - id: "update-go-module"
    agent: "builder"
    description: "Update Go module path from github.com/jbrinkman/kiro-krew to github.com/jbrinkman/kairon and update all import statements"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "go.mod declares module github.com/jbrinkman/kairon"
      - "No Go files contain imports of github.com/jbrinkman/kiro-krew"
      - "go mod tidy runs without errors"
      - "All Go files compile without import errors"
    validation_commands:
      - "grep -q 'module github.com/jbrinkman/kairon' go.mod"
      - "go mod tidy"
      - "go build ./..."

  - id: "update-taskfile"
    agent: "builder"
    description: "Update Taskfile.yml configuration: BINARY_NAME to kairon, ldflags module path, build paths, sync:check paths"
    dependencies: ["rename-directories", "update-go-module"]
    acceptance_criteria:
      - "BINARY_NAME variable set to kairon"
      - "All ldflags reference github.com/jbrinkman/kairon"
      - "Build paths point to ./cmd/kairon"
      - "Sync check paths reference .kairon/"
      - "No kiro-krew references in Taskfile.yml"
    validation_commands:
      - "grep -q 'BINARY_NAME: kairon' Taskfile.yml"
      - "grep -q 'github.com/jbrinkman/kairon/internal/version' Taskfile.yml"

  - id: "update-npm-config"
    agent: "builder"
    description: "Update package.json and .releaserc.json with kairon references"
    dependencies: []
    acceptance_criteria:
      - "package.json has name kairon"
      - "package.json repository URL is https://github.com/jbrinkman/kairon.git"
      - ".releaserc.json asset paths reference dist/release/kairon"
      - "No kiro-krew references in package.json or .releaserc.json"
    validation_commands:
      - "grep -q '\"name\": \"kairon\"' package.json"
      - "grep -q 'jbrinkman/kairon' package.json"
      - "grep -q 'dist/release/kairon' .releaserc.json"

  - id: "update-github-workflows"
    agent: "builder"
    description: "Update GitHub Actions workflows to reference kairon binaries and assets"
    dependencies: ["update-taskfile"]
    acceptance_criteria:
      - ".github/workflows/release.yml references dist/release/kairon binaries"
      - "Release asset labels reference kairon"
      - "No kiro-krew references in workflow files"
    validation_commands:
      - "grep -q 'kairon' .github/workflows/release.yml"

  - id: "update-config-yaml"
    agent: "builder"
    description: "Update .kairon/config.yaml with new repository name, label, and paths"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "repo field is jbrinkman/kairon"
      - "label field is kairon"
      - "sessions_dir is .kairon/sessions"
      - "log_dir is .kairon/logs"
      - "No kiro-krew references in config.yaml"
    validation_commands:
      - "grep -q 'repo: jbrinkman/kairon' .kairon/config.yaml"
      - "grep -q 'label: kairon' .kairon/config.yaml"

  - id: "update-template-config"
    agent: "builder"
    description: "Update embedded template config.yaml in cmd/kairon/templates/kairon/"
    dependencies: ["rename-directories", "update-config-yaml"]
    acceptance_criteria:
      - "Template config.yaml uses kairon label"
      - "Template config.yaml uses .kairon/ paths"
      - "No kiro-krew references in template config"
    validation_commands:
      - "grep -q 'label: kairon' cmd/kairon/templates/kairon/config.yaml"

  - id: "update-go-source"
    agent: "builder"
    description: "Update all Go source files: string literals, comments, environment variables (KIRO_KREW_* → KAIRON_*), paths (.kiro-krew/ → .kairon/), binary name references"
    dependencies: ["update-go-module"]
    acceptance_criteria:
      - "No Go files contain kiro-krew in string literals"
      - "Environment variables use KAIRON_ prefix"
      - "Path references use .kairon/"
      - "Binary name references use kairon"
      - "Code compiles without errors"
    validation_commands:
      - "go build ./..."

  - id: "update-test-files"
    agent: "builder"
    description: "Update all test files with kairon references, test fixtures, and assertions"
    dependencies: ["update-go-source"]
    acceptance_criteria:
      - "Test files use kairon references"
      - "Test fixtures use .kairon/ paths"
      - "All tests pass"
    validation_commands:
      - "go test ./..."

  - id: "update-kairon-scripts"
    agent: "builder"
    description: "Update shell scripts in .kairon/scripts/ directory with new path references"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "All scripts use .kairon/ paths"
      - "No kiro-krew references in script files"
      - "Scripts maintain executable permissions"
    validation_commands:
      - "test -x .kairon/scripts/worktree-create.sh"

  - id: "update-helper-scripts"
    agent: "builder"
    description: "Update helper scripts in scripts/ directory (template-sync-summary.sh, compare-templates.go)"
    dependencies: ["rename-directories", "update-go-module"]
    acceptance_criteria:
      - "Scripts use .kairon/ and cmd/kairon/ paths"
      - "No kiro-krew references in scripts"
      - "Scripts execute without errors"
    validation_commands:
      - "bash -n scripts/template-sync-summary.sh"

  - id: "update-integration-tests"
    agent: "builder"
    description: "Update test_integration.sh and validate_integration.sh with kairon references"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "Scripts reference kairon binary"
      - "Scripts use .kairon/ paths"
      - "Scripts use KAIRON_* environment variables"
      - "No kiro-krew references remain"
    validation_commands:
      - "grep -q 'kairon' test_integration.sh"

  - id: "update-readme"
    agent: "builder"
    description: "Update README.md with comprehensive kairon references (title, prose, commands, paths, URLs)"
    dependencies: []
    acceptance_criteria:
      - "Title is Kairon"
      - "All prose uses Kairon (capitalized)"
      - "All technical references use kairon (lowercase)"
      - "All paths use .kairon/"
      - "All URLs reference kairon"
      - "No kiro-krew references remain"
    validation_commands:
      - "grep -q '# Kairon' README.md"
      - "grep -q '.kairon/' README.md"

  - id: "update-docs"
    agent: "builder"
    description: "Update all documentation files in docs/ directory with kairon references"
    dependencies: []
    acceptance_criteria:
      - "All docs use Kairon for prose"
      - "All docs use kairon for technical references"
      - "All docs use .kairon/ paths"
      - "No kiro-krew references remain in docs"
    validation_commands:
      - "grep -q 'Kairon' docs/evaluation.md"

  - id: "update-changelog"
    agent: "builder"
    description: "Update CHANGELOG.md header (structure only, not historical entries)"
    dependencies: []
    acceptance_criteria:
      - "Header references Kairon"
      - "Historical entries unchanged"
    validation_commands:
      - "grep -q 'Kairon' CHANGELOG.md"

  - id: "update-contributing"
    agent: "builder"
    description: "Update CONTRIBUTING.md with kairon references"
    dependencies: []
    acceptance_criteria:
      - "Uses Kairon for project name"
      - "Uses jbrinkman/kairon for repository"
      - "Uses kairon for commands"
      - "Uses .kairon/ for paths"
      - "No kiro-krew references"
    validation_commands:
      - "grep -q 'Kairon' CONTRIBUTING.md"

  - id: "update-agent-prompts"
    agent: "builder"
    description: "Update all agent prompt markdown files in .kiro/agents/ with .kairon/ paths, KAIRON_* env vars, and kairon commands"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "All prompts use .kairon/ paths"
      - "All prompts use KAIRON_* environment variables"
      - "All prompts use kairon for commands"
      - "No kiro-krew references in agent prompts"
    validation_commands:
      - "grep -q '.kairon/' .kiro/agents/krew-lead-prompt.md"

  - id: "update-skills"
    agent: "builder"
    description: "Update all skill documentation files with .kairon/ paths and cmd/kairon/ template paths"
    dependencies: ["rename-directories"]
    acceptance_criteria:
      - "All skills use .kairon/ paths"
      - "All skills use cmd/kairon/ template paths"
      - "Sync mappings table updated in builder-conventions"
      - "No kiro-krew references in skills"
    validation_commands:
      - "grep -q '.kairon/' .kiro/skills/builder-conventions/SKILL.md"
      - "grep -q 'cmd/kairon/' .kiro/skills/builder-conventions/SKILL.md"

  - id: "sync-agent-templates"
    agent: "builder"
    description: "Copy updated agent files from .kiro/agents/ to cmd/kairon/templates/kiro/agents/"
    dependencies: ["update-agent-prompts"]
    acceptance_criteria:
      - "Template agent files match live agent files"
      - "No kiro-krew references in template agents"
      - "All agent prompts use .kairon/ paths"
    validation_commands:
      - "diff -r .kiro/agents/ cmd/kairon/templates/kiro/agents/"

  - id: "sync-script-templates"
    agent: "builder"
    description: "Copy updated scripts from .kairon/scripts/ to cmd/kairon/templates/kairon/scripts/"
    dependencies: ["update-kairon-scripts"]
    acceptance_criteria:
      - "Template scripts match live scripts"
      - "No kiro-krew references in template scripts"
      - "Scripts maintain executable permissions"
    validation_commands:
      - "diff -r .kairon/scripts/ cmd/kairon/templates/kairon/scripts/"

  - id: "sync-eval-templates"
    agent: "builder"
    description: "Update embedded evaluation templates (fixtures, rubrics, cases) with kairon references"
    dependencies: []
    acceptance_criteria:
      - "All eval templates use Kairon for project name"
      - "All eval templates use jbrinkman/kairon for repository"
      - "All eval templates use .kairon/ paths"
      - "No kiro-krew references in eval templates"
    validation_commands:
      - "grep -q 'Kairon' cmd/kairon/templates/kairon/evals/fixtures/codebase-context.md"

  - id: "sync-skills-template"
    agent: "builder"
    description: "Update embedded sentinel-protocol skill template with .kairon/ paths"
    dependencies: ["update-skills"]
    acceptance_criteria:
      - "Sentinel protocol template uses .kairon/ paths"
      - "No kiro-krew references in template"
    validation_commands:
      - "grep -q '.kairon/' cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md"

  - id: "validate-complete"
    agent: "validator"
    description: "Execute comprehensive validation: build, tests, formatting, template sync, linting, directory structure, module path"
    dependencies: [
      "rename-directories",
      "update-go-module",
      "update-taskfile",
      "update-npm-config",
      "update-github-workflows",
      "update-config-yaml",
      "update-template-config",
      "update-go-source",
      "update-test-files",
      "update-kairon-scripts",
      "update-helper-scripts",
      "update-integration-tests",
      "update-readme",
      "update-docs",
      "update-changelog",
      "update-contributing",
      "update-agent-prompts",
      "update-skills",
      "sync-agent-templates",
      "sync-script-templates",
      "sync-eval-templates",
      "sync-skills-template"
    ]
    acceptance_criteria:
      - "go mod tidy runs without errors"
      - "task build produces dist/kairon binary"
      - "task test passes all tests"
      - "task fmt:check passes"
      - "task sync:check passes"
      - "task lint passes"
      - "No kiro-krew in source files (excluding historical paths)"
      - "Directory .kairon/ exists"
      - "Directory cmd/kairon/ exists"
      - "Module path is github.com/jbrinkman/kairon"
    validation_commands:
      - "go mod tidy"
      - "task build"
      - "task test"
      - "task fmt:check"
      - "task sync:check"
      - "task lint"
      - "test -f dist/kairon"
      - "test -d .kairon"
      - "test -d cmd/kairon"
      - "grep -q 'module github.com/jbrinkman/kairon' go.mod"
```

## Risk Mitigation

### High-Risk Areas

1. **Go Module Path Changes**: Breaking imports across 80+ Go files
   - Mitigation: Sequential execution with build verification after each phase
   - Rollback: Git revert if compilation fails

2. **Template Synchronization**: Must maintain parity between live and embedded templates
   - Mitigation: Explicit sync tasks with diff-based validation
   - Verification: `task sync:check` enforces parity

3. **Historical Artifact Preservation**: Must not modify historical files
   - Mitigation: Explicit exclusion patterns in all file operations
   - Verification: Manual spot-check of preserved directories

4. **CI/CD Pipeline**: Release workflows must reference correct binary names
   - Mitigation: Update workflows before merging
   - Verification: Test workflow file syntax before commit

### Testing Strategy

1. **Build Verification**: Run `task build` after each major phase
2. **Test Suite**: Run `task test` to verify no behavioral regressions
3. **Template Sync**: Run `task sync:check` to verify template parity
4. **Integration Tests**: Run integration test scripts with new binary name
5. **Manual Verification**: Spot-check critical files and directories

## Success Criteria

The rename is complete when:

1. ✅ All 126+ files have been updated with kairon references
2. ✅ Directory structure uses `.kairon/` and `cmd/kairon/`
3. ✅ Go module path is `github.com/jbrinkman/kairon`
4. ✅ Binary builds as `dist/kairon`
5. ✅ All tests pass (`task test`)
6. ✅ Build succeeds (`task build`)
7. ✅ Template sync verified (`task sync:check`)
8. ✅ Code formatting valid (`task fmt:check`)
9. ✅ Linting passes (`task lint`)
10. ✅ Historical artifacts preserved and unchanged
11. ✅ No "kiro-krew" references remain in source code (excluding historical paths)
12. ✅ CI/CD workflows reference correct binary and asset names
