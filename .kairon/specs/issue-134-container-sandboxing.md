# Container-based Sandboxing for Eval Command

**Issue**: #134  
**Closes**: #134  
**Created**: 2026-06-20  

## Solution Approach

Implement Docker-based container isolation for the `kiro-krew eval` command to eliminate security risks from agent execution. Each eval test case will run in a separate Docker container with:

1. **Complete isolation** - No host filesystem access beyond copied project code
2. **GitHub CLI mocking** - Replace GitHub skills with logging-only mock versions  
3. **Universal project support** - Dynamic toolchain detection and installation
4. **Resource controls** - CPU, memory, and execution time limits
5. **Clean teardown** - Automatic container cleanup after each test

## Relevant Files

### New Files to Create
- `internal/eval/sandbox/` - Container orchestration package
  - `container.go` - Docker container lifecycle management
  - `project_detector.go` - Universal project type detection
  - `mock_github.go` - GitHub CLI skill mocking utilities  
  - `resource_limits.go` - Container resource constraint definitions
- `internal/eval/sandbox/testdata/` - Mock GitHub skill templates
  - `github-cli-mock/` - Mock GitHub CLI skill implementation
- `internal/eval/dockerfile/` - Container image definitions
  - `base.Dockerfile` - Minimal base image with common tools
  - `templates/` - Language-specific Dockerfile templates

### Files to Modify
- `internal/eval/runner.go` - Update `invokeAgent()` to use container execution
- `internal/eval/types.go` - Add container configuration types
- `cmd/kiro-krew/cmd/eval.go` - Add container control flags
- `go.mod` - Add Docker client dependencies

### Files for Reference
- `.kiro/skills/discover-qa-tools/SKILL.md` - Project detection patterns
- `.kiro-krew/evals/cases/*/` - Existing test case structure  
- `Taskfile.yml` - Build patterns for container inclusion

## Team Orchestration

This feature requires coordination between security (containerization) and compatibility (universal project support):

1. **Container Infrastructure** - Docker management, resource limits, lifecycle
2. **Project Detection** - Language/framework detection, toolchain installation  
3. **GitHub Mocking** - Skill replacement system, response simulation
4. **Integration** - Seamless eval command integration with existing test cases

## Step-by-Step Task Breakdown

### Phase 1: Container Infrastructure
**Task 1.1**: Create base container management
- **File**: `internal/eval/sandbox/container.go`
- **Acceptance**: Can create, start, copy files to, execute commands in, and cleanup Docker containers
- **Dependencies**: Add Docker client to go.mod

**Task 1.2**: Implement resource limits
- **File**: `internal/eval/sandbox/resource_limits.go`  
- **Acceptance**: Containers enforce CPU (1 core), memory (512MB), timeout (5 min) limits
- **Dependencies**: Task 1.1

**Task 1.3**: Create base Dockerfile
- **File**: `internal/eval/dockerfile/base.Dockerfile`
- **Acceptance**: Minimal Ubuntu/Alpine image with git, curl, bash installed
- **Dependencies**: None

### Phase 2: Universal Project Support  
**Task 2.1**: Implement project type detection
- **File**: `internal/eval/sandbox/project_detector.go`
- **Acceptance**: Detects Go, Node.js, Python, Rust, Java projects from config files
- **Dependencies**: None

**Task 2.2**: Create toolchain installation templates
- **File**: `internal/eval/dockerfile/templates/`
- **Acceptance**: Dockerfile snippets for installing each detected toolchain
- **Dependencies**: Task 2.1, Task 1.3

**Task 2.3**: Dynamic Dockerfile generation
- **File**: `internal/eval/sandbox/container.go` (extend)
- **Acceptance**: Generates custom Dockerfile based on project detection results
- **Dependencies**: Task 2.1, Task 2.2

### Phase 3: GitHub CLI Mocking
**Task 3.1**: Create mock GitHub CLI skill
- **File**: `internal/eval/sandbox/testdata/github-cli-mock/`
- **Acceptance**: Drop-in replacement skill that logs operations instead of executing them
- **Dependencies**: None

**Task 3.2**: Implement skill replacement system  
- **File**: `internal/eval/sandbox/mock_github.go`
- **Acceptance**: Replaces `.kiro/skills/github-cli/` with mock version in container
- **Dependencies**: Task 3.1

**Task 3.3**: Mock response simulation
- **File**: `internal/eval/sandbox/mock_github.go` (extend)
- **Acceptance**: Mock returns realistic GitHub API responses for common operations
- **Dependencies**: Task 3.2

### Phase 4: Integration
**Task 4.1**: Update eval runner for container execution
- **File**: `internal/eval/runner.go`
- **Acceptance**: `invokeAgent()` function uses container execution when sandboxing enabled
- **Dependencies**: Phase 1, Phase 2, Phase 3

