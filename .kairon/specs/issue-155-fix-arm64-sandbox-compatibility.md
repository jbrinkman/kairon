# Design Specification: Fix Sandbox kiro-cli Installation for ARM64 Systems

**Issue Reference:** Closes #155

## Problem Statement

The evaluation framework's sandbox mode fails on ARM64 (aarch64) systems when attempting to install kiro-cli in Alpine containers. The current implementation relies on an installer script that doesn't properly handle ARM64 architecture detection, and containers are created without platform-specific configuration.

**Current Error:**
```
❌ (agent failed)
     Error: installing kiro-cli: unsupported architecture:aarch64
```

## Solution Approach

Replace the installer script approach with direct binary downloads using confirmed AWS URLs and implement host architecture detection for platform-specific container creation. This eliminates container-side architecture detection complexity and ensures reliable installation on both x86_64 and ARM64 systems.

### Key Changes

1. **Host Architecture Detection**: Detect architecture once on the host system during container configuration
2. **Platform-Specific Container Creation**: Use Docker's `--platform` flag to ensure proper container architecture
3. **Direct Binary Download**: Download appropriate kiro-cli binary directly without relying on installer script
4. **Simplified Installation**: Extract and place binary in standard location with verification

## Relevant Files

### Files to Modify

- `internal/eval/sandbox/container.go` - Core container management and kiro-cli installation
- `internal/eval/runner.go` - Container creation configuration

### Files Referenced

- `internal/eval/types.go` - Container configuration types
- `internal/eval/sandbox/mock_github.go` - GitHub mocking (no changes needed)
- `internal/eval/sandbox/resource_limits.go` - Resource management (no changes needed)

## Team Orchestration

This is a focused infrastructure fix requiring:

1. **Architecture Detection Module**: Add host architecture detection utility
2. **Container Platform Configuration**: Modify container creation to specify platform
3. **Binary Installation Module**: Replace installer script with direct binary download
4. **Verification Module**: Ensure kiro-cli is properly installed and functional

## Step-by-Step Task Breakdown

### Task 1: Add Host Architecture Detection
**Acceptance Criteria:**
- Add function to detect host system architecture (`runtime.GOARCH` or `uname -m`)
- Map Go architecture names to container platform names
- Return appropriate Docker platform string (`linux/amd64`, `linux/arm64`)

### Task 2: Implement Platform-Specific Container Creation
**Acceptance Criteria:**
- Modify `NewContainer()` or `Create()` to accept platform parameter
- Add platform specification to Docker `ImagePullOptions` and `ContainerCreate`
- Ensure containers use correct architecture images

### Task 3: Replace Installer Script with Direct Binary Download
**Acceptance Criteria:**
- Remove current `curl | sh` installer script approach
- Implement direct download from confirmed AWS URLs:
  - ARM64: `https://desktop-release.q.us-east-1.amazonaws.com/latest/kirocli-aarch64-linux-musl.zip`
  - x86_64: `https://desktop-release.q.us-east-1.amazonaws.com/latest/kirocli-x86_64-linux-musl.zip`
- Add ZIP extraction functionality
- Place binary in `/usr/local/bin/kiro-cli` with proper permissions (755)

### Task 4: Add Installation Verification
**Acceptance Criteria:**
- Verify kiro-cli binary exists and is executable
- Run `kiro-cli --version` to confirm functionality
- Provide clear error messages for installation failures

### Task 5: Update Container Configuration Flow
**Acceptance Criteria:**
- Modify `createContainerConfig()` in runner.go to detect and pass architecture
- Ensure `invokeAgentInContainer()` uses platform-aware container creation
- Maintain backward compatibility with existing configuration

### Task 6: Add Architecture-Specific Testing
**Acceptance Criteria:**
- Add unit tests for architecture detection on different systems
- Add integration tests for ARM64 and x86_64 container creation
- Test binary download and installation on both architectures
- Verify existing evaluation test cases pass on both architectures

## Validation Commands

### Pre-Implementation Verification
```bash
# Verify current failure on ARM64 system
kiro-krew eval <agent> --sandbox

# Check Docker platform support
docker buildx ls
docker run --platform=linux/arm64 alpine:3.19 uname -m
docker run --platform=linux/amd64 alpine:3.19 uname -m
```

### Post-Implementation Validation
```bash
# Test ARM64 sandbox execution
kiro-krew eval <agent> --sandbox

# Test x86_64 sandbox execution (on x86_64 system)
kiro-krew eval <agent> --sandbox

# Verify container architecture
docker run --platform=linux/arm64 alpine:3.19 uname -m  # Should show aarch64
docker run --platform=linux/amd64 alpine:3.19 uname -m  # Should show x86_64

# Test binary installation verification
kiro-cli --version  # Should work in both container types

# Run full evaluation test suite
cd .kiro-krew/evals && go test ./...
```

## Implementation Details

### Architecture Detection Implementation
```go
func detectHostArchitecture() (string, error) {
    switch runtime.GOARCH {
    case "amd64":
        return "linux/amd64", nil
    case "arm64":
        return "linux/arm64", nil
    default:
        return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
    }
}
```

### Binary URL Selection
```go
func getKiroCLIDownloadURL(platform string) (string, error) {
    baseURL := "https://desktop-release.q.us-east-1.amazonaws.com/latest/"
    switch platform {
    case "linux/amd64":
        return baseURL + "kirocli-x86_64-linux-musl.zip", nil
    case "linux/arm64":
        return baseURL + "kirocli-aarch64-linux-musl.zip", nil
    default:
        return "", fmt.Errorf("unsupported platform: %s", platform)
    }
}
```

### Container Platform Configuration
```go
pullOptions := image.PullOptions{
    Platform: platform, // e.g., "linux/arm64"
}

containerConfig := &container.Config{
    Image: imageName,
    // ... other config
}

// Specify platform during container creation
resp, err := c.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, &platform, "")
```

## Constraints and Considerations

### Backward Compatibility
- Preserve existing x86_64 functionality without changes to user workflow
- Maintain current error handling and timeout mechanisms
- Keep installation within existing container setup phase

### Error Handling
- Provide clear error messages for unsupported architectures
- Handle download failures gracefully with retry logic
- Maintain existing timeout and resource limit mechanisms

### Security Considerations
- Verify binary checksums if available
- Use HTTPS for all downloads
- Set appropriate file permissions (755) for binary

### Performance Considerations
- Cache binary downloads if possible to avoid repeated downloads
- Keep installation time comparable to current script approach
- Minimize additional container startup overhead

## Risk Mitigation

### Primary Risks
1. **Binary URL Changes**: AWS URLs might change or become unavailable
   - **Mitigation**: Add fallback to installer script if direct download fails
   
2. **Platform Detection Failures**: Host architecture detection might fail
   - **Mitigation**: Default to x86_64 with clear warning message

3. **Container Platform Incompatibility**: Some Docker installations might not support `--platform`
   - **Mitigation**: Fall back to platform-agnostic container creation

### Testing Strategy
- Test on both ARM64 and x86_64 systems before merge
- Add automated tests that don't require specific hardware
- Use Docker BuildKit emulation for cross-platform testing in CI

## Success Metrics

1. **Functional**: ARM64 systems can successfully run `kiro-krew eval <agent> --sandbox`
2. **Compatibility**: x86_64 systems continue to work without changes
3. **Performance**: Installation time remains comparable (< 30s increase)
4. **Reliability**: All existing evaluation test cases pass on both architectures
5. **User Experience**: Clear error messages for any remaining edge cases