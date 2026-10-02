# Design Specification: Taskfile for Standardized Build Operations

**Issue**: #89 - Add Taskfile for standardized build and test operations  
**Closes**: #89

## Solution Approach

Implement a comprehensive Taskfile.yml using the Task build tool (https://taskfile.dev) to standardize all build, test, and development operations. The Taskfile will provide a consistent interface for both human developers and AI agents, integrating seamlessly with the existing JSON version metadata system from Issue #88.

The approach follows Go project conventions while providing cross-platform compatibility and atomic version metadata updates during builds.

## Architecture Overview

```
Taskfile.yml (build orchestration)
    ├── build task → go build + version metadata update
    ├── test task → go test with coverage
    ├── lint/fmt tasks → code quality
    └── dev task → development build + watch mode

.kiro/build-instructions.md → directs agents to use Task commands
```

## Relevant Files

### Files to Create
- `Taskfile.yml` - Main Task build configuration in project root
- `.kiro/build-instructions.md` - Agent guidance for using Task commands
- `scripts/update-version.sh` - Script for atomic version metadata updates

### Files to Modify
- `README.md` - Update build instructions to reference Taskfile
- `internal/version/version.go` - Ensure compatibility with build-time metadata

### Files Referenced (Integration Points)
- `internal/version/version.json` - Updated by build task with commit hash and timestamp
- `go.mod` - Referenced by Task for Go module operations
- `test_integration.sh` - May be enhanced to use Task commands
- `.kiro/agents/builder-prompt.md` - Will reference build instructions

## Team Orchestration

**Builder Agent Tasks:**
1. Create Taskfile.yml with all required tasks
2. Create build instructions for AI agents
3. Create version update script
4. Update README documentation
5. Verify all tasks work cross-platform

**Validator Agent Tasks:**
1. Verify all Task commands execute successfully
2. Test cross-platform compatibility
3. Validate version metadata updates are atomic
4. Confirm integration with existing workflows

## Step-by-Step Task Breakdown

### Task 1: Create Core Taskfile.yml
**Acceptance Criteria:**
- Create `Taskfile.yml` in project root with Task v3 schema
- Include `build`, `test`, `clean`, `dev`, `lint`, `fmt`, `version:update` tasks
- Build task must update version metadata with git commit hash and timestamp
- All tasks must work on Linux, macOS, and Windows
- Tasks follow Go project conventions (use `go build`, `go test`, etc.)
- Include task descriptions for documentation

### Task 2: Implement Version Metadata Integration
**Acceptance Criteria:**
- Build task updates `internal/version/version.json` atomically
- Add current git commit hash and build timestamp to JSON
- Preserve existing version and prerelease fields
- Handle cases where git is not available (fallback gracefully)
- Version updates must not leave corrupted JSON files
- Create temporary file and atomic rename for safety

### Task 3: Create Agent Build Instructions
**Acceptance Criteria:**
- Create `.kiro/build-instructions.md` with clear Task command guidance
- Instruct agents to use `task build` instead of `go build`
- Document all available tasks and their purposes
- Provide troubleshooting guidance for common issues
- Include cross-platform command examples

### Task 4: Add Development and Quality Tasks
**Acceptance Criteria:**
- `dev` task for development builds (faster, no optimization)
- `lint` task using `go vet` and other Go linters if available
- `fmt` task for code formatting with `go fmt`
- `clean` task to remove build artifacts and temporary files
- `version:update` task for manual version number updates
- Tasks should be composable (e.g., `dev` may skip linting)

### Task 5: Update Documentation
**Acceptance Criteria:**
- Update README.md with Taskfile installation and usage instructions
- Add "Available Tasks" section listing all tasks with descriptions
- Replace manual build instructions with Task command references
- Ensure documentation works for both developers and CI/CD systems
- Maintain backward compatibility notes for existing workflows

## Implementation Details

### Taskfile.yml Structure
```yaml
version: '3'

vars:
  BINARY_NAME: kiro-krew
  BUILD_DIR: ./dist
  VERSION_FILE: internal/version/version.json

tasks:
  build:
    desc: Build the application with version metadata
    deps: [update-build-metadata]
    cmds:
      - mkdir -p {{.BUILD_DIR}}
      - go build -ldflags "-X github.com/jbrinkman/kiro-krew/internal/version.BuildDate={{.BUILD_TIME}}" -o {{.BUILD_DIR}}/{{.BINARY_NAME}} ./cmd/kiro-krew
    vars:
      BUILD_TIME:
        sh: date -u +%Y-%m-%dT%H:%M:%SZ

  update-build-metadata:
    desc: Update version metadata with git info
    cmds:
      - task: _update-version-json
    internal: true

  _update-version-json:
    desc: Atomically update version.json with build metadata
    cmds:
      - |
        COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        
        # Create temporary file with updated metadata
        jq --arg commit "$COMMIT_HASH" --arg timestamp "$BUILD_TIME" \
           '. + {"commit_hash": $commit, "build_timestamp": $timestamp}' \
           {{.VERSION_FILE}} > {{.VERSION_FILE}}.tmp
        
        # Atomic rename
        mv {{.VERSION_FILE}}.tmp {{.VERSION_FILE}}
    internal: true

  test:
    desc: Run all tests with coverage
    cmds:
      - go test -v -race -coverprofile=coverage.out ./...
      - go tool cover -html=coverage.out -o coverage.html

  dev:
    desc: Development build (faster, no optimization)
    deps: [update-build-metadata]
    cmds:
      - go build -o {{.BINARY_NAME}} ./cmd/kiro-krew

  lint:
    desc: Run linters and static analysis
    cmds:
      - go vet ./...
      - go fmt ./...

  fmt:
    desc: Format code
    cmds:
      - go fmt ./...

  clean:
    desc: Clean build artifacts
    cmds:
      - rm -rf {{.BUILD_DIR}}
      - rm -f {{.BINARY_NAME}}
      - rm -f coverage.out coverage.html

  version:update:
    desc: Update version number in version.json
    prompt: This will update the version number. Continue?
    cmds:
      - |
        echo "Current version: $(jq -r '.version' {{.VERSION_FILE}})"
        read -p "Enter new version: " NEW_VERSION
        jq --arg version "$NEW_VERSION" '.version = $version' {{.VERSION_FILE}} > {{.VERSION_FILE}}.tmp
        mv {{.VERSION_FILE}}.tmp {{.VERSION_FILE}}
        echo "Updated to version: $NEW_VERSION"
```

### Agent Build Instructions (.kiro/build-instructions.md)
```markdown
# Build Instructions for AI Agents

## Overview
This project uses Task (https://taskfile.dev) for standardized build operations. Use Task commands instead of direct `go build` or `go test`.

## Installation
Install Task if not available:
```bash
# macOS
brew install go-task/tap/go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Windows
choco install go-task
```

## Available Commands

- `task build` - Build production binary with version metadata
- `task dev` - Quick development build
- `task test` - Run all tests with coverage
- `task lint` - Run linters and formatting
- `task clean` - Remove build artifacts
- `task version:update` - Update version number

## Key Differences from Direct Go Commands

1. **Version Metadata**: `task build` automatically updates version.json with git commit and timestamp
2. **Atomic Updates**: Version file updates are atomic (no corrupted files)
3. **Cross-Platform**: All tasks work on Linux, macOS, and Windows
4. **Consistent Interface**: Same commands work for humans and CI/CD

## Troubleshooting

- If Task is not available, fall back to: `go build ./cmd/kiro-krew`
- Version metadata updates require git; will fallback gracefully
- All tasks respect existing go.mod configuration
```

### Version Metadata Updates
The build system will extend the existing version.json structure:
```json
{
  "version": "0.5.0",
  "prerelease": "",
  "commit_hash": "abc123f",
  "build_timestamp": "2024-01-15T14:30:00Z"
}
```

## Validation Commands

### Verify Task Installation and Tasks
```bash
# Install Task (if needed)
task --version

# List all available tasks
task --list

# Test each task
task clean
task build
task test
task dev
task lint
task fmt
```

### Verify Version Metadata Integration
```bash
# Build and check version metadata
task build
cat internal/version/version.json
# Should show updated commit_hash and build_timestamp

# Verify binary includes metadata
./dist/kiro-krew about
# Should show git commit and build time
```

### Verify Cross-Platform Compatibility
```bash
# Test on different platforms
task build  # Should work on Linux, macOS, Windows
task test   # Cross-platform test execution
task clean  # Cross-platform cleanup
```

### Verify Atomic Version Updates
```bash
# Test atomic updates don't leave corrupt files
task build &
task build &
wait
# version.json should be valid JSON (not corrupted)
jq . internal/version/version.json
```

### Verify Agent Integration
```bash
# Test that agents can use Task commands
cd .kiro
ls -la build-instructions.md
# Should exist and contain Task guidance
```

## Risk Mitigation

### Version Metadata Safety
- Use temporary files and atomic rename for version.json updates
- Validate JSON structure before committing changes
- Graceful fallback when git is unavailable
- Preserve existing version/prerelease fields

### Cross-Platform Compatibility
- Use Task's built-in cross-platform support
- Avoid shell-specific commands (prefer Task templates)
- Test date commands work on all platforms
- Handle Windows path separators correctly

### Backward Compatibility
- Existing `go build` commands continue to work
- No breaking changes to version.go interface
- Taskfile is additive (doesn't replace existing workflows)
- Clear migration path in documentation

## Dependencies

### Required Dependencies
- **Task**: Build tool (https://taskfile.dev)
- **jq**: JSON processing (for version metadata updates)
- **git**: For commit hash (graceful fallback if missing)

### Integration with Issue #88
- Builds on existing JSON version metadata system
- Preserves version.json structure and parsing
- Adds build metadata without breaking existing fields
- Maintains embed-based approach for runtime access

## Future Extensibility

### Planned Enhancements
- Integration with CI/CD pipelines (GitHub Actions)
- Release task for automated releases
- Docker build tasks
- Integration testing task with environment setup
- Performance benchmarking tasks

### Extensible Task Structure
```yaml
# Future tasks can be added easily
release:
  desc: Create and tag a release
  deps: [build, test]
  cmds:
    - task: version:update
    - git tag v{{.VERSION}}
    - gh release create v{{.VERSION}}

docker:
  desc: Build Docker image
  deps: [build]
  cmds:
    - docker build -t kiro-krew:{{.VERSION}} .
```

This design establishes a robust, extensible build system that provides the foundation for future automation while maintaining simplicity and developer experience.
