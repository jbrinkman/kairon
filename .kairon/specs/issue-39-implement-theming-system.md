# Theming System Design Specification

**Issue**: #39 - Implement theming system for TUI with separate theme library  
**Closes**: #39  
**Author**: Kiro Architect Agent  
**Date**: 2024-06-02  

## Problem Statement

The Kiro-krew TUI currently uses hardcoded colors in `internal/tui/tui.go` that create poor contrast, particularly medium gray text (`lipgloss.Color("8")`) for console entries. The application needs a flexible theming system to improve readability and provide user customization options.

## Solution Approach

Implement a comprehensive theming system with:
1. Separate theme configuration files stored in `.kiro-krew/themes/`
2. Theme loader integrated with existing config system
3. Theme-based color replacement for all hardcoded lipgloss styles
4. Backward compatibility with automatic default theme fallback
5. Extensible architecture for future UI components

## Architecture Overview

```
.kiro-krew/
├── config.yaml           # Extended with theme field
├── themes/                # Theme directory (new)
│   ├── default.yaml       # Default theme
│   ├── high-contrast.yaml # High contrast theme
│   └── light.yaml         # Light theme
└── ...

internal/
├── config/
│   ├── config.go          # Extended Config struct
│   └── themes.go          # Theme loader (new)
└── tui/
    ├── tui.go             # Modified to use theme colors
    ├── commands.go        # Modified to use theme colors
    └── styles.go          # Centralized styles (new)
```

## Relevant Files

### Files to Create
- `internal/config/themes.go` - Theme loading and management
- `internal/tui/styles.go` - Centralized style definitions
- `.kiro-krew/themes/default.yaml` - Default theme configuration
- `.kiro-krew/themes/high-contrast.yaml` - High contrast theme
- `.kiro-krew/themes/light.yaml` - Light mode theme

### Files to Modify
- `internal/config/config.go` - Add theme field and theme loading
- `internal/tui/tui.go` - Replace hardcoded styles with theme-based styles
- `internal/tui/commands.go` - Use centralized styles for formatting

### Files Referenced
- `.kiro-krew/config.yaml` - Add theme field
- `go.mod` - Existing dependencies are sufficient

## Detailed Design

### Theme Configuration Format

Each theme file uses YAML format with semantic color names:

```yaml
name: "Default"
description: "Default kiro-krew theme"
colors:
  # Core UI colors
  primary: "#00AAFF"         # Accent/highlight color
  secondary: "#888888"       # Secondary text
  success: "#00AA00"         # Success states
  warning: "#FFAA00"         # Warning states
  error: "#FF0000"           # Error states
  
  # Text colors
  text_primary: "#FFFFFF"    # Main text
  text_secondary: "#CCCCCC"  # Secondary text
  text_muted: "#888888"      # Muted text
  
  # UI element colors  
  prompt: "#00AAFF"          # Command prompt
  separator: "#00AAFF"       # UI separators
  activity: "#CCCCCC"        # Activity log text
  
  # Background colors (for future use)
  background: "#000000"
  surface: "#111111"
```

### Theme Loader Implementation

The theme loader will:
1. Load theme files from `.kiro-krew/themes/` directory
2. Parse YAML theme configurations
3. Validate color values (hex, ANSI codes, named colors)
4. Provide fallback to default theme if specified theme is missing
5. Cache parsed themes for performance

### Style System Integration

Replace current hardcoded styles:
```go
// Current (hardcoded)
var (
    promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
    activityStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// New (theme-based)
type Styles struct {
    Prompt    lipgloss.Style
    Activity  lipgloss.Style
    Separator lipgloss.Style
    Success   lipgloss.Style
    Warning   lipgloss.Style
    Error     lipgloss.Style
}

func NewStyles(theme *Theme) *Styles {
    return &Styles{
        Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Prompt)),
        Activity:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.Activity)),
        // ... etc
    }
}
```

## Team Orchestration

This is a single-component feature that can be implemented incrementally:

1. **Config Team**: Extend config system with theme support
2. **Theme Team**: Create theme files and loader
3. **UI Team**: Replace hardcoded styles with theme-based styles
4. **Integration Team**: Wire everything together and test

No external coordination required - all changes are within the kiro-krew codebase.

## Step-by-Step Task Breakdown

### Phase 1: Theme Infrastructure (Priority: High)

#### Task 1.1: Extend Config System
- **Files**: `internal/config/config.go`
- **Changes**: Add `Theme string` field to Config struct
- **Validation**: Theme field is optional, defaults to "default"
- **Acceptance**: Config loads with theme field, backward compatible

