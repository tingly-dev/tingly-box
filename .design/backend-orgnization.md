# Backend Organization

Living document. Records the audit of the Go backend, the target layout, and the
status of each reorganization step. Update the **Status** table as steps land.

Audit date: 2026-10-01. LOC are non-test Go lines unless noted. Items marked
*(inferred)* come from structure/naming, not function-level diffs.

## 1. Findings

### 1.1 Layering inversions

| Problem | Evidence |
|---|---|
| `internal/server/config` (7.6k LOC) does not depend on `server` but is a core dependency of the repo | 35 non-test files outside `internal/server/` import it: command, protocolserver, usecase, agent, dataio, protocoltest, mcp, tbclient, app, appconfig, middleware, probe, vision |
| It mixes six concerns | core config + watcher; routing rules; providers/profiles/scenarios; guardrails/enterprise settings; client-config writers (`apply_config_*`, ~2k LOC); migration (`migration.go`) |
| `Server` is a ~60-field god-struct | `internal/server/server.go` |
| No common module contract | `RegisterRoutes` has 5+ signatures; ad hoc constructors and auth-middleware plumbing |
| Root-level handlers that are really modules | `guardrails_handler.go` (2310 LOC, largest file), `log_handler`, `load_balance_handler` (+simulator), `token_handler`, `trace_handler`, `status_handler`, `model_request_handler` |
| Non-HTTP things under `server/module/` | `tokenrefresh` (background worker), `quotawindow`, `providerquota.Manager` (runtime services) |
| Load balancing split across two packages | `server/load_balance_handler.go` vs `protocolserver/load_balance.go` |

### 1.2 Dev/test code in production packages

- `internal/protocoltest` (~12.7k): only `cli/harness` imports it (non-test). Never imported by server code.
- `cli/harness` (~4k): standalone `package main`, no importers.
- `vmodel` (~8.9k): production (`server`, `module/virtualmodel`) mixed with test tooling (`benchmark`, `vmodeltest`). `protocoltest/virtual_server.go` vs `vmodel/virtualserver` may be redundant.
- `pkg/notify`: server uses only the `system` provider + `Multiplexer`; webhook/discord/email/slack providers + `examples` (~1.3k) have no production importer. **Needs product decision.**
- `internal/servertest` (3.3k): all `_test.go`.
- `internal/task` (960): zero importers. `.design/db.md:199` wrongly claims it is the real task subsystem.

### 1.3 Duplication

