# DeepSeek

Provider-specific findings for DeepSeek's OpenAI-compatible Chat Completions
endpoint (`api.deepseek.com/v1/chat/completions`). Request/response
transforms live in `internal/protocol/ops/request_openai_deepseek.go` and
`response_openai_extensions.go`.

## Tool-call message validation (verified live, 2026-09-17)

DeepSeek validates the tool-call shape of the message list strictly. The
gateway hit this when Codex (Responses API) is routed to DeepSeek and the
history contains an interrupted or injected tool call; the general fix is
`RepairResponsesToolCalls`, see `.design/protocol-responses.md`.

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

- `.design/protocol-responses.md`: the repair step and the
  protocol difference behind it.
- Same class of failure in other gateways: LiteLLM #32992, cc-switch
  #7074 / #7400, opencodex #4870, gptme #3846.

## First-party Search and Fetch (#2484)

The built-in `mcp_web_search` and `mcp_web_fetch` tools can use DeepSeek's
`POST /v1/tools/search` and `POST /v1/tools/fetch` APIs. Enable the desired
MCP/Server Tools as before; backend selection does not enable a disabled tool
or change `native_websearch_strip` semantics.

### Credentials and default selection

Hosted calls resolve an **enabled API-key provider** whose OpenAI base (or
fallback API base) has the exact host `api.deepseek.com`. The key is read on
each call, never copied into source Env or persisted as a second secret. The
provider's proxy is respected. If multiple credentials qualify, the lowest
provider UUID wins deterministically. Anthropic-only DeepSeek bases are
normalized to the same host's `/v1/tools/...` endpoints.

- With a source's `SERPER_API_KEY` configured, Search/Fetch continue using
  Serper/Jina.
- Without that key, an available DeepSeek credential makes both tools use
  DeepSeek. Without either credential, Fetch still uses Jina; Search is not
  ready.
- Advanced overrides use the source's existing environment map:
  `WEB_SEARCH_BACKEND=auto|deepseek|serper` and
  `WEB_FETCH_BACKEND=auto|deepseek|jina`. Explicit DeepSeek selection without a
  usable provider is unconfigured; it does not silently fall back.
- Standalone `mcp-builtin` also accepts `DEEPSEEK_API_KEY` and optional
  `DEEPSEEK_BASE_URL`. Hosted provider-backed calls run in-process so that
  credentials stay out of the stdio subprocess and usage is recorded by the
  gateway. Standalone stdio is not a gateway billing/observability surface.

Search accepts `query`, `count` (1–50; upstream default 10), `need_content`,
`ori_question`, and the existing domain allow/block lists. By default results
render `summary`, falling back to `snippet`, rather than full content. Full
content is opt-in with `need_content=true`. Scores, authority and dates are
annotated without filtering low-scoring results. `ori_question` is forwarded
**only if explicitly provided**; conversation messages are never automatically
sent to search. Fetch accepts `url`; the legacy `prompt` is optional and is
not sent to DeepSeek.

### Gateway API

Model-authenticated clients can call `/v1/tools/search` and `/v1/tools/fetch`
directly, or append `/tools/search` and `/tools/fetch` to their existing
`/tingly/:scenario[/v1]` base. Bodies and upstream error envelopes are passed
through without chat/protocol conversion. Only the resolved provider's Bearer
key is sent upstream. Team-scoped sharing keys cannot use these direct APIs
to bypass their assigned model/provider routing.

HTTP 408, 429 and 5xx responses are retried at most twice with cancellable
500 ms / 1 s backoff. Other 4xx responses are not retried. MCP callers receive
a clear top-up message on 402; direct callers retain the upstream 402 error
envelope. No cross-vendor fallback or client-side QPS limiter is applied.

### Request billing

DeepSeek currently charges **CNY 0.005 per successful request producing
results**, independently of `count`. Empty searches, empty fetched content,
failures and timeouts contribute zero billable requests. One usage event is
recorded per logical invocation, not per retry:

- `event_type=requests`, model `deepseek-tools/search` or `deepseek-tools/fetch`;
- `billable_requests`, price snapshot `unit_price=0.005`, `currency=CNY`;
- `request_cost_cny` is separate from all token counters (which remain zero).

The measures survive raw queries and daily pre-aggregation, and are exposed
in stats, time series and request records. The Dashboard displays CNY request
spend separately from token usage. Existing token records migrate with
`event_type=tokens` and zero request cost. Provider, latency, error status,
and available caller/scenario/rule identity are retained for observability.

API reference: <https://api-docs.deepseek.com/zh-cn/guides/search/>.
