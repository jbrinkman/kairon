# Design Specification: GitHub Polling Optimization

**Problem:** Current polling implementation checks GitHub API every 5 minutes but causes UI freezes during network calls

**Requirements:** Maintain real-time feel, reduce API calls, prevent UI blocking, handle rate limits gracefully

## Solution Approach

Transform the synchronous polling mechanism into an asynchronous, intelligent system with multiple optimization layers:

1. **Asynchronous Polling**: Move GitHub API calls to background goroutines to prevent UI blocking
2. **Intelligent Backoff**: Implement exponential backoff with jitter to reduce unnecessary API calls
3. **Conditional Requests**: Use ETags and Last-Modified headers to avoid downloading unchanged data
4. **Rate Limit Handling**: Detect and respect GitHub rate limits with automatic throttling
5. **Event-Based Updates**: Implement webhook support for real-time notifications when available
6. **Local Caching**: Cache issue data with TTL to reduce redundant API calls

This approach maintains responsiveness while being respectful of API limits and providing near real-time updates.

## Relevant Files

### Core Implementation Files
- `internal/watcher/watcher.go` - Refactor polling loop for async operation
- `internal/github/client.go` - Add conditional request support and rate limit handling
- `internal/github/cache.go` - **NEW** - Implement local issue caching with TTL
- `internal/github/ratelimit.go` - **NEW** - Rate limit detection and backoff logic

### Configuration Files
- `internal/config/config.go` - Add new polling configuration options
- `.kiro-krew/config.yaml` - Update with new polling parameters

### TUI Integration Files
- `internal/tui/tui.go` - Add async polling status indicators
- `internal/tui/commands.go` - Add polling status and cache management commands

## Team Orchestration

### Implementation Phases
1. **Phase 1**: Implement asynchronous polling (prevents UI blocking)
2. **Phase 2**: Add intelligent caching and conditional requests (reduces API calls)
3. **Phase 3**: Implement rate limit handling (graceful degradation)
4. **Phase 4**: Add webhook support for real-time events (optional enhancement)

### Agent Coordination
- **Builder**: Implements core async polling and caching logic
- **Validator**: Verifies no UI blocking and proper rate limit handling
- **Documenter**: Updates configuration documentation

## Step-by-Step Task Breakdown

### Task 1: Implement Asynchronous Polling
**Objective:** Prevent UI freezes by moving GitHub calls to background goroutines

**Changes:**
- Modify `Watcher.pollLoop()` to use separate goroutine for API calls
- Add result channels for communicating poll results back to main thread
- Implement polling state machine (idle, polling, waiting, error)
- Add context cancellation for graceful shutdown

**Acceptance Criteria:**
- UI remains responsive during GitHub API calls
- Polling continues in background without blocking user input
- Watcher can be stopped cleanly without hanging

### Task 2: Add Intelligent Caching
**Objective:** Reduce API calls through local issue caching with TTL

**New Files:**
- `internal/github/cache.go` - Issue cache with TTL and ETag support

**Changes:**
- Add cache layer before GitHub API calls in `ListIssues()`
- Store issue data with timestamps and ETag/Last-Modified metadata
- Implement cache invalidation based on TTL (default 2 minutes)
- Add cache statistics for monitoring

**Acceptance Criteria:**
- Identical subsequent requests served from cache within TTL
- Cache automatically expires and refreshes stale data
- Cache hit/miss statistics available for monitoring

### Task 3: Implement Conditional Requests
**Objective:** Use HTTP conditional headers to avoid downloading unchanged data

**Changes:**
- Modify GitHub CLI calls to include conditional headers when available
- Store and use ETag/Last-Modified values from previous responses
- Handle 304 Not Modified responses appropriately
- Fall back gracefully when conditional headers not supported

**Acceptance Criteria:**
- GitHub API returns 304 for unchanged data when ETags match
- Bandwidth usage reduced for repeated requests of unchanged issues
- Graceful fallback when conditional requests unavailable

### Task 4: Add Rate Limit Handling
**Objective:** Detect and respect GitHub rate limits with automatic throttling

**New Files:**
- `internal/github/ratelimit.go` - Rate limit detection and backoff

**Changes:**
- Parse rate limit headers from GitHub responses
- Implement exponential backoff when approaching limits
- Add jitter to prevent thundering herd problems
- Expose rate limit status in TUI

**Acceptance Criteria:**
- Automatic throttling when rate limit headers indicate low quota
- Exponential backoff with jitter prevents rate limit violations
- Rate limit status visible to users in TUI
- Graceful degradation rather than hard failures

### Task 5: Update Configuration System
**Objective:** Add configurable parameters for polling optimization

**Configuration Additions:**
```yaml
polling:
  interval: 5m                    # Base polling interval
  cache_ttl: 2m                  # Local cache time-to-live
  rate_limit_buffer: 100         # API calls to keep in reserve
  max_backoff: 30m               # Maximum backoff interval
  enable_conditional: true       # Use ETag/Last-Modified headers
  enable_webhooks: false         # Enable webhook support (future)
```

**Changes:**
- Add polling configuration struct to `config.Config`
- Update config validation and defaults
- Maintain backward compatibility with existing `poll_interval`

**Acceptance Criteria:**
- All new polling parameters configurable via YAML
- Sensible defaults that work without configuration changes
- Backward compatibility with existing configurations

### Task 6: Add TUI Status Indicators
**Objective:** Provide visibility into polling status and performance

**Changes:**
- Add polling status indicator to main TUI view
- Show cache statistics (hit rate, size, TTL remaining)
- Display rate limit status and next refresh time
- Add commands to manually trigger polling or clear cache

**New Commands:**
- `poll now` - Trigger immediate poll ignoring cache
- `poll status` - Show detailed polling and rate limit status
- `cache clear` - Clear local issue cache
- `cache stats` - Show cache performance statistics

**Acceptance Criteria:**
- Real-time polling status visible in TUI
- Users can see when next poll will occur
- Cache and rate limit information accessible via commands
- Manual poll trigger works correctly

## Validation Commands

```bash
# Test async polling doesn't block UI
kiro-krew
# In TUI: verify input remains responsive during polling

# Test rate limit handling
# (Simulate by temporarily lowering rate limit buffer)
poll now  # Should show throttling when limits approached

# Test cache effectiveness
cache stats  # Should show hit rates after repeated polls
poll now     # Should bypass cache
poll status  # Should show cache usage

# Test configuration
# Verify all new config options load correctly
grep -A 10 "polling:" .kiro-krew/config.yaml

# Test graceful shutdown
# Start watcher, stop immediately - should not hang
watch start
watch stop
```

## Performance Improvements Expected

- **UI Responsiveness**: Eliminate 100-500ms freezes during API calls
- **API Usage**: Reduce calls by 60-80% through intelligent caching
- **Bandwidth**: Reduce data transfer by 40-60% via conditional requests  
- **Reliability**: Graceful handling of rate limits prevents service disruption
- **User Experience**: Real-time status feedback and manual controls

## Backward Compatibility

- Existing `poll_interval` configuration remains functional
- Default behavior unchanged for users who don't modify config
- All new features opt-in via configuration or gracefully degrade
- No breaking changes to existing watcher API
