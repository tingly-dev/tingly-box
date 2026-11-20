# Capture example: Claude Code 2.1.329

What `tb vmodel --capture` records when a real client talks to it. These are two
captures of `claude -p "say hi"` (Claude Code 2.1.329, OAuth token) against a
local `tb vmodel`: once as a gateway/proxy would see it, once pretending to talk
to api.anthropic.com directly (`_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1`).
Design and rationale: [`.design/vmodel-capture.md`](../../../.design/vmodel-capture.md);
usage: [`vmodel/README.md`](../../README.md#capturing-what-a-real-client-sends).

```
claude-code-2.1.329/
  oauth-proxy/      one capture directory per run
  oauth-direct/
    meta.json            argv, listen URL, model ids; "redacted": true
    index.jsonl          one line per request: seq, conn, byte ranges, method, target, status
    raw/conn-0001.in     the request, as the client sent it (redacted, see below)
    raw/conn-0001.out    the response, as we sent it
redact.py               how the example was redacted
```

**These are redacted copies, not faithful captures.** A real capture is verbatim
and contains credentials, device/session ids and whatever the client embeds in
its prompt (the working directory, and every `CLAUDE.md` and skill found up the
directory tree). `redact.py` was run over both before they were checked in; it
changes only the request side. See [what is redacted](#what-is-redacted).

## Read one

```sh
cd vmodel/examples/capture/claude-code-2.1.329/oauth-direct
cat index.jsonl                                   # one request, ranges into raw/
sed -n '1,/^\r$/p' raw/conn-0001.in | tr -d '\r'  # request line and headers, as sent
tail -c +"$(( $(sed -n '1,/^\r$/p' raw/conn-0001.in | wc -c) + 1 ))" raw/conn-0001.in | head -c 600   # body start
tr -d '\r' < raw/conn-0001.out                    # what we answered (chunked SSE)
```

The headers of `oauth-direct`, verbatim from the file (the `anthropic-beta`
value is cut here only for width):

```
POST /v1/messages?beta=true HTTP/1.1
Accept: application/json
Authorization: Bearer sk-ant-oat01-[redacted]
Content-Type: application/json
User-Agent: claude-cli/2.1.329 (external, sdk-cli)
X-Claude-Code-Session-Id: 00000000-0000-4000-8000-000000000001
X-Stainless-Arch: x64
X-Stainless-Lang: js
X-Stainless-OS: Linux
X-Stainless-Package-Version: 0.128.0
X-Stainless-Retry-Count: 0
X-Stainless-Runtime: node
X-Stainless-Runtime-Version: v26.12.1
X-Stainless-Timeout: 600
anthropic-beta: claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,…
anthropic-dangerous-direct-browser-access: true
anthropic-version: 2023-06-01
x-app: cli
x-claude-1p-request-class: main
x-claude-code-prompt-id: 00000000-0000-4000-8000-000000000002
x-claude-code-request-class: main
x-claude-code-sandy-plover: []
x-client-request-id: 00000000-0000-4000-8000-000000000003
Connection: keep-alive
Host: 127.0.0.1:18162
Accept-Encoding: gzip, deflate, br, zstd
Content-Length: 12144
```

## What only a raw capture shows

- **Header names and order as the client wrote them**: `X-Stainless-*` capitalised,
  `anthropic-beta`, `x-app` and `x-claude-*` lower-case, `Connection`/`Host` after.
  `net/http` would have shown `X-App`, `Anthropic-Beta`, … in a map with no order.
- **`anthropic-beta` order**, which is itself the thing that changes per release.
- **The body's key order and bytes** (`model, messages, system, tools, metadata,
  max_tokens, thinking, context_management, output_config, [diagnostics], stream`).
- **One request per run.** `index.jsonl` has a single line in both cells. An
  earlier vmodel answered Claude Code with a malformed stream, so the client
  quietly retried without streaming and every capture held a second request; a
  capture makes that visible and `example_test.go` pins it.

## Proxy vs direct

| | `oauth-proxy` | `oauth-direct` |
| --- | --- | --- |
| extra betas | – | `prompt-caching-scope`, `tool-provenance`, `thinking-binding-controls`, `cache-diagnosis` |
| extra headers | – | `x-claude-1p-request-class`, `x-claude-code-prompt-id`, `x-claude-code-request-class`, `x-claude-code-sandy-plover`, `x-client-request-id` |
| billing header (`system[0]`) | `cc_version=…; cc_entrypoint=sdk-cli;` | adds `cch`, `cc_prompt_id`, `cc_turn_origin`, `cc_prompt_index`, `cc_turn_index` |
| body | – | adds `diagnostics` |

## What is redacted

`redact.py` keeps header names, casing and order and the body's JSON structure
and key order, and recomputes `Content-Length` and the `index.jsonl` ranges.

- **Credentials**: `Authorization` / `x-api-key` keep only the scheme and token kind
  (`Bearer sk-ant-oat01-[redacted]`). The token used was a fake one anyway.
- **Ids**: every UUID becomes `00000000-0000-4000-8000-0000000000NN`, numbered by
  first appearance (so the prompt id repeated in a header and in the billing
  header still matches); the 64-hex `device_id` becomes zeros.
- **`cch`** becomes `xxxxx`: it is a hash of the exact body, which no longer matches.
- **Paths** under `/home`, `/Users`, `/root`, `/dev/shm`, `/tmp` are replaced.
- **Long text**: every JSON string over 120 characters is cut to its first 60
  characters plus `…[redacted N chars]`. That is Claude Code's own system prompt,
  tool descriptions and the `<system-reminder>` blocks (environment, skills,
  agent types). The shapes, tool names and cache-control markers stay; the
  proprietary text does not. The unredacted bodies were about 88 KB.
- `raw/conn-N.out` (our own answer) and timestamps are left as they were.

`vmodel/capture/example_test.go` checks the example against its index, the header
casing/order, the proxy/direct differences and that nothing above has crept back.

## How these were made

```sh
mkdir -p /dev/shm/cc/home /dev/shm/cc/work && echo '{"hasCompletedOnboarding":true}' > /dev/shm/cc/home/.claude.json
tb vmodel --capture ./cap/oauth-direct --json --port 18162 &        # prints {"url": ...}
( cd /dev/shm/cc/work && env -i PATH="$PATH" TERM=xterm HOME=/dev/shm/cc/home \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 DISABLE_ERROR_REPORTING=1 DISABLE_AUTOUPDATER=1 \
    ANTHROPIC_BASE_URL=http://127.0.0.1:18162 CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-fake \
    _CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1 \
    claude -p "say hi" --model claude-sonnet-4-6 < /dev/null )
python3 vmodel/examples/capture/redact.py ./cap/oauth-direct        # then copy the directory here
```

Run the client from a working directory (and `HOME`) whose parents hold no
`CLAUDE.md` and no `.claude/skills`: Claude Code embeds all of them in the first
user message, so a capture taken inside your home directory contains your private
instructions. `redact.py` cuts that text, but do not rely on it for secrets: look
at the capture before sharing it.
