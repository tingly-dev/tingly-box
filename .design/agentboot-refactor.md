# Agentboot Refactor

Design notes for simplifying the `agentboot` module. Agentboot was split into its
own Go module (`github.com/tingly-dev/tingly-box/agentboot`, see `go.work`) and a
new `AgentService` was added to expose its capabilities. In practice the module is
exercised by a **single feature** — the remote-control bot — yet it carries two
complete, parallel paradigms plus speculative generality. This doc records the
current state and the target design.

> **Status (2026-07-23): completed.** Sections 1–4 retain the original problem
> analysis and phased decision record. The current code no longer has the
> parallel callback paradigm, provider-specific root dependency, duplicate
> launch contracts, or legacy Claude Launcher/ControlManager path. See
> `agentboot/README.md` for the current package map and §7 for the completion
> addendum.

Reference point: the **Claude Agent SDK** (`query()` stream + `ClaudeSDKClient`,
a single `can_use_tool` permission callback, one options struct, one typed
message union). The goal is to converge agentboot onto that shape.

---

## 1. Current state

### 1.1 Module layout

```
agentboot/                         (~11.5k non-test LoC)
├── agentboot.go      (157)  AgentBoot registry + session-store query methods
├── service.go        (124)  AgentService — façade wrapper over AgentBoot
├── types.go          (397)  core types, BOTH paradigms' interfaces
├── config.go         (27)   DefaultConfig / DefaultPermissionConfig
├── driver.go         (58)   AgentDriver interface + LaunchSpec        ── engine
├── transport.go      (66)   AgentTransport interface + EventKind       ── engine
├── runner.go         (327)  Runner: process+protocol → ExecutionHandle ── engine
├── handle.go         (230)  ExecutionHandle + runnerHandle + Controlled ── engine
├── events.go         (71)   StreamEvent union (Message/Approval/Ask/Error) ── engine
├── run.go            (130)  RunWithPrompter + Prompter + MessageSink   ── engine (convenience)
├── message.go        (478)  AgentMessage hierarchy + MessageFromEvent  ── LEGACY
├── handler.go        (96)   CompositeHandler + MessageHandler glue     ── LEGACY
├── builder.go        (90)   func adapters for CompositeHandler         ── LEGACY
├── session_bridge.go (21)   NewClaudeStore one-liner
├── ask/              (~1.3k) interactive permission/ask subsystem (Handler, Prompter, registry)
├── claude/           (~6.7k) Claude agent: driver, transport, accumulator, messages,
│                             formatter, tool_renderer, cli_discovery, session store/parser
├── common/           (~318)  Event, SessionStore, SessionMetadata, errors
├── process/          (~515)  Process abstraction + OS exec + fake factory
├── protocol/         (~271)  JSONL decoder + JSON encoder (stdin/stdout wire)
└── prompt/           (~332)  FakePrompter — test-only, UNUSED in prod
```

### 1.2 The engine (good — keep)

The execution pipeline is clean, layered, and testable. It already mirrors the
SDK's streaming client.

```
Agent.Execute(ctx, prompt, opts) (ExecutionHandle, error)
  └─ Runner.Execute                        (runner.go)
       ├─ AgentDriver.Prepare → LaunchSpec (driver.go; claude/driver.go)
       ├─ process.Factory.Start            (process/)
       ├─ protocol.Decoder / Encoder       (protocol/)
       └─ AgentTransport                    (transport.go; claude/transport.go)
            ├─ Classify(ev) → EventKind {Ignore|Message|Control|TerminalSuccess|TerminalError}
            ├─ AccumulateMessage(ev) → []any  (rich agent-specific messages)
            └─ EncodeControlResponse(...)     (ControlResponse → wire)

ExecutionHandle (handle.go):
  Events() <-chan StreamEvent   // MessageEvent | ApprovalRequestEvent | AskRequestEvent | ErrorEvent
  Respond(reqID, ControlResponse) error   // ApprovalResponse | AskResponse
  Wait() (*Result, error)
  Cancel()
```

`NewControlledHandle` (handle.go:222) lets in-process agents (mocks) drive a
handle without the process pipeline.

`RunWithPrompter(ctx, handle, prompter, sink)` (run.go:61) is the convenience
consumer: it loops `Events()`, routes Approval/Ask to a `Prompter`
(OnApproval/OnAsk), feeds `MessageEvent.Raw` to a `MessageSink`, and returns
`handle.Wait()`. **This is the canonical way to consume a handle.**

### 1.3 The legacy paradigm (redundant — remove)

A second, older callback API does the same job differently:

