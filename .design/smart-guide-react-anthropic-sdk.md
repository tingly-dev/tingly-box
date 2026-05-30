# Smart Guide: in-house ReAct loop on the official Anthropic SDK

> Pre-launch, low-risk replacement of the `tingly-agentscope` runtime.
> The Claude-Code-based redesign (`.design/smart-guide-on-claude-code.md`) is
> the north star but too large to land before launch. This plan ships a stable
> `@tb` now and is a **down payment** on that redesign — the tool handlers and
> session semantics here carry straight over.

## Goal

Replace `github.com/tingly-dev/tingly-agentscope` under Smart Guide with a small
hand-rolled ReAct loop on the **official** `github.com/anthropics/anthropic-sdk-go`
(`v1.37.0`, already used by `agentboot`). Surgical: only `smart_guide/` internals
change; the bot-layer contract is preserved so behavior, session semantics, and
UX are unchanged.

## Anthropic-first simplifications (decided)

We are anthropic-first, so **skip all provider-compat layers**:
- No provider abstraction, no OpenAI/Gemini format conversion.
- Session store persists **native `anthropic.MessageParam`** directly (via the
  SDK's `Message.ToParam()` and `Message.Accumulate(event)`), instead of a
  neutral `StoredMessage` type. This supersedes "section 4" below.
- Local SDK is `tingly-dev/anthropic-sdk-go` **v1.45.0** (submodule under
  `libs/anthropic-sdk-go`, wired via the existing `replace` in root `go.mod`;
  the `v1.37.0` require line is overridden by the replace). Confirmed APIs:
  `Messages.NewStreaming`, `(*Message).Accumulate`, `Message.ToParam`,
  `MessageParam`, `ToolUnionParam`, `ToolParam`, `ToolResultBlock`.

## Constraints / invariants (do not change)

- Public surface of `internal/remote_control/smart_guide/` stays compatible:
  `NewTinglyBoxAgentWithSession`, `ExecuteWithHandler`, `GetExecutor`,
  `SetWorkingDirectory`, and `handler.go` (`StreamHandler`, `Approver`,
  `CompletionResult`).
- `@tb` semantics unchanged: conversation preserved across turns, **pwd floats
  freely per turn**, tool approval via `IMPrompter`, streaming to chat.
- Talk to the tingly-box gateway exactly as today (baseURL + apiKey; `Model` =
  bot UUID rule routing).
- `agentboot` / `@cc` untouched.

## What changes

### 1. Dependency
- Root `go.mod`: add `github.com/anthropics/anthropic-sdk-go v1.37.0`.
- Remove `tingly-agentscope` + `/extension` requires after the swap;
  `go mod tidy`.

### 2. ReAct loop (new, replaces agentscope `ReActAgent`)
Client:
```go
client := anthropic.NewClient(
    option.WithBaseURL(baseURL),  // gateway
    option.WithAPIKey(apiKey),
)
```
Loop (per `ExecuteWithHandler` call):
```
messages = history ++ {user: text}
for i in 0..MaxIterations:
    stream = client.Messages.NewStreaming(ctx, {Model, System, MaxTokens, Tools, Messages})
    acc = anthropic.Message{}                  // SDK accumulator
    for ev := range stream:                    // forward text deltas
        acc.Accumulate(ev)
        if text delta -> StreamHandler.OnMessage(text)
    messages ++= acc.ToParam()                 // assistant turn (text + tool_use)
    if acc.StopReason == "tool_use":
        for block in acc.Content where type==tool_use:
            result, isErr = dispatch(block.Name, block.Input, toolCtx)   // approval inside
            toolResults ++= ToolResultBlock(block.ID, result, isErr)
        messages ++= {user: toolResults}
        continue
    break                                       // final answer
persist(messages); StreamHandler.OnComplete(CompletionResult{...})
```
Handle: `ctx` cancellation (`/stop`), `MaxIterations` cap, API errors →
`StreamHandler.OnError`.

### 3. Tools (define schemas + dispatcher; drop agentscope toolkit)
`[]anthropic.ToolUnionParam` with name/description/`InputSchema`, plus a
`map[string]handler` dispatcher. Underlying logic is **already ours** for most:

| Tool | Source |
|---|---|
| `bash` | reuse `ToolExecutor` (allowlist + approval) from `tools_bash.go` |
| `get_status` | reuse `GetStatusFunc` |
| `change_workdir` | reuse `UpdateProjectFunc` + `ToolExecutor.SetWorkingDirectory` |
| `send_file` | reuse `ToolContext.SendFile` (+ approval) |
| `read` / `write` / `edit` | **reimplement** as small plain-Go handlers (these came from `agentscope/extension/tools`); ~tens of lines each, 10MB cap, mkdir-on-write, exact-match edit |

The tool *registration wrappers* (`tools_register.go`, the agentscope
`tool.Toolkit` types in `tools_bash.go`/`tools_send_file.go`) are replaced by
plain schema definitions + dispatch; the executor logic stays.

### 4. Session store — native params (no neutral type)
Per the anthropic-first decision, there is **no `StoredMessage`**. The engine
owns history as `[]anthropic.MessageParam`; the store persists it verbatim.
- `session_store.go`: `Save/Load/Delete(chatID, []anthropic.MessageParam)`,
  one JSON file per chat. Verified to round-trip byte-for-byte through
  `encoding/json`. Replaces the old incremental `AddMessage/ClearMessages`.
- The completion callback snapshots `agent.History()` and `Save`s the whole
  thing after each run; the executor `Load`s it before the run. The old
  pre-execution user-message append is gone.
- **Migration:** pre-launch, old agentscope-format `*-smartguide` session files
  are simply ignored (new filename `*-smartguide.json`); no converter needed.

## Files (as shipped)

- New engine package `internal/afk/` (Agent Framework Kit / Away From Keyboard):
  `engine.go` (ReAct loop + `Tool`
  and `StreamSink` interfaces), `client.go` (gateway client), `engine_test.go`.
  Lives in the **root module** (not agentboot) so it can use the SDK v1.45 APIs
  wired by the root `replace`; agentboot pins an older SDK without that replace.
- New: `smart_guide/tools_file.go` (native read/write/edit).
- Rewrite: `smart_guide/agent.go` (TinglyBoxAgent drives the engine; keeps public
  methods, adds `History()` / `LastAssistantText()`), `session_store.go`,
  `tools_register.go` (now `BuildTools() []anthropic.Tool`), `tools_bash.go`
  (native `sh -c` exec; tools implement the engine `Tool` interface),
  `tools_send_file.go` (engine `Tool` signature).
- Deleted: `smart_guide/tools_handoff.go` (agentscope-typed, unregistered).
- Touch: `bot/agent_smart_guide.go`, `bot/bot_agent.go`, `bot/bot_command.go`
  (drop agentscope imports; native params; store `Save/Delete`).
- Keep as-is: `config.go`, `prompts.go`, `prompts/`, `handoff.go`, `tools.go`
  (`ToolExecutor` had no agentscope coupling).
- `go.mod` / `go.sum`: drop `tingly-agentscope`.

## Phases

0. **Dependency + skeleton**: add anthropic-sdk-go to root module; stub
   `react_loop.go` compiling against the SDK.
1. **Loop + streaming**: implement the loop with text streaming to
   `StreamHandler`; no tools yet; verify a plain Q&A turn end to end.
2. **Tools**: schemas + dispatcher; port bash/get_status/change_workdir/send_file
   executor calls; implement read/write/edit; wire approval.
3. **Session store**: `StoredMessage` + converters; persist/load; swap
   `agent_smart_guide.go` message usage.
4. **Cutover**: route `TinglyBoxAgent` through the new loop; remove the
   agentscope `ReActAgent` embedding.
5. **Delete agentscope**: remove imports + `go.mod` requires; `go mod tidy`;
   port the meaningful tests (`tools_test.go`, `agent_test.go`,
   `tools_send_file_test.go`) to the new types.
6. **Verify**: build + tests; manual @tb smoke (navigate / read / edit /
   send_file / approval / multi-turn).

## Risks
- **Network for `go get`** in restricted envs (anthropic-sdk-go already in
  `go.sum` graph via agentboot, but root module needs it).
- **Streaming event shape**: rely on the SDK's `Accumulate` helper rather than
  hand-parsing deltas.
- **Session round-trip**: own the stored schema; don't try to round-trip raw SDK
  param structs.
- **Gateway auth header**: confirm `option.WithAPIKey` sends what the gateway
  expects (x-api-key); else use `WithAuthToken`.

## Why this is not throwaway
The read/write/edit/bash/get_status/change_workdir/send_file handlers become,
respectively, native-tool reliance and the MCP-server handlers in the Claude
Code redesign. Only the loop *host* changes later (in-process SDK →
claude subprocess). Session-anchor/logical-pwd decoupling is a future concern;
today's semantics are preserved as-is.
