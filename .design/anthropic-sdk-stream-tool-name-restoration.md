# Restoring tool names in Anthropic streaming responses

Context for `internal/client/claude_tool_names.go` and the `remap*RequestToolNames` /
`restoreToolNames*` helpers in `internal/client/claude_client.go` (PR #1634).

## Why tool names are rewritten at all

On the Claude OAuth path, Anthropic classifies some traffic as third-party and bills
it to Extra Usage instead of the subscription (surfacing as a hard 400
`You're out of extra usage` when that bucket is empty). Tool naming appears to be one
of the signals. Clients other than Claude Code send snake_case tool names, so the
gateway folds them towards the shape Claude Code itself sends:

- MCP-namespaced names (`mcp__<server>__<tool>`, and tingly-box's own
  `tingly_box_mcp__…` via `internal/tool.NormalizeToolName`) pass through **verbatim** —
  Claude Code sends them lowercase and unfolded, and the namespace is what the client
  routes on. A single-underscore `mcp_` prefix is *not* a namespace and folds normally.
- `oauthToolRenameMap` wins for well-known Claude Code tools so official spellings are
  preserved (`ls` → `LS`, not `Ls`).
- Everything else folds mechanically to TitleCase (`read_file` → `ReadFile`).
- Collisions are skipped: if a fold target is already taken by another tool in the
  request, that name is left alone (a wrong reverse-map entry would mis-dispatch
  tool results).

An earlier report described a hard ~16-snake-case-name cutoff; later probing did not
reproduce it (five arms up to 48 lowercase names at 100% density all passed). The exact
classifier trigger is unconfirmed. The fold is kept because it is a no-op at worst.

## Why the rename must cover the whole request

`planToolRenames` computes the plan once from `tools[]`; three sites must agree with it:

1. `tools[]` itself,
2. `tool_choice.OfTool.Name` (a pin naming a folded-away tool is a hard 400 from
   Anthropic: `Tool 'foo__bar' not found in provided tools`),
3. `tool_use` blocks in prior assistant turns (each follow-up turn would otherwise
   re-send the original snake_case names, compounding with conversation length).

Folding a site independently is wrong because the plan deliberately skips colliding
folds — an independent fold could pin `tool_choice` to a name not in `tools[]`, or to
the wrong tool. Hence the single entry point `remap{,Beta}RequestToolNames(req)`.

## Why restore happens at the HTTP layer for streams

Non-streaming responses are decoded `Message` objects; `restoreToolNamesInMessage`
rewrites them before returning. A streamed response is handed to the caller as an SSE
stream, and the SDK (pinned `anthropic-sdk-go` fork, `tingly-dev-v1.66.0`) offers no
per-request, decoded-event hook:

- Both `MessagesNewStreaming` and `BetaMessagesNewStreaming` go through the same
  `requestconfig.ExecuteNewRequest` path as non-streaming calls; middleware runs there.
- The SDK never sets `Accept-Encoding`, so Go's transport does transparent gzip
  decoding before middleware sees the body — plaintext.
- `anthropic-sdk-go`'s `ssestream` has a decoder registry (`RegisterDecoder`, looked up by the exact
  Content-Type value), but nothing registers in it, so the only hook in use is the raw
  `io.Reader`. (The Codex client registers in openai-go's `ssestream` under a Content-Type
  parameter its round tripper sets; `.design/codex.md` §6.)
- Middlewares re-run per retry attempt; the Claude OAuth client sets `MaxRetries(0)`.

So `Guard`/`GuardBeta` attach `restoreToolNamesMiddleware(reverseMap)` via
`option.WithMiddleware` when the plan is non-empty. `sseToolNameRewriter` rewrites the
body line-at-a-time to preserve streaming; a tool name appears in exactly one event
type (`content_block_start` with `content_block.type == "tool_use"`), everything else
passes through byte-for-byte, including malformed lines.

## Test layout

- `claude_tool_names_test.go` — unit coverage of the fold, the plan, and every request
  site (v1 types; Beta has a smoke test).
- `claude_tool_names_wire_test.go` — httptest round trips asserting the serialized
  body Anthropic would receive and the stream the client reads back (Beta full, v1
  compact, MCP-only no-op).
- `claude_round_tripper_test.go` / `claude_client_test.go` — restore helpers and
  Guard-level wiring.
