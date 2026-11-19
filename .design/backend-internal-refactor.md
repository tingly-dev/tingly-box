# Backend `internal/` review and phased refactor plan

Status: **phase 1 (review) done.** Dead-code removal and the P3 boundary fixes are planned as separate PRs; P2 and P4–P5 wait for sign-off (§6, §7).
Date: 2026-11-19, measured on `main` at `f5235a0f5`; rebased onto `25e8e5f99` (upstream touched `internal/` only in codex status code, numbers unchanged).
Relation to [`backend-organization.md`](./backend-organization.md) (audit of 2026-10-01): that document's steps 1–6 landed; this one re-audits what is left, re-checks its backlog against the current tree (§5) and turns it into reviewable PRs. Behaviour must not change in any phase.

## 1. Method

Everything below is mechanical evidence, not reading impressions. Re-run it before acting on a number; line numbers are on `main` before PR 1a/1b.

- **Import graph** — `go list -f '{{.ImportPath}}|{{.Imports}}|{{.TestImports}}|{{.XTestImports}}' ./...`, then grouped by top-level package. Production and test imports are kept apart.
- **Unreferenced symbols** — a small `go/packages` + `go/types` pass (`Tests: true`) that records every package-level func/type/var/const and method defined in a non-test file under `internal/`, then counts uses from non-test files vs `_test.go` files. "Prod-unused" = zero uses outside the definition (so it includes self-recursion and uses from other dead code, which PR 1b handled by iterating to a fixpoint). `golang.org/x/tools/cmd/deadcode` panics on Go 1.26 here (`rta` nil deref), so it was not used. The result was identical for `GOOS=linux`, `windows` and `darwin` (the `!windows` / `windows` files add no differences), and nothing is behind a build tag except `e2e` tests.
- **Function length** — `go/ast` line count per `FuncDecl`.
- **Cross-check against `.design/*.md`** — every symbol PR 1b deletes was grepped in `.design/`, `.note/` and package READMEs. Anything a design doc points at was **kept** (see class B in §3.1).

Limits of the method: methods are not interface-aware (704 prod-unused methods are mostly interface implementations, so they are only counted, not listed); reflection / `go:linkname` use would not show (none found); only the root module's `internal/`, `pkg/`, `vmodel/`, `cli/` and `gui/wails3` were loaded. `gui/wails3` does not build on this box (no gtk4/webkit), but its import of `internal/` was type-checked for the symbol pass.

## 2. Architecture snapshot

`internal/` is ~152k non-test Go lines in 42 top-level directories. Biggest: `protocol` 26.0k, `server` 26.3k (incl. 30 `module/*` directories), `protocoltest` 13.9k (dev tooling), `protocolserver` 11.1k, `client` 9.4k, `config` 9.2k, `command` 8.7k, `db` 7.0k, `mcp` 6.6k, `guardrails` 4.6k, `routing` 4.2k.

Intended layering (from `backend-organization.md` §2 and the per-package design docs) and what it actually is:

```
cli/tingly-box ─▶ command ─▶ app ─▶ server ─▶ server/module/* , protocolserver
                                  │              │
 gateway core:   protocolserver ──┼─▶ routing ─▶ limits, loadbalance, obs
                   │ ▲            │      │
                   ▼ │            ▼      ▼
                 toolengine    recording  typ ◀──▶ (see §4.1)
                   │
                   ▼
   protocol ─▶ catalog ─▶ db      client ─▶ protocol, vision, vmodel/client
   config ─▶ db, client, guardrails/*, routing/smartrouting
```

What is clean and should be protected:

- **`limits`** (698 LOC) imports only `ai` and `clock`; `routing/{limits,limits_actor,stage_limits}.go` adapt it into the selector pipeline; `server/limits.go` is the wiring and `server/module/limits` the HTTP surface (`routing-limits.md`). The only smell is `server/limits.go:49-157` (policy warm-start from the usage DB, ~110 lines of logic sitting in the server root) — §3.4.
- **`loadbalance`** (1.3k) and **`clock`** are leaves (`ai`, `clock` only).
- **`usecase`** obeys its contract in `usecase-layer.md` (imports only `agent`, `config`, `typ`, `loadbalance`, `protocol`).
- **`server/module/*`**: the 30 directories under it are HTTP modules (`module.Module`, wired only in `server_modules.go`) plus three helper packages (`apierr`, `bind`, `paginate`). Apart from the helpers, `module/provider → providerquota` and `module/{projects,scenario} → statusline`, each is imported by `internal/server` only.
- **`worker/*`** has exactly one importer (`server`).
- **Boundary rules for the sibling modules hold**: `projects` is imported only by `server/server.go`, `server/server_control.go` and `server/module/projects/*` (its facade); `remote`/`imbot`/`agentboot` are only imported (`db → remote`, `command → remote`, `server → remote`).

