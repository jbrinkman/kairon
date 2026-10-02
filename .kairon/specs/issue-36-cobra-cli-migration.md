# Design Specification: Migrate CLI to Cobra Framework

**Issue**: #36  
**Title**: Migrate CLI from switch-based implementation to Cobra framework  
**Closes**: #36

## Solution Approach

Replace the current switch-based CLI implementation with Cobra framework to leverage built-in CLI features and reduce maintenance overhead. The migration will:

- Replace manual argument parsing with Cobra's command structure
- Eliminate custom help handling by using Cobra's built-in help system
- Preserve all existing functionality while improving maintainability
- Keep the default TUI behavior when no commands are provided

### Architecture Decision

Use Cobra's standard command pattern where:
- Root command handles the default TUI behavior (no args)
- Subcommands handle specific operations (init, update, eval)
- `eval diff` becomes a subcommand of `eval`

## Relevant Files

### Files to Modify
- `cmd/kiro-krew/main.go` - Replace switch logic with Cobra root command
- `go.mod` - Add Cobra dependency

### Files to Create
- `cmd/kiro-krew/cmd/root.go` - Root command definition
- `cmd/kiro-krew/cmd/init.go` - Init command implementation
- `cmd/kiro-krew/cmd/update.go` - Update command implementation
- `cmd/kiro-krew/cmd/eval.go` - Eval command with diff subcommand

### Files Referenced (No Changes)
- `internal/eval/runner.go` - `eval.Run(agent string) error`
- `internal/eval/diff.go` - `eval.Diff(runA, runB string) error`
- `internal/tui/tui.go` - `tui.Run(w, manager, cfg) error`
- `internal/config/config.go` - `config.Load() (*Config, error)`
- `internal/agent/manager.go` - `agent.NewManager(cfg) *Manager`
- `internal/watcher/watcher.go` - `watcher.New(cfg, manager) *Watcher`
- `cmd/kiro-krew/templates/` - Embedded template filesystem

## Team Orchestration

This is a pure refactoring task with no external dependencies:

1. **Builder Agent**: Implement the Cobra migration following this spec
2. **Validator Agent**: Verify all commands work identically to before
3. **No coordination needed**: Self-contained change in `cmd/kiro-krew/`

## Step-by-Step Task Breakdown

### Task 1: Add Cobra Dependency
**Acceptance Criteria**:
- [ ] Add `github.com/spf13/cobra` to go.mod
- [ ] Run `go mod tidy` successfully

### Task 2: Create Root Command Structure
**File**: `cmd/kiro-krew/cmd/root.go`  
**Acceptance Criteria**:
- [ ] Define root command that starts TUI when no subcommands provided
- [ ] Set up command description and usage
- [ ] Import and embed templates filesystem
- [ ] Initialize config, agent manager, and watcher
- [ ] Handle graceful shutdown (defer manager.StopAll(), w.Stop())

### Task 3: Implement Init Command
**File**: `cmd/kiro-krew/cmd/init.go`  
**Acceptance Criteria**:
- [ ] Create init command with description "Extract project templates"
- [ ] Reuse existing `extractTemplates()` function logic
- [ ] Call `extractTemplates("templates", ".", false)` (non-force mode)
- [ ] Handle errors appropriately

### Task 4: Implement Update Command  
**File**: `cmd/kiro-krew/cmd/update.go`  
**Acceptance Criteria**:
- [ ] Create update command with description "Update project templates (force overwrite)"
- [ ] Reuse existing `extractTemplates()` function logic
- [ ] Call `extractTemplates("templates", ".", true)` (force mode)
- [ ] Handle errors appropriately

### Task 5: Implement Eval Command with Diff Subcommand
**File**: `cmd/kiro-krew/cmd/eval.go`  
**Acceptance Criteria**:
- [ ] Create eval command with description "Run evaluations or show diff between runs"
- [ ] Handle optional agent parameter: `eval.Run(agent)`
- [ ] Create diff subcommand with description "Compare two evaluation runs"
- [ ] Diff requires exactly 2 args: `eval.Diff(runA, runB)`
- [ ] Show proper usage messages for invalid arguments

### Task 6: Update Main Function
**File**: `cmd/kiro-krew/main.go`  
**Acceptance Criteria**:
- [ ] Replace entire switch-based logic with `cmd.Execute()`
- [ ] Remove manual help handling code
- [ ] Remove `helpData` map and help functions
- [ ] Keep templates embed directive
- [ ] Move `extractTemplates()` and `writeTemplateFile()` functions to shared location

### Task 7: Preserve Template Access
**Acceptance Criteria**:
- [ ] Ensure templates embed.FS is accessible from all commands
- [ ] Move template functions to package level or shared module
- [ ] Verify init and update commands can access embedded templates

## Validation Commands

### Functional Verification
```bash
# Build successfully
go build ./cmd/kiro-krew

# Test default behavior (should start TUI)
./kiro-krew

# Test help system
./kiro-krew --help
./kiro-krew -h
./kiro-krew init --help
./kiro-krew update --help  
./kiro-krew eval --help
./kiro-krew eval diff --help

# Test commands work identically
./kiro-krew init
./kiro-krew update
./kiro-krew eval
./kiro-krew eval someagent
./kiro-krew eval diff run1 run2

# Test case insensitivity (Cobra default)
./kiro-krew INIT
./kiro-krew Init
./kiro-krew UPDATE
./kiro-krew EVAL
```

### Regression Testing
```bash
# Verify no behavior changes
# Before migration, run: ./kiro-krew init
# After migration, run: ./kiro-krew init
# Compare: Should create identical files

# Test error handling
./kiro-krew eval diff                     # Should show usage error
./kiro-krew eval diff run1                # Should show usage error  
./kiro-krew nonexistent                   # Should show command not found
```

### Build Verification
```bash
# Ensure clean build
go mod tidy
go build ./cmd/kiro-krew
go test ./...
```

## Implementation Notes

### Critical Constraints
- **DO NOT** modify existing internal packages (`internal/eval`, `internal/tui`, etc.)
- **DO NOT** duplicate template files - reuse existing embed.FS
- **MUST** preserve exact same command interface for users
- **MUST** preserve default TUI startup behavior when no args provided

### Cobra Migration Patterns
- Use `cobra.Command` struct with `Use`, `Short`, `Long`, and `RunE` fields
- Leverage Cobra's automatic help generation
- Use `cmd.Args` validation (e.g., `cobra.ExactArgs(2)` for eval diff)
- Handle errors with `return err` in `RunE` functions

### Template Access Strategy
Move template-related functions to package level in main.go or create shared module that can be imported by command files. Ensure the embed.FS is accessible from all command implementations.
