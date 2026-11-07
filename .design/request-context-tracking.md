# Request Context Tracking and Logging Design

**Status**: Implementation in progress
**Related**: `.design/logging-redesign.md` (UI/architecture focus)

---

## Overview

This document defines the standard approach for tracking request context throughout the request lifecycle and ensuring consistent logging across all handler stages, particularly during failover scenarios.

## Problem Statement

### Context Fragmentation

Previously, request context (provider, model, service ID) was set in multiple places with different key names:

- `"provider"` / `"model"` in handlers
- `constant.CtxKeyProvider` / `constant.CtxKeyModel` in middleware
- Inconsistent updates during failover retries

This caused several issues:

1. **Incorrect final logging**: After successful failover, logs showed the initially-selected failed model instead of the final successful model
2. **Scattered updates**: Context updates were duplicated across multiple files
3. **SSOT violation**: No single source of truth for context updates

### Example Bug

```
Request flow:
1. Initial selection: virtual-fail-500 (affinity lock)
2. First attempt: fails with 500
3. Failover retry: switches to echo-model
4. Second attempt: succeeds with 200

But final log shows:
  lb_service_id=vmodel-builtin-anthropic/virtual-fail-500  (WRONG)
  routed_model=virtual-fail-500                             (WRONG)

Should show:
  lb_service_id=vmodel-builtin-anthropic/echo-model         (CORRECT)
  routed_model=echo-model                                    (CORRECT)
```

---

## Solution Design

### SSOT Principles

1. **Single canonical source**: All context keys defined in `constant/tracking_ctx.go`
2. **Standardized functions**: All updates use functions from `internal/server/tracking_context.go`
3. **Consistent lifecycle**: Context set once at start, updated only at specific transition points

### Context Keys

All keys defined in `internal/constant/tracking_ctx.go`:

```go
const (
    CtxKeyRule           = "tracking_rule"
    CtxKeyProvider       = "tracking_provider"         // *typ.Provider
    CtxKeyModel          = "tracking_model"            // string (actual model used)
    CtxKeyRequestModel   = "tracking_request_model"    // string (user requested)
    CtxKeyScenario       = "tracking_scenario"
    CtxKeyStreamed       = "tracking_streamed"
    CtxKeyStartTime      = "tracking_start_time"
    CtxKeyFirstTokenTime = "tracking_first_token_time"
    CtxKeyCacheHit       = "tracking_cache_hit"
    CtxKeySessionID      = "tracking_session_id"
    CtxKeyLBServiceID    = "tracking_lb_service_id"
    CtxKeyLBTactic       = "tracking_lb_tactic"
)
```

### Standardized Functions

All functions in `internal/server/tracking_context.go`:

#### 1. Initial Context Setup

```go
// SetTrackingContext - Set all tracking metadata at request start
func SetTrackingContext(c *gin.Context, rule *typ.Rule, provider *typ.Provider, 
    actualModel, requestModel string, streamed bool)
```

**When to call**: Once at the beginning of request processing, after routing selection

**Sets**:
- `CtxKeyRule`, `CtxKeyProvider`, `CtxKeyModel`
- `CtxKeyRequestModel`, `CtxKeyStreamed`, `CtxKeyStartTime`
- `CtxKeyScenario` (extracted from path if not set)

#### 2. Failover Context Updates

```go
// UpdateTrackingForFailover - Update provider/model during failover retry
func UpdateTrackingForFailover(c *gin.Context, provider *typ.Provider, model string)
```

**When to call**: In each failover retry attempt, before executing the attempt

**Sets**:
- `CtxKeyProvider` (new provider for this attempt)
- `CtxKeyModel` (new model for this attempt)
- `CtxKeyLBServiceID` (new service ID for this attempt)

**Why needed**: Ensures logging/middleware see the current attempt, not the initial failed attempt

#### 3. Specialized Updates

```go
// SetCacheHit - Record cache hit status
func SetCacheHit(c *gin.Context, isHit bool)

// GetFirstTokenTime - Retrieve first token time
func GetFirstTokenTime(c *gin.Context) (time.Time, bool)

// CalculateTTFT - Calculate Time To First Token
func CalculateTTFT(c *gin.Context) int64
```

---

## Request Lifecycle

### Normal Flow (No Failover)

```
1. Request arrives
   ↓
2. Routing selection (affinity/smart_routing/lb)
   - Selects provider, service, model
   - Sets CtxKeyLBServiceID, CtxKeyLBTactic
   ↓
3. Handler entry
   - Calls SetTrackingContext() with selected provider/model
   ↓
4. Transform and dispatch
   - Uses reqCtx.RequestModel (set to selected model)
   ↓
5. Provider call
   - Returns success or error
   ↓
6. Middleware logging
   - Reads CtxKeyModel, CtxKeyProvider, CtxKeyLBServiceID
   - Logs final request details
```

### Failover Flow

```
1. Request arrives
   ↓
2. Routing selection
   - Selects virtual-fail-500 (affinity lock)
   - Sets CtxKeyLBServiceID=vmodel-builtin-anthropic/virtual-fail-500
   ↓
3. Handler entry
   - Calls SetTrackingContext(provider=Virtual, model=virtual-fail-500)
   ↓
4. Failover dispatch starts
   - Logs failover_start with initial context
   ↓
5. First attempt
   - Logs failover_attempt 1/2: trying virtual-fail-500
   - Calls UpdateTrackingForFailover(provider=Virtual, model=virtual-fail-500)
   - Executes attempt → fails with 500
   - Logs failover_retry (WARN): switching to echo-model
   ↓
6. Second attempt
   - Logs failover_attempt 2/2: trying echo-model
   - Calls UpdateTrackingForFailover(provider=Virtual, model=echo-model)
   - Executes attempt → succeeds with 200
   - Logs failover_success: succeeded on attempt 2
   ↓
7. Middleware logging
   - Reads CtxKeyModel=echo-model (CORRECT!)
   - Reads CtxKeyProvider=Virtual (Anthropic)
   - Reads CtxKeyLBServiceID=vmodel-builtin-anthropic/echo-model (CORRECT!)
   - Logs final request details with CORRECT model
```

