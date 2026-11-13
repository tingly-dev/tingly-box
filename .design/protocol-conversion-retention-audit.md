# Protocol conversion retention audit — 2026-11-13

Protocol converters can lose usage delivered after a finish reason, tool inputs
provided at block start or in final snapshots, and machine-readable failures.
This audit records independent protocol fixes and their regression contracts.
They affect ordinary agent turns as well as context summarization.

## Fixed and covered by regression tests

| Upstream → client | Defect | Contract after the fix |
|---|---|---|
| Anthropic → Chat, streaming | Ignored initial tool input; `max_tokens` became `stop`/`tool_calls` | Preserve initial JSON when no input deltas arrive, prefer deltas otherwise, and return `length` for truncation. MCP callbacks receive the same arguments. Initial text/thinking are retained. |
| Anthropic → Chat/Responses, nonstream | Re-encoding SDK `any` inputs rounded large integer IDs | Read the original tool input JSON; use typed input only for programmatically constructed blocks. |
| Anthropic → Chat, nonstream | Read `text` instead of `thinking` from a thinking block | Preserve visible thinking as the existing `reasoning_content` extension. |
| Responses → Chat/Anthropic, streaming | Tools were announced but `.done` arguments never reached the client; final output was skipped after any streamed content | Reconcile function arguments and content per output item/part. Fill missing snapshot suffixes once; retain separately completed text, refusal, reasoning summaries and tools. |
| Responses → Chat, nonstream | Visible reasoning summaries were omitted | Preserve summaries as `reasoning_content`. |
| Responses → Chat, stream and nonstream | Tool presence overrode incomplete status | Preserve `length`/`content_filter` even when partial tool calls are present. |
| Responses → Chat/Anthropic, streaming | Failure codes were omitted, or `param` was used as the code | Preserve `error`/`response.failed` code and message. Do not turn failure into successful completion. |
| Chat → Anthropic V1/Beta, streaming | Every error after finish reason was treated as a dirty socket close | Explicit SDK/SSE provider errors remain failures. The pre-existing dirty-transport-teardown salvage behavior stays covered separately. |
| Google → Chat/Anthropic V1/Beta, streaming | Returned at finish reason, losing usage-only tails and late failures | Drain the iterator before emitting terminal success. Preserve final usage; report late errors and missing terminal events. |
| Google → Chat/Anthropic, streaming | Missing Chat tool indices; text/tool Anthropic indices collided | Stable distinct tool indices; close text before opening tool blocks; resume subsequent text in a fresh block. Return `tool_use` when appropriate. |
| Google → Chat/Anthropic, stream and nonstream | Dropped thought-token/cache detail; counted cached prompt tokens twice in Anthropic | Normalize usage centrally: cached tokens are a prompt subset, generated tokens include thoughts, and details remain available. |

Google's V1/Beta stream handlers now share a lifecycle implementation while
retaining their different finish-reason mappings. A streaming failure already
written to the client must not be followed by a separate HTTP JSON error body.

The focused tests are `stream/conversion_retention_test.go`,
`stream/google_retention_test.go`, and `nonstream/conversion_retention_test.go`.
They assert actual output fields, argument bytes (including integers greater
than 2^53), block indices, terminal ordering and absence of duplicate content.
Existing golden-sequence, request replay, tool-result, image, cache and thinking
regressions remain part of the broader protocol suite. The wire golden now
expects `length` for Anthropic truncation; the two repaired R3 IR round-trip
exceptions have been removed rather than continuing to exempt those routes.

## Scope checked, with remaining capability gaps

The existing `internal/protocoltest/ir_roundtrip_gaps.go` still records request
losses through the Anthropic intermediate representation: output limits,
sampling/metadata, structured output, reasoning effort, developer messages and
image detail, plus some response identity and estimated-usage differences.
A green suite with those explicit exceptions is not full IR equivalence.

This is not a claim that every protocol has a lossless mapping to every other
protocol. The request conversion and other response directions still have
explicit work remaining:

- Chat/Anthropic → Responses still lack full visible reasoning/refusal mapping;
  preserving input/output token accounting does not establish content parity.
- Responses file-ID-only images, input files/audio and some multimodal tool
  results are unsupported or filtered by request conversion. In particular,
  Responses → Anthropic currently keeps only text in array-valued function
  outputs. Existing image tests cover supported URL/base64 paths, not these
  missing cases.
- Custom tools, hosted tools and MCP outputs need a capability policy and
  protocol-specific argument/result mappings. Treating all of them as ordinary
  function calls is not sufficient; some paths currently skip or coerce them.
- Encrypted compaction/reasoning state and provider-signed thought blocks have no
  general cross-provider representation. Visible reasoning summaries are not
  equivalent to those opaque states.
- The legacy Chat/Anthropic → Google stream helpers still have narrower field
  coverage. They are not selected by `dispatchGoogle` (Google is an upstream
  target there). Google upstream → Responses clients remains explicitly
  unsupported. Google → Chat streaming usage is present on the wire, but its
  current handler signature still does not return usage for gateway accounting.

Future work should make unsupported conversions explicit at the capability
boundary, add request-side negative tests for silent filtering, and extend the
provider/client acceptance matrix. Passing these regression tests does not
establish complete cross-provider compatibility or readiness for remote compaction.

## Repeatable gate

```sh
go test ./internal/protocol/... ./internal/forwarding/... \
  ./internal/protocolserver ./internal/client ./internal/protocoltest \
  -skip '^TestAnthropicNonStreamRequest$'
```

`TestAnthropicNonStreamRequest` is an existing manual local-server/provider test;
it is not part of the credential-free gate. This standalone fix does not add
compact endpoints, client configuration defaults, vmodel behavior or a new CLI
acceptance command. Those changes belong to the separate compaction work.
Live-provider summary quality, opaque-state replay and real production load
remain separate gates.
