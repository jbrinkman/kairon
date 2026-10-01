# Design Specification: Plan-and-Execute Architecture with Validated Task Delegation

**Issue:** #273 - Implement Plan-and-Execute Architecture with Validated Task Delegation  
**Closes:** #273

## Solution Approach

Transform kiro-krew from a fixed-pipeline orchestrator into a validated plan-and-execute system that enables task parallelization, dynamic routing, and scalable agent composition. The architect agent will produce machine-readable execution plans embedded within design specifications, and krew-lead will validate and execute these plans mechanically while maintaining strict policy enforcement.

### Core Architecture Principles

1. **Separation of Planning and Execution**: The architect (with full issue context) produces the execution plan; krew-lead (mechanistic scheduler) validates and executes it under fixed policy constraints.

2. **Machine-Readable Plan Artifact**: Execution plans are embedded as structured YAML within markdown specs, defining a task DAG with agent assignments, dependencies, and validation commands.

3. **Agent Registry Auto-Discovery**: Available agents are discovered from `.kiro/agents/*.json` files, enabling addition of new agents without modifying orchestrator logic.

4. **Validated Execution**: Plans undergo schema validation, DAG acyclicity checks, and agent resolution before execution. Invalid plans trigger architect retry with failure context.

5. **Task Parallelization**: Independent tasks (no dependency edges) execute concurrently via topological sorting and parallel process spawning.

6. **Immutable Policy Gates**: The QA feedback loop, retry/incident escalation, and PR lifecycle remain owned by krew-lead and cannot be altered by plan content.

### Key Innovations

- **Plan Validation as Retry Boundary**: Invalid plans are architect failures that trigger retry with diagnostic context, not immediate incident escalation.
- **Topological Task Execution**: Tasks are dispatched in dependency order, with parallel execution of independent work.
- **Agent Registry Pattern**: Agent availability is a runtime property discovered from config files, not hardcoded in orchestrator logic.
- **Policy/Routing Separation**: krew-lead enforces workflow policy (QA gates, retries, PR creation); the architect defines routing and sequencing.

## Relevant Files

### New Files to Create

- `internal/plan/types.go` - Plan artifact data structures and types
- `internal/plan/parser.go` - Plan parsing from markdown/YAML
- `internal/plan/validator.go` - Plan validation (schema, DAG, agent resolution)
- `internal/plan/executor.go` - Plan execution with topological sort and parallelization
- `internal/plan/registry.go` - Agent registry auto-discovery
- `internal/plan/parser_test.go` - Parser unit tests
- `internal/plan/validator_test.go` - Validator unit tests
- `internal/plan/executor_test.go` - Executor unit tests
- `internal/plan/registry_test.go` - Registry unit tests

### Files to Modify

- `.kiro/agents/architect-prompt.md` - Add plan artifact production requirements
- `.kiro/agents/krew-lead-prompt.md` - Replace static workflow with plan-based execution
- `internal/agent/manager.go` - Add parallel task spawning support
- `internal/config/config.go` - Add plan execution configuration fields

### Reference Files (No Changes)

- `.kiro/agents/*.json` - Agent configurations (source of truth for registry)
- Existing specs in `.kiro-krew/specs/` - Historical reference (no migration)

## Team Orchestration

This implementation follows a layered dependency structure enabling significant parallelization:

### Layer 1: Foundation (Parallel Execution)
Tasks 1-3 can execute in parallel as they have no interdependencies:
- Task 1: Define plan artifact schema and types
- Task 2: Implement agent registry auto-discovery
- Task 3: Create plan parser for YAML extraction

### Layer 2: Validation and Execution (Parallel Execution)
Tasks 4-5 depend on Layer 1 but can execute in parallel with each other:
- Task 4: Implement plan validation (depends on Tasks 1, 2, 3)
- Task 5: Implement plan executor with parallelization (depends on Tasks 1, 2)

### Layer 3: Integration (Sequential Execution)
Task 6 integrates all components and depends on Layer 2:
- Task 6: Update krew-lead to use plan-based execution (depends on Tasks 4, 5)

### Layer 4: Specification Enhancement (Sequential Execution)
Task 7 finalizes the architect's plan production capability:
- Task 7: Update architect prompt to produce plan artifacts (depends on Task 1)

