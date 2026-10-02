# Design Specification: Fix Sandbox kiro-cli Installation Permission Errors

**Issue:** #165  
**Title:** Fix sandbox kiro-cli installation permission errors with comprehensive testing  
**Closes:** #165

## Problem Analysis

### Root Cause
The sandbox container setup has a permission mismatch:
1. Dockerfile creates non-root user `sandbox` via `USER sandbox` directive
2. `InstallKiroCLI()` attempts to install to `/usr/local/bin/` which requires root privileges
3. Installation command `mv kiro-cli /usr/local/bin/kiro-cli` fails with permission denied
4. Container runs as `sandbox` user but needs root access for system directory installation

### Current Architecture Issues
- **Timing Problem**: User switch happens before kiro-cli installation in Dockerfile generation
- **Permission Mismatch**: Installation runs as non-root but targets root-owned directory
- **Security Constraint**: Must maintain non-root execution for agent operations

## Solution Approach

### Selected Strategy: Multi-Stage Installation (Option B)
**Rationale:** Maintains security while ensuring reliable installation without modifying Alpine base image or adding sudo complexity.

**Implementation:** Modify Dockerfile generation to install kiro-cli during root phase, then switch to sandbox user.

### Key Architectural Changes

1. **Dockerfile Generation Restructure**
   - Move kiro-cli installation before `USER sandbox` directive
   - Install as root during container build phase
   - Ensure binary is executable by sandbox user after user switch

2. **Installation Process Redesign**
   - Split installation into build-time vs runtime phases
   - Build-time: Download and install kiro-cli as root
   - Runtime: Verify installation as sandbox user

## Relevant Files

### Primary Implementation Files
- `internal/eval/sandbox/container.go` (lines 240-295): Dockerfile generation logic
- `internal/eval/sandbox/container.go` (lines 292-350): Installation and verification logic  
- `internal/eval/runner.go` (lines 420-450): Container configuration

### New Test Files Required
- `internal/eval/sandbox/installation_test.go`: Unit tests for installation logic
- `internal/eval/sandbox/integration_installation_test.go`: End-to-end installation tests
- `internal/eval/sandbox/permission_test.go`: Permission handling tests

### Supporting Files
- `internal/eval/sandbox/mock_github.go`: GitHub CLI mocking (existing)
- `internal/eval/sandbox/resource_limits.go`: Resource management (existing)

## Team Orchestration

### Development Phases
1. **Phase 1: Core Fix** - Modify Dockerfile generation and installation logic
2. **Phase 2: Test Infrastructure** - Create comprehensive test suite
3. **Phase 3: Validation** - End-to-end testing and cross-platform verification

### Component Dependencies
- **Container Management** → **Installation Logic** → **Verification Process** → **Test Suite**
- Each component must be testable independently
- Installation verification must work across both AMD64 and ARM64 platforms

## Step-by-Step Task Breakdown

### Task 1: Modify Dockerfile Generation Logic
**Acceptance Criteria:**
- [ ] Move kiro-cli installation commands before `USER sandbox` directive
- [ ] Maintain proper binary permissions for sandbox user access
- [ ] Preserve existing toolchain installation patterns

**Implementation Details:**
```go
// In generateDockerfile(), restructure order:
// 1. Install system packages (existing)
// 2. Install toolchain templates (existing) 
// 3. Install kiro-cli as root (NEW LOCATION)
// 4. Add user and set permissions (existing)
// 5. Switch to USER sandbox (existing)
```

### Task 2: Refactor Installation Process  
**Acceptance Criteria:**
- [ ] Split `InstallKiroCLI()` into build-time and runtime components
- [ ] Create `addKiroCLIToDockerfile()` for build-time installation
- [ ] Modify `InstallKiroCLI()` to handle runtime verification only
- [ ] Maintain cross-platform download URL generation

**Implementation Details:**
- `addKiroCLIToDockerfile(platform)`: Generates installation commands for Dockerfile
- `InstallKiroCLI()`: Simplified to verification-only for runtime
- Platform detection logic remains unchanged

### Task 3: Implement Installation Unit Tests (80% Coverage Target)
**Acceptance Criteria:**
- [ ] Test Dockerfile generation with kiro-cli installation commands
- [ ] Test platform-specific download URL generation
- [ ] Test installation command construction for AMD64/ARM64
- [ ] Mock installation failures and verify error handling
- [ ] Test binary permission verification logic

