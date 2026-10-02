# Design Specification: GitHub Actions CI/CD with Taskfile-only Commands

**Issue**: #90 - Add GitHub Actions for CI/CD with Taskfile-only commands  
**Closes**: #90

## Solution Approach

Implement comprehensive GitHub Actions workflows that exclusively use Taskfile commands for all operations. This ensures consistency between local development and CI/CD environments while leveraging the standardized build system from Issue #89 and the JSON version metadata from Issue #88.

The architecture follows GitHub Actions best practices with three distinct workflows: PR validation, post-merge version updates, and semantic releases. All workflows are secured and use Taskfile as the single source of truth for operations.

## Architecture Overview

```
GitHub Events → Workflows → Taskfile Commands → Go Operations
    ├── Pull Request → ci.yml → task fmt/lint/test/build
    ├── Push to main → update-version.yml → task version:update-commit  
    └── Manual trigger → release.yml → task version:set + task build:release
```

## Relevant Files

### Files to Create
- `.github/workflows/ci.yml` - PR validation workflow
- `.github/workflows/update-version.yml` - Post-merge version metadata updates
- `.github/workflows/release.yml` - Semantic release workflow

### Files to Modify
- `Taskfile.yml` - Add missing CI/CD tasks: `version:update-commit`, `version:set`, `build:release`

### Files Referenced (Integration Points)
- `internal/version/version.json` - Updated by workflows via Taskfile tasks
- `go.mod` - Determines Go version for workflow setup
- `.kiro/build-instructions.md` - Referenced for Taskfile usage patterns

## Team Orchestration

**Builder Agent Tasks:**
1. Create all three GitHub Actions workflow files
2. Add missing Taskfile tasks for CI/CD operations
3. Ensure cross-platform build support in release workflow
4. Configure secure GitHub token usage

**Validator Agent Tasks:**
1. Verify workflows validate correctly with GitHub Actions syntax
2. Test that all Taskfile commands work in CI environment
3. Validate security practices (no secret exposure in logs)
4. Confirm multi-platform builds work correctly

## Step-by-Step Task Breakdown

### Task 1: Create PR Validation Workflow (ci.yml)
**Acceptance Criteria:**
- Create `.github/workflows/ci.yml` triggered on all PRs
- Use Go 1.25.0 as specified in go.mod
- Install Task runner before any operations
- Run `task fmt`, `task lint`, `task test`, `task build` in sequence
- Block PR merge if any task fails
- Cache Go modules for performance
- No direct Go commands - only Taskfile tasks

### Task 2: Create Post-Merge Version Update Workflow (update-version.yml)
**Acceptance Criteria:**
- Create `.github/workflows/update-version.yml` triggered on main branch pushes
- Install Task and run `task version:update-commit` 
- Commit updated version metadata back to main branch
- Use `github-actions` bot for automated commits
- Include proper commit message with conventional commit format
- Ensure atomic operation (no race conditions)

### Task 3: Create Semantic Release Workflow (release.yml)
**Acceptance Criteria:**
- Create `.github/workflows/release.yml` with manual trigger (workflow_dispatch)
- Analyze commit history using conventional commits for version determination
- Run `task version:set` with calculated next version
- Run `task build:release` for multi-platform builds (Linux amd64, macOS arm64, Windows amd64)
- Create GitHub release with generated notes and attached binaries
- Tag release with semantic version
- Use semantic-release or similar tool for version calculation

### Task 4: Add Missing Taskfile Tasks
**Acceptance Criteria:**
- Add `version:update-commit` task to update metadata with commit hash and timestamp
- Add `version:set` task to set new version number from parameter
- Add `build:release` task for cross-platform builds with optimizations
- Tasks must handle jq operations for JSON manipulation
- Include proper error handling and atomic file operations
- All tasks work on GitHub Actions runners (Ubuntu, macOS, Windows)

### Task 5: Implement Security and Best Practices
**Acceptance Criteria:**
- Use minimal GitHub token permissions (contents: write for commits/releases)
- No sensitive data in workflow logs
- Pin action versions to specific commits for security
- Use official GitHub Actions from verified publishers
- Include timeout settings to prevent runaway workflows
- Implement proper error handling and failure notifications

## Implementation Details

