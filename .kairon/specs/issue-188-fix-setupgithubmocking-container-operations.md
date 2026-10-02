# Design Specification: Fix SetupGitHubMocking Container Operations

**Issue**: #188 - Fix SetupGitHubMocking using host filesystem instead of container operations

**Closes**: #188

## Problem Analysis

The `SetupGitHubMocking` function in `internal/eval/sandbox/mock_github.go` incorrectly uses host-side filesystem operations (`os.MkdirAll`, `os.WriteFile`, `os.RemoveAll`) instead of container operations. This causes evaluation failures on systems with read-only root filesystems (like macOS with System Integrity Protection) because the code attempts to create directories at `/workspace/.kiro/skills/github-cli` on the host rather than inside the container.

## Solution Approach

Replace all host filesystem operations with container-native operations using the existing Container methods:
- `c.ExecWithOutput()` for directory creation and removal
- `c.CopyToContainer()` via Docker API for file copying (requires tar archive creation)

The solution maintains the same functional behavior while ensuring all operations occur within the container's filesystem.

## Relevant Files

### Files to Modify
- `internal/eval/sandbox/mock_github.go` - Replace host operations with container operations

### Files for Reference/Testing  
- `internal/eval/sandbox/container.go` - Contains `CopyTo`, `ExecWithOutput` methods
- `internal/eval/sandbox/container_test.go` - Contains existing GitHub mocking tests
- `internal/eval/sandbox/installation_test.go` - Contains `SetupGitHubMocking` usage
- `internal/eval/sandbox/testdata/github-cli-mock/` - Mock skill files to copy

## Team Orchestration

This is a single-file bug fix that requires:
1. **Builder**: Implement container-based filesystem operations
2. **Validator**: Verify existing tests pass and container operations work correctly

No architectural changes or cross-team coordination needed.

## Step-by-Step Task Breakdown

### Task 1: Replace Directory Operations
**Acceptance Criteria:**
- Replace `os.MkdirAll(filepath.Dir(skillPath), 0755)` with `c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/.kiro/skills"})`
- Replace `os.RemoveAll(skillPath)` with `c.ExecWithOutput(ctx, []string{"rm", "-rf", "/workspace/.kiro/skills/github-cli"})`
- Replace `os.MkdirAll(skillPath, 0755)` with `c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/.kiro/skills/github-cli"})`

### Task 2: Replace File Copy Operations  
**Acceptance Criteria:**
- Create a new method `CopyToContainerFromEmbed` that copies embedded files to container using tar archives
- Replace the `fs.WalkDir` loop that uses `os.WriteFile` with container copy operations
- Ensure file permissions (0755) are preserved in the container

### Task 3: Update Method Signature (if needed)
**Acceptance Criteria:**  
- Ensure `SetupGitHubMocking` works with container context
- Remove dependency on `workspacePath` parameter since container paths are fixed at `/workspace`

### Task 4: Maintain Embedded File Support
**Acceptance Criteria:**
- Preserve `//go:embed testdata/github-cli-mock/*` functionality
- Ensure all mock skill files are copied correctly to container
- Maintain the same directory structure in container as before

## Implementation Details

### New Container File Copy Method
```go
func (c *Container) CopyToContainerFromEmbed(ctx context.Context, embedFS embed.FS, srcDir, destDir string) error {
    // Create tar archive from embedded files
    // Use Docker CopyToContainer API with proper tar format
    // Preserve file permissions and directory structure
}
```

### Directory Operations
- Use `/workspace/.kiro/skills/github-cli` as fixed container path
- Use `mkdir -p` for recursive directory creation
- Use `rm -rf` for cleanup (with error handling for non-existent paths)

## Validation Commands

### Unit Tests
```bash
go test ./internal/eval/sandbox/ -v -run TestContainer_GitHubMockingSetup
go test ./internal/eval/sandbox/ -v -run TestGitHubCLIMocking  
```

### Integration Tests  
```bash
go test ./internal/eval/sandbox/ -v -run TestSetupGitHubMocking
```

### Manual Verification
```bash
# Create test container and verify mock setup
docker run --rm -it alpine:3.19 sh -c "ls -la /workspace/.kiro/skills/github-cli/"
```

## Risk Mitigation

### Constraints Adherence
- **Maintain mock skill functionality**: All existing mock files must be copied correctly
- **No container operation breakage**: Existing container methods remain unchanged
- **Preserve embedded files**: `testdata/github-cli-mock` structure maintained

### Error Handling
- Check container is running before operations
- Handle missing directory creation gracefully  
- Provide clear error messages for container operation failures
- Ensure cleanup happens even on partial failures

### Backward Compatibility
- Method signature can remain the same (workspacePath ignored in container mode)
- All existing tests should pass without modification
- No changes to caller code required

