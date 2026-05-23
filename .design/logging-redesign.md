# Logging Redesign: Correlated Model-Request Traces

Status: in progress on branch `claude/lucid-keller-hn71e`.
Route chosen: **lightweight logrus correlation** (not a rewrite onto the
recording pipeline). Frontend target: **two views (Requests / System)**
with smart-routing folded in as a per-request drill-down.

## Why

The Logs page splits logs into three tabs — **Model / System / Smart** —
and the split backfires:

- The "Model Requests" tab is not model-centric. It and "System Logs"
  call the **same** endpoint `/api/v1/system/logs`; the only difference
  is a client-side filter `pathPrefix="/tingly/"`
  (`frontend/src/pages/system/LogsPage.tsx:228`). What it shows is the
  HTTP **access log** (status / latency / path) produced by
  `internal/server/middleware/multi_mode_memory_log.go` — no model
  semantics: no conversion, no upstream call, no error reason. Hence
  "model 里看不到对应日志".
- **protocol and client logs leak into "System".** Both packages call
  the global `logrus.*` directly (no structured fields, no context). In
  `pkg/obs/multi_logger.go:316`, `WriteEntry` defaults a sourceless entry
  to `system`. So the most request-relevant detail (conversion warnings,
  client errors, retries, auth) sinks into the System tab, disconnected
  from the request it belongs to. Hence "protocol 和 client 日志不充分，
  也没落入 model request 范畴".
- **No correlation id.** A single request scatters across four places —
  inbound access log, smart-routing decision, protocol conversion
  (leaked to system), upstream client call (leaked to system) — with no
  shared id tying them together. Even the recording pipeline's
  `obs.Record.RequestID` is freshly generated at emit time
  (`internal/server/protocol_recording.go:307`), not threaded through.
- **Wrong axis.** The split is by transport path prefix (an accidental,
  leaky boundary), not by semantics. That is why the split feels
  counterproductive.

### Three parallel observability systems (context)

The Logs page only consumes (A), the weakest of the three:

| System | Location | Captures | Surfaced in |
|---|---|---|---|
| A. logrus logging (`pkg/obs.MultiLogger`) | `pkg/obs/multi_logger.go` | text/json/memory, bucketed by source | **Logs page** (model/system/smart) |
| B. request recording (`internal/obs.Record/Sink`, `ProtocolRecorder`) | `internal/server/protocol_recording.go`, `internal/obs/` | original→transformed request, response, stream chunks, steps, duration | Prompt recording page; **off by default**, opt-in per scenario |
| C. usage tracking (`UsageTracker`) | `internal/server/tracking.go` | tokens, provider, model, latency | Dashboard / DB |

This redesign fixes (A). It deliberately keeps (B) and (C) separate, but
aligns (A)'s correlation id with (B)'s `RequestID` so a later
convergence onto a single source of truth stays open.

## What this is not

- **Not** a rewrite onto `internal/obs.Record`. Full request/response
  bodies stay the job of the recording pipeline (B); the Requests view
  links to a recording when one exists, otherwise shows structured stage
  logs.
- **Not** a new persistent store. Aggregation is over the existing
  in-memory sinks + JSON log; no DB table is added.
- **Not** a change to usage tracking or load-balancing behavior.

## How

### Core idea

A "model request" becomes a **correlated trace**: one `request_id`
threaded through the whole pipeline, and logs categorized by **scope +
stage** instead of transport path.

- scope `model_request` → everything tied to a `request_id`
- scope `system` → genuine non-request logs (startup, config, jobs,
  unattached panics)
- stage ∈ `inbound | routing | transform | upstream | response`

### Backend

1. **request_id (foundation).** Earliest AI-route middleware generates
   `request_id` (reuse `X-Request-Id` header, else uuid). Store in
   `c.Set("request_id", id)` and inject into `c.Request.Context()` so
   protocol (which already receives a context via
   `internal/protocol/transform/chain.go:87 WithContext`) and client can
   read it. In the same place, build a bound entry
   `multiLogger.GetLogrusLogger(LogSourceModelRequest).WithField("request_id", id)`
   and stash it in the context so downstream code logs without holding
   the MultiLogger. Make `ProtocolRecorder.emit` reuse this id instead of
   `uuid.New()` so (A) and (B) line up.

2. **New source + scoped logger helper.** Add
   `LogSourceModelRequest = "model_request"` and a memory sink in
   `pkg/obs/multi_logger.go`. Add `obs.LogFromContext(ctx) *logrus.Entry`
   returning a no-op-safe entry when no request id / logger is present.
   Field convention: `source=model_request` + `request_id` + `stage`
   (+ `provider/model/attempt/status/latency_ms` as relevant).

3. **Migrate protocol/client logging (the bulk).** Replace global
   `logrus.*` in `internal/protocol/` and `internal/client/` with
   `obs.LogFromContext(ctx).WithField("stage", ...)`, threading `ctx`
   where missing. Add structured fields (transform steps, upstream
   status, retry attempt, latency). This is what fixes both the
   "insufficient" and the "leaked to system" problems.

4. **Access log carries request_id.**
   `internal/server/middleware/multi_mode_memory_log.go` adds
   `request_id` to its fields so the inbound envelope joins the stage
   logs by id.

5. **API (define model + swagger first, per CLAUDE.md).**
   - `GET /api/v1/requests` — list aggregated by `request_id` (join of
     http envelope + model_request + smart_routing sources); filters
     `scenario/provider/model/status/level/limit`.
   - `GET /api/v1/requests/:id` — full per-request stage timeline.
   - `/api/v1/system/logs` narrowed to **only** the `system` source
     (exclude http/model_request/smart_routing) so the System tab is
     genuinely system-only.

### Frontend

- `LogsPage.tsx`: three tabs → two tabs **Requests / System**; drop the
  `pathPrefix="/tingly/"` hack.
- New `RequestsViewer`: one row per request (time / scenario /
  provider→model / status / latency / error badge), expandable to a
  stage timeline (inbound → routing → transform → upstream → response).
  Reuse `SmartRoutingLogViewer`'s trace rendering for the routing stage.
- System tab keeps `SystemLogViewer` (no path filter; now truly system).
- "View full request/response" entry on a row opens the recording (B)
  when present, else shows structured stage logs.

> Note: client API SDK is codegen (swagger). New endpoints land as
> placeholders on the frontend side until regenerated — see CLAUDE.md.

## Sequencing (separate PRs/commits)

1. **Foundation** — request_id generation/propagation +
   `LogSourceModelRequest` + `LogFromContext` + access log carries id.
   No behavior change; independently verifiable. ← *starting here*
2. **Stop the bleed** — migrate protocol/client to the scoped logger.
   Even with no frontend change, this re-attaches the leaked logs and
   makes them correlatable by id.
3. **API + frontend two views.**
4. **Cleanup** — narrow the System endpoint, remove the path-prefix hack,
   fold smart routing into the drill-down.

## Risks

- Deep protocol/client call sites may lack a `ctx`; each needs threading
  (most time-consuming, most error-prone — but it is the root cause of
  the missing logs).
- Per-`request_id` aggregation over ring buffers: the http sink (1000)
  and the model_request sink must have comparable capacity, or a
  request's stage logs may be evicted before its envelope when expanded.
  Keep capacities aligned.