### Layer 5: Testing and Validation (Sequential Execution)
Tasks 8-9 validate the complete implementation:
- Task 8: Integration testing of plan execution flow (depends on Tasks 6, 7)
- Task 9: Backward compatibility verification (depends on Task 8)

**Parallelization Opportunities**: Tasks within Layers 1 and 2 can be executed concurrently, significantly reducing implementation time.

## Machine-Readable Plan Artifact Schema

The plan artifact is embedded within the markdown spec as a YAML code block with the language identifier `kiro-plan`:

````markdown
```yaml
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Implement database schema"
    dependencies: []
    acceptance_criteria:
      - "Create User model with validation"
      - "Add migration scripts"
    validation_commands:
      - "go test ./internal/models/..."
  
  - id: "task-2"
    agent: "builder"
    description: "Implement API handlers"
    dependencies: []
    acceptance_criteria:
      - "Create authentication endpoints"
      - "Add request validation"
    validation_commands:
      - "go test ./internal/api/..."
  
  - id: "task-3"
    agent: "builder"
    description: "Integrate frontend"
    dependencies: ["task-1", "task-2"]
    acceptance_criteria:
      - "Wire UI to API endpoints"
      - "Add token management"
    validation_commands:
      - "npm test -- --coverage"
```
````

### Schema Specification

```yaml
version: string                      # Plan format version (currently "1.0")
tasks: []Task                        # Ordered list of tasks

Task:
  id: string                         # Unique task identifier (e.g., "task-1")
  agent: string                      # Agent name from registry (e.g., "builder")
  description: string                # Human-readable task description
  dependencies: []string             # List of task IDs this task depends on
  acceptance_criteria: []string      # Task-specific success criteria
  validation_commands: []string      # Commands to verify task completion
```

### Schema Validation Rules

1. **Version**: Must be "1.0"
2. **Task IDs**: Must be unique within the plan
3. **Dependencies**: All referenced task IDs must exist in the plan
4. **Agent Names**: All agents must exist in the discovered registry
5. **DAG Acyclicity**: Dependency graph must be acyclic (no circular dependencies)
6. **Completeness**: Every task must have an assigned agent

## Step-by-Step Task Breakdown

### Task 1: Define Plan Artifact Schema and Types

**Objective**: Create Go types for the plan artifact and validation rules

**Files**: `internal/plan/types.go`

**Implementation Details**:
- Define `Plan` struct with version and tasks slice
- Define `Task` struct with id, agent, description, dependencies, acceptance criteria, validation commands
- Add `Validate()` method to `Plan` for basic schema validation
- Define error types for schema violations: `ErrInvalidVersion`, `ErrDuplicateTaskID`, `ErrMissingAgent`
- Include YAML struct tags for parsing

**Acceptance Criteria**:
- [ ] `Plan` and `Task` types defined with proper field validation
- [ ] YAML struct tags enable direct unmarshaling from YAML blocks
- [ ] Schema validation method checks version, unique task IDs, non-empty agent assignments
- [ ] Error types distinguish between different validation failure modes
- [ ] Unit tests verify schema validation catches malformed plans

**Dependencies**: None

**Validation Commands**:
```bash
go test ./internal/plan -run TestPlanTypes
go test ./internal/plan -run TestSchemaValidation
```

---

### Task 2: Implement Agent Registry Auto-Discovery

**Objective**: Build agent registry from `.kiro/agents/*.json` configurations

**Files**: `internal/plan/registry.go`, `internal/plan/registry_test.go`

**Implementation Details**:
- Define `AgentRegistry` type with map of agent name to config path
- Implement `DiscoverAgents(agentDir string)` function that:
  - Reads all `*.json` files from specified directory
  - Parses each file to extract agent `name` field
  - Returns registry mapping agent names to their configs
  - Returns error for duplicate agent names
- Implement `Contains(agentName string)` method for agent resolution
- Cache registry to avoid repeated file system access

**Acceptance Criteria**:
- [ ] Registry discovers all agents from `.kiro/agents/*.json`
- [ ] Agent names are extracted from JSON `name` field
- [ ] Duplicate agent names across files are detected and reported
- [ ] Registry provides fast lookup for agent name validation
- [ ] Unit tests verify discovery with multiple agent configs