- `MessageHandler` (types.go:58) = `OnMessage(any) + OnError + OnComplete + OnApproval + OnAsk`
- subset interfaces: `MessageStreamer`, `ApprovalHandler`, `AskHandler`, `CompletionCallback`
- `CompositeHandler` (handler.go) composes them; `builder.go` adds func adapters
- `CompletionResult` (types.go:118) — separate from `Result`
- `message.go` — the `AgentMessage`/`BaseMessage` hierarchy
  (`InitMessage`, `AssistantMessage`, `PermissionRequestMessage`, `ResultMessage`,
  `StreamDeltaMessage`, …) + `MessageFromEvent` + `marshalToMap`

This paradigm predates the `ExecutionHandle`/`StreamEvent` engine and is now only
held alive by two consumers (see §2).

### 1.4 Consumers (who actually uses agentboot)

Production registers **only** the Claude agent. No mock agent is registered
(`internal/server/module/imbot/manager.go:91`, `internal/command/remote.go:368`).

| Consumer | Path | Uses |
|---|---|---|
| Claude bot executor | `remote/control/bot/agent_claude_code.go` | **new** engine: `AgentBoot.GetDefaultAgent()` + `agent.Execute` + manual `handle.Events()` loop |
| Stream writer | `remote/control/bot/bot_stream.go` | **legacy** `MessageStreamer`+`CompletionCallback`; `OnMessage(any)` |
| Smart-guide executor | `remote/control/bot/agent_smart_guide.go` | **legacy** `CompositeHandler` via `agent.ExecuteWithHandler` |
| Smart-guide agent | `remote/control/smart_guide/agent.go` | takes `agentboot.MessageHandler` — but runs a **tingly-agentscope** ReAct agent, NOT agentboot's pipeline |
| IM prompter | `remote/channel/imchannel/imprompter.go` | implements `Prompter` (OnApproval/OnAsk); natively uses `ask.Request`/`ask.Result` |
| Session manager | `remote/session/manager.go` | implements `agentsession.Store` (passed via `ExecutionOptions.Store`) |
| Boot wiring | `internal/server/module/imbot/manager.go`, `internal/command/remote.go` | `agentboot.New` + `RegisterAgent` |

`AgentService` (service.go) and `RunWithPrompter` (run.go) are used **only by
agentboot's own examples** — the real consumer reaches past the service to
`AgentBoot` directly and hand-rolls the event loop.

---

## 2. Problems

### P-1 — Two paradigms for one job

The engine (§1.2) and the legacy callbacks (§1.3) are parallel implementations of
"run an agent, stream messages, answer permission prompts, get a result." ~870
lines (`handler.go` + `builder.go` + `message.go`) exist only to support the old
shape.

### P-2 — `message.go` is dead on the live path

The runner emits raw `*claude.AssistantMessage` / `claude.Message` via
`MessageEvent.Raw`. `bot_stream.go:66` `OnMessage` only ever hits the
`*claude.AssistantMessage` and `claude.Message` cases. The other branches —
`agentboot.AgentMessage`, `agentboot.Event` + `MessageFromEvent`,
`map[string]interface{}` — are vestiges of the mock/legacy path and never fire in
production (no mock agent registered). The entire `message.go` (478 lines) is
removable once those dead branches are deleted.

### P-3 — Four representations of "permission request"

One tool approval is copied through four shapes on the way in and four on the way
out:

```
inbound :  claude wire → ApprovalRequestEvent → PermissionRequest → ask.Request
outbound:  ask.Result  → PermissionResult     → ApprovalResponse  → claude wire
```

- `agent_claude_code.go:154-202` copies `ApprovalRequestEvent`→`PermissionRequest`
  and `PermissionResult`→`ApprovalResponse` field by field.
- `imprompter.go:631-674` copies `PermissionRequest`→`ask.Request` and
  `ask.Result`→`PermissionResult` field by field.

The SDK has exactly one request shape and one result shape (`can_use_tool`).

### P-4 — The "service" isn't used; query methods duplicated

`AgentService` wraps `AgentBoot` and re-exposes `ListProjects` / `ListSessions` /
`GetSession` / `GetSessionSummary` / `Execute` / `ExecuteSession`. The query
methods duplicate `AgentBoot`'s (`agentboot.go:128-149`). Nothing in
`internal/remote` calls it.

### P-5 — Convenience helper bypassed

`agent_claude_code.go:142-207` re-implements `RunWithPrompter` inline (the only
addition is an `autoApprove` short-circuit and a streaming sink — both
expressible as a wrapping prompter + a sink).

