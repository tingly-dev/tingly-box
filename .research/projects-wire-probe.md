# What a real Projects run sends to its model (Claude Code 2.1.259)

Date: 2026-11-21. Probe: [`projects-wire-probe/probe.py`](projects-wire-probe/probe.py).
Purpose: ground the request routing of [`.design/vmodel-script.md`](../.design/vmodel-script.md)
(§2) in what real clients send, before building the script engine.

## Setup

The real `tingly-project` server (built from this branch with the out-of-tree
`go.work`) and the real `claude` 2.1.259 CLI it launches, against a scripted
Anthropic Messages API written in Python that logs every request (selected
headers, model, tools, system head, message roles, the ids of the last assistant
message, `metadata.user_id`) and keeps every body.

- `HOME`, `CLAUDE_CONFIG_DIR`, data dir and the git repository under
  `/dev/shm/tp149057/<run>` (clean ancestors: see finding 8), a fake API key,
  `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `DISABLE_TELEMETRY=1`,
  `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`. Nothing left 127.0.0.1.
- The journey: user message → coordinator calls `start_thread("Add CHANGELOG")`
  → the thread `Write`s CHANGELOG.md, runs `git add && git commit` (an approval
  card; the probe answers it through `POST /threads/{id}/respond`), reports →
  the coordinator answers the check-in. Run `sub` adds one `Agent` call at the
  start of the thread.

Both runs completed: the thread ended `ready` with 1 commit on its `tp/…`
branch and the coordinator posted its final answer.

## Requests seen

`one` (8 requests):

| seq | conversation | messages (roles) | last assistant ids | our answer |
| --- | --- | --- | --- | --- |
| 1 | – | `HEAD /api/hello` | | 200 |
| 2 | coordinator | u s | – | `start_thread` |
| 3 | coordinator | u s a u s | toolu (start_thread) | text |
| 4 | – | `HEAD /api/hello` (the thread's CLI) | | 200 |
| 5 | thread | u s | – | text + `Write` |
| 6 | thread | u s a u s | toolu (Write) | text + `Bash` (approval card) |
| 7 | thread | … a u s | toolu (Bash) | final text |
| 8 | coordinator | u s a u s a u s | text only | text |

`sub` (13 requests): as above plus, in the thread's session, the subagent's two
requests interleaved with the thread's own, one extra thread request after its
final answer, and two coordinator check-ins (one per thread report).

## Findings

1. **No request-class header.** `x-claude-code-request-class` and
   `x-claude-code-agent-type` were absent on every request, with
   `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1` set, through a custom
   `ANTHROPIC_BASE_URL`. (The compat notes describe them for 2.1.328+; 2.1.259
   does not send them here.) Routing must not depend on them.
2. **The billing line says "subagent".** `system[0].text` is
   `x-anthropic-billing-header: cc_version=…; cc_entrypoint=…;` and a subagent's
   carries `cc_is_subagent=true;`. The coordinator's entrypoint was `sdk-cli`,
   the thread's and subagent's `tingly-box`.
3. **Session ids do not separate subagents.** `X-Claude-Code-Session-Id` equals
   `metadata.user_id.session_id`; coordinator and thread each have their own, but
   the subagent sends its parent thread's id. Session id is good for diagnostics
   and for telling processes apart, not for routing within a process.
4. **Continuation by our own ids works.** Every follow-up request's last
   assistant message carried the `tool_use` id (or the text) the mock had just
   sent, verbatim, including across the coordinator's turns (seq 8 continues
   from the text of seq 3). This is the routing key the design proposes.
5. **`messages` contains `role: "system"` entries.** 2.1.259 interleaves
   system-role messages (the skills list, `<total_tokens>`) into `messages`, so
   the last message is often `system`, not `user`. "Last user message" and
   "last assistant message" have to skip them. The probe's first version keyed
   on `messages[-1]`, missed the `tool_result` and called `start_thread` 20 times
   (Projects started 4 threads, each with its own session): exactly the class
   of bug a declarative router must make impossible.
6. **Background subagents add turns.** `Agent` ran in the background: the parent
   got a "launched" `tool_result` immediately and continued; the subagent's
   requests interleaved with the parent's; when it finished, a
   `<task-notification>` user message produced one more parent turn (and so a
   second report and a second coordinator check-in). A script must be able to
   express "after the notification, answer once more".
7. **Identifying conversations.** Coordinator: tools include
   `mcp__tingly-projects__start_thread` (33 tools), `system` contains "You are the
   project coordinator", and the first turn's user message carries the project
   context and the `<event source="web" … trust="principal">` envelope; check-ins
   are a last text block with `kind="check-in"` (the context text also mentions
   "check-in", so a bare substring match is wrong). Thread: `system` contains
   `Your task: <title>` and `in your own worktree (<cwd>)`; 35 tools. Subagent:
   first user message is the `prompt` we sent; 21 tools; billing line as above.
8. **Ancestor directories leak into requests.** A first run under `$TMPDIR`
   (inside the user's home) put `/home/x0079/CLAUDE.md` and the user's private
   skills into the prompt: Claude Code walks up from the working directory. Under
   `/dev/shm` only the bundled skills remained. A scripted run must use a work
   directory with clean ancestors and should warn when it cannot.
9. **No auxiliary requests** (title generation, classifiers, `count_tokens`)
   with `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`; only `HEAD /api/hello`
   once per CLI process. Background rules are still needed for other settings
   and versions, but strict mode is viable.
10. **The approval card** arrived on `GET /threads/{id}/requests` as an
    `approval` event whose `payload.request_id` (not a top-level field) is what
    `/respond` takes.
11. Model id is the configured default (`demo-sonnet`) on every request; the CLI
    logs `unrecognized_model` and carries on.

## Consequences for the design

- Routing order: continuation by emitted ids → bind of a new conversation →
  background → strict failure (unchanged), with roles `system` skipped and the
  billing line's `cc_is_subagent` as the subagent signal instead of a header.
- Selectors need `last_text_contains` (last text block of the last user
  message) besides "anywhere in the message".
- Journeys run in a work directory with clean ancestors.