### CI Workflow (.github/workflows/ci.yml)
```yaml
name: CI

on:
  pull_request:
    branches: [main]

jobs:
  validate:
    name: Validate PR
    runs-on: ubuntu-latest
    timeout-minutes: 10
    
    steps:
    - name: Checkout code
      uses: actions/checkout@v4
      
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.25.0'
        cache: true
        
    - name: Install Task
      uses: arduino/setup-task@v2
      with:
        version: 3.x
        
    - name: Format check
      run: task fmt
      
    - name: Lint
      run: task lint
      
    - name: Test
      run: task test
      
    - name: Build
      run: task build
```

### Version Update Workflow (.github/workflows/update-version.yml)
```yaml
name: Update Version Metadata

on:
  push:
    branches: [main]

jobs:
  update-version:
    name: Update Version Metadata
    runs-on: ubuntu-latest
    timeout-minutes: 5
    
    steps:
    - name: Checkout code
      uses: actions/checkout@v4
      with:
        token: ${{ secrets.GITHUB_TOKEN }}
        
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.25.0'
        
    - name: Install Task
      uses: arduino/setup-task@v2
      with:
        version: 3.x
        
    - name: Update version metadata
      run: task version:update-commit
      
    - name: Commit updated metadata
      run: |
        git config --local user.email "github-actions[bot]@users.noreply.github.com"
        git config --local user.name "github-actions[bot]"
        
        if git diff --quiet internal/version/version.json; then
          echo "No changes to commit"
        else
          git add internal/version/version.json
          git commit -m "chore: update version metadata [skip ci]"
          git push
        fi
```

### Release Workflow (.github/workflows/release.yml)
```yaml
name: Release

on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Version to release (leave empty for auto-detection)'
        required: false
        default: ''

jobs:
  release:
    name: Create Release
    runs-on: ubuntu-latest
    timeout-minutes: 15
    
    steps:
    - name: Checkout code
      uses: actions/checkout@v4
      with:
        fetch-depth: 0
        token: ${{ secrets.GITHUB_TOKEN }}
        
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.25.0'
        
    - name: Install Task
      uses: arduino/setup-task@v2
      with:
        version: 3.x
        
    - name: Determine next version
      id: version
      run: |
        if [ -n "${{ github.event.inputs.version }}" ]; then
          echo "version=${{ github.event.inputs.version }}" >> $GITHUB_OUTPUT
        else
          # Auto-detect version based on conventional commits
          CURRENT_VERSION=$(jq -r '.version' internal/version/version.json)
          # Simple increment for now - can be enhanced with semantic-release
          NEXT_VERSION=$(echo $CURRENT_VERSION | awk -F. '{$NF = $NF + 1;} 1' | sed 's/ /./g')
          echo "version=$NEXT_VERSION" >> $GITHUB_OUTPUT
        fi
        
    - name: Set version
      run: task version:set VERSION=${{ steps.version.outputs.version }}
      
    - name: Build release artifacts
      run: task build:release
      
    - name: Create release
      uses: actions/create-release@v1
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      with:
        tag_name: v${{ steps.version.outputs.version }}
        release_name: Release v${{ steps.version.outputs.version }}
        body: |
          Release v${{ steps.version.outputs.version }}
          
          ## Changes
          Auto-generated release from GitHub Actions.
        draft: false
        prerelease: false
        
    - name: Upload release assets
      uses: actions/upload-release-asset@v1
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      with:
        upload_url: ${{ steps.create_release.outputs.upload_url }}
        asset_path: ./dist/
        asset_name: kiro-krew-binaries
        asset_content_type: application/zip
```

### Required Taskfile Tasks

Add to `Taskfile.yml`:

```yaml
  version:update-commit:
    desc: Update version metadata with current commit info
    cmds:
      - |
        COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        
        jq --arg commit "$COMMIT_HASH" --arg timestamp "$BUILD_TIME" \
           '. + {"commit_hash": $commit, "build_timestamp": $timestamp}' \
           {{.VERSION_FILE}} > {{.VERSION_FILE}}.tmp
        mv {{.VERSION_FILE}}.tmp {{.VERSION_FILE}}

  version:set:
    desc: Set new version number
    cmds:
      - |
        VERSION="${VERSION:-{{.CLI_ARGS}}}"
        if [ -z "$VERSION" ]; then
          echo "Error: VERSION parameter required"
          exit 1
        fi
        
        jq --arg version "$VERSION" '.version = $version' {{.VERSION_FILE}} > {{.VERSION_FILE}}.tmp
        mv {{.VERSION_FILE}}.tmp {{.VERSION_FILE}}
        echo "Updated to version: $VERSION"

  build:release:
    desc: Build release artifacts for multiple platforms
    cmds:
      - mkdir -p {{.BUILD_DIR}}/release
      - task: build:linux
      - task: build:macos
      - task: build:windows
      
  build:linux:
    desc: Build for Linux amd64
    cmds:
      - GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X github.com/jbrinkman/kiro-krew/internal/version.BuildDate={{.BUILD_TIME}}" -o {{.BUILD_DIR}}/release/kiro-krew-linux-amd64 ./cmd/kiro-krew
    vars:
      BUILD_TIME:
        sh: date -u +%Y-%m-%dT%H:%M:%SZ
        
  build:macos:
    desc: Build for macOS arm64
    cmds:
      - GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X github.com/jbrinkman/kiro-krew/internal/version.BuildDate={{.BUILD_TIME}}" -o {{.BUILD_DIR}}/release/kiro-krew-macos-arm64 ./cmd/kiro-krew
    vars:
      BUILD_TIME:
        sh: date -u +%Y-%m-%dT%H:%M:%SZ
        
  build:windows:
    desc: Build for Windows amd64
    cmds:
      - GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X github.com/jbrinkman/kiro-krew/internal/version.BuildDate={{.BUILD_TIME}}" -o {{.BUILD_DIR}}/release/kiro-krew-windows-amd64.exe ./cmd/kiro-krew
    vars:
      BUILD_TIME:
        sh: date -u +%Y-%m-%dT%H:%M:%SZ
```

## Validation Commands

### Verify Workflow Syntax
```bash
# Install GitHub CLI and validate workflows
gh workflow list
gh workflow view ci.yml --yaml | yamllint -
gh workflow view update-version.yml --yaml | yamllint -
gh workflow view release.yml --yaml | yamllint -
```

### Verify Taskfile Tasks
```bash
# Test all new CI/CD tasks locally
task version:update-commit
cat internal/version/version.json  # Should show updated commit/timestamp

task version:set VERSION=1.0.0
cat internal/version/version.json  # Should show version=1.0.0

task build:release
ls -la dist/release/  # Should show multi-platform binaries
```

### Verify Local CI Simulation
```bash
# Simulate CI workflow locally
task fmt && task lint && task test && task build
echo "PR validation would pass"

# Test version update workflow
task version:update-commit
git diff internal/version/version.json  # Should show changes

# Test release workflow
task version:set VERSION=0.6.0
task build:release
ls -la dist/release/kiro-krew-*  # Should show all platform binaries
```

### Verify Security Configuration
```bash
# Check for hardcoded secrets
grep -r "ghp_\|ghs_" .github/workflows/ && echo "Found hardcoded tokens!" || echo "No hardcoded tokens found"

# Verify minimal permissions
grep -A5 "permissions:" .github/workflows/*.yml
# Should only show contents: write where needed
```

## Risk Mitigation

### Security Measures
- Use minimal GitHub token permissions
- Pin GitHub Actions to specific commit hashes
- No sensitive data in workflow logs or artifacts
- Atomic file operations for version updates
- Skip CI commit to prevent infinite loops

### Reliability Measures
- Workflow timeouts prevent runaway processes
- Proper error handling in all tasks
- Atomic JSON operations prevent corruption
- Cross-platform compatibility testing
- Graceful fallback when git operations fail

### Dependency Management
- Pin Task version to prevent breaking changes
- Use official, verified GitHub Actions only
- Cache Go modules for faster builds
- Version detection from go.mod ensures consistency

## Dependencies

### Required for Workflows
- **Issues #88 & #89**: JSON version metadata and Taskfile tasks
- **Task**: Build tool for standardized operations
- **jq**: JSON manipulation in Taskfile tasks
- **GitHub CLI**: For release operations (in release workflow)

### GitHub Repository Configuration
- **Workflow permissions**: Actions must have write access to contents
- **Branch protection**: Optional but recommended for main branch
- **Secrets**: Standard GITHUB_TOKEN is sufficient (no custom secrets needed)

## Future Extensibility

### Planned Enhancements
- Integration with semantic-release for sophisticated version determination
- Deploy workflows for different environments
- Performance benchmarking in CI
- Security scanning integration
- Docker image builds and publishing
- Integration testing with external dependencies

### Configurable Release Strategy
The release workflow can be enhanced to support:
- Pre-release versions (alpha, beta, rc)
- Release candidate workflows
- Automated changelog generation
- Integration with package managers
- Notification systems (Slack, email)

This design provides a robust, secure, and extensible CI/CD foundation that maintains strict adherence to the Taskfile-only constraint while enabling sophisticated automation workflows.