**Dependencies**: None

**Validation Commands**:
```bash
go test ./internal/plan -run TestAgentRegistry
go test ./internal/plan -run TestRegistryDiscovery
```

---

### Task 3: Create Plan Parser

**Objective**: Extract and parse plan artifacts from markdown specs

**Files**: `internal/plan/parser.go`, `internal/plan/parser_test.go`

**Implementation Details**:
- Implement `ParsePlanFromMarkdown(content []byte)` function that:
  - Scans markdown for fenced code block with `kiro-plan` language identifier
  - Extracts YAML content from the block
  - Unmarshals YAML into `Plan` struct
  - Returns error if multiple plan blocks found
  - Returns `nil, nil` if no plan block found (for backward compatibility)
- Use regex or markdown parser to locate fenced blocks
- Validate YAML syntax before unmarshaling

**Acceptance Criteria**:
- [ ] Parser extracts plan from markdown with `kiro-plan` code block
- [ ] Parser returns error if multiple plan blocks exist
- [ ] Parser returns `nil, nil` gracefully if no plan block exists
- [ ] YAML syntax errors are caught and reported with line numbers
- [ ] Unit tests cover valid plans, malformed YAML, missing plans, multiple plans

**Dependencies**: None

**Validation Commands**:
```bash
go test ./internal/plan -run TestPlanParser
go test ./internal/plan -run TestParserEdgeCases
```

---

### Task 4: Implement Plan Validation

**Objective**: Validate plans for schema correctness, DAG acyclicity, and agent resolution

**Files**: `internal/plan/validator.go`, `internal/plan/validator_test.go`

**Implementation Details**:
- Define `Validator` struct containing `AgentRegistry` reference
- Implement `ValidatePlan(plan *Plan, registry *AgentRegistry)` function that performs:
  - **Schema validation**: Version check, unique task IDs, non-empty fields
  - **Dependency resolution**: All referenced task IDs exist
  - **Agent resolution**: All agent names exist in registry
  - **DAG acyclicity**: No circular dependencies (use topological sort or DFS)
- Return structured validation errors listing all violations (not just first failure)
- Implement cycle detection algorithm using DFS with visited/recursion stack tracking

**Acceptance Criteria**:
- [ ] Validator checks schema validity (version, task IDs, required fields)
- [ ] Validator resolves all task dependencies to existing tasks
- [ ] Validator resolves all agent names against registry
- [ ] Validator detects circular dependencies and reports the cycle
- [ ] Validation returns all errors together, not just first failure
- [ ] Unit tests cover valid plans, unknown agents, missing dependencies, cycles

**Dependencies**: Task 1 (types), Task 2 (registry), Task 3 (parser)

**Validation Commands**:
```bash
go test ./internal/plan -run TestPlanValidator
go test ./internal/plan -run TestCycleDetection
```

---

### Task 5: Implement Plan Executor with Parallelization

**Objective**: Execute validated plans with topological sorting and parallel task dispatch

**Files**: `internal/plan/executor.go`, `internal/plan/executor_test.go`

**Implementation Details**:
- Define `Executor` struct containing `AgentManager` and `Config` references
- Implement `ExecutePlan(plan *Plan, issueNumber int, repo string)` function that:
  - Performs topological sort of tasks based on dependencies
  - Groups tasks into execution layers (tasks with no unfulfilled dependencies)
  - Spawns agent processes for all tasks in current layer concurrently
  - Waits for layer completion before proceeding to next layer
  - Collects exit codes and handles task failures
  - Returns execution summary with status per task
- Implement `TopologicalSort(tasks []Task)` helper using Kahn's algorithm
- Implement layer grouping: tasks are ready when all dependencies completed
- Use `sync.WaitGroup` for concurrent task execution within a layer
- Handle partial layer failures: continue with independent tasks, skip dependent tasks

**Acceptance Criteria**:
- [ ] Executor performs topological sort to determine execution order
- [ ] Independent tasks execute in parallel within same layer
- [ ] Executor waits for layer completion before next layer
- [ ] Task failures are recorded but don't crash executor
- [ ] Dependent tasks skip execution if dependencies fail
- [ ] Execution summary reports status of each task
- [ ] Unit tests verify parallel execution, dependency ordering, failure handling