- **Protocol stack.** `stage` is *not* a duplicate: `stage/*bridge` wrap `stream`/`nonstream`, and `protocolserver`, `toolengine`, `guardrails` use it on the production path. The open question is whether `stream` is eventually folded into `stage`. Each conversion pair (anthropic↔openai↔google) has parallel implementations in `request`, `stream`, `nonstream` *(inferred)*.
- Claude Code impersonation is spread over `protocol/ops`, `protocol/transform`, `client/claude_*`.
- `max_tokens` and `thinking` rewrites each exist 2–3 times (`protocol/ops`, `protocol/transform`, `protocolserver/transform`).
- `protocolserver/transform` has 2 importers → fold into `protocolserver` or `protocol/transform`.
- `client`: xai/kimi/opencode/claude round-trippers are near-clones (header stamping); one-line `ListModels` delegates; `probe_rewrite.go` belongs in `internal/probe`.
- Cross-cutting handler patterns: 4+ error shapes (~150 `gin.H{"error":…}`, 303 `"success": false`, `apierr` used in 3 files); no shared bind/validate (81 `ShouldBindJSON`); no shared pagination; CORS applied twice (the two copies were byte-for-byte equivalent — the audit's first reading of "inconsistent" was wrong; the swagger-group copy never ran because the engine-level one answers OPTIONS via the NoRoute chain).
- Tool layers: `toolengine.ServerToolExecutor` and `servertool.Executor` both "execute one MCP call"; two incompatible `ToolCall` types.
- Session persistence in 4 places (`afk/session`, `agentboot/history`, `remote/session`, DB remote tables) — needs a field-level diff before merging.
- OAuth CLI (`command/oauth.go`) re-implements flow orchestration and keeps its own `supportedProviders()` *(inferred)*.

### 1.4 Module boundaries (go.work)

Keep: `ai`, `imbot`, `agentboot`, `gui/wails3`. Weak: `swagger` (sole consumer `internal/server`), `afk` (sole consumer `remote/control`). Root `go.mod` carries redundant `replace`s; `ai`/`imbot` declare go 1.25.6 vs 1.26. `pkg/{daemon,envsubst,fs,network}` are tiny with no external consumers; `pkg/auth` is JWT/API-token signing, not OAuth.

### 1.5 Stale docs / leftovers

- `internal/otel/README.md` and `.design/otel.md` say `pkg/otel`.
- `guardrails_runtime_adapter.go` unexported wrappers kept "so pre-move call sites compile unchanged"; `webui_handler.go` `WebHandler` extraction left half-done.
- `server.go` comment references nonexistent `module/visionproxy`.

## 2. Target domains

| Domain | Packages |
|---|---|
| Gateway core | server (route shell), protocol, protocolserver, client, routing, loadbalance, forwarding, typ, catalog, vision, probe |
| Config | `internal/config` (+ `clientconfig`, `migration` later) |
| Credentials | ai/oauth, ai/quota, pkg/auth, module/oauth, tokenrefresh |
| Observability | obs, otel, recording |
| Agent runtime | afk, agentboot, desk, remote, imbot |
| Guardrails | guardrails |
| MCP/tools | mcp, tool, toolengine, servertool |
| Persistence | db, dataio, lock |
| CLI/GUI | command, cli, gui |
| Dev/test tooling | protocoltest, harness, vmodel test parts, servertest |

## 3. Steps and status

Steps are stacked as separate commits on one branch, in this order.

| # | Step | Status |
|---|---|---|
| 0 | This document | done |
| 1 | `internal/server/config` → `internal/config` (pure move, no behaviour change) | done |
| 2 | Shared HTTP helpers: `apierr`, `bind`, `paginate`; remove duplicate CORS | done (see §4) |
| 3 | `Module` interface + migrate module registration | todo |
| 4 | Move non-HTTP modules out of `server/module/` (`tokenrefresh`, `quotawindow`, `providerquota`) | todo |
| 5 | Split `guardrails_handler.go` into a module | todo |
| 6 | Remove `webui_handler.go` / `guardrails_runtime_adapter.go` migration leftovers | todo |
| later | Delete `internal/task`; relocate `protocoltest`/`harness`; protocol/client dedupe; session-store diff; `pkg/notify` decision; fold `swagger`/`afk` | not started |

## 4. Decisions

- Pure moves are done with `tingly-go move` so they stay type-checked and mechanical; each step is verified with `go build ./... && go vet ./...` plus tests of the touched packages.
- Behaviour changes (error shape, CORS) are kept out of the pure-move commits.

- **Error shapes are wire contract.** Three shapes exist (`{"error":{message,type}}`, `{"error":"…"}`, `{"success":false,"error":"…"}`) and the web UI consumes each, so `apierr` offers one named writer per shape (`Send`, `Message`, `Failure`) instead of unifying them. Unifying shapes is a separate, frontend-coordinated change. `bind.JSON` takes the writer so each endpoint keeps its shape.
- Step 2 migrated every single-key `gin.H{"error": …}` / `{"success": false, "error": …}` response under `internal/server` (~460 lines) and 25 `ShouldBindJSON` sites that answered with the bare binder error. 33 other bind sites use bespoke messages and are left as-is; so are 11 multi-key error bodies.
- `paginate.Limit/Offset` replaced four hand-rolled "default 100, clamp N" parsers (`log_handler` ×3, `model_request_handler`, `sharing`). `desk`'s limit keeps its own semantics (0 = service default, unclamped).
- `sharing`'s private `sendError` was a copy of `apierr.Send`; removed.

## 5. Open questions

1. `pkg/notify` extra providers: reserved feature or delete?
2. Fold `swagger` and `afk` into the root module?
3. Long-term direction of `stage` vs `stream`.
