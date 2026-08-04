// Package middleware provides the Gin middleware used by the tingly-box
// server: HTTP access logging, CORS, authentication, response compression,
// per-route rate limiting, and IO-deadline management for streaming routes.
//
// # Global vs per-route
//
// Only a thin stack runs globally on every request (server.setupMiddleware in
// internal/server/server_routes.go):
//
//	Request
//	  │
//	  ├─ gin.Recovery   — panic → 500, prevents process crash
//	  ├─ MemoryLog      — structured HTTP access log + in-memory ring buffer
//	  └─ CORS           — Access-Control-* headers
//
// Auth, rate limiting, gzip, and IO-timeout clearing are NOT global — each is
// applied to the specific route group that needs it at registration time.
//
// # Components
//
// MemoryLog (memory_log.go)
//
// Logs one structured entry per request — method, path, status, latency,
// error — to the multi-mode logger (text + JSON files via pkg/obs.MultiLogger)
// and an in-memory ring buffer (50 entries per source, pkg/obs.defaultMemorySinkEntries).
//
// For AI-routed requests, the entry is enriched with routing metadata after
// the handler returns. These fields are written into the gin context by
// SetTrackingContext (internal/protocolserver/tracking_context.go) and read
// back here:
//
//   - request_model   — model name the client requested
//   - routed_model    — model name actually forwarded to the provider
//   - routed_provider — provider name selected by the routing pipeline
//   - api_style       — provider API style (e.g. openai, anthropic)
//   - base_url        — provider API base URL
//   - scenario        — agent scenario (e.g. "claude_code", "openai")
//   - lb_service_id   — load-balancer service id chosen for this request
//   - lb_tactic       — load-balancer tactic name
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
// Two distinct auth modes, each applied to its route group at registration:
//
//   - UserAuthMiddleware — web-UI / management routes; validates a static
//     bearer token from config; on success sets user_id to the default admin
//     (db.DefaultAdminUserID) so usage records have a stable owner.
//
//   - ModelAuthMiddleware — AI-endpoint routes; supports three methods in
//     priority order:
//     1. JWT API tokens (multi-tenant, "tb-share-*" prefix, validated from DB)
//     2. Global config model token ("tingly-box-*" prefix)
//     3. Enterprise context JWT (X-TBE-Context-JWT header, HS256/RS256)
//     A token carrying the "sk-tbe-" virtual-key prefix is rejected here with
//     a pointer to the dedicated /tbe/* endpoints.
//
// CORS (cors.go)
//
// Applies permissive Access-Control-Allow-* headers required for the
// single-page web UI.  Preflight OPTIONS requests are handled and short-
// circuited before auth runs.
//
// Gzip (gzip.go)
//
// Per-route response compression for endpoints that can return large JSON
// (usage stats, time series, records). Registered via
// swagger.WithMiddleware(middleware.Gzip()); never on streaming/SSE routes.
//
// RateLimit (ratelimit.go)
//
// A fixed-window failed-attempt blocker keyed by client IP: after maxAttempts
// POSTs within windowSize the IP is blocked for blockDuration. It is scoped to
// specific auth paths passed to RateLimitMiddleware. Note: it is a failed-
// attempt limiter, not a token bucket, and it is not wired into the default
// route stack today.
//
// ClearServerIOTimeouts (io_timeout.go)
//
// Applied to the AI protocol route groups (/tingly/:scenario and
// /tingly/:scenario/v1, see internal/protocolserver/routes.go) only.
// Clears the per-connection read/write deadlines armed by http.Server's
// ReadTimeout/WriteTimeout so long-running SSE streams and large request
// bodies are bounded by the upstream provider timeout and client disconnect,
// not by wall-clock from request start (issue #1384).
package middleware