**Dependencies**: Task 1 (types), Task 2 (registry)

**Validation Commands**:
```bash
go test ./internal/plan -run TestPlanExecutor
go test ./internal/plan -run TestTopologicalSort
go test ./internal/plan -run TestParallelExecution
```

---

### Task 6: Update Krew-Lead to Use Plan-Based Execution

**Objective**: Replace fixed workflow with plan validation and execution

**Files**: `.kiro/agents/krew-lead-prompt.md`, `internal/agent/manager.go`

**Implementation Details**:

**Krew-Lead Prompt Changes**:
- Replace step 5 ("Execute Tasks") with new plan-based workflow:
  1. Read spec file at `.kiro-krew/specs/issue-<number>-*.md`
  2. Parse plan artifact using plan parser
  3. If no plan found, fallback to legacy sequential delegation (backward compatibility)
  4. If plan found:
     - Discover agent registry from `.kiro/agents/`
     - Validate plan against registry
     - If validation fails, delegate back to architect with validation errors (retry boundary)
     - If validation passes, execute plan with parallel task dispatch
  5. Proceed to QA loop with spec-level validation commands (policy gate)
- Add validation failure retry: include full validation error report in architect retry message
- Maintain QA loop as policy gate between implementation and PR (unchanged)
- Add attempt tagging to architect retry: `[attempt:N]` with validation failure context

**Agent Manager Changes** (`internal/agent/manager.go`):
- Add `SpawnTask(taskID, agentName, issueNumber, repo, taskDescription string)` method
- Extend worktree naming to include task ID: `issue-<number>-task-<taskID>-<pid>`
- Add task-level logging: `.kiro-krew/logs/issue-<number>-task-<taskID>.log`
- Add `SpawnParallelTasks(tasks []TaskSpec, issueNumber, repo string)` for concurrent spawning
- Return task execution results with per-task exit codes

**Acceptance Criteria**:
- [ ] Krew-lead reads and parses plan artifact from architect's spec
- [ ] Krew-lead validates plan before execution
- [ ] Invalid plans trigger architect retry with validation errors
- [ ] Valid plans are executed with parallel task dispatch
- [ ] QA loop remains as policy gate after plan execution
- [ ] Fallback to sequential delegation if no plan artifact present
- [ ] Agent manager supports parallel task spawning
- [ ] Task-level logging and worktrees isolate concurrent work

**Dependencies**: Task 4 (validator), Task 5 (executor)

**Validation Commands**:
```bash
go test ./internal/agent -run TestPlanBasedExecution
go test ./internal/plan -run TestKrewLeadIntegration
```

---

### Task 7: Update Architect Prompt to Produce Plan Artifacts

**Objective**: Require architect to embed machine-readable plans in specs

**Files**: `.kiro/agents/architect-prompt.md`

**Implementation Details**:
- Add "Machine-Readable Plan Artifact" section to architect prompt with:
  - Plan schema documentation with field descriptions
  - Example plan showing task structure, dependencies, and agent assignments
  - Instruction to embed plan as fenced YAML block with `kiro-plan` language identifier
  - Guidance on task granularity: tasks should be builder-executable units
  - Dependency specification rules: tasks must reference other task IDs, not descriptions
  - Agent assignment guidelines: use `builder` for implementation, `validator` for verification
- Update "Design Specification Requirements" section to include plan artifact as required component
- Add plan validation self-check: architect must verify task IDs are unique, dependencies are valid, agents are known
- Provide plan template in prompt for consistency

**Plan Template Example**:
````markdown
```yaml
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Brief task description"
    dependencies: []
    acceptance_criteria:
      - "Specific criterion 1"
      - "Specific criterion 2"
    validation_commands:
      - "command to verify task"
```
````

**Acceptance Criteria**:
- [ ] Architect prompt requires plan artifact in all new specs
- [ ] Plan schema is fully documented in prompt
- [ ] Examples show proper task structure and dependency specification
- [ ] Architect prompt includes self-check guidelines for plan validity
- [ ] Task granularity guidance ensures builder-executable tasks
- [ ] Agent assignment rules documented (builder for implementation, etc.)

**Dependencies**: Task 1 (schema types)

