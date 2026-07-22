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
//
// The access log deliberately records no request/response bodies. Mirroring
// bodies here (wrapping c.Request.Body / c.Writer) is unstable — it interferes
// with streaming, Flush/Hijack, and large or Expect-100-continue uploads — for
// little gain. Bodies that matter for diagnosis are recorded where they are
// understood: the handler, and the model_request client stage (correlated to
// this entry by request_id).
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
//
// ClearServerIOTimeouts (io_timeout.go)
//
// Applied to the AI protocol route groups (/tingly/:scenario[/v1]) only.
// Clears the per-connection read/write deadlines armed by http.Server's
// ReadTimeout/WriteTimeout so long-running SSE streams and large request
// bodies are bounded by the upstream provider timeout and client disconnect,
// not by wall-clock from request start (issue #1384).
package middleware