#### Task 1.2: Create Theme Loader
- **Files**: `internal/config/themes.go`
- **Implementation**:
  - `Theme` struct matching YAML format
  - `LoadTheme(name string)` function
  - Theme validation and error handling
  - Default theme fallback logic
- **Acceptance**: Can load theme from `.kiro-krew/themes/default.yaml`

#### Task 1.3: Create Default Themes
- **Files**: 
  - `.kiro-krew/themes/default.yaml`
  - `.kiro-krew/themes/high-contrast.yaml` 
  - `.kiro-krew/themes/light.yaml`
- **Content**: Complete color definitions for each theme
- **Validation**: Colors improve contrast over current hardcoded values
- **Acceptance**: Theme files parse successfully, colors are valid

### Phase 2: Style System Refactoring (Priority: High)

#### Task 2.1: Create Centralized Styles
- **Files**: `internal/tui/styles.go`
- **Implementation**:
  - `Styles` struct with lipgloss.Style fields
  - `NewStyles(theme *Theme)` constructor
  - Style helper methods for consistent application
- **Acceptance**: All TUI styles accessible through styles struct

#### Task 2.2: Integrate Theme Loading in TUI
- **Files**: `internal/tui/tui.go`
- **Changes**:
  - Load theme in `newModel()` function
  - Replace `promptStyle` and `activityStyle` globals
  - Add styles to model struct
- **Acceptance**: TUI initializes with theme-based styles

### Phase 3: Style Replacement (Priority: High)

#### Task 3.1: Replace Hardcoded Colors in tui.go
- **Files**: `internal/tui/tui.go`
- **Changes**: 
  - Remove global style variables
  - Use `m.styles.Prompt` and `m.styles.Activity` in View()
  - Apply theme colors to all UI elements
- **Acceptance**: No hardcoded lipgloss colors remain

#### Task 3.2: Update Command Output Styling
- **Files**: `internal/tui/commands.go`
- **Changes**:
  - Use themed styles for status output formatting
  - Apply consistent coloring to help text
  - Use themed colors for error/success messages
- **Acceptance**: All command output uses theme colors

### Phase 4: Configuration Integration (Priority: Medium)

#### Task 4.1: Update Config Loading
- **Files**: `internal/config/config.go`
- **Changes**: Integrate theme loading in `Load()` function
- **Acceptance**: Config automatically loads specified theme

#### Task 4.2: Add Theme to Sample Config
- **Files**: `.kiro-krew/config.yaml`
- **Changes**: Add commented theme example
- **Acceptance**: Users can see how to specify themes

### Phase 5: Testing & Documentation (Priority: Medium)

#### Task 5.1: Add Error Handling
- **Files**: All theme-related files
- **Changes**: 
  - Graceful fallback when theme files are missing
  - Invalid color value handling
  - Theme parsing error messages
- **Acceptance**: Application never crashes due to theme issues

#### Task 5.2: Theme Validation
- **Implementation**: Validate theme completeness and color format
- **Acceptance**: Invalid themes are rejected with clear error messages

## Validation Commands

```bash
# Test default theme loading
go run cmd/kiro-krew/main.go

# Test theme switching
echo "theme: high-contrast" >> .kiro-krew/config.yaml
go run cmd/kiro-krew/main.go

# Test theme fallback (invalid theme)
echo "theme: nonexistent" >> .kiro-krew/config.yaml
go run cmd/kiro-krew/main.go

# Verify theme files exist
ls -la .kiro-krew/themes/

# Test theme parsing
go test ./internal/config/... -v

# Visual verification of colors
# 1. Start TUI
# 2. Check prompt color (should not be ANSI "6")  
# 3. Check activity text color (should not be ANSI "8")
# 4. Verify improved contrast for readability
```

## Success Criteria

1. **Contrast Issue Resolved**: Activity text uses high-contrast colors instead of medium gray
2. **Theme System Functional**: Users can specify themes in config.yaml
3. **Multiple Themes Available**: At least 3 predefined themes (default, high-contrast, light)
4. **Backward Compatible**: Existing configs work without modification
5. **Extensible Architecture**: Easy to add new theme colors and UI elements
6. **No Breaking Changes**: All existing functionality preserved
7. **Performance**: No noticeable impact on TUI responsiveness

## Future Extensions

- User-defined theme creation capability
- Theme hot-reloading without restart
- Terminal color profile detection
- Theme-specific component variations (borders, icons)
- Theme preview/selection command

## Risk Mitigation

- **Theme File Missing**: Automatic fallback to hardcoded default theme
- **Invalid Colors**: Color validation with fallback values  
- **Performance Impact**: Theme loading once at startup, styles cached
- **Breaking Changes**: All changes are additive to existing code