**Validation Commands**:
```bash
# Manual verification: architect produces valid plans
# Integration test in Task 8 will verify end-to-end
```

---

### Task 8: Integration Testing of Plan Execution Flow

**Objective**: Verify end-to-end plan execution with real agents

**Files**: `test_integration.sh`, `internal/plan/integration_test.go`

**Implementation Details**:
- Create integration test that:
  1. Creates a synthetic issue spec with plan artifact
  2. Spawns krew-lead agent with the issue
  3. Verifies plan parsing, validation, and execution
  4. Checks that parallel tasks execute concurrently (via timing)
  5. Validates that dependent tasks wait for predecessors
  6. Confirms task-level logs and artifacts are created
- Add test case for invalid plan triggering architect retry
- Add test case for plan with no dependencies (full parallelism)
- Add test case for plan with linear dependencies (sequential execution)
- Add test case for plan with mixed dependencies (layered execution)
- Use mock agents or simple echo agents for fast testing

**Acceptance Criteria**:
- [ ] Integration test creates and processes synthetic issue with plan
- [ ] Test verifies parallel execution of independent tasks
- [ ] Test verifies sequential execution of dependent tasks
- [ ] Test verifies invalid plan triggers architect retry
- [ ] Test confirms task-level logging and artifacts
- [ ] Test suite completes in reasonable time (< 2 minutes)

**Dependencies**: Task 6 (krew-lead integration), Task 7 (architect plan production)

**Validation Commands**:
```bash
./test_integration.sh
go test ./internal/plan -run TestIntegration -v
```

---

### Task 9: Backward Compatibility Verification

**Objective**: Ensure existing workflow works without plan artifacts

**Files**: `internal/plan/compatibility_test.go`, existing test suites

**Implementation Details**:
- Create compatibility test that:
  1. Uses a spec without plan artifact (legacy format)
  2. Verifies krew-lead falls back to sequential delegation
  3. Confirms builder and validator are invoked correctly
  4. Validates PR creation and labeling work as before
- Run existing integration tests to confirm no regressions
- Verify existing historical specs remain unchanged
- Add logging to distinguish plan-based vs. legacy execution paths

**Acceptance Criteria**:
- [ ] Specs without plan artifacts execute via legacy sequential workflow
- [ ] Krew-lead detects absence of plan and falls back gracefully
- [ ] Builder and validator invocation unchanged in legacy mode
- [ ] PR creation and issue labeling work correctly
- [ ] All existing integration tests pass without modification
- [ ] Logging distinguishes plan-based from legacy execution

**Dependencies**: Task 8 (integration testing)

**Validation Commands**:
```bash
go test ./internal/plan -run TestBackwardCompatibility
./test_integration.sh  # All existing tests must pass
go test ./... -v       # Full test suite
```

---

## Validation Commands

### Unit Tests
```bash
# Plan types and schema validation
go test ./internal/plan -run TestPlanTypes -v
go test ./internal/plan -run TestSchemaValidation -v

# Agent registry
go test ./internal/plan -run TestAgentRegistry -v
go test ./internal/plan -run TestRegistryDiscovery -v

# Plan parser
go test ./internal/plan -run TestPlanParser -v
go test ./internal/plan -run TestParserEdgeCases -v

# Plan validator
go test ./internal/plan -run TestPlanValidator -v
go test ./internal/plan -run TestCycleDetection -v

# Plan executor
go test ./internal/plan -run TestPlanExecutor -v
go test ./internal/plan -run TestTopologicalSort -v
go test ./internal/plan -run TestParallelExecution -v

# Integration
go test ./internal/plan -run TestIntegration -v
go test ./internal/plan -run TestBackwardCompatibility -v
```

### Integration Tests
```bash
# Full integration test suite
./test_integration.sh

# Validation with existing tests
go test ./... -v

# Build verification
task build
```

### Manual Verification
```bash
# Verify agent discovery
ls -la .kiro/agents/*.json

# Verify spec format
cat .kiro-krew/specs/issue-273-*.md

# Start kiro-krew and process a test issue
kiro-krew
> watch start
```

## Configuration Changes

Add plan execution configuration to `.kiro-krew/config.yaml`:

