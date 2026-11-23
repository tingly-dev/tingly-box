# Permission grants reach a running Claude Code: probe

Question (user, 2026-11-23): after `request_permission` and one yes on the card,
does the action simply run, in the thread's own Claude Code, with no second card
and no silent auto-mode block? Can the grant be injected into the running
`claude` instead of Projects running the command itself?

Claude Code 2.1.259, isolated HOME, fake API key, scripted Anthropic mock whose
auto-mode classifier answer is unparseable (so the classifier blocks
everything). Two probes:

- `permission-grant-probe/probe.py <scenario>`: bare `claude -p` stream-json;
  the host pushes rules with the `apply_flag_settings` control request (what
  agentboot's `ApplySettings` / `engine.RefreshSettings` send).
- `permission-grant-probe/e2e.py CLAUDE TINGLY_PROJECT [early|late|deny]`: the
  real Projects server and a real project thread in auto mode (model
  `claude-sonnet-4-6`, which keeps auto); the mock coordinator starts the
  thread, the mock thread calls `request_permission(command)` and Bash; the
  probe answers the cards.

## Findings

| Scenario | Result |
|---|---|
| auto, no rule (`control`) | blocked: "Auto mode could not evaluate this action…", 10 classifier calls |
| auto, `Bash(./tool.sh *)` pushed mid-turn, then the call (`early`) | **ran**, 0 classifier calls: an allow rule is checked before the classifier, and a rule pushed mid-turn holds from the next tool call (no restart) |
| auto, call blocked, rule pushed, same call again (`late`) | **ran** on the retry |
| auto, `Bash(python3 -c *)` / `Bash(node -e *)` pushed (`interp`) | **still blocked**: auto mode drops wildcard rules for interpreters |
| auto, exact `Bash(python3 -c "…")` pushed (`interpexact`) | **ran** |
| auto, exact rules for `a \| b` | the whole-line rule alone is not enough (blocked); with a rule for each part as well it ran. `cd . && a` and `FOO=1 a` ran with the whole-line rule |
| default mode, `can_use_tool` answered allow + `addRules` (session) | ran; the repeat did not ask again |
| e2e `early` | 1 card (request_permission), the thread's own Bash ran, 0 classifier calls; notes "The user granted…" and "Ran with the user's grant…; it succeeded." |
| e2e `late` | the first call blocked (10 classifier calls), 1 card, the retry ran |
| e2e `deny` | 1 card denied, the tool said "Do not run it", the Bash call stayed blocked |
| e2e `folder` (folder session, auto) | 1 card, the thread's own Bash ran, 0 classifier calls |

## What it means

- Injection works: a grant written into the thread's settings and pushed with
  `apply_flag_settings` lets the thread's **own** next call run, in its own
  cwd, env and shell, with the result in its own transcript. Projects does not
  need to run the command; that option is dropped.
- A grant must be an exact command (plus each part of a pipeline), not an
  interpreter wildcard, or auto mode drops it and the approved call is still
  blocked.
- Before this change a thread under a launch profile got a rule only at its
  next turn, so the approved call was blocked again in the same turn.
- In auto mode a blocked call never reaches `can_use_tool` (until 3 blocks in
  a row, see memory `cc-auto-mode-silent-downgrade`), so there is no pending
  native prompt to answer: the grant comes first (or after the block), and the
  model's own call is the action.

Design: `projects/.design/thread-permissions.md` §3.