### P-6 — Misc

- `prompt/` package: test-only, unused in prod.
- `Result` (types.go) has many getters of unclear use (`GetStatus`,
  `GetMessagesByType`, `GetCostUSD`, `GetMessageChain`, …).
- `session_bridge.go` is a one-function file.

---

## 3. Target design

Two layers. The **engine** stays. **`AgentService` (kept, per decision) becomes
the single public façade.** The legacy paradigm is deleted.

```
┌─────────────────────────────────────────────────────────────┐
│ AgentService  (service.go)  ── the ONE public surface        │
│   Query:   ListProjects / ListSessions / GetSession /        │
│            GetSessionSummary                                  │
│   Stream:  Execute(...) (ExecutionHandle, error)             │
│   Run:     Run(ctx, req, prompter, sink) (*Result, error)    │  ← new, wraps RunWithPrompter
│   (AgentBoot demoted to internal registry; query methods     │
│    live on the service, not duplicated)                       │
└───────────────┬─────────────────────────────────────────────┘
                │ uses
┌───────────────▼─────────────────────────────────────────────┐
│ Engine (unchanged):                                          │
│   Agent / Runner / AgentDriver / AgentTransport              │
│   ExecutionHandle / StreamEvent / ControlResponse            │
│   Prompter (OnApproval/OnAsk) + MessageSink                  │
│   process/ protocol/ common/ claude/ ask/                    │
└─────────────────────────────────────────────────────────────┘

DELETED: handler.go, builder.go, message.go,
         MessageHandler/MessageStreamer/ApprovalHandler/AskHandler/
         CompletionCallback/CompletionResult,
         (P2) prompt/, unused Result getters
```

### 3.1 Façade decision: keep `AgentService` (named `Service`)

Per decision, `AgentService` is promoted to *the* entry point and `AgentBoot`
becomes an internal registry detail.

- Move the query methods (`ListProjects`, `ListSessions`, `GetSession`,
  `GetSessionSummary`) so they live on the service only; drop the duplicates from
  `AgentBoot` (or unexport `AgentBoot` query methods so the service is the sole
  caller).
- Add the high-level streaming convenience to the service:

```go
// RunRequest bundles what a high-level caller needs to start a run.
type RunRequest struct {
    AgentType   AgentType   // "" → default agent
    ProjectPath string
    Prompt      string
    Opts        ExecutionOptions // session id, resume, env, permission mode, store, …
}

// Run executes and drives the handle to completion via RunWithPrompter.
// prompter answers Approval/Ask; sink receives MessageEvent.Raw (nil to drop).
func (s *Service) Run(ctx context.Context, req RunRequest, prompter Prompter, sink MessageSink) (*Result, error)
```

`agent_claude_code.go` then becomes (sketch):

```go
prompter := autoApproveIf(autoApprove, e.deps.IMPrompter) // wrap, don't branch in the loop
sink := func(raw any) { _ = streamWriter.OnMessage(raw) }
result, err := e.deps.AgentService.Run(ctx, agentboot.RunRequest{
    ProjectPath: projectPath, Prompt: req.Text,
    Opts: agentboot.ExecutionOptions{ SessionID: sessionID, Resume: shouldResume, … , Store: e.deps.SessionMgr },
}, prompter, sink)
```

— deleting the 65-line `for ev := range handle.Events()` switch.

### 3.2 Permission/ask representation: collapse toward the event types

Make the `Prompter` consume the **event types** directly, removing one hop:

```go
type Prompter interface {
    OnApproval(ctx context.Context, req ApprovalRequestEvent) (ApprovalResponse, error)
    OnAsk(ctx context.Context, req AskRequestEvent) (AskResponse, error)
}
```

This deletes `PermissionRequest`/`PermissionResult`/`AskRequest`/`AskResult` from
`types.go` and the field-copy block in `agent_claude_code.go`. `imprompter.go`
keeps its single `event → ask.Request → ask.Result → response` conversion.
(P2 option: let the prompter speak `ask.Request`/`ask.Result` natively to remove
that hop too.)

### 3.3 Smart-guide decoupling

`smart_guide` runs a tingly-agentscope agent and only borrows agentboot's
`CompositeHandler`/`MessageHandler` as a callback bundle. It should own a small
local callback type (a streamer + an approval func) instead of importing
agentboot's deleted interfaces. After this, `handler.go` / `builder.go` have no
users and are deleted.

---

## 4. Migration plan (phased)

### P0 — delete dead code (no behavior change) — DONE

Done in this branch (~3,660 LoC removed). What shipped:

1. `bot_stream.go` `OnMessage`: dropped the dead `agentboot.AgentMessage` and
   `agentboot.Event`+`MessageFromEvent` branches; kept `*claude.AssistantMessage`,
   `claude.Message`, and the smart-guide `map[string]interface{}` path. Removed
   the now-dead `handleAgentMessage`, `handleAgentbootEvent`, `toolFieldsFromRaw`,
   the unused `OnApproval`/`OnAsk`/`OnComplete` stubs, and the `MessageStreamer`/
   `CompletionCallback` assertions.
2. `smart_guide` now owns a local callback contract
   (`smart_guide/handler.go`: `StreamHandler`, `CompletionResult`, `Approver`).
   `AgentConfig.Handler` → `Approver`; `ExecuteWithHandler` takes `StreamHandler`.
   The bot's `messageTrackingWrapper` gained `OnComplete` and is passed directly
   (no more `CompositeHandler`). `*imchannel.IMPrompter` satisfies `Approver`
   structurally via its existing `OnApproval`.
3. Deleted `message.go`, `handler.go`, `builder.go`. The `EventType*` string
   constants (still used by `bot_stream.go`) moved to `agentboot/eventtype.go`.
4. Deleted `MessageHandler`, `MessageStreamer`, `ApprovalHandler`, `AskHandler`,
   `CompletionCallback`, `CompletionResult` from `types.go`.
5. Also removed: the three `//go:build e2e` legacy tests
   (`claude_e2e_test.go`, `runner_e2e_test.go`, `launcher_e2e_test.go`), the two
   `//go:build ignore` legacy examples (`claude/examples/server`, `.../query`),
   the legacy `TestMessageHandler` helper + `TestCompositeHandler_*` tests, and an
   unused `Manager.msgHandler` field. All were written against the removed
   paradigm with zero new-paradigm coverage.

Verified: `go build ./...` + `go test ./...` green in both the `agentboot`
module and the root module (`remote/control/...`, `remote/...`).

### P1 — make `AgentService` the real façade — DONE (this PR)

Scope of this PR is the façade only; the permission-type collapse (item 9 /
§3.2) is deferred to a separate PR. What shipped:

6. Query methods (`ListProjects` / `ListSessions` / `GetSession` /
   `GetSessionSummary`) now live on `AgentService` and read `boot.store`
   directly. The duplicate `AgentBoot.ListProjects` / `ListRecentSessions` /
   `GetSessionSummary` were removed (`AgentBoot` is now registry-only).
7. Added `AgentService.Run(ctx, RunRequest, Prompter, MessageSink)` (wraps
   `RunWithPrompter`) plus `RunRequest`. `Execute*` now accept an empty
   `AgentType` to mean the default agent.
8. Threaded `*agentboot.AgentService` through the bot in place of
   `*agentboot.AgentBoot`: creation sites (`internal/server/module/imbot`,
   `internal/command/remote.go`), `bot.NewManager` / `Manager`,
   `NewBotHandler` / `BotHandler`, `ExecutorDependencies`, and the test harness.
   `agent_claude_code.go` now calls `AgentService.Run` with an `autoApprovePrompter`
   wrapper + a streaming sink, deleting the ~90-line hand-rolled event loop.
   `command_integration.go` uses `AgentService.ListSessions`.

Behavior note: non-fatal `ErrorEvent`s (rare decoder-level errors) are now
logged by `RunWithPrompter` instead of being printed to chat as `[ERROR] …`.
Fatal errors are unchanged — still surfaced via the returned error (incl. the
session-conflict message). `streamWriter.OnError` remains live on the
smart-guide path.

Deferred to a follow-up PR (item 9 / §3.2): collapse `Prompter` onto the event
types and remove `PermissionRequest` / `AskRequest` / `PermissionResult` /
`AskResult`, updating `imprompter.go` and `smart_guide`.

Verified: `go build ./...` + `go test ./...` green in both modules; `go vet`
clean on the changed packages.

### P1.5 — collapse permission/ask representation (§3.2) — DONE (this PR)