```yaml
# Plan execution settings
plan:
  max_parallel_tasks: 4           # Maximum concurrent tasks per layer
  task_timeout: 30m               # Timeout for individual tasks
  enable_parallelization: true    # Toggle for parallel vs sequential execution
```

Update `internal/config/config.go` to include:

```go
type PlanConfig struct {
    MaxParallelTasks      int           `yaml:"max_parallel_tasks"`
    TaskTimeout          time.Duration `yaml:"task_timeout"`
    EnableParallelization bool         `yaml:"enable_parallelization"`
}

type Config struct {
    // ... existing fields ...
    Plan PlanConfig `yaml:"plan"`
}
```

Default values:
- `max_parallel_tasks`: 4
- `task_timeout`: 30 minutes
- `enable_parallelization`: true

## Policy Gate Preservation

The following workflow elements remain **immutable policy gates** controlled by krew-lead, not alterable by plan content:

1. **QA Feedback Loop**: Validator ↔ Builder iteration remains under krew-lead control
   - Triggered after plan execution completes
   - Uses spec-level validation commands
   - Retry limit enforced by `max_qa_retries` config
   - Plan cannot skip or alter QA loop behavior

2. **Retry and Incident Escalation**: Agent failure handling remains krew-lead policy
   - Per-agent retry attempts (4-stage process)
   - Incident report generation on exhaustion
   - Plan validation failures trigger architect retry (not incident)
   - Plan cannot alter retry limits or escalation thresholds

3. **PR Lifecycle**: Issue labeling and PR creation remain krew-lead controlled
   - PR creation only after QA gate passes
   - `<label>-done` and `<label>-failed` labeling
   - Worktree cleanup after PR verification
   - Plan cannot trigger PR creation prematurely

4. **Worktree Management**: Git worktree lifecycle remains system-controlled
   - Worktree creation before agent spawn
   - Worktree cleanup after PR verification
   - Orphaned worktree detection and removal
   - Plan cannot alter worktree structure or cleanup

## Backward Compatibility Strategy

### No Migration Required
- Historical specs in `.kiro-krew/specs/` remain unchanged
- Old specs serve as archival records of past executions
- Specs are never reused for code generation (one issue = one spec = one PR)

### Graceful Fallback
- If plan artifact not found in spec, krew-lead falls back to legacy sequential delegation
- Parser returns `nil, nil` for missing plans (not an error)
- Legacy workflow: architect → sequential builder delegation → validator → PR
- Logging distinguishes execution mode: `[plan-based]` vs `[legacy-sequential]`

### Detection Logic
```go
plan, err := plan.ParsePlanFromMarkdown(specContent)
if err != nil {
    // Malformed plan is an error - retry architect
    return fmt.Errorf("invalid plan: %w", err)
}
if plan == nil {
    // No plan found - use legacy workflow
    log.Printf("[legacy-sequential] no plan artifact, using sequential delegation")
    // ... existing sequential workflow code ...
} else {
    // Plan-based execution
    log.Printf("[plan-based] executing plan with %d tasks", len(plan.Tasks))
    // ... new plan execution code ...
}
```

## Edge Cases and Error Handling

### Plan Validation Failures
- **Unknown Agent**: Validation error lists unknown agent names → architect retry with registry contents
- **Circular Dependencies**: Validation error shows cycle path → architect retry with cycle details
- **Missing Dependencies**: Validation error lists unresolved task IDs → architect retry with task list
- **Schema Violations**: Validation error specifies field/constraint → architect retry with schema documentation

### Execution Failures
- **Task Failure in Layer**: Dependent tasks skip execution, independent tasks continue
- **Partial Layer Completion**: Executor reports which tasks succeeded/failed, continues to next viable layer
- **Agent Process Crash**: Task marked failed, execution summary includes exit code, depends on agent manager retry logic
- **Timeout**: Task timeout enforcement at agent manager level, treated as task failure

### Architect Retry Context
When plan validation fails, krew-lead provides structured context to architect:

```
[attempt:2] Plan validation failed. Fix the plan artifact.

Validation Errors:
- Unknown agent: "security-validator" (available agents: architect, builder, validator, documenter)
- Circular dependency detected: task-3 → task-5 → task-7 → task-3
- Task "task-4" depends on non-existent task "task-99"

Plan validation rules:
1. All agent names must exist in .kiro/agents/ directory
2. Task dependencies must reference existing task IDs
3. Dependency graph must be acyclic (no circular references)
4. All tasks must have unique IDs

Current agent registry:
- architect
- builder  
- validator
- documenter

Rewrite the plan artifact to address these validation failures.
```

