# Design Specification: Semantic Versioning with JSON Metadata

**Issue**: #88 - Implement semantic versioning with JSON metadata file  
**Closes**: #88

## Solution Approach

Replace the hardcoded "dev" version string with a semantic versioning system using a `version.json` file as the single source of truth for version metadata. The JSON file will be embedded into the binary at compile time via Go's `embed` package, eliminating runtime file system access while providing a clean developer experience for version management.

This approach mirrors .NET's `Directory.Build.props` pattern where editing a properties file automatically affects the next build without requiring build script modifications.

## Architecture Overview

```
version.json (source file) 
    ↓ (go:embed at compile time)
embedded JSON in binary
    ↓ (parsed at init() time)  
version.Version string (runtime variable)
    ↓ (used by)
about command display
```

## Relevant Files

### Files to Create
- `version.json` - Version metadata in project root
- Update handling in `internal/version/version.go`

### Files to Modify
- `internal/version/version.go` - Add JSON embedding and parsing
- No changes needed to `internal/tui/commands.go` (uses version.Info() already)

### Files Referenced (No Changes)
- `internal/tui/tui.go` - References `version.Version` for "dev" detection
- `cmd/kiro-krew/main.go` - Entry point (no version-related code)

## Team Orchestration

This is a self-contained change requiring only:
1. **Builder Agent**: Implement the version system according to this specification
2. **Validator Agent**: Verify build works and displays correct version

No coordination with external systems or breaking changes to existing APIs.

## Step-by-Step Task Breakdown

### Task 1: Create version.json File
**Acceptance Criteria:**
- Create `version.json` in project root with initial version 0.5.0
- JSON structure: `{"version": "0.5.0", "prerelease": ""}`
- File committed to repository (not in .gitignore)
- JSON structure extensible for future metadata

### Task 2: Update version.go Implementation  
**Acceptance Criteria:**
- Add `import _ "embed"` directive
- Add `//go:embed version.json` directive with `versionJSON []byte` variable
- Create `init()` function to parse embedded JSON at startup
- Parse JSON into struct with Version and Prerelease fields
- Replace hardcoded `Version = "dev"` with parsed value
- Preserve existing `BuildDate`, `GoVersion`, and `Arch` variables
- Maintain backward compatibility with `Info()` function signature
- Handle JSON parse errors gracefully (fallback to "unknown")

### Task 3: Verify Integration
**Acceptance Criteria:**
- `about` command displays "0.5.0" instead of "dev"  
- Build date and other fields remain unchanged
- No runtime file system access (version read from embedded data only)
- Version parsing happens once at startup, not on each access

## Implementation Details

### version.json Structure
```json
{
  "version": "0.5.0",
  "prerelease": ""
}
```

**Rationale**: Simple, extensible structure. `prerelease` field supports future pre-release versioning (e.g., "alpha", "beta", "rc1").

### Go Implementation Pattern
```go
package version

import (
    _ "embed"
    "encoding/json"
    "runtime"
)

//go:embed version.json  
var versionJSON []byte

type VersionInfo struct {
    Version    string `json:"version"`
    Prerelease string `json:"prerelease"`
}

var (
    Version   = "unknown" // Will be set from JSON at init
    BuildDate = "unknown"
    GoVersion = runtime.Version()
    Arch      = runtime.GOOS + "/" + runtime.GOARCH
)

func init() {
    var info VersionInfo
    if err := json.Unmarshal(versionJSON, &info); err == nil {
        Version = info.Version
        if info.Prerelease != "" {
            Version += "-" + info.Prerelease
        }
    }
}
```

### Build Process Integration
- **No changes required** - `go:embed` works with standard `go build`
- Build date still set via ldflags: `-ldflags "-X github.com/jbrinkman/kiro-krew/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"`
- Version now comes from embedded JSON, not ldflags

## Validation Commands

### Verify Version Display
```bash
# Build and test the about command
go build ./cmd/kiro-krew
./kiro-krew about
# Should show "Version: 0.5.0" instead of "Version: dev"
```

### Verify Embedded Content
```bash
# Verify version.json is embedded (not read from disk)
strings ./kiro-krew | grep "0.5.0"
# Should find version string in binary

# Test without version.json file present
mv version.json version.json.bak
./kiro-krew about
# Should still show 0.5.0 (from embedded data)
mv version.json.bak version.json
```

### Verify Build Integration
```bash
# Test that changing version.json affects next build
echo '{"version": "0.6.0", "prerelease": ""}' > version.json
go build ./cmd/kiro-krew  
./kiro-krew about
# Should show "Version: 0.6.0"

# Restore original version
echo '{"version": "0.5.0", "prerelease": ""}' > version.json
```

### Verify Prerelease Support
```bash
# Test prerelease version handling
echo '{"version": "0.5.0", "prerelease": "alpha"}' > version.json
go build ./cmd/kiro-krew
./kiro-krew about  
# Should show "Version: 0.5.0-alpha"
```

## Risk Mitigation

- **JSON Parse Errors**: Default to "unknown" if JSON is malformed
- **Missing Fields**: Handle optional prerelease field gracefully  
- **Backward Compatibility**: Maintain existing `version.Info()` function signature
- **Build Process**: No impact on existing build workflows (go:embed works automatically)

## Future Extensibility

The JSON structure supports future additions:
- Build metadata (build number, commit hash from JSON)
- Release notes URLs
- Feature flags or configuration
- Marketing version vs. technical version

This change establishes the foundation for more sophisticated version management while solving the immediate "dev" version problem.
