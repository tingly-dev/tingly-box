# DeepSeek

Provider-specific findings for DeepSeek's OpenAI-compatible Chat Completions
endpoint (`api.deepseek.com/v1/chat/completions`). Request/response
transforms live in `internal/protocol/ops/request_openai_deepseek.go` and
`response_openai_extensions.go`.

## Tool-call message validation (verified live, 2026-09-17)

DeepSeek validates the tool-call shape of the message list strictly. The
gateway hit this when Codex (Responses API) is routed to DeepSeek and the
history contains an interrupted or injected tool call; the general fix is
`RepairResponsesToolCalls`, see `.design/responses-tool-call-repair.md`.

Probe: hand-written message lists, `max_tokens: 16`, one `shell` function
tool declared. `a`/`b` are two parallel tool calls in one assistant message.

| # | Model | Message shape | HTTP | Error / note |
|---|---|---|---|---|
| T1 | deepseek-chat | `tool` message with `tool_call_id: ""`, then user (pre-repair shape of a Codex automation orphan) | 400 | `Messages with role 'tool' must be a response to a preceding message with 'tool_calls'` |
| T2 | deepseek-chat | user, assistant(a,b), tool a, user (pre-repair shape of an interrupted parallel call) | 400 | `An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'. (insufficient tool messages following tool_calls message)` |
| T3 | deepseek-chat | user, assistant(a,b), tool a, tool b = placeholder, user (repaired) | 200 | model answers the new user turn |
| T4 | deepseek-chat | `tool` message with a `tool_call_id` but no preceding `tool_calls`, then user | 400 | same text as T1 |
| T5 | deepseek-chat | orphan output rewritten as user text `[tool output: automation_update]\n…`, then user (repaired) | 200 | model reads the quoted output |
| T6 | deepseek-reasoner | T3 shape, assistant `tool_calls` **without** `reasoning_content` | 200 | |
| T7 | deepseek-reasoner | T3 shape, assistant `tool_calls` with `reasoning_content: ""` (what `convertThinkingToReasoningContent` emits) | 200 | |
| T8 | deepseek-chat + `thinking: {type: enabled}` | T7 shape | 200 | |

End-to-end: the same three Codex histories (automation orphan, interrupted
parallel call, orphan with a stale `call_id`) were run through the real
server path (`PreprocessInputData` → SDK unmarshal →
`ConvertOpenAIResponsesToChat`) and the resulting bodies posted as-is. All
three returned 200.

Takeaways:

- Both directions are enforced: every `tool_calls` id needs a `tool`
  message right after, and every `tool` message needs a preceding
  `tool_calls`. Consecutive `user` messages are accepted.
- The parenthesised suffix distinguishes the cases: "insufficient tool
  messages" means some but not all calls were answered (parallel call
  interrupted midway); the "must be a response to a preceding message"
  text means a dangling `tool` message.
- DeepSeek's docs say `reasoning_content` must be passed back in thinking
  mode during tool-call loops, but as of this probe both an empty string and
  an absent field are accepted. The existing transform that fills `""` is
  sufficient; keep the probe in mind if DeepSeek tightens this.

## Related

- `.design/responses-tool-call-repair.md`: the repair step and the
  protocol difference behind it.
- Same class of failure in other gateways: LiteLLM #32992, cc-switch
  #7074 / #7400, opencodex #4870, gptme #3846.
