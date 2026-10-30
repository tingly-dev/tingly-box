# vmodel scripts — scripted multi-step interactions

## Problem

`vmodel` is a good deterministic stand-in for an LLM, but a *specific
interaction sequence* — "call `Read`, get the result, call `Edit`, hit a 529,
retry, answer" — had no quick way to be written down and exercised. The
existing pieces cover fragments: always-fail mocks (one shape), `sequence`
(a status program, text only), a single-tool mock. Standing up an ad-hoc
`httptest.Server` per scenario reintroduces what vmodel exists to remove.

## Decision: engine and schema in vmodel, authoring layers on top

| Concern | Where | Why |
| --- | --- | --- |
| Simple, repeatable interactions | **vmodel**, config-driven (YAML) | Wire-correct streaming, usage, mid-stream failure for all three protocols come for free, and the config style already exists (`lb` faults, harness scenarios). |
| Logic that depends on the request | Python `tingly.Server` (a provider) | Arbitrary code belongs in a real language, not a DSL. Unchanged. |
| Writing and launching scripts from Python | `tingly.vmodel` (PR2) | A thin authoring layer that compiles to the **same** YAML schema; it adds no semantics of its own. |

A script **is** a `vmodel.SequenceConfig`: the existing sequence engine
(`vmodel/sequence.go`, see [`vmodel-sequence.md`](./vmodel-sequence.md)),
generalised from "a status per request" to "a full outcome per request".
Per-request `Snapshot()`, the atomic cursor and `on_exhaust` are unchanged.

## Schema

```yaml
id: refactor-flow          # optional; defaults to the file name
on_exhaust: clamp          # loop (default) | clamp | fail
steps:
  - say: "Let me look at the file."
    tool: {name: Read, arguments: {file_path: /tmp/a.go}}
  - tool: {name: Edit, arguments: {file_path: /tmp/a.go, old_string: foo, new_string: bar}}
  - 529                                        # bare number == {status: 529}
  - say: "Done."
    usage: {input: 1200, output: 40, cache_read: 1000}
```

One step = one request's outcome:

| Field | Meaning |
| --- | --- |
| `status` | `200`/omitted → success; `400–599` → pre-content error envelope (type/message derived from the status). A bare number is shorthand for this. |
| `say` | Response text. Empty on a plain success falls back to `default_content`, then a module default. Empty on a `tool` step means *no* lead-in text. |
| `tool` | `{name, arguments, id?}` — one tool call. Without an `id`, each *served request* gets a unique `toolu_<script>_<n>` (so `repeat:`, loops and clamping never repeat one); an explicit `id` is used as written (1–64 of `A-Za-z0-9_-`, the shape real APIs accept; generated ids are sanitised and truncated to it). `arguments` omitted means `{}`. A tool step with no `say` has no text block — exactly what the script says. |
| `stop_reason` | Override, in a **protocol-neutral vocabulary** (Anthropic's words): `end_turn`, `tool_use`, `max_tokens`, `stop_sequence`. OpenAI renders them as `stop`, `tool_calls`, `length`, `stop`. Anything else is rejected. Defaults: `end_turn` (`tool_use` on a tool step). |
| `usage` | `{input, output, cache_read, cache_write, reasoning}` advertised on the stream of `/messages` and `/chat/completions`. The direct `/responses` endpoint estimates usage from the text and ignores `usage` / `stop_reason` (its own pre-existing rendering). |
| `midstream` | `{mode: close\|event\|eof, after_events: N}` — a success whose stream is cut. Not combinable with an error `status`. |
| `repeat` | Serve the step N consecutive times. |
| `error_message`, `error_type` | Override the error envelope of an error step. |

Top level: `id`, `name`, `description`, `delay`, `default_content`, `on_exhaust`, `steps`.

**Parsing is strict**, at every nesting level (`usage:`, `midstream:`, `tool:` included). Unknown fields (`sya:`, `args:` for `arguments:`, `input_tokens:`),
out-of-range statuses, error steps carrying a body, unknown modes, `stop_reason: tool_use` without a tool and
duplicate ids fail with a step-numbered message (`vmodel.ParseScript`). A typo
must never silently become a default step.

### What a step is *not* (yet)

No request matching (`when:`), no *implicit* conversation detection (sessions are explicit, below), no `think` blocks,
no multiple tool calls per step, no request capture. See *Phases*.

## Serving: the script directory

`<config-dir>/vmodels/*.yaml|yml` — each file becomes one model, registered
under its id in **both** protocol registries (so the same file answers
`/messages` and `/chat/completions`/`/responses`), and **each protocol runs its
own independent copy of the program** — its own cursor. This is deliberate: a
request on one wire (a probe, a second client, a stray `chat()` call) must never
consume a step of the other, or a test's outcome would depend on call order it
cannot see. Within one protocol the cursor is still shared by every client of
that model — isolate a run with a session (below); rewriting the file is the
reset.

### Sessions: `<id>@<session>`

Callers of one model share its cursor, which makes concurrent tests (or a probe
plus an agent) consume each other's steps. A client requests the model
**`<id>@<session>`** — e.g. `read-edit@test-42` — to get an independent run:
its own cursor, created on first use, on every wire (Anthropic, OpenAI chat,
Responses). The plain `<id>` is the shared default session, unchanged.

- **Explicit, never inferred.** The session is a string the caller chose. A
  conversation fingerprint (hash of the first message) was rejected: two
  conversations with the same prompt would collide, and a magic key is exactly
  the kind of hidden coupling that makes a test hard to read.
- **In the model name** because it is the one thing every client can set —
  any SDK, any agent CLI's model option — and it shows up in every log and
  trace, which a header would not. Script ids cannot contain `@`, so the split
  is unambiguous.
- **Strict.** A session is 1–64 characters of `A-Za-z0-9._-`; a model that has
  no sessions (a built-in) with a `@` suffix is a 404, not silently the default;
  sessions are not listed in `/models`. Rewriting the script file restarts every
  session. A script remembers at most `vmodel.MaxSessions` (1024) sessions —
  the least recently used is forgotten, so a client minting ids cannot grow
  memory without bound.
- **Direct endpoints only.** The gateway's rule rewrites the requested model to
  the provider's, so a session suffix does not survive `/tingly/*`; through the
  gateway a script runs in its default session (the harness gets a fresh env per
  run, so it is isolated anyway).

**Which protocol to use.** The gateway converts between protocols, so scripts
need only one: the docs, `harness script` (default `claude`) and the Python
`Testbed` lead with the Anthropic side, and anything else rides on conversion.
The direct `/virtual/openai/*` endpoints stay, since they bypass conversion and
give an independent native-OpenAI reference when diagnosing a conversion bug.

There is no watcher and no API. `virtualserver.scriptStore.Refresh` runs at the
top of every vmodel entrypoint and compares a directory signature (names +
mtime + size) — one `ReadDir` and a few `stat`s when nothing changed. So:

- **write a file, call the model** — no restart, no registration step;
- an **edited** file is re-registered with a fresh cursor (the program restarts);
- a **deleted** file unregisters its model;
- a file that **fails to parse** (or loses an id race) serves nothing — what is
  served always matches what is on disk — and the reason is logged and appended
  to the `404 Model not found` message, so a broken script explains itself.
  Failed files are retried on every directory change (a freed id, a swap);
- readers that bypass the HTTP handlers (the management UI's model listing)
  call `Service.RefreshScripts()` first;
- a script **never shadows** a built-in model, and the first file (by name)
  owns a duplicated id.

The per-request cost is deliberate: a watcher would add a goroutine and a
platform dependency for a development convenience, and "visible on the very
next request" is the property that makes Python's `write → call` flow (PR2)
race-free. The directory defaults to the real config dir, so a script written
for a test also works against a developer's running tb.

## Harness: `harness script <file>…`

Drives each script through the in-process gateway (built-in rule → vmodel
provider → the agent's wire format) with **one request per step**, then checks
each response against what its step declares: error steps by HTTP status, tool
steps by tool name and text, plain steps by text, `midstream` steps by the
stream *not* reaching its terminal event (`message_stop` / `response.completed`). Expectations are
resolved by the same `vmodel.Sequence` the server runs, so defaults, tool ids
and `repeat` cannot drift between the check and the engine.

- `--agent claude|codex|opencode` picks the wire format (Anthropic messages /
  OpenAI Responses); `--no-stream` skips mid-stream steps.
- Shipped scripts in `cli/harness/testdata/scripts/` run in CI as the `script`
  harness leg and in `go test ./cli/harness`.

This verifies "does my script do what I wrote, through the real pipeline" — it
is also the fastest way to preview a script. Driving a script with a *real*
agent CLI (`harness agent … --script`) is deferred.

## Phases

1. **PR1 — engine, YAML, directory serving, harness** *(this change)*.
2. **PR2 — Python `tingly.vmodel`** *(see below)*.
3. **Deferred until there is a consumer:** `when:` request matching and
   implicit per-conversation detection (explicit sessions exist; this would key agents that cannot set the model name),
   a management API / CLI / UI (remote or non-file registration), request
   capture for asserting on client behaviour, recording → script, a webhook
   step, `think` blocks and multiple tool calls per step.

## Python: `tingly.vmodel` (PR2)

Python has no way to push a script to tb — and needs none: the script
directory *is* the push channel. `tingly.vmodel` is two small, stdlib-only
pieces, and a script written with it is the same file a human would write:

- **`Script`** — a chain builder (`.say()` `.tool()` `.error()` `.cut()`) that
  emits the schema above as **JSON, which is valid YAML**, so no YAML library is
  needed on the Python side and tb's strict parser remains the only authority
  on what a script means. It adds no semantics: anything expressible in Python
  is expressible in a hand-written file, and the e2e test feeds tb every step
  kind Python can write.
- **`Testbed`** — the part that makes it usable without setup. `Testbed(*scripts)`
  finds a tb binary (`$TINGLY_TB_BIN` / `tingly-box` / `tb`), starts it with a
  throwaway `--config-dir` on a free port, reads the model token tb generated,
  writes the scripts into `vmodels/`, and exposes ready base URLs
  (`anthropic_base`, `openai_base`) and one-line `messages()` / `chat()` calls.
  `Testbed.attach(*scripts)` instead writes into an already-running tb's config
  dir and removes only what it added. Scripts are written atomically (temp file
  + rename, dot-prefixed so tb ignores the temp), and `add()` confirms tb loaded
  the file — on failure it raises `ScriptError` carrying tb's own message, taken
  from the `404 Model not found (script load errors: …)` that PR1 added for
  exactly this.

It targets the direct `/virtual/{anthropic,openai}` endpoints, which need no
provider or rule — the model token is the only credential. Driving a script
through the `/tingly/<scenario>` pipeline (provider + rule) is what
`harness script` covers; real agents are pointed at `anthropic_base`.

Limits, by design: attach mode needs filesystem access to the config dir (a
remote tb needs the deferred management API). `stop_reason=` takes the
neutral vocabulary (`end_turn`, `tool_use`, `max_tokens`, `stop_sequence`).
Usage on a step is advertised on **streamed** responses only (existing vmodel
behaviour), so assert on it with `stream: true`.

## Files

- `vmodel/sequence.go` — `SequenceStep` (full outcome, `UnmarshalYAML`), `Sequence` (`NextFor`, sessions), `ResolvedStep`.
- `vmodel/script.go` — `ParseScript`, `SequenceConfig.Validate`.
- `vmodel/types.go` — `ToolCallConfig.ID`, strict `UnmarshalYAML`.
- `vmodel/{anthropic,openai}/sequence_model.go` — `Snapshot` carries tool/usage/stop reason.
- `vmodel/virtualserver/scripts.go` — the directory store; `service.go` `SetScriptDir`; `handler.go` refresh hooks.
- `internal/server/server.go` — wires `<config-dir>/vmodels`.
- `cli/harness/script.go`, `testdata/scripts/` — the harness command and shipped scripts.
- `sdk/python/tingly/vmodel.py` — `Script`, `Testbed`, `ScriptError`; `examples/vmodel_flow.py` (the demo), `tests/test_vmodel.py` (unit), `tests/test_vmodel_e2e.py` (real tb).