## Success Metrics

### Functional Requirements
- [ ] Architect produces valid plan artifacts in all new specs
- [ ] Krew-lead validates plans before execution
- [ ] Invalid plans trigger architect retry (not incident escalation)
- [ ] Valid plans execute with task parallelization
- [ ] Independent tasks execute concurrently (verified via timing)
- [ ] Dependent tasks execute in correct order (after dependencies)
- [ ] Task failures are isolated (don't crash executor)
- [ ] QA loop remains as policy gate after plan execution
- [ ] PR creation only occurs after QA gate passes
- [ ] Backward compatibility: specs without plans use legacy workflow

### Performance Requirements
- [ ] Parallel task execution reduces total time for multi-task issues
- [ ] Agent registry caching avoids repeated file system access
- [ ] Plan parsing/validation completes in < 100ms for typical plans
- [ ] Topological sort handles plans with 50+ tasks efficiently

### Maintainability Requirements
- [ ] Adding new agent requires only `.kiro/agents/<agent>.json` file
- [ ] Plan schema is versioned (`version: "1.0"`) for future evolution
- [ ] Validation errors are actionable (include specific fixes)
- [ ] Logging distinguishes plan-based from legacy execution
- [ ] Unit test coverage > 85% for plan package

## Future Enhancements (Out of Scope)

These are explicitly **not included** in this implementation:

1. **Dynamic Re-Planning**: Mid-flight plan updates based on execution results
2. **Specialized Validators**: Security, architecture, CI, prompt evaluation agents (infrastructure is ready, agents are follow-on work)
3. **Plan Optimization**: Automatic task decomposition or dependency inference
4. **Cross-Issue Dependencies**: Tasks depending on other issues' completion
5. **Conditional Execution**: Tasks with conditional logic based on previous task results
6. **Plan Templating**: Reusable plan fragments or parameterized plans
7. **Web UI for Plans**: Visual DAG editor or execution visualization

## Implementation Notes

### Topological Sort Algorithm
Use Kahn's algorithm for stable, efficient topological sorting:

```
1. Calculate in-degree (number of dependencies) for each task
2. Add all tasks with in-degree 0 to ready queue (Layer 0)
3. While ready queue not empty:
   a. Execute all tasks in current layer concurrently
   b. For each completed task:
      - Remove it from graph
      - Decrement in-degree of dependent tasks
      - Add tasks that reach in-degree 0 to next layer
4. If any tasks remain with in-degree > 0, cycle detected (validation should prevent this)
```

### Parallel Execution Pattern
```go
var wg sync.WaitGroup
results := make(chan TaskResult, len(layer))

for _, task := range layer {
    wg.Add(1)
    go func(t Task) {
        defer wg.Done()
        exitCode := spawnTaskAgent(t.ID, t.Agent, issueNumber, repo, t.Description)
        results <- TaskResult{TaskID: t.ID, ExitCode: exitCode}
    }(task)
}

wg.Wait()
close(results)

// Collect results
for result := range results {
    // Process task completion
}
```

### Agent Registry Caching
```go
var (
    registryCache     *AgentRegistry
    registryCacheMu   sync.RWMutex
    registryCacheTime time.Time
)

func GetAgentRegistry() (*AgentRegistry, error) {
    registryCacheMu.RLock()
    if time.Since(registryCacheTime) < 5*time.Minute {
        defer registryCacheMu.RUnlock()
        return registryCache, nil
    }
    registryCacheMu.RUnlock()
    
    registryCacheMu.Lock()
    defer registryCacheMu.Unlock()
    
    registry, err := DiscoverAgents(".kiro/agents")
    if err != nil {
        return nil, err
    }
    
    registryCache = registry
    registryCacheTime = time.Now()
    return registry, nil
}
```

---

**This specification enables kiro-krew to deliver on its promise of parallel task execution, scales gracefully with the addition of specialized agents, and maintains strict separation between planning intelligence (architect) and execution policy (krew-lead).**
