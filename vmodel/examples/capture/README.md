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

**These are lightly redacted copies, not faithful captures.** A real capture is
verbatim, and Claude Code embeds facts about its environment in the request: the
working directory and home path, and `<system-reminder>` blocks listing the
skills, the agent types and every `CLAUDE.md` found up the directory tree.
`redact.py` masks exactly that and nothing else (see [what is
redacted](#what-is-redacted)); everything else, including Claude Code's own
system prompt and tool descriptions, is as sent.

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
Authorization: Bearer sk-ant-oat01-[REDACTED]
Content-Type: application/json
User-Agent: claude-cli/2.1.329 (external, sdk-cli)
X-Claude-Code-Session-Id: 24bc0780-4faf-4003-9022-f3ab59a89433
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
x-claude-code-prompt-id: 4cbe247f-8724-4d02-92db-93b5e521ba12
x-claude-code-request-class: main
x-claude-code-sandy-plover: []
x-client-request-id: 4ead72de-daab-4e50-82df-d846299c9ac2
Connection: keep-alive
Host: 127.0.0.1:18182
Accept-Encoding: gzip, deflate, br, zstd
Content-Length: 78915
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

`redact.py` masks what is private about the machine and leaves the rest alone:
header names, casing and order, the body's JSON structure and key order, the
client's system prompt and tool descriptions. It recomputes `Content-Length` and
the `index.jsonl` ranges.

- **Paths**: `/home/<user>/…` and `/Users/<user>/…` become `/home/user/[REDACTED]`.
  (This example ran under `/dev/shm`, so there was nothing to mask there.)
- **Environment reminders**: a `<system-reminder>` block is kept whole only if it
  is the environment, the model name, the token budget or the date. Any other
  one (skills, agent types, deferred tools, `CLAUDE.md`, attribution settings)
  keeps its first line and closing tag and its content becomes `[REDACTED]`:

  ```
  <system-reminder>
  The following skills are available for use with the Skill tool:
  [REDACTED]
  </system-reminder>
  ```
- **Credentials**: `Authorization` / `x-api-key` keep the scheme and token kind:
  `Bearer sk-ant-oat01-[REDACTED]`. The token used was a fake one anyway.

Not masked, deliberately: the device id, session id and request ids (random
values generated for a throwaway `HOME`), and `cch`. Because the body changed,
the `cch` hash in the direct billing header no longer verifies against it.
`raw/conn-N.out` (our own answer) and timestamps are as recorded.

`vmodel/capture/example_test.go` checks the example against its index, the header
casing/order and the proxy/direct differences, and fails if a home path, an
unmasked reminder or a credential comes back.

## How these were made

```sh
mkdir -p /dev/shm/cc/dot /dev/shm/cc/work && echo '{"hasCompletedOnboarding":true}' > /dev/shm/cc/dot/.claude.json
tb vmodel --capture ./cap/oauth-direct --json --port 18162 &        # prints {"url": ...}
( cd /dev/shm/cc/work && env -i PATH="$PATH" TERM=xterm HOME=/dev/shm/cc/dot \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 DISABLE_ERROR_REPORTING=1 DISABLE_AUTOUPDATER=1 \
    ANTHROPIC_BASE_URL=http://127.0.0.1:18162 CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-fake \
    _CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1 \
    claude -p "say hi" --model claude-sonnet-4-6 < /dev/null )
python3 vmodel/examples/capture/redact.py ./cap/oauth-direct        # then copy the directory here
```

Run the client from a working directory (and `HOME`) whose parents hold no
`CLAUDE.md` and no `.claude/skills`: Claude Code embeds all of them in the first
user message, so a capture taken inside your home directory contains your private
instructions. `redact.py` masks the skills, agent types and `CLAUDE.md` reminders,
but it cannot know everything: read the result before sharing it.
