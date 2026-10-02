# Design Specification: Console View Scrolling Support

## Issue Summary
**Issue #57**: Console view does not support scrolling to view previous content

The main console view in kiro-krew lacks scrolling capability for long sessions with watch commands. Users cannot access content that has scrolled off-screen.

## Current Architecture Analysis

### Current Console Rendering
- Main console uses `activityLines []string` to store content
- Rendering in `renderBaseView()` truncates to visible area: `lines[len(lines)-activityHeight:]`
- No scrolling mechanism - only shows most recent content
- Prompt box remains at bottom with separator

### Existing Scrolling Implementation
- Agent output view already has scrolling via `viewport.Model` in `OutputView`
- ViewManager handles switching between console and agent output views
- Mouse wheel/keyboard events work in agent output view

## Proposed Solution

### 1. Add Viewport to Console View
Create a console-specific viewport similar to OutputView:
- Add viewport to main model struct
- Initialize viewport in console mode
- Handle mouse wheel, trackpad, and keyboard events
- Preserve existing prompt box functionality

### 2. Implementation Changes

#### File: `internal/tui/tui.go`
- Add `consoleViewport viewport.Model` to model struct
- Initialize in `newModel()` 
- Update `renderBaseView()` to use viewport content
- Handle scroll events in `Update()` method for console view
- Forward mouse wheel and keyboard scroll events to viewport

#### Key Events to Handle:
- Mouse wheel up/down
- Page Up/Page Down keys  
- Arrow up/down for line-by-line scrolling
- Home/End for jumping to top/bottom

### 3. Scroll State Management
- Maintain scroll position when switching between views
- Auto-scroll to bottom when new content arrives (unless user scrolled up)
- Visual indicator when scrolled up from bottom

### 4. Content Management
- Feed `activityLines` content to viewport
- Maintain existing `maxActivityLines` limit
- Handle viewport content updates efficiently

## Acceptance Criteria Implementation

### Mouse Wheel Scrolling
- Handle `tea.MouseWheelUpMsg` and `tea.MouseWheelDownMsg`
- Forward to console viewport when in console view
- Preserve existing behavior in agent output view

### Trackpad Scrolling
- Same mouse wheel events handle trackpad gestures
- Cross-terminal compatibility maintained

### Previous Content Access
- All content in `activityLines` accessible via scrolling
- Respect `maxActivityLines` configuration limit

### Prompt Box Functionality
- Prompt remains fixed at bottom
- Input handling preserved when viewport has focus
- Visual separation maintained

### No Shortcut Interference  
- Preserve existing hotkeys (F1 help, F2 toggle views, etc.)
- Only intercept scroll-related keys when in console view

## Technical Details

### Viewport Configuration
```go
viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
```

### Event Handling Priority
1. Existing overlay handling (highest priority)
2. Console viewport scroll events (console view only)  
3. View switching (F2)
4. Existing hotkeys
5. Input handling (lowest priority)

### Content Synchronization
- Update viewport content when `activityLines` changes
- Maintain scroll position during content updates
- Auto-scroll to bottom for new content (unless manually scrolled up)

## Files to Modify
1. `internal/tui/tui.go` - Main implementation
2. `internal/tui/view_state.go` - Console scroll state preservation (if needed)

## Testing Requirements
1. Mouse wheel scrolling works in various terminals
2. Keyboard navigation (PgUp/PgDown, arrows)
3. Content accessibility during long sessions
4. Prompt functionality preserved
5. View switching maintains scroll positions
6. No regression in existing hotkeys

## Implementation Approach
1. Add console viewport to model struct
2. Initialize viewport in newModel()
3. Modify renderBaseView() to use viewport
4. Add scroll event handling in Update()
5. Test across different terminal applications