9. `Prompter` now consumes the stream event types directly:

   ```go
   type Prompter interface {
       OnApproval(ctx, req ApprovalRequestEvent) (ApprovalResponse, error)
       OnAsk(ctx, req AskRequestEvent) (AskResponse, error)
   }
   ```

   Deleted from `agentboot/types.go`: `PermissionRequest`, `PermissionResult`,
   `AskRequest`, `AskResult`, and the dead `PermissionResponse`. `RunWithPrompter`
   now passes the event straight to the prompter and the returned response straight
   to `handle.Respond` — the two field-copy blocks are gone.

   `imprompter.go` keeps a single `event → ask.Request → ask.Result → response`
   conversion via the new `ask.FromApprovalEvent` / `Result.ToApprovalResponse`
   (replacing `FromPermissionRequest` / `ToPermissionResult`). `smart_guide`'s
   `Approver` and its two request builders now use `ApprovalRequestEvent` /
   `ApprovalResponse`. `agent_claude_code.go`'s `autoApprovePrompter` and the
   `httpserver` example updated to the event types.

   Also removed as now-dead: the `ask.UserPrompter` / `legacyPrompterWrapper` /
   `ToLegacyUserPrompter` shim and `ask.Request.ToPermissionRequest`,
   `IMPrompter.PromptPermission`, `claude.Launcher.SendPermissionRequest`, and the
   entire `prompt/` package (test-only `FakePrompter`, unused in prod — item 11).

   Note: the prompter's `Remember` flag is no longer carried on the response to the
   agent; AlwaysAllow caching stays internal to the ask subsystem (matches the
   prior behavior — `RunWithPrompter` already dropped it at the boundary).

Verified: `go build ./...` + `go test ./...` + `go vet` green in both modules.

### P2 — deeper cleanup — TRIMMED BY REAL EVENT MODEL (this PR)

`agentboot` is its own Go module and is kept as a reusable library, so a coherent
public surface is retained even where the single consumer (the bot) doesn't use
it. But "coherent" is the bar: an accessor that is *misleading* against the real
event model, or has *no semantic consumer*, is not coherent — it's noise. Applying
that test:

**Removed (3 `*Result` getters):**

- `GetToolUseMessages` / `GetToolResultMessages` — misleading. `Result.Events`
  holds the CLI's **raw top-level** stream events (runner.go appends each decoded
  `ev` verbatim). `tool_use` / `tool_result` are *not* top-level events in this
  pipeline — they are content blocks inside `assistant` messages; the only
  standalone top-level `tool_use` is an SDK duplicate the formatter explicitly
  suppresses (`formatter.go`, `seenAssistantToolIDs`). So these getters return
  either nothing or that suppressed-duplicate noise, while *looking* symmetric
  with `GetAssistantMessages`. To read tool calls you must walk assistant content
  blocks, so the getters are a trap.
- `GetStatus` — zero callers and no semantic consumer. The authoritative terminal
  signal is the `result` event + `IsSuccess`.

**Kept (the genuinely coherent surface):**

- `GetMessagesByType` (generic filter) and `GetMessageChain` (conversational view)
  — foundational queries.
- `GetAssistantMessages` / `GetUserMessages` — `assistant` / `user` *are* real
  top-level events, so these wrappers return correct data.
- `TextOutput` (used by the bot), and the scalar accessors `GetSessionID` /
  `GetCostUSD` / `IsSuccess` (httpserver example) — session / cost / success map
  directly to this product's LLM-gateway domain.

**Inlined `session_bridge.go`** (`NewClaudeStore`) into `agentboot.New` and
deleted the file. Its only non-redundant value would have been converting the
concrete `*ccsession.Store` to the `common.SessionStore` interface — but `ab.store`
is already typed as that interface, so the conversion is implicit at assignment.
With a single internal caller and no external consumer, the exported one-line
passthrough was ceremony. (If Codex/Gemini backends land later, re-introduce a
symmetric set of `New*Store` constructors then.)

**Item 10** (make `ask.Request`/`ask.Result` the prompter's native types) is still
not pursued: it would create an `agentboot → ask → agentboot` import cycle (the
`Prompter` interface lives in `agentboot`), and the single `event → ask.Request →
ask.Result → response` hop in `imprompter.go` is already minimal.

Net through P2: the earlier P0/P1/P1.5 deletions removed a genuinely redundant
*parallel paradigm*; P2 removed only misleading/orphan accessors and kept one
coherent query + scalar surface.

### P3 — per-query state and lifecycle isolation — DONE

Reference implementation: the locally installed Python `claude-agent-sdk`
0.2.126. Its one-shot `query()` creates a fresh transport and query controller
per call, treats decode/process failures as terminal, and closes stdin before
terminate/kill escalation. Agentboot now adopts those lifecycle properties:

1. `Runner` stores an `AgentTransportFactory`, not an `AgentTransport`.
   `Execute` calls the factory once, so Claude's mutable message accumulator and
   routing context cannot leak across concurrent bot chats. `Agent` and
   `Launcher` no longer cache a transport.
2. `RunnerConfig` makes the existing `StreamBufferSize` and
   `DefaultExecutionTimeout` settings effective. A per-call zero timeout uses
   the default; a negative timeout explicitly disables it.
3. Terminal results close the protocol encoder/stdin first. A bounded grace
   period lets Claude flush its session file before the runner escalates to
   `Kill`.
4. `ResultError`, `ProcessError`, and `ProtocolError` preserve the actual
   failure class, CLI subtype/details, exit code, and wrapped cause.
   `Result.ExitCode` and `Result.Error` now agree with `Wait`.
5. Fatal decoder errors stop the process instead of waiting forever, and a
   stream-json EOF without a result returns `ErrNoTerminalResult`.

Deliberately not copied from the SDK: persistent bidirectional clients, hooks,
in-process MCP servers, rewind/task controls, and dynamic model/permission
mutation. None is required by the current remote-control product path.

Regression coverage includes two overlapping runs on one Agent (routing
isolation), configured buffer/timeout, non-zero process exit, malformed JSON,
missing terminal result, structured result errors, and encoder-close behavior.

### Verification per phase

- Module: `cd agentboot && go build ./... && go test ./...`
- Root: `go build ./... && go test ./remote/control/... ./remote/...`
- Manual smoke: `@cc` execution (stream + a permission prompt) and `@tb`
  smart-guide reply, since those exercise both consumers.

---

## 5. Expected outcome

- One execution paradigm (engine + `Service` façade) instead of two.
- One permission representation flowing through the prompter instead of four.
- `AgentService` becomes the actually-used public surface.
- ~1,000–1,500 LoC removed; the Claude path and smart-guide path behave the same.
- Clear extension seam preserved: a future agent implements `AgentDriver` +
  `AgentTransport`, registers via the service — no new paradigm needed.

---

## 6. Open questions / risks

- **Smart-guide callback type**: confirm the minimal interface it needs
  (assistant-text streaming + completion banner + approval). It currently relies
  on `messageTrackingWrapper` + `SmartGuideCompletionCallback`; those move with it.
- **`ExecutionOptions.Store`** (`agentsession.Store`) lifecycle calls
  (SetRunning/SetFailed/SetCompleted) stay in the runner — unaffected.
- **Other importers** (`cli/harness/agent.go`, `remote/scenario/builtin/...`)
  must be re-checked against any signature change to `Prompter`/types before P1.

---

## 7. Completion addendum (2026-07-23)

The refactor continued through the remaining structural boundaries:

1. Root public models and Runner lifecycle were split into cohesive files.
2. Claude discovery/environment, message/content, and session composition were
   separated without changing the Claude Code CLI backend.
3. Historical session access became `common.SessionReader` and is injected with
   `WithSessionReader`; the root package no longer imports `claude/session`.
   `claude.NewService` owns Claude Agent + history composition.
4. Runtime lifecycle persistence is named `session.LifecycleStore`.
5. `process.LaunchSpec` is the one concrete launch contract; the root name is a
   compatibility alias.
6. Repository consumers use `AgentService` methods instead of reaching through
   `Boot()`. Low-level registry APIs remain deprecated for one compatibility
   window.
7. The unused Claude `Launcher`, stateful `ControlManager`/request builders, and
   duplicate JSONL helpers were removed. Runtime control is exclusively
   `ExecutionHandle → Runner → Transport`, and JSONL IO is owned by `protocol`.
8. Provider assumptions were removed from root configuration and execution
   options. Claude's history directory is configured with
   `claude.WithProjectsDir`, Claude permission constants live only in the Claude
   adapter, and transport routing uses provider-neutral `ControlMetadata`.
9. `common.Event` is the single raw event contract; root and protocol aliases
   were removed.
10. Persistent Claude profile policy and runtime permission control remain
    separate. `ExecutionOptions.PermissionMode` is an optional per-run override:
    empty emits no CLI flag and inherits the selected settings file's
    `defaultMode`; a non-empty session control such as `/yolo` becomes Claude
    Code's `--permission-mode` argument and therefore has higher precedence.
    Turning `/yolo` off clears the override rather than sending `default`.
    See `.design/remote-cc-profile.md` §2.1 for the scope and precedence rules.

Validation includes the agentboot module suite, core race tests, production
composition consumers, and a real Claude Code CLI E2E.

---

## 8. Positioning addendum (2026-08-04): internal-first, Claude-honest

A follow-up review asked what remains of the original "generic multi-agent
platform" ambition and settled the module's positioning:

1. **AgentBoot is an internal library**, not a public one. It keeps its own Go
   module (the dependency-direction boundary — agentboot never imports
   tingly-box internals — is real value), but API existence is now judged by
   "used by tingly-box or by tests", not "coherent for hypothetical external
   consumers".
2. **The provider-neutral façade is honest only at the execution layer.** The
   Runner/Driver/Transport/process/protocol pipeline is genuinely agent-neutral
   and pays rent through the test infrastructure (fake process factory,
   fixtures). The message layer is not neutral and stops pretending:
   `MessageEvent.Raw` carries Claude concrete types and consumers type-switch
   on them. That is accepted, not hidden.
3. **The extension seam stays; the ceremony goes.** A future agent still plugs
   in by implementing `AgentDriver` + `AgentTransport` and registering via
   `AgentService.RegisterAgent` — no vocabulary is pre-reserved for it.

What shipped under that bar:

- **Compat window closed.** The deprecated `AgentBoot` registry
  (`agentboot.go`) was merged into `AgentService` — one type owns config,
  registry, and session reader. Deleted: `agentboot.New`, `Service.Boot()`,
  `MustGetAgent`, `GetAgent`/`GetDefaultAgent` (internalized as
  `resolveAgent`), `ListAgents`, `ResumeSession`, `GetConfig`. Zero external
  callers existed.
- **`Agent` interface slimmed** to `Execute` / `IsAvailable` / `Type`.
  `SetDefaultFormat`/`GetDefaultFormat` had no production callers and forced
  no-op stubs on `smart_guide.TinglyBoxAgent` (which is not even an
  agentboot Agent — it only shares the `AgentType` routing vocabulary). The
  default format still flows through `RunnerConfig`/`ExecutionOptions`.
- **`ask/` halved.** Deleted the never-used `DefaultHandler` state machine
  (`handler.go`), `StdinPrompter`/`NoOpPrompter`/`DenyAllPrompter`
  (`stdin_prompter.go`), and `Mode`/`Config`/`TypeConfirmation`/
  `TypeTextInput` plus orphan helpers from `types.go`. The surviving surface —
  `Request`/`Result`/`Response`, `FromApprovalEvent`/`ToApprovalResponse`,
  `ToolHandlerRegistry` + normalize/format/parse helpers — gained its first
  unit tests (`ask_test.go`; the package previously had zero).
- **`eventtype.go` moved out.** The string constants were the bot's
  smart-guide map-frame vocabulary, produced by `smart_guide` literals and
  consumed only by `remoteagent/stream.go`; they now live as unexported
  constants next to that consumer.
- **Config single-sourced.** `EnableStreamJSON` never branched behavior and
  was deleted from both `agentboot.Config` and `claude.Config`.
- **`Result` trimmed to the used surface**: `TextOutput`, `GetSessionID`,
  `GetCostUSD`, `IsSuccess`, and raw `Events`. The generic event-query
  getters (`GetMessagesByType`, `GetMessageChain`, `GetAssistantMessages`,
  `GetUserMessages`) kept in §P2 under the library posture had zero callers
  under the internal-first posture and were removed.
- **README repositioned**: Claude-first with a neutral execution core kept
  for testability and as the extension seam, instead of "unified layer for
  AI coding agents".

Verified: `go build ./... && go vet ./... && go test ./...` green in the
agentboot module; root module builds and `remote/control/...`,
`remote/channel/...`, `internal/server/module/imbot/...` tests green.

---

## 9. Directory-tree round (2026-08-04, follow-up to §8)

Applied as sequential commits on one branch, each independently green:

1. **Deprecated aliases deleted** (root `LaunchSpec`, `common.SessionStore`,
   `session.Store`, `claude.SDKTask*`, `ResultSubtypeError`, `EnvBunEnv`,
   `DefaultBundled*`). `AgentDriver.Prepare` and `claude.Driver` now speak
   `process.LaunchSpec` directly.
2. **`session/` micro-package folded into the root package** — the 19-line
   `LifecycleStore` interface now lives in `agentboot/lifecycle.go` next to
   `ExecutionOptions.Store`.
3. **`ask/` moved to `remote/control/ask`** (done manually): agentboot itself
   never consumed it, and its content is IM permission-UX vocabulary. The
   engine's ApprovalRequestEvent/AskRequestEvent control routing stays in
   agentboot; only the IM request/response presentation types moved.
4. **`common/` dissolved**: the wire `Event` (+ constructors) moved into
   `protocol/` — the decoder's product type now lives with the decoder and
   `protocol` has zero intra-module dependencies; the remainder (SessionReader,
   SessionMetadata, SessionEvent tree, errors) renamed to `history/`, which
   says what it is instead of "common".

Resulting tree: root engine+service, `history/`, `process/`, `protocol/`,
`claude/` (+`claude/session`, `claude/fixture`).

## 10. IM rendering extraction (2026-08-15)

The §9 open item is resolved: `claude/formatter.go` + `tool_renderer.go`
moved to `remote/control/render` (package `render`), the only consumer's
side of the decision — `remoteagent/stream.go` was the sole caller and now
imports `render.TextFormatter` / `render.NewTextFormatter` instead of
`claude.TextFormatter`. The `claude` package keeps the message-type
hierarchy (`Message`, `SystemMessage`, `AssistantMessage`, …) that rendering
operates on; `render` imports `agentboot/claude` for those types, preserving
the one-way dependency direction (`agentboot` never imports back into
`tingly-box`).

Two small surface changes fell out of the move:

- `SystemMessage`'s retry-notice accessors (`retryAttempt`/`retryDelayMS`/
  `retryReason`) are now exported (`RetryAttempt`/`RetryDelayMS`/
  `RetryReason`) — the CLI-version-spelling fallback logic they encapsulate
  belongs with the message model, not duplicated in the rendering package,
  so the accessors had to become part of `claude`'s public surface rather
  than being reimplemented across the package boundary.
