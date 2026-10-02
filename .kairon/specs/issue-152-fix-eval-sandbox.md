# Design Specification: Fix eval --sandbox Docker Container Setup

**Issue:** #152  
**Title:** Fix eval --sandbox: install kiro-cli, setup GitHub mocking, configure environment  
**Closes:** #152

## Solution Approach

The `eval --sandbox` functionality fails because the Alpine Linux container lacks essential dependencies and proper environment setup. The solution involves enhancing the `invokeAgentInContainer()` function in `internal/eval/runner.go` to:

1. Install `kiro-cli` binary (Alpine musl variant)  
2. Setup GitHub CLI mocking using existing infrastructure
3. Configure proper environment variables
4. Ensure all dependencies are available

This leverages existing mock GitHub infrastructure while adding the missing kiro-cli installation and environment configuration.

## Relevant Files

### Files to Modify
- `internal/eval/runner.go` - `invokeAgentInContainer()` function needs container setup enhancement
- `internal/eval/sandbox/container.go` - Add kiro-cli installation method
- `internal/eval/types.go` - May need environment variable configuration updates

### Files to Reference
- `internal/eval/sandbox/mock_github.go` - Existing GitHub mocking infrastructure
- `internal/eval/sandbox/testdata/github-cli-mock/` - Mock GitHub CLI skill
- `cmd/kiro-krew/cmd/eval.go` - Eval command configuration

## Team Orchestration

**Phase 1: Container Setup Enhancement**
- Builder implements kiro-cli installation in container setup
- Builder integrates GitHub mocking setup in container lifecycle

**Phase 2: Environment Configuration**  
- Builder configures environment variable propagation
- Builder ensures PATH setup for mock binaries

**Phase 3: Integration Testing**
- Validator verifies kiro-cli is available in container
- Validator confirms GitHub mocking is active
- Validator tests complete eval --sandbox workflow

## Step-by-Step Task Breakdown

### Task 1: Enhance Container Setup in invokeAgentInContainer()

**Acceptance Criteria:**
- [ ] Container setup phase installs kiro-cli binary before agent execution
- [ ] GitHub mocking is configured during container startup
- [ ] Environment variables are properly propagated to container
- [ ] PATH includes mock binary directories

**Implementation Steps:**
1. Detect container architecture (x86_64 vs ARM64) for correct kiro-cli variant
2. Add kiro-cli installation commands using installer script or direct download
3. Call existing `SetupGitHubMocking()` and `ConfigureMockGitHubPath()` methods
4. Configure environment variables from host system
5. Set up proper PATH for mock binaries

### Task 2: Add kiro-cli Installation Method to Container

**Acceptance Criteria:**
- [ ] Container has working `kiro-cli` binary in PATH
- [ ] Installation uses Alpine musl-compatible variant
- [ ] Installation is architecture-aware (x86_64/ARM64)
- [ ] Installation is cached/optimized for container reuse

**Implementation Steps:**
1. Add `InstallKiroCLI()` method to `Container` struct
2. Detect container architecture using `uname -m` or Docker API
3. Download appropriate musl variant:
   - x86_64: `kirocli-x86_64-linux-musl.zip`
   - ARM64: `kirocli-aarch64-linux-musl.zip`
4. Extract and place binary in `/usr/local/bin/`
5. Verify installation with `kiro-cli --version`

### Task 3: Integrate GitHub Mocking Setup

**Acceptance Criteria:**
- [ ] Mock GitHub CLI skill is installed in container workspace
- [ ] Container PATH includes mock GitHub CLI binary
- [ ] Agents use mock responses instead of real GitHub API
- [ ] Mock operations are logged for debugging

**Implementation Steps:**
1. Call `SetupGitHubMocking()` during container startup
2. Call `ConfigureMockGitHubPath()` to configure PATH
3. Ensure mock skill is copied to `/workspace/.kiro/skills/github-cli/`
4. Verify mock `gh` binary is executable and in PATH

### Task 4: Configure Environment Variable Propagation

**Acceptance Criteria:**
- [ ] Essential kiro-krew environment variables are passed to container
- [ ] Container inherits necessary host environment settings
- [ ] Agent execution has proper environment context

**Implementation Steps:**
1. Identify required environment variables:
   - `KIRO_CLI_DISABLE_TELEMETRY=1`
   - `ISSUE_NUMBER` (if available)
   - `REPO` (if available)  
   - `KIRO_KREW_WATCHER_PID` (if available)
2. Update container configuration to include environment variables
3. Ensure environment is set before kiro-cli execution

### Task 5: Update Container Resource Configuration

**Acceptance Criteria:**
- [ ] Container has sufficient resources for kiro-cli installation and execution
- [ ] Resource limits are appropriate for agent workloads
- [ ] Container startup time remains reasonable

**Implementation Steps:**
1. Review default resource limits in `sandbox.DefaultLimits()`
2. Ensure memory allocation supports kiro-cli and agent execution
3. Configure CPU quota for responsive agent performance
4. Set appropriate timeout for agent execution

## Validation Commands

### Container Setup Verification
```bash
# Test container creation and setup
kiro-krew eval validator --sandbox --case simple-validation

# Verify kiro-cli installation
docker exec <container> kiro-cli --version

# Verify GitHub mocking
docker exec <container> which gh
docker exec <container> gh auth status

# Check environment variables
docker exec <container> env | grep -E "(KIRO|ISSUE|REPO)"
```

### Integration Testing
```bash
# Full sandbox evaluation test
kiro-krew eval builder --sandbox --case basic-implementation

# Verify no real GitHub API calls (check mock logs)
docker exec <container> cat /tmp/gh-mock.log

# Test multiple agents in sandbox
kiro-krew eval architect --sandbox
kiro-krew eval validator --sandbox
```

### Error Condition Testing
```bash
# Test with resource constraints
kiro-krew eval builder --sandbox --resource-limit memory=256MB

# Test timeout handling
kiro-krew eval builder --sandbox --resource-limit timeout=30s

# Test network isolation
docker exec <container> curl -m 5 github.com || echo "Network properly isolated"
```

## Technical Implementation Details

### kiro-cli Installation Strategy

The container setup will use the official kiro-cli installer script which automatically detects musl systems and downloads the correct variant:

```bash
# In container setup phase
apk add --no-cache curl bash ca-certificates
curl -fsSL https://cli.kiro.dev/install | bash
```

Alternative approach for offline/cached installation:
- Download appropriate musl binary during container build
- Extract and install directly without network dependency

### Architecture Detection

Container architecture detection ensures correct binary variant:

```bash
ARCH=$(uname -m)
case $ARCH in
  x86_64) KIRO_ARCH="x86_64-linux-musl" ;;
  aarch64) KIRO_ARCH="aarch64-linux-musl" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
```

### GitHub Mocking Integration

The existing mock infrastructure in `internal/eval/sandbox/mock_github.go` provides:
- `SetupGitHubMocking()` - Copies mock skill to workspace
- `ConfigureMockGitHubPath()` - Updates PATH for mock binary
- Mock `gh` script that returns realistic responses

Integration involves calling these methods during container startup phase.

### Environment Variable Configuration

Essential variables for agent execution:
```go
envVars := map[string]string{
    "KIRO_CLI_DISABLE_TELEMETRY": "1",
    "PATH": "/workspace/.kiro/skills/github-cli:/usr/local/bin:/usr/bin:/bin",
}
// Add kiro-krew specific variables if available
if issueNum := os.Getenv("ISSUE_NUMBER"); issueNum != "" {
    envVars["ISSUE_NUMBER"] = issueNum
}
```

### Container Lifecycle Flow

1. **Container Creation** - Create Alpine container with resource limits
2. **Dependency Installation** - Install curl, bash, ca-certificates
3. **kiro-cli Installation** - Download and install appropriate musl variant  
4. **GitHub Mocking Setup** - Copy mock skill and configure PATH
5. **Environment Configuration** - Set necessary environment variables
6. **Agent Execution** - Run kiro-cli with proper context
7. **Cleanup** - Stop and remove container

## Error Handling and Recovery

### Installation Failures
- Network timeout during kiro-cli download → Retry with exponential backoff
- Architecture detection failure → Default to x86_64 with warning
- Binary extraction failure → Clear error message with troubleshooting steps

### Runtime Failures  
- kiro-cli not found in PATH → Verify installation and PATH configuration
- Mock GitHub setup failure → Fall back to network-disabled container
- Environment variable issues → Log missing variables with recommendations

### Resource Constraints
- Memory limits → Provide actionable error with suggested memory increase
- CPU throttling → Warn about potential performance impact
- Timeout exceeded → Suggest timeout increase in error message

## Dependencies and Constraints

### Prerequisites
- Docker daemon running on host system
- Internet access for kiro-cli installation (unless cached)
- Sufficient system resources for container execution

### Limitations  
- Alpine Linux musl compatibility requirement
- Container isolation prevents real GitHub API access
- Resource limits may affect agent performance
- Container startup overhead impacts evaluation timing

### Compatibility
- Must work on both x86_64 and ARM64 host systems
- Compatible with existing eval framework and test cases
- Maintains existing container security and isolation