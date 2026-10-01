# Backend Organization

Living document. Records the audit of the Go backend, the target layout, what has
landed, and what is left. Sections 1–2 are the audit as it stood on 2026-10-01,
before any step landed; where a finding turned out wrong or has since been fixed,
§4 says so. Update §3 and §5 as work lands.

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

## 3. Status

Steps 1–6 landed as separate PRs, each one goal, in this order of dependency.

| Step | Change | PR |
|---|---|---|
| 1 | `internal/server/config` → `internal/config` (pure move) | #1927 |
| 2 | Shared HTTP helpers `apierr` / `bind` / `paginate`; duplicate CORS removed | #1929 |
| 3 | Background workers out of `server/module/` → `internal/worker/` | #1930 |
| 4 | Guardrails adapter / `WebHandler` migration leftovers cleared | #1931 |
| 5 | `module.Module` contract; guardrails admin extracted to `module/guardrails` | #1932 |
| 6 | Every HTTP module adopts the contract; one module list for server and OpenAPI | #1933 |

Everything in the table is merged. The backlog is §5.

### What the server layer looks like now

- `internal/config` is a leaf-level dependency, not a child of `server`.
- `internal/server/module/module.go` defines `Routes` (Public/V1/V2 groups, Engine, Manager, UserAuth), `Module` (`RegisterRoutes(*Routes)`) and `Mount`. Every package under `server/module/` implements it and no package-level `RegisterRoutes` functions remain. The load-balancer API (`load_balance_handler.go`) and the gateway routes (`protocolserver`) register separately and are not `Module`s.
- `internal/server/server_modules.go` (`engineModules`, `apiModules`) is the only place that decides which modules exist and how they are built. The running server and OpenAPI generation both call it, with a `schema` flag for the few runtime side effects that must not run at generation time.
- `internal/worker/` holds the background workers (`tokenrefresh`, `quotawindow`).
- Error responses go through `module/apierr`, request binding through `module/bind`, list limits/offsets through `module/paginate`.

## 4. Decisions and corrections to the audit

### Decisions

- **Pure moves use `tingly-go move`**, so they stay type-checked and mechanical. Each step was verified with `go build ./... && go vet`, the touched packages' tests, and — for anything touching routes — `openapi.json` regenerating byte-identical.
- **Behaviour changes stay out of pure-move commits.** The only intentional ones are listed below.
- **Error shapes are wire contract.** Three shapes exist (`{"error":{message,type}}`, `{"error":"…"}`, `{"success":false,"error":"…"}`) and the web UI consumes each, so `apierr` has one named writer per shape (`Send`, `Message`, `Failure`) instead of unifying them. `bind.JSON` takes the writer so each endpoint keeps its shape. Unifying shapes is a separate change that needs the frontend.
- **Scope of the helper migration.** ~460 single-key error responses and 25 `ShouldBindJSON` sites that answered with the bare binder error moved to the helpers. 33 bind sites with bespoke messages and 11 multi-key error bodies were left alone on purpose. `paginate` replaced four hand-rolled "default 100, clamp N" parsers; `desk`'s limit keeps its own semantics (0 = service default, unclamped). `sharing`'s private `sendError` was a copy of `apierr.Send` and is gone.
- **One module list.** The server and OpenAPI generation each used to carry a ~100-line copy of the wiring, and they had already drifted (sharing/team, `imbot` error handling, desk). Handler-owned extras that used to be parameters moved onto the handler (`desk.Handler.WithGate`, `mcp` reads its own sub-handlers, `oauth` registers its callback routes, `notify.BotAPIHandler` is its own `Module`).
- **Dependency edge removed.** `tokenrefresh` depended on the OAuth HTTP module for one 3-line `oauth.Option`; `WithKimiDeviceID` moved to `ai/oauth/options.go`.

### Intentional behaviour changes

- The swagger-group CORS middleware is gone. It never ran: the engine-level `middleware.CORS()` answers preflights through the NoRoute chain.
- `oauth.RegisterRoutes` no longer calls `router.Router.Use(authMiddleware)` on the shared `/api/v1` group, which already carried auth; previously every module registered after oauth authenticated twice. `debug` and `virtualmodel` still attach auth per route so the generated spec records it.

### Corrections to the audit (§1)

- **CORS was not "inconsistent".** The two copies were byte-for-byte equivalent; the problem was only duplication.
- **`providerquota` is an HTTP module**, not a runtime service: it has `handler.go` and `routes.go`, and its `Manager` is a consumer-side interface. Only `tokenrefresh` and `quotawindow` were workers.
- **`WebHandler` was not half-finished.** It carries status, log, request-trace and token handlers; only its comments (and `server.go`'s, which still said `aimodel` / `module/visionproxy`) were stale.
- **`protocol/stage` is not a duplicate of `stream`.** `stage/*bridge` wrap `stream`/`nonstream`, and `protocolserver`, `toolengine` and `guardrails` use it on the production path. The open question is whether `stream` is eventually folded into `stage`.
- Audit claims marked *(inferred)* in §1.3 are structural readings, not function-level diffs; check before acting on them.

## 5. Backlog and open questions

Not started, roughly in order of payoff and risk. Numbers are from `main` after step 6.

1. **Delete `internal/task`** (~960 LOC): zero importers outside itself. `.design/db.md` wrongly names it as the real task subsystem.
2. **Move dev/test tooling out of the production view**: `internal/protocoltest` (~12.7k LOC, imported only by `cli/harness`), `cli/harness`, `internal/servertest`, test parts of `vmodel`.
3. **`Server` is still a ~60-field struct** (`server.go`, 681 LOC). Now that modules share a contract, each module's dependencies can be injected instead of read off `*Server`.
4. **Remaining helper adoption**: 11 multi-key error bodies and ~33 bespoke `ShouldBindJSON` sites; root handlers (`log_handler`, `load_balance_handler`, `token_handler`, …) could become modules.
5. **Protocol/client dedupe**: Claude Code impersonation logic split across `protocol/ops`, `protocol/transform`, `client/claude_*`; near-clone round-trippers in `client`; `protocolserver/transform` has two importers.
6. **Session persistence** lives in four stores (`afk/session`, `agentboot/history`, `remote/session`, DB remote tables); needs a field-level diff before merging anything.
7. **Observability / docs hygiene**: `internal/otel/README.md` and `.design/otel.md` still say `pkg/otel`.

Open questions:

1. `pkg/notify`: the webhook/discord/email/slack providers and `examples` (~1.3k LOC) have no production importer — reserved feature or delete?
2. Fold `swagger` and `afk` into the root module? Each has a single consumer.
3. Long-term direction of `stage` vs `stream`.