- `render`'s own `getStr`/`getInt`/`getBool` map-access helpers are a small,
  intentional duplication of `claude/utils.go`'s private equivalents rather
  than newly-exported `claude` API — those helpers are also used by
  `claude/transport.go` (core engine code), and exporting them purely to
  serve the rendering package would have widened the module's public
  surface for no reason beyond convenience.

This is the library-vs-product-presentation boundary the module's
"internal-first, Claude-honest" positioning (§8) calls for: `agentboot`
owns the Claude wire protocol and message model, `tingly-box` owns how that
data is shown to a user (IM formatting is chat-surface specific, not a
concern of a Claude Code CLI wrapper). Verified: `go build`, `go vet`, and
`go test` green in both the `agentboot` module and the root module's
`remote/control/...` tree.

## 11. Public API elevation (2026-08-15)

§8 judged API existence by "used by tingly-box or by tests" — an informal,
easy-to-drift rule enforced only by whoever remembers to check before adding
an export. With a second production consumer of the Claude integration now
on the table (team-wide reuse, not just the remote-control bot), that rule
needed to become a checkable contract instead of tribal knowledge.

**Tried and reverted: `internal/` for `process`/`protocol`/`history`/
`claude/session`.** All four had zero consumers outside this module —
confirmed by grepping every `agentboot` import site in the root `tingly-box`
module, not assumed. They were moved under `internal/process`,
`internal/protocol`, `internal/history`, `claude/internal/session` (later
consolidated to a single `internal/` root: `internal/claude/session`) to
make the boundary compiler-enforced. On review this didn't clear its own
bar: the packages have no consumer today, so the move blocked a purely
hypothetical future violation, not an observed one, at the cost of directory
nesting on a module that already has no external distribution (it's
consumed exclusively via `go.work` path replacement, by one repo's worth of
engineers, under the same `.design/` review discipline as everything else
here). Reverted back to flat `process/`, `protocol/`, `history/`,
`claude/session/`. If a real drift shows up in review — someone genuinely
reaching past `AgentService` into engine internals — reconsider `internal/`
at that point, backed by an actual instance instead of a hypothetical one.

**Kept: the README `## Public API` section**, listing exactly what
`tingly-box` imports today: the `AgentService` façade +
`ExecutionHandle`/`StreamEvent` surface, the Claude message model
(`MessageEvent.Raw`'s concrete types, accepted as non-neutral per §8),
Claude production config (`Config`/`PermissionMode`/`ContextKey*`), and the
`NewAgentWithFactory`/`Driver`/`Transport`/`claude/fixture` test-harness
seam. This is the reviewable contract, kept by documentation and code review
rather than the compiler — consistent with how the rest of this repo governs
itself (`.design/*.md` + review, per `CLAUDE.md`).

Deliberately not done: no semver git tag. `agentboot` is consumed exclusively
through the repo's `go.work` path replacement, never `go get` at a pinned
version, so a tag would track nothing a consumer's build actually resolves
against. If `agentboot` is ever published or consumed via a real module
version, revisit this.

Verified: `go build`, `go vet`, and `go test` green in both the `agentboot`
module and the root module (`go build ./...`,
`go test ./remote/control/...`).