Package-level Go import cycles do not exist (the compiler forbids them). The cycles that exist are **between top-level groups**, via different sub-packages (§4.1). They are what makes splitting or moving packages hard, so they come before any move in the plan.

## 3. Findings

Each item: evidence → problem → risk → phase (P1…P5, §6). Risk is the risk of the *change*, not of the finding.

### 3.1 Dead code

| # | Evidence | Problem | Risk | Phase |
|---|---|---|---|---|
| A1 | `internal/task/` (960 LOC + 721 LOC of tests, 10 files): zero importers in the whole workspace (`go list` graph; only its own `manager_test.go` imports it). `.design/db.md:205` and `backend-organization.md:32,123` already say so. | A whole unused task-queue subsystem. | none | **P1a** |
| A2 | ~80 package-level funcs/types/vars (plus a few whole dead files) with zero references anywhere (prod or test), e.g. `protocol/request/anthropic_v1_to_openai.go:66,71,140` (`ConvertAnthropicToolsToOpenAIWithTransformedSchema`, …), `protocol/token/anthropic.go:11,38` (`EstimateAnthropicTokens`), `server/server_types.go` (9 unused DTOs/helpers: `GenerateCurlCommand`, `NewMockRequest`, `ProbeResponse`, …), `toolengine/virtual_registry_adapter.go` (whole file), `mcp/local/adapter.go:22,125,130`, `client/claude_round_tripper.go:15,76,96` (three unused beta tables), `obs/processor.go:14`, `typ/id.go:73`, `protocol/transform/print.go` (debug printers). Full list: the P1b diff. | Dead weight that makes the protocol packages look larger and more duplicated than they are (the v1/beta conversion pairs flagged in the old audit are partly these). | none — compiler plus an `unused`-style check proves it; vet and tests pass | **P1b** |
| A3 | **Unwired features** — zero references, but a design doc or a comment says they are meant to exist: `middleware/ratelimit.go` (failed-attempt IP limiter; `middleware/doc.go:78-84` says "not wired into the default route stack today"), `middleware/cors.go:28` `CORSWithConfig`, `obs/resolver.go:21,122` (`ResolveRecord`, `WalkSession`, reader side of the recording CAS), `guardrails/core/credential_alias.go:86,121` (`AliasContent`/`RestoreContent`), `config/apply_config_claude.go:294,372` (`ApplyNotifyHooks`/`ApplyImHooks`; with them the embedded `internal/script/tingly-notify.sh` and `tingly-im-hook.sh` have no reader — only `tingly-statusline.sh` is read, `agent/rule_bridge.go:100`), `protocol/ops/request_openai_codex.go:27` (`ApplyCodexResponsesTransform`; named in `protocol-client-boundary.md:56`, `protocol-transform.md:89`, `responses-namespace-tools.md:196`), `protocol/request/responses_to_chatgpt.go` (321 LOC, `ConvertResponseInputToChatGPTFormat` …), `protocol/stream/any_to_google.go:24,214` (OpenAI/Anthropic → Google *stream* converters, 367 LOC; `ttft.md:55,71` documents `sendGoogleStreamChunk` there; the non-stream twin `protocol/nonstream/any_to_google.go` is test-only). | Not safe to delete without a decision: either finish wiring (the rate limiter is a security feature — nothing throttles failed logins today) or delete with the doc references. **Kept in P1b on purpose.** | low if deleted with docs, but a product call | decision → **P2** |
| A4 | Packages with no production importer: `protocol/assembler/streamemit` (≈530 LOC + README, `doc.go` `doc.go:21` names a consumer `internal/server/scenario_recording.go` that no longer exists), `protocol/stage/openaibridge` and `protocol/stage/responsesbridge` (≈1.07k LOC; only `protocoltest` imports them; `protocol-stage.md:12-13` lists them as the Chat/Responses bridges while `protocolserver` uses only `anthropicbridge`), `protocol/test` (3 test files only), `pkg/notify/examples` and the webhook/discord/email/slack providers (old open question #1). | Either not-yet-wired design or abandoned. | low | decision → P2 |
| A5 | Test-only production symbols: 86 funcs, 5 vars, 3 types, 11 consts, 318 methods are referenced only from `_test.go` (e.g. `config/config.go:179,186,194` `WithDisableMigration`/`WithDisableBuiltIn`/`NewDefaultConfig`, `db/*_store.go` `NewProviderStore`/`NewUsageStore`/…, `protocolserver/transform/transform_clean_header.go:116,147`, `vision/visionproxy/stub.go:38,47`). Plus helper files shipped in non-test files: `routing/test_helpers.go`, `client/transport_test_helper.go`, `advisortest/`, `recording/recordingtest/`. | Some are real test seams (keep). The ones that mirror a deleted production path are dead with their tests (`CleanSystemMessages`, `CreateHTTPClientWithProxy`, …). | low; each needs its tests removed too | P2 |
| A6 | Left out of P1b on purpose: 13 unused `With*` options (`server/server_options.go:83,146,164,173,181`, `worker/tokenrefresh/refresher.go:94,101,108`, `protocol/transform/chain.go:81,91`, …), 57 consts (iota-like enums such as `typ/skill.go` `IDESource*`, `typ/type.go:149-153` `ThinkingMode*`), 8 DTO types in `server/module/{notify,oauth,usage}`, 2 funcs in `server/module/{oauth,guardrails}/handler.go` (`generateRandomSuffix`:1367, `validateInlineGuardrailsEditing`:1770), 28 funcs in `protocoltest`. | Enum sets and option APIs are API shape, not dead logic; the module DTOs are swagger-adjacent. The two handler funcs are plain dead and can go with the `server/module` pass. | none–low | P2 |
| A7 | Stale docs: `.design/db.md:205` (`internal/task`), `.design/codex-config.md:111,229` (`RenderCodexModelCatalog`), `.design/multimodal-content.md:65`, `.design/quota-backoff.md:19`; `backend-organization.md` §1.2/§5.1. | Point at code that no longer exists. | none | `internal/task` references fixed in P1a, the rest in P1b |

### 3.2 Duplicate implementations

Re-checked against the 2026-10-01 audit; several of its items are already gone.

| # | Evidence | Problem | Risk | Phase |
|---|---|---|---|---|
| B1 | Two packages named `transform`: `protocol/transform` (1.75k LOC, request transform chain: base / consistency / vendor / rule thinking / max-tokens rewrite) and `protocolserver/transform` (1.5k LOC: MCP tool injection/strip, output-limit, thinking compaction, websearch strip). The second has two importers, both in `protocolserver` (`rule_flags.go:10`, `protocol_transform.go:13`). They implement the same `Transform` shape and are applied by the same chain. | Which one a new transform goes into is a coin flip; the names hide that the second one is gateway-scoped (needs MCP/server context). | low (import path rename) | P3 |
| B2 | Client-config writers (`ApplyClaudeConfig`, `ApplyCodexConfig`, opencode, DSH, WorkBuddy, `cc_slots`, `cc_default_switch`, status): `internal/config/apply_config*.go`, `cc_*.go`, `client_config_status.go` ≈ 3.2k non-test LOC. The same concern is also in `ai/agent/*` (`CodexParams.Apply`, `InstallStatusLineScript` `ai/agent/apply_support.go:419`), `internal/agent/rule_bridge.go`, `server/module/configapply` (809 + dsh/workbuddy handlers) and `command/agent_command.go` (`codex-config.md` §9 lists the five layers). | One concern in five places; `config` also owns persistence, migration and rules. `config` is imported by 29 server files and 8 command files, so every change there rebuilds a lot. | medium (moves, no logic) | P4 |
| B3 | `max_tokens` / thinking budget rewrites: the old audit listed 2–3 copies. Now `protocolserver/transform/transform_max_tokens.go:121` `fitThinkingBudget` delegates to `protocol/ops/request_thinking.go:122` `FitAnthropicThinkingBudget`, and `protocol/transform/openai_max_tokens_rewrite.go` only handles the OpenAI field-name rewrite. | **Already consolidated.** Nothing to do. | — | closed |
| B4 | Claude-Code impersonation: `protocol/ops/request_anthropic_claude_code_compat.go`, `protocol/transform/claude_code_compat.go` + `claude_code_version.go`, `client/claude_{round_tripper,betas,cch,version,tool_names}.go` (≈2.0k LOC together). The rule in `protocol-client-boundary.md` ("ops owns *what*, round tripper owns *when*") is followed; the split is by design. | Not a duplicate. Keep; only dead leftovers in `client/claude_round_tripper.go` were removed (A2). | — | closed |
| B5 | Round-tripper header stampers: `client/kimi_round_tripper.go:44-55`, `xai_round_tripper.go:26-31`, `opencode_round_tripper.go` have the same `struct{http.RoundTripper}` + `Header.Set` shape. | ~100 lines, three call sites. A `headerRoundTripper(inner, func(*http.Request))` would shave ~60 lines but costs readability of vendor-specific files. | low, low payoff | P5 (optional) |
| B6 | Protocol conversion matrix: per pair a `request/`, `stream/` and `nonstream/` file (e.g. `openai_to_anthropic` ×3: 356 + 477 + 251 LOC; Google: request 325+440, stream 967, nonstream 464). `anthropic_view.go` (925 LOC) already unifies v1/beta on the request side; `stream` and `nonstream` have no equivalent. The unreferenced Google-outbound stream converters (A3) are 367 of the 967 Google stream lines. | Large, but real behaviour — each is covered by golden tests. Folding `stream` into `stage` is the open question in `backend-organization.md` §5. | high | **not planned** until the Google direction is decided (A3) |
| B7 | `internal/toolengine` (2.5k) vs `protocolserver/servertool` (334): the old audit named two `Executor`s; `servertool.Executor` (`executor.go:27`) was unreferenced and is gone (P1b). `toolround.ToolCall` (`protocol/stage/toolround/toolround.go:43`) is now the only tool-call shape in the loop. | Resolved by A2. | — | closed |
| B8 | Session persistence in four stores, OAuth CLI (`command/oauth.go`) vs `module/oauth` (1.4k LOC `handler.go`), `command` vs `usecase` vs `module/*` for rule/provider/agent ops. | Taken from the old audit and **not re-verified** (needs a field-level diff). `usecase-layer.md` already covers the third. | n/a | unverified |

### 3.3 Dev/test code in the production tree

| # | Evidence | Problem | Risk | Phase |
|---|---|---|---|---|
| C1 | `internal/protocoltest` (13.9k non-test LOC, 43 test files): imported by `cli/harness` only (prod) — but it imports `server`, `protocolserver`, `config`, `mcp`, `appconfig`, `server/module/debug`. `protocoltest/duo_spec.go:14` → `internal/server`. | A second application built on the real server, living under `internal/`, counted in every "which packages depend on `server`" query. | medium (move + path rewrite) | P4 |
| C2 | `cli/harness` (standalone `package main`, no importers), `vmodel/{benchmark,vmodeltest}` (+ `examples/*`), `internal/servertest` (13 test files only), `internal/testguard`, `internal/advisortest`, `internal/recording/recordingtest` (moved to `protocolserver/recordingtest` in P3-2). `vmodel/vmodeltest` is imported by `protocoltest` only. | Test-only packages mixed with production ones. `recordingtest` imports `protocolserver`, which imports `recording` → the only package-level seam of the `recording ↔ protocolserver` group cycle (§4.1). | low | P3 |
| C3 *(routing part done in P3-3)* | `routing/test_helpers.go` (non-`_test` file, ~220 lines of fixtures and mocks used by tests only: `ServiceForTest`, `OpenAIRequestForTest`, `mockAffinityStore`, …) and `client/transport_test_helper.go:20`, `protocol/ids/ids.go:70` `SetGeneratorForTest`. | Test seams compiled into the binary. | low | P3 |
| C4 | `internal/script/*.sh` (two of three scripts unreferenced, A3). `internal/web/dist` (embedded by `internal/assets.go`, placeholder keeps the build working) and `obs/PLANNING.md` (linked from `recording.md:6`) are needed and stay. | Orphaned scripts only. | none | P2 (with A3) |

### 3.4 Code in the wrong place

| # | Evidence | Problem | Risk | Phase |
|---|---|---|---|---|
| D1 | `server/limits.go:49-156` (`warmStartLimits`, `warmStartPolicy`, `seedWindow`; reads `db.UsageStore` minutes and seeds `limits` windows) and `server/limits.go:178-218` (`limitsDirectory`, a team/key directory adapter). | Policy logic in the server root; the `limits` package cannot import `db`, so the adapter belongs next to the module that consumes it. | low | P4 |
| D2 | Root handlers that are modules in all but name: `server/load_balance_handler.go` (398) + `load_balance_simulator.go` (346), `log_handler.go` (393), `model_request_handler.go` (381), `token_handler.go`, `trace_handler.go`, `status_handler.go`; `WebHandler` (`webui_handler.go`) wraps four of them. Load balancing is split between this and `protocolserver/load_balance.go`. | Not on the `Module` contract, so they are absent from `server_modules.go`; `server.go` keeps their deps (`server.go` is 692 lines, `NewServer` alone is 346). | medium | P4 |
| D3 | `config/model_resolve.go:155,210,321` calls `client.ModelLister` and `config/server_settings.go:307-314` calls `client.SetTransportConfig`. | `config` (a data package) performs network model listing and mutates the client package's global transport config; this is the only `config → client` edge and half of the `config`/`client`/`protocol` group cycle (§4.1). | medium (needs a seam) | P4 |
| D4 | `server/server_control.go:12` imports `command/options` for `options.DefaultProjectsDir`; `command` imports `server` and `app` imports `server`. | A constant that belongs in `config`/`constant` pulls the CLI package into the server's imports, which is what closes the `server → command → server` group cycle. | low | P3 |
| D5 | `typ/type.go:9-12` imports `loadbalance`, `routing/smartrouting`, `tool`; `db/rule_store.go:11` imports `routing/smartrouting`; `db/provider_store.go:15` imports `protocol`; `obs/session.go:7` imports `typ`; `catalog/model_list.go:7` imports `db`; `guardrails/utils/credential_store.go` imports `db` and `config/guardrails.go` imports `guardrails/{core,evaluate,utils}`. | The "model" packages (`typ`, `db`, `obs`, `catalog`) reach up into behaviour packages (`routing`, `protocol`, `guardrails`) for type definitions. This is what ties `typ`, `db`, `routing`, `config`, `guardrails`, `catalog`, `client`, `protocol`, `vision`, `vmodel` into one 12-group strongly connected component. | medium–high (type moves) | P5 |
| D6 | `client/vmodel_client.go:8` imports `vmodel/client`; `vmodel` is production (`server`, `module/virtualmodel`) *and* test tooling (§3.3). | The vmodel runtime and its benchmark share one tree. | low | P3 |

### 3.5 Oversized files and functions (non-test)

Counts on `main`; none of these is wrong by itself, they set the order for P4.

| File / function | Size | Note |
|---|---|---|
| `server/module/guardrails/handler.go` | 2296 LOC | largest production file; `UpdateGuardrailsPolicy` 131 lines, `InstallGuardrailsRegistryPolicy` 135 |
| `server/module/oauth/handler.go` | 1424 LOC | `AuthorizeOAuth` 277 lines, `createProviderFromToken` 210, `RefreshOAuthToken` 141 |
| `server/server.go` `NewServer` | 346 lines | the ~60-field `Server` struct wiring (old backlog #3) |
| `server/server_webui_api.go` `UseWebAPIEndpoints` | 275 lines | route table, not logic |
| `server/module/usage/routes.go` `RegisterRoutes` | 262 lines | route table |
| `protocol/stream/openai_responses_to_anthropic_converter.go` `processEvent` | 306 lines | state machine; golden-tested |
| `protocol/stream/openai_passthrough.go` `HandleOpenAIChatStream` | 291 lines | |
| `command/server.go` `startServerWithHook` | 234 lines | |
| `decision/convert.go` `ConvertRequest` / `ConvertResponse` | 233 / 231 lines | |
| `db/bot_access_store.go` 1115, `db/usage_record.go` 1058, `catalog/provider_catalog.go` 1007, `toolengine/generic_stream_interceptor.go` 968, `command/tui/quickstart.go` 935, `protocol/request/anthropic_view.go` 925, `mcp/runtime/runtime.go` 905, `config/migration.go` 905, `config/apply_config_codex.go` 895 | | the next tier; splitting is mechanical (same package) |

## 4. Dependency graph

### 4.1 Group-level strongly connected components

Top-level directories (production imports only; `internal/protocol/...` counts as `protocol`):

| SCC | Members | The sub-package edges that close the cycle |
|---|---|---|
| **S1** (12 groups) | `typ`, `routing`, `config`, `db`, `catalog`, `guardrails`, `client`, `protocol`, `forwarding`, `vision`, `obs`, `vmodel` | `typ → routing/smartrouting`, `typ → loadbalance`, `typ → tool` (`typ/type.go:9-12`); `db → routing/smartrouting` (`rule_store.go:11`), `db → protocol` (`provider_store.go:15`); `catalog → db` (`model_list.go:7`); `config → client` (`model_resolve.go:15`, `server_settings.go:10`), `config → guardrails/{core,evaluate,utils}`, `config → routing/smartrouting`; `guardrails/utils → db`; `client → vmodel/client` (`vmodel_client.go:8`), `client → vision/imagegen`; `protocol/stage/upstream → client, forwarding` (`upstream/openai.go:15`, `anthropic.go:14`, `upstream.go:21`); `forwarding → client` (`anthropic.go:10`); `protocol/ops → catalog`; `obs → typ` (`session.go:7`) |
| **S2** | `server`, `command`, `app` | `server → command/options` (`server_control.go:12`); `command → server` (`command/server.go:23`); `app → server` (`app/server_manager.go:13`), `command → app` |
| **S3** *(cut in P3-2)* | `protocolserver`, `recording` | `protocolserver → recording`; `recording/recordingtest → protocolserver` (`helpers.go:26`) |

None of these is a Go import cycle: each edge lands in a different sub-package (`smartrouting`, `options`, `recordingtest`, `stage/upstream`, …). They matter because they decide what can move where. In particular, `.design/protocol-client-boundary.md` states the dependency is one-way (`client → protocol`); in fact `protocol/stage/upstream` depends back on `client` (and on `forwarding`, which depends on `client`). The boundary document describes `protocol` outside `stage/`.

### 4.2 What the cycles mean for the plan

- **S3 and S2 are cheap to cut** (move `recordingtest` to a `_test` helper or into `protocolserver`; move `DefaultProjectsDir` out of `command/options`). Both are P3.
- **S1 cannot be cut by moving packages**; it needs type moves: `smartrouting` types and `loadbalance.Service` out of `typ`'s imports (or `typ` split into a type-only leaf), `protocol/stage/upstream` and `forwarding` moved to the `client` side or behind a small interface, `config`'s model listing behind a seam. That is why it is P5 and why PRs before it must not add new edges into S1.
- Leaves worth protecting from now on: `limits`, `loadbalance`, `clock`, `constant`, `tool`, `decision`, `lock`, and `usecase` (contract-checked).

### 4.3 Packages with one importer (merge/inline candidates)

`internal/protocolserver/transform` ← `protocolserver` (B1); `internal/guardrails/{pipeline,mutate}` ← `protocolserver` / `pipeline`; `internal/protocol/stage/{anthropicbridge,sdkstream,upstream}` ← `protocolserver`; `internal/toolengine` ← `protocolserver`; `internal/remotehost` ← `server/module/imbot` (116 LOC); `internal/mcp/local` ← `server/module/mcp`; `internal/otel/exporter` ← `otel`; `internal/server/hooks` ← `server`; `internal/worker/*` ← `server`; `internal/command/tui` ← `command`; `pkg/daemon` ← `command`. A single importer is not a defect (several are deliberate seams); `remotehost` (116 LOC) and `server/hooks` are the only ones small enough to question.

## 5. Status of the 2026-10-01 backlog

| Old item | Now |
|---|---|
| 1. Delete `internal/task` | **Done in P1a.** |
| 2. Move dev/test tooling (`protocoltest`, `cli/harness`, `servertest`, `vmodel` test parts) | Still open → C1/C2, P3/P4. |
| 3. Server god-struct (~60 fields) | Still open → D2, P4. `server.go` is 692 lines, `NewServer` 346. |
| 4. Remaining helper adoption (11 multi-key errors, ~33 bespoke binds) / root handlers → modules | Not re-measured; root handlers → D2. |
| 5. Protocol/client dedupe | Mostly closed: B3, B4, B7 resolved or by design; B1 (`transform` ×2) and B5 remain; the protocol-side dead leftovers went in P1b. |
| 6. Session persistence ×4 | Unverified (B8). |
| 7. Docs hygiene | `otel` paths fixed earlier; P1a and P1b fix the references they break (A7). |
| Open question 1 (`pkg/notify`) | Still open (A4). |
| Open question 2 (fold `swagger` / `afk` into root) | Out of scope for `internal/`. |
| Open question 3 (`stage` vs `stream`) | Still open (B6). |

## 6. Phased plan

One reviewable PR per row, ordered by risk. Pure moves and renames (P3–P5) use `tingly-go move` / `tingly-go rename` so every reference is rewritten and type-checked; `tingly-go prune` is the cross-check for "is this still referenced" before each deletion. Every PR: `go build ./internal/... ./pkg/... ./vmodel/... ./cli/...`, `go vet` (including tests), the affected packages' tests, `GOOS=windows|darwin go build`, and for anything touching routes `openapi.json` regenerating byte-identical (isolated `HOME`).

| Phase | PR | Content | Risk |
|---|---|---|---|
| **P1a** | **PR 1a — delete `internal/task`** | The unused task subsystem and the doc lines that name it (A1, A7). | none |
| **P1b** | **PR 1b — delete unreferenced symbols** | ~80 unreferenced funcs/types/vars and 4 dead files, with the comment/doc references they break (A2, A7). Everything A3/A6 keeps. | none |
| P2a | PR 2 — decide the "unwired" set | Per A3/A4, after your decision: either wire or delete (rate limiter, `CORSWithConfig`, `obs/resolver`, credential alias, Claude hook installers + two scripts, codex responses transform, ChatGPT conversion, Google outbound stream, `streamemit`, `openaibridge`/`responsesbridge`, `pkg/notify` providers). Deleting also fixes the docs that name them. | low |
| P2b | PR 3 — test-only production symbols and dead options | A5/A6: `With*` options, DTOs, the two dead handler funcs, symbols that only a test uses (their tests go too). | low |
| P3a | PR 4 — test tooling out of the way | C2/C3: `recordingtest` → `protocolserver/recordingtest` (cuts S3); `routing/test_helpers.go` fixtures into `_test.go` plus a small `routing/routingtest`. `testguard`/`advisortest`/`servertest` stay. **`internal/web/dist/PLACEHOLDER.md` and `obs/PLANNING.md` must stay** (`internal/assets.go` embeds `web/dist`; `recording.md:6` links the planning doc). | low |
| P3b | PR 5 — small boundary fixes | B1 (rename `protocolserver/transform` → `protocolserver/servertransform` — **done, P3-4**), D4 (`DefaultProjectsDir` out of `command/options` into `config`, cuts S2 — **done, P3-1**). D6 dropped: `client` needs `vmodel/client`. | low |
| P4a | PR 6 — move `protocoltest` + `cli/harness` out of `internal/` | C1: no logic change; tooling directory (e.g. `tools/protocoltest`) importing `internal/…` through a main module path. Needs `task`/CI path updates. | medium |
| P4b | PR 7 — `server` root handlers become modules | D2: `load_balance`, `log`, `model_request`, `token`, `trace`, `status` onto `module.Module`; also moves D1 (`limits.go` warm start) into `module/limits`. `openapi.json` must stay identical. | medium |
| P4c | PR 8 — client-config writers out of `config` | B2: `internal/clientconfig` (the old audit's own suggestion), pure move with `tingly-go move`; `config` keeps persistence/rules/migration. | medium |
| P4d | PR 9 — split the 1k+ line files | 3.5: `guardrails` and `oauth` module handlers first (same package, mechanical). | low (no-op moves) |
| P5 | PR 10+ — break S1 | D3, D5, the `stage/upstream → client` edge: type-only leaf package for `typ`, `config` model-listing seam. One edge per PR, with an import-direction check (`go list`) in CI so S1 cannot regrow. | high |
| — | not planned | B6 (protocol matrix; waits on the Google decision), B8 (needs field-level diff). | — |

## 7. Questions for the owner

1. **A3** — wire or delete each unwired feature? Two are security/product-relevant: the failed-login rate limiter (`middleware/ratelimit.go`) and credential aliasing. Recommendation: wire the limiter (it protects login), delete the rest.
2. **A4** — are `openaibridge`/`responsesbridge` and `streamemit` planned work? They carry design docs; recommendation: keep until the stage pipeline decision, delete `pkg/notify` providers/examples if no plan.
3. **C1** — is `internal/protocoltest` allowed to move to a non-`internal/` path (e.g. `tools/`)? It must keep importing `internal/…`, so the target must stay inside the root module.
4. OK to enforce an import-direction check in CI after P5?