**Test Categories:**
```go
func TestDockerfileGeneration_IncludesKiroCLI(t *testing.T)
func TestKiroCLIDownloadURL_SupportedPlatforms(t *testing.T)
func TestInstallationCommands_CrossPlatform(t *testing.T)
func TestInstallationVerification_PermissionChecks(t *testing.T)
```

### Task 4: Create Integration Test Suite
**Acceptance Criteria:**
- [ ] End-to-end container creation with kiro-cli installation
- [ ] Verify `kiro-cli --version` execution as sandbox user
- [ ] Test `kiro-cli chat --help` command functionality
- [ ] Cross-platform testing (linux/amd64, linux/arm64)
- [ ] Container lifecycle management in tests

**Test Structure:**
```go
func TestContainerIntegration_KiroCLIInstallation(t *testing.T)
func TestKiroCLIExecution_SandboxUser(t *testing.T)  
func TestCrossPlatform_InstallationVerification(t *testing.T)
```

### Task 5: Enhance Error Reporting and Validation
**Acceptance Criteria:**
- [ ] Detailed error context including container ID and platform
- [ ] Installation command output capture and reporting
- [ ] Specific verification failure reporting (missing vs not executable vs version fail)
- [ ] Network failure handling and retry logic

### Task 6: Performance and Compatibility Testing
**Acceptance Criteria:**
- [ ] Ensure installation doesn't significantly impact container startup time
- [ ] Validate backwards compatibility with existing container API
- [ ] Test resource limit preservation
- [ ] Environment variable propagation verification

## Validation Commands

### Unit Test Execution
```bash
# Run all sandbox package tests
go test ./internal/eval/sandbox/... -v -cover

# Run specific installation tests  
go test ./internal/eval/sandbox/ -run TestInstallation -v

# Check test coverage (target: 80%+)
go test ./internal/eval/sandbox/ -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Integration Test Validation
```bash
# Run integration tests with Docker
go test ./internal/eval/sandbox/ -run Integration -v

# Cross-platform validation
GOOS=linux GOARCH=amd64 go test ./internal/eval/sandbox/ -run CrossPlatform
GOOS=linux GOARCH=arm64 go test ./internal/eval/sandbox/ -run CrossPlatform
```

### Manual Verification Commands
```bash
# Verify container creation and kiro-cli installation
docker run --rm alpine:3.19 /bin/sh -c "which kiro-cli && kiro-cli --version"

# Test sandbox user permissions
docker run --rm alpine:3.19 /bin/sh -c "su sandbox -c 'kiro-cli --version'"

# Validate GitHub CLI mocking setup
docker run --rm alpine:3.19 /bin/sh -c "su sandbox -c 'gh --version'"
```

## Security and Compliance Considerations

### Security Constraints Maintained
- Container continues to run agent operations as non-root `sandbox` user
- kiro-cli binary installed with appropriate permissions (755) 
- No privilege escalation during runtime operations
- Container isolation preserved

### Backwards Compatibility
- Existing `ContainerConfig` API unchanged
- Resource limits configuration preserved  
- Environment variable patterns maintained
- Mock GitHub CLI setup unchanged

## Risk Mitigation

### Technical Risks
- **Risk:** Build-time installation failure breaks container creation
- **Mitigation:** Comprehensive error handling and installation verification
- **Risk:** Cross-platform binary compatibility issues  
- **Mitigation:** Platform-specific testing and validation

### Performance Risks
- **Risk:** Installation increases container startup time
- **Mitigation:** Move to build-time reduces runtime overhead
- **Risk:** Network failures during kiro-cli download
- **Mitigation:** Proper error handling and retry mechanisms

## Success Metrics

### Functional Success
- [ ] 100% sandbox evaluation success rate for kiro-cli installation
- [ ] Cross-platform compatibility (AMD64/ARM64) verified
- [ ] All existing container functionality preserved

### Quality Success  
- [ ] 80%+ test coverage achieved for sandbox installation logic
- [ ] All unit and integration tests passing
- [ ] No regression in container startup performance
- [ ] Enhanced error reporting provides actionable feedback

### Security Success
- [ ] Non-root agent execution maintained
- [ ] Container isolation preserved
- [ ] No security vulnerabilities introduced