**Task 4.2**: Add container configuration types
- **File**: `internal/eval/types.go`
- **Acceptance**: Structs for container config, project detection results, resource limits
- **Dependencies**: Task 4.1

**Task 4.3**: Add CLI flags for container control
- **File**: `cmd/kiro-krew/cmd/eval.go`  
- **Acceptance**: `--sandbox`, `--no-sandbox`, `--resource-limit` flags control container usage
- **Dependencies**: Task 4.2

**Task 4.4**: Update existing test cases
- **File**: Various test case YAML files
- **Acceptance**: All existing eval test cases pass with sandboxing enabled
- **Dependencies**: Task 4.3

### Phase 5: Testing & Documentation
**Task 5.1**: Create sandbox integration tests
- **File**: `internal/eval/sandbox/container_test.go`
- **Acceptance**: Tests cover container lifecycle, project detection, resource limits
- **Dependencies**: Phase 4

**Task 5.2**: Update documentation  
- **File**: `docs/evaluation.md`
- **Acceptance**: Documents container sandboxing, resource limits, GitHub mocking
- **Dependencies**: Task 5.1

## Validation Commands

```bash
# Verify Docker is available
docker version

# Build and test container infrastructure  
go test ./internal/eval/sandbox/... -v

# Test project detection
go run ./cmd/kiro-krew eval --sandbox --list architect

# Run containerized eval test
go run ./cmd/kiro-krew eval --sandbox architect basic-spec-generation

# Verify GitHub operations are mocked (should show log entries, not real API calls)
go run ./cmd/kiro-krew eval --sandbox builder create-github-issue

# Test resource limits (should timeout)  
KIRO_KREW_EVAL_TIMEOUT=1s go run ./cmd/kiro-krew eval --sandbox builder long-running-task

# Verify cleanup (no dangling containers)
docker ps -a | grep kiro-krew-eval

# Run full eval suite with sandboxing
go run ./cmd/kiro-krew eval --sandbox
```

## Technical Specifications

### Container Execution Flow
1. **Project Analysis**: Detect project type, generate appropriate Dockerfile
2. **Image Build**: Create custom image with detected toolchain + kiro-krew + kiro-cli
3. **Container Start**: Launch with resource limits, copy project code to `/workspace`
4. **Skill Mocking**: Replace GitHub CLI skill with mock version 
5. **Agent Execution**: Run kiro-cli with test prompt in isolated environment
6. **Result Capture**: Extract stdout/stderr, mock operation logs
7. **Cleanup**: Stop and remove container, cleanup temporary files

### Resource Limits
- **CPU**: 1.0 cores maximum
- **Memory**: 512MB maximum  
- **Execution Time**: 5 minutes (configurable via KIRO_KREW_EVAL_TIMEOUT)
- **Disk**: 1GB temporary space
- **Network**: Outbound HTTP/HTTPS allowed, GitHub API blocked

### Mock GitHub CLI Interface
```bash
# Operations return success but log instead of executing
gh issue create --title "Test" --body "Test" 
# → Logs: [MOCK] gh issue create --title "Test" --body "Test"
# → Returns: {"number": 12345, "url": "https://github.com/test/test/issues/12345"}

gh pr create --title "Test PR"
# → Logs: [MOCK] gh pr create --title "Test PR"  
# → Returns: {"number": 42, "url": "https://github.com/test/test/pull/42"}
```

### Project Detection Matrix
| Files Found | Detected Type | Toolchain Installed |
|-------------|---------------|-------------------|
| `go.mod`, `go.sum` | Go | golang:1.21+ |
| `package.json` | Node.js | node:20+ + npm |
| `requirements.txt`, `pyproject.toml` | Python | python:3.11+ + pip |
| `Cargo.toml` | Rust | rust:1.70+ |
| `pom.xml`, `build.gradle` | Java | openjdk:17+ + maven/gradle |
| `Taskfile.yml` | Task runner | go-task/task |

### Configuration Structure
```go
type ContainerConfig struct {
    Image          string            `json:"image"`
    ResourceLimits ResourceLimits    `json:"resource_limits"`
    Environment    map[string]string `json:"environment"`
    WorkspaceDir   string            `json:"workspace_dir"`
    MockGitHub     bool              `json:"mock_github"`
}

type ResourceLimits struct {
    CPULimit    float64       `json:"cpu_limit"`     // cores
    MemoryLimit int64         `json:"memory_limit"`  // bytes
    Timeout     time.Duration `json:"timeout"`       // execution timeout
}
```

This design provides complete isolation while maintaining compatibility with existing eval test cases and supporting any project type through dynamic toolchain detection.