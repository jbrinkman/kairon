# Design Specification: Add Linux Support for Kiro-Krew

**Issue**: #151 - Add Linux support: cross-compilation and binary releases  
**Closes**: #151

## Solution Approach

Add comprehensive Linux support to kiro-krew by implementing Go cross-compilation for Linux targets (amd64 and arm64), updating the Taskfile build system, and modifying release workflows to include Linux binaries in GitHub releases. This ensures broad platform compatibility while maintaining existing macOS functionality.

The approach leverages Go's built-in cross-compilation capabilities and follows established patterns in the current build system. All changes will be additive to preserve existing workflows.

## Relevant Files

### Files to Modify:
- `Taskfile.yml` - Add Linux build targets and update release task
- `.github/workflows/release.yml` - Update manual release workflow to include Linux binaries  
- `.releaserc.json` - Add Linux binaries to semantic release assets
- `README.md` - Add Linux installation instructions

### Files Created:
- `.kiro-krew/specs/issue-151-add-linux-support.md`

### Files Referenced:
- `cmd/kiro-krew/` - Main application entry point
- `internal/version/version.json` - Version metadata (existing pattern)
- `go.mod` - Go module configuration (confirms Go 1.25.0 compatibility)

## Team Orchestration

This is a self-contained build system enhancement that requires:
1. **Builder Agent**: Implement all Taskfile and workflow changes
2. **Validator Agent**: Test cross-compilation and verify Linux binary functionality
3. **Documenter Agent**: Update README with Linux installation instructions

No coordination with external teams required. The change is backward compatible and additive.

## Step-by-Step Task Breakdown

### Task 1: Add Linux Build Targets to Taskfile.yml
**Acceptance Criteria:**
- Add `build:linux:amd64` task with proper GOOS/GOARCH environment variables
- Add `build:linux:arm64` task with proper GOOS/GOARCH environment variables  
- Update `build:release` task to include both Linux targets alongside existing macOS
- Use consistent ldflags with version metadata (-s -w for optimization)
- Output binaries to `{{.BUILD_DIR}}/release/` with platform-specific names

**Implementation Notes:**
- Follow existing `build:macos` pattern for consistency
- Use same BUILD_TIME variable expansion as macOS build
- Binary names: `kiro-krew-linux-amd64` and `kiro-krew-linux-arm64`

### Task 2: Update GitHub Release Workflow
**Acceptance Criteria:**
- Modify `.github/workflows/release.yml` manual release section to upload Linux binaries
- Update `gh release create` command to include both Linux artifacts
- Use descriptive labels for each binary in release assets
- Maintain existing macOS binary upload functionality

**Implementation Notes:**
- Add both Linux binaries to the manual release upload command
- Labels should clearly identify platform and architecture
- Preserve existing workflow structure and permissions

### Task 3: Update Semantic Release Configuration
**Acceptance Criteria:**
- Modify `.releaserc.json` to include Linux binaries in GitHub release assets
- Add entries for both `kiro-krew-linux-amd64` and `kiro-krew-linux-arm64`
- Use consistent labeling with manual release workflow
- Maintain existing macOS asset configuration

**Implementation Notes:**
- Add two new asset entries to the `@semantic-release/github` plugin configuration
- Follow existing asset pattern with path and label fields

### Task 4: Update Documentation
**Acceptance Criteria:**
- Add Linux installation section to README.md under Installation section
- Include download instructions for both amd64 and arm64 architectures
- Provide installation commands for Linux users
- Mention Go installation requirements for building from source

**Implementation Notes:**
- Add after existing macOS installation instructions
- Include curl/wget examples for downloading releases
- Note architecture detection methods for users

## Validation Commands

### Cross-compilation Verification:
```bash
# Test Linux builds complete successfully
task build:linux:amd64
task build:linux:arm64
task build:release

# Verify output files exist
ls -la dist/release/kiro-krew-linux-*

# Check binary metadata (requires file command)
file dist/release/kiro-krew-linux-amd64
file dist/release/kiro-krew-linux-arm64
```

### Build System Integration:
```bash
# Verify all builds work together
task clean
task build:release
ls -la dist/release/  # Should show macOS + Linux binaries

# Test development build still works
task dev
./kiro-krew --version
```

### Release Workflow Validation:
```bash
# Simulate release build process
task version:set VERSION=1.0.1-test
task build:release
ls -la dist/release/

# Verify version metadata in Linux binaries
./dist/release/kiro-krew-linux-amd64 --version 2>/dev/null || echo "Expected: Linux binary not executable on macOS"
```

### Documentation Verification:
```bash
# Check README updates are properly formatted
grep -A 10 -B 2 "Linux" README.md
grep -A 5 "amd64\|arm64" README.md
```

## Technical Considerations

### Go Cross-compilation:
- Go 1.25.0 has excellent Linux cross-compilation support
- Static linking is default for Linux builds, ensuring broad compatibility
- CGO is disabled by default in cross-compilation (good for portability)

### Binary Naming Convention:
- Format: `kiro-krew-{os}-{arch}`
- Consistent with Go toolchain conventions
- Clear identification for users downloading releases

### Release Asset Management:
- GitHub allows multiple assets per release
- Semantic release plugin supports asset arrays
- Manual and automatic releases will both include Linux binaries

### Backward Compatibility:
- All existing functionality preserved
- macOS builds unchanged
- Existing development workflows unaffected
- No breaking changes to configuration or usage