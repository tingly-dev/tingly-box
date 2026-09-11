---
name: managed-agent-e2e
description: Run and extend the Managed Agent (Tasks) end-to-end journeys — real tingly-box server, real `claude` CLI, real git, a scripted model, and the built web UI under Playwright. Use before reporting any Managed Agent change as done, when the user asks to "test the tasks feature end to end", after touching internal/managedagent, agentboot/claude, or frontend/src/pages/tasks, and to add a journey for a new behaviour. The journeys are the proof; a change without a green run is not done.
---

# Managed Agent end-to-end journeys

The feature promises a handful of user journeys. Each one is a Go test that
boots an isolated tingly-box (temp config dir, own ports), talks to it over
HTTP exactly as the web UI and IM do, and lets the **real `claude` CLI** run
against a model we control. Nothing is mocked below the HTTP API except the
model itself. The rule of this skill: **do not tell the user a Managed Agent
change works until these are green.** Unit tests catch regressions in pieces;
the journeys catch the things that only show up when the pieces meet (they
found: tool results never reaching the timeline, `--resume` of a session the
CLI never saved, the parent session's identity leaking into the child CLI,
`echo` being auto-approved so the permission flow was never exercised).

## Run

```bash
task test:e2e:agent        # API journeys, ~1 min
task test:e2e:agent:ui     # browser journey (builds the UI first), ~1 min
task test:e2e:agent:all    # both
```

Without `task`:

```bash
TB_MANAGED_AGENT_E2E=1 go test -count=1 -v -timeout 30m \
  ./internal/managedagent/e2e/ ./internal/managedagent/agentrun/ -run 'Journey|RealCLI|FullStack'
```

Prerequisites (each missing one is reported as a *skip*, never a pass — read
the skip reasons in `-v` output):

| Need | Why | If missing |
|---|---|---|
| `claude` on PATH | the journeys drive the real CLI | install Claude Code |
| `git` | provisioning and diffs | install git |
| `internal/web/dist` built **before** `go test` compiles | the browser journey serves the embedded UI | `task web:dist` (or `cd frontend && pnpm build && cp -R dist/* ../internal/web/dist/`) |
| `frontend/node_modules/playwright` | browser driver | `pnpm install --frozen-lockfile` in `frontend/` |
| a Chromium | headless browser | `TB_E2E_CHROME=/path/to/chrome`; in the cloud sandbox use the `ui-preview` skill's Chrome for Testing at `/tmp/chrome/chrome-linux64/chrome` (auto-detected) |

Root containers: the CLI refuses `bypassPermissions` as root unless
`IS_SANDBOX=1` exactly; `TestMain` sets it for the test process. Running as a
normal user needs nothing.

Server logs are noisy under `-v`; filter with `| grep -v 'level='`.

## The journeys

| Test | File | What it proves |
|---|---|---|
| `TestJourney_LocalFolderInPlace` | `local_folder_test.go` | allowlist: nothing listable before the folder is handed over (403, top level empty) → start with `local_path` (that is the grant) → answer → diff shows the edit → push refused (in place) → folder is recent + a local source → only it is browsable, its parent stays 403 → second task reuses the workspace → non-git folder works with an empty diff → missing folder is 400 |
| `TestJourney_PermissionPrompt` | `permission_test.go` | a write command → `approval_request` → `waiting_input` → approve → command ran, output in the log **and** back to the model → deny → nothing ran, model told → `bypassPermissions` → no question → bad mode is 400 |
| `TestJourney_InterruptThenResume` | `interrupt_test.go` | slow model → interrupt → `idle` (not failed) → next message continues → archive is final |
| `TestJourney_ArchiveWhileRunning` | `interrupt_test.go` | archive mid-turn stops the CLI and stays archived |
| `TestJourney_FailureThenRetry` | `failure_test.go` | model 400 → `failed` with the reason → send again → `idle`, error cleared |
| `TestJourney_AllPermissionModesStart` | `failure_test.go` | every advertised mode starts a turn on the installed CLI |
| `TestJourney_Browser` | `browser_test.go` + `browser/managed_agent.mjs` | the built UI: type a path in the dialog → told it is outside the allowlist → use it anyway → Start → answer on the detail page → steer → **Allow** a command → result → Folders page lists the folder → the dialog now browses it (and only it). Screenshots per step in `TB_E2E_OUT` |
| `TestFullStack_SessionOverHTTP` | `agentrun/full_stack_test.go` | git repository source: clone → answer → resume → diff → push lands the branch on origin → archive |
| `TestRealCLI_SessionRoundTrip` | `agentrun/real_cli_test.go` | the Launcher alone with the real CLI and the virtual upstream |

## How the model is controlled

`harness_test.go` has two upstreams:

- **virtual** (`bootStack(t, nil)`): the harness's `protocoltest` virtual
  server; every request gets the fixed text containing
  `protocoltest.VirtualMockAnswerMarker`. Use for "the agent answers".
- **scripted** (`bootStack(t, newScriptedUpstream(t, turns...))`): an
  Anthropic-Messages look-alike that pops one `upstreamTurn` per request —
  `Text`, `Bash` (a tool_use the CLI will execute), `Status` (an error),
  `Delay` (interruptible). Exhausted scripts answer a fixed text so the CLI
  always finishes. `up.Queue(...)` adds turns mid-test; `up.LastRequestJSON()`
  shows what the CLI sent back (assert a `tool_result` reached the model).

Facts that bit us, keep them in mind when scripting:

- The CLI auto-approves read-only commands (`echo`, `ls`, …). A permission
  journey needs a **write** (`touch x.txt && echo marker`).
- Turn boundaries: assert on the log (`turnEnded(ev, before)` — the closing
  `status` event), not on the status field, which lags a beat when a new
  turn starts.
- Every journey boots its own stack; never share sessions across tests.

## Adding a journey

1. Write the user-facing promise as the test's doc comment (arrow form, as
   above). If you cannot phrase it as something the user does and sees, it
   is a unit test, not a journey.
2. Script the model with `upstreamTurn`s; drive only the HTTP API (or the
   UI); assert on events, status, files on disk, and `LastRequestJSON()`.
3. Add it to the table above and to `.design/managed-agent.md` §14.
4. Run the whole set, not just the new one.

## When a journey fails

- The failure message dumps the event log; the browser journey leaves
  screenshots (`NN-FAILED.png` is the moment it stopped).
- Reproduce the CLI's behaviour outside tb before changing tb: a scripted
  upstream + `claude --print --output-format stream-json --input-format
  stream-json --permission-prompt-tool stdio` shows exactly what the CLI
  emits (this is how the `echo` auto-approve and the `user`-message
  tool_result shape were found).
- Do not weaken an assertion to get green. If the product changed on
  purpose, change the journey's promise and its doc comment together.