---

## Implementation Guidelines

### For Handler Authors

#### When implementing a new AI handler:

1. **After routing selection**, call `SetTrackingContext()`:
```go
provider, service, err := s.selectProvider(c, rule)
// ...
SetTrackingContext(c, rule, provider, actualModel, requestModel, isStreaming)
```

2. **During failover**, use `dispatchWithPriorityFailover` - it automatically calls `UpdateTrackingForFailover()`:
```go
s.dispatchWithPriorityFailover(c, rule, provider, actualModel,
    func(p *typ.Provider, retryModel string) {
        reqCtx.RequestModel = retryModel  // Important: update request context
        // ... dispatch logic
    })
```

3. **Never manually set context keys** - always use the standardized functions

### For Framework Authors

#### When adding new tracking metadata:

1. **Add key to `constant/tracking_ctx.go`**:
```go
CtxKeyNewMetadata = "tracking_new_metadata"
```

2. **Add alias in `server/tracking_context.go`**:
```go
ContextKeyNewMetadata = constant.CtxKeyNewMetadata
```

3. **Add getter/setter in `server/tracking_context.go`**:
```go
func SetNewMetadata(c *gin.Context, value string) {
    c.Set(ContextKeyNewMetadata, value)
}

func GetNewMetadata(c *gin.Context) string {
    if val, exists := c.Get(ContextKeyNewMetadata); exists {
        return val.(string)
    }
    return ""
}
```

4. **Update `SetTrackingContext()` if needed at request start**

5. **Update `UpdateTrackingForFailover()` if needed during retry**

---

## Middleware Integration

### Logging Middleware

**File**: `internal/server/middleware/multi_mode_memory_log.go`

The middleware reads context keys set by handlers:

```go
// Read tracking metadata
if am := c.GetString(constant.CtxKeyModel); am != "" {
    fields["routed_model"] = am
}
if pv, exists := c.Get(constant.CtxKeyProvider); exists {
    if p, ok := pv.(*typ.Provider); ok && p != nil {
        fields["routed_provider"] = p.Name
        fields["api_style"] = string(p.APIStyle)
        fields["base_url"] = p.APIBase
    }
}
if svcID := c.GetString(constant.CtxKeyLBServiceID); svcID != "" {
    fields["lb_service_id"] = svcID
}
```

**Critical**: These values are read at **request end**, so they must reflect the **final successful state** after failover.

---

## Testing

### Unit Tests

Test context updates in failover scenarios:

```go
func TestUpdateTrackingForFailover(t *testing.T) {
    c, _ := gin.CreateTestContext(nil)
    
    provider := &typ.Provider{UUID: "prov-1", Name: "TestProvider"}
    model := "test-model"
    
    UpdateTrackingForFailover(c, provider, model)
    
    // Verify context was updated
    assert.Equal(t, model, c.GetString(constant.CtxKeyModel))
    assert.Equal(t, provider, c.Get(constant.CtxKeyProvider))
    assert.Equal(t, "prov-1/test-model", c.GetString(constant.CtxKeyLBServiceID))
}
```

### Integration Tests

Test complete failover flow with logging verification:

```go
func TestFailoverLoggingContext(t *testing.T) {
    // Setup: Configure rule with T1=fail-500, T2=echo-model
    // Enable affinity to lock to T1
    
    // Act: Send request
    resp := callAPI(t, "virtual-fail-500")
    
    // Assert: Response succeeds (failover worked)
    assert.Equal(t, 200, resp.StatusCode)
    
    // Assert: Final log shows echo-model, not virtual-fail-500
    logs := captureLogs(t)
    finalLog := findFinalLog(logs)
    assert.Equal(t, "echo-model", finalLog["routed_model"])
    assert.Equal(t, "vmodel-builtin-anthropic/echo-model", finalLog["lb_service_id"])
}
```

---

## Future Improvements

### 1. Structured Context Object

Consider encapsulating context in a struct:

```go
type TrackingContext struct {
    Rule         *typ.Rule
    Provider     *typ.Provider
    Model        string
    RequestModel string
    Scenario     string
    // ... other fields
}

func (tc *TrackingContext) ApplyTo(c *gin.Context) {
    c.Set(ContextKeyRule, tc.Rule)
    c.Set(ContextKeyProvider, tc.Provider)
    // ... etc
}
```

### 2. Context Validation

Add validation to ensure context is set correctly:

```go
func ValidateTrackingContext(c *gin.Context) error {
    if c.GetString(constant.CtxKeyModel) == "" {
        return errors.New("CtxKeyModel not set")
    }
    // ... other validations
}
```

### 3. Automatic Context Propagation

Consider middleware that automatically propagates context through goroutine boundaries for concurrent processing.

---

## References

- **Architecture**: `.design/logging-redesign.md`
- **Constants**: `internal/constant/tracking_ctx.go`
- **Functions**: `internal/server/tracking_context.go`
- **Failover**: `internal/server/failover_dispatch.go`
- **Middleware**: `internal/server/middleware/multi_mode_memory_log.go`
- **Implementation**: `.sdlc/docs/failover-complete-fix-20260617.md`
