# Design Specification: Durable Incident Logging System

**Issue:** #65  
**Closes:** #65  
**Created:** 2026-06-07  
**Author:** Architect Agent  

## Problem Statement

Currently, incident logs are written to `specs/incidents/` within temporary worktrees, causing them to appear in PRs and be deleted when worktrees are cleaned up. We need a durable logging system that persists outside worktree lifecycle while maintaining complete incident reports.

## Solution Approach

### High-Level Strategy
1. **Leverage Existing Infrastructure**: The incident logging system already exists in `internal/incidents/` and correctly stores logs in `~/.kiro-krew/incidents/<repo-name>/`
2. **Redirect Worktree Logging**: Intercept incident writes that currently go to worktree locations and redirect to durable storage
3. **Add REPL Integration**: Implement `logs` command in TUI for browsing and viewing incidents
4. **Maintain Format Compatibility**: Preserve existing incident report format without modification

### Storage Architecture
- **Primary Storage**: `~/.kiro-krew/incidents/<repo-name>/`
- **File Format**: `incident-<issue>-<attempt>-<timestamp>.md`
- **Project Isolation**: Repository-based subdirectories for multi-project support
- **Persistence**: Survives worktree deletion, system reboots, and repository changes

## Relevant Files

### Files to Modify
- `internal/tui/commands.go` - Add `logs` command handler
- `internal/tui/tui.go` - Integrate logs viewing functionality  
- `cmd/kiro-krew/cmd/logs.go` - Create new logs command (if not using TUI)
- `internal/session/manager.go` - Redirect incident logging from worktree to durable storage

### Files Already Implemented (Leverage)
- `internal/incidents/logger.go` - Core logging functionality 
- `internal/incidents/storage.go` - Storage operations
- `cmd/kiro-krew/cmd/log_incident.go` - CLI logging interface

### Files to Create
- `internal/tui/logs_view.go` - TUI component for browsing incident logs
- `internal/session/incident_redirect.go` - Worktree-to-durable logging bridge

## Data Structures

### Incident Storage Format
```
~/.kiro-krew/incidents/
├── kiro-krew/                    # Repository name
│   ├── incident-65-1-20260607-211905.md
│   ├── incident-65-2-20260607-212130.md
│   └── incident-63-1-20260607-183045.md
└── other-project/
    └── incident-42-1-20260607-120000.md
```

### Log Entry Structure (Existing)
```markdown
# Incident Report: <title>

## Summary
<brief description of failure>

## Attempts
### Attempt N
- Action: <what was attempted>
- Result: <outcome/error>

## Root Cause Analysis
<technical analysis>

## Recommended Actions
<action items>

## Context
<metadata: issue, worktree, repo, task>
```

### REPL Integration Points
```go
type LogsViewState struct {
    Incidents []incidents.IncidentInfo
    Selected  int
    ViewMode  LogViewMode // List | Detail
    Content   string      // For detail view
}

type LogViewMode int
const (
    ListMode LogViewMode = iota
    DetailMode
)
```

## Team Orchestration

### Component Dependencies
1. **Core Logging** (already exists): `internal/incidents/*`
2. **REPL Integration**: Depends on TUI command system  
3. **Worktree Redirection**: Depends on session management
4. **CLI Commands**: Independent, can be implemented in parallel

### Integration Points
- **Session Manager**: Must detect worktree incident writes and redirect
- **TUI Commands**: Add to existing command router in `handleCommand()`
- **File System**: Ensure `.kiro-krew/incidents` directory permissions
- **Git Integration**: Repository name detection (already implemented)

## Step-by-Step Task Breakdown

### Phase 1: Worktree Redirection
**Acceptance Criteria**: Incidents written to worktree locations are redirected to durable storage

1. **Identify Worktree Write Points**
   - Search codebase for `specs/incidents/` writes
   - Locate session manager incident logging calls
   - Map current incident creation workflow

2. **Implement Redirection Layer** 
   - Create `internal/session/incident_redirect.go`
   - Intercept worktree incident writes
   - Route to `incidents.IncidentLogger` instead
   - Preserve original content format

3. **Update Session Manager**
   - Modify incident logging calls in `internal/session/manager.go`
   - Replace worktree file writes with durable logger calls
   - Maintain backward compatibility for existing workflows

### Phase 2: REPL Logs Command
**Acceptance Criteria**: Users can run `logs` in REPL to view incident history

1. **Add Command Handler**
   - Extend `internal/tui/commands.go` with `logs` case
   - Implement incident list retrieval using `IncidentLogger.ListIncidents()`
   - Handle command parsing and validation

2. **Create Logs View Component**
   - Implement `internal/tui/logs_view.go`
   - Create list view showing incidents with metadata
   - Add navigation controls (up/down, select, back)
   - Implement detail view for full incident content

3. **Integrate with TUI State**
   - Add `LogsViewState` to main model
   - Implement view switching logic
   - Handle keyboard navigation and commands
   - Ensure proper cleanup on exit

### Phase 3: Enhanced User Experience  
**Acceptance Criteria**: Logs command provides intuitive browsing with search and filtering

1. **Add Filtering Options**
   - Filter by issue number: `logs 65`
   - Filter by date range: `logs --since=2d`
   - Filter by repository: `logs --repo=kiro-krew`

2. **Improve Display Format**
   - Syntax highlighting for incident content
   - Compact list view with key metadata
   - Pagination for large incident lists
   - Status indicators (success/failure/retry)

3. **Add Export Functionality**
   - Export incident as file: `logs export 65-1`
   - Copy to clipboard functionality
   - JSON export for programmatic access

## Validation Commands

### Functional Verification
```bash
# Test incident logging redirection
./kiro-krew log-incident 65 1 "Test incident content"

# Verify storage location
ls ~/.kiro-krew/incidents/kiro-krew/
cat ~/.kiro-krew/incidents/kiro-krew/incident-65-1-*.md

# Test REPL logs command  
./kiro-krew
> logs
> logs 65
> logs --help
```

### Integration Testing
```bash
# Create worktree and verify no incidents appear in PR
git worktree add --track -b test-logging origin/main test-logging
cd test-logging
./kiro-krew log-incident 99 1 "Integration test incident"

# Verify incident not in worktree but in durable storage
find . -name "*incident*" -type f | grep -v ".kiro-krew/incidents"
ls ~/.kiro-krew/incidents/kiro-krew/incident-99-*
```

### Performance Verification  
```bash
# Test with multiple incidents
for i in {1..10}; do
    ./kiro-krew log-incident 100 $i "Performance test incident $i"
done

# Verify logs command responsiveness
time echo "logs" | ./kiro-krew --non-interactive
```

### Cross-Repository Testing
```bash
# Test multi-project isolation
cd /tmp && git clone https://github.com/example/other-project
cd other-project
kiro-krew log-incident 1 1 "Different project incident"

# Verify separation
ls ~/.kiro-krew/incidents/
ls ~/.kiro-krew/incidents/other-project/
ls ~/.kiro-krew/incidents/kiro-krew/
```

## Implementation Notes

### Security Considerations
- Incident files stored with 0644 permissions (readable by user)  
- No sensitive data should be logged in incidents
- Repository name validation to prevent directory traversal

### Error Handling
- Graceful degradation if `~/.kiro-krew` directory inaccessible
- Fallback to temporary storage with user warning
- Clear error messages for permission issues

### Backward Compatibility
- Existing incident files in worktrees remain readable
- CLI `log-incident` command maintains same interface
- Incident file format unchanged

### Performance Optimization  
- Lazy loading of incident content in TUI
- Indexing by timestamp for efficient date filtering
- Caching of repository name detection
