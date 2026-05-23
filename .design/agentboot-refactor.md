# Agentboot Refactor

Design notes for simplifying the `agentboot` module. Agentboot was split into its
own Go module (`github.com/tingly-dev/tingly-box/agentboot`, see `go.work`) and a
new `AgentService` was added to expose its capabilities. In practice the module is
exercised by a **single feature** — the remote-control bot — yet it carries two
complete, parallel paradigms plus speculative generality. This doc records the
current state and the target design.

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
| Claude bot executor | `internal/remote_control/bot/agent_claude_code.go` | **new** engine: `AgentBoot.GetDefaultAgent()` + `agent.Execute` + manual `handle.Events()` loop |
| Stream writer | `internal/remote_control/bot/bot_stream.go` | **legacy** `MessageStreamer`+`CompletionCallback`; `OnMessage(any)` |
| Smart-guide executor | `internal/remote_control/bot/agent_smart_guide.go` | **legacy** `CompositeHandler` via `agent.ExecuteWithHandler` |
| Smart-guide agent | `internal/remote_control/smart_guide/agent.go` | takes `agentboot.MessageHandler` — but runs a **tingly-agentscope** ReAct agent, NOT agentboot's pipeline |
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
module and the root module (`internal/remote_control/...`, `remote/...`).

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

Net: the refactor stops after P1.5. The earlier P0/P1/P1.5 deletions removed a
genuinely redundant *parallel paradigm*; this PR removes only the
misleading/orphan accessors and keeps one coherent query + scalar surface.

### Verification per phase

- Module: `cd agentboot && go build ./... && go test ./...`
- Root: `go build ./... && go test ./internal/remote_control/... ./remote/...`
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
