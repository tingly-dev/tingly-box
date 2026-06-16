// Package middleware provides Gin middleware for the tingly-box server.
//
// # Middleware Stack
//
// Middleware is applied globally in server.setupMiddleware() in the order below.
// Every inbound request passes through all layers before reaching a handler.
//
//	Request
//	  │
//	  ├─ gin.Recovery          — panic → 500, prevents process crash
//	  ├─ MultiModeMemoryLog    — structured HTTP log + in-memory ring buffer
//	  ├─ ErrorLog              — debug/error log with expr-based path filter
//	  ├─ CORS                  — Access-Control-* headers
//	  └─ Auth (per-route)      — UserAuth or ModelAuth, applied at route level
//
// # Components
//
// MultiModeMemoryLogMiddleware (multi_mode_memory_log.go)
//
// Logs every HTTP request to both a persistent multi-mode logger (text + JSON
// file via pkg/obs.MultiLogger) and an in-memory circular buffer (500 entries).
//
// For AI-routed requests, the log entry is enriched with routing metadata after
// the handler returns — these fields are written into the gin context by
// SetTrackingContext (internal/server/tracking_context.go):
//
//   - request_model   — model name the client requested
//   - routed_model    — model name actually forwarded to the provider
//   - routed_provider — provider name selected by the routing pipeline
//   - scenario        — agent scenario (e.g. "claude_code", "openai")
//
// Non-AI routes (system/management APIs) produce no routing fields.
// Request bodies are stored only for 4xx/5xx responses to limit memory use;
// they are referenced by a body_ref ID that can be retrieved via
// GET /api/v1/log/request/:id. The in-memory diagnostic structures are bounded
// on two axes — entry/body count AND a total-byte budget (LRU-evicted) — so a
// burst of large error bodies cannot grow the heap without limit.
//
// BadRequestSink (bad_request_sink.go)
//
// A dedicated, expr-filtered disk sink (not a middleware) owned by the unified
// MultiModeMemoryLogMiddleware. It writes filtered request/response detail to a
// lumberjack-rotated file (<configDir>/log/bad_requests.log). An Expr expression
// controls which requests are recorded; the default captures anything under
// /api/ or /tbe/ with a 4xx/5xx status, and can be reloaded at runtime. Bodies
// are captured once by the unified middleware and fed to this sink (no second
// buffering pass); the on-disk copy keeps the full captured body for diagnostic
// fidelity even when the tighter in-memory store truncates it, and flags
// request_body_truncated / response_body_truncated when the body was clipped.
//
// AuthMiddleware (auth.go)
//
// Two distinct auth modes are applied at route registration time:
//
//   - UserAuthMiddleware — web-UI routes; validates a static bearer token from
//     config; sets client_id="user_authenticated".
//
//   - ModelAuthMiddleware — AI-endpoint routes; supports three methods in
//     priority order:
//     1. JWT API tokens (multi-tenant, "tb-share-*" prefix, validated from DB)
//     2. Global config token ("tingly-box-*" prefix)
//     3. Enterprise context JWT (X-TBE-Context-JWT header, HS256/RS256)
//
// CORS (cors.go)
//
// Applies permissive Access-Control-Allow-* headers required for the
// single-page web UI.  Preflight OPTIONS requests are handled and short-
// circuited before auth runs.
//
// RateLimit (ratelimit.go)
//
// Token-bucket rate limiter keyed by client IP.  Limits are configurable
// per scenario and fall back to a global default.
package middleware
