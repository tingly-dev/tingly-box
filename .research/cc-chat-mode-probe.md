# Claude Code chat-profile flags (Claude Code 2.1.259)

Date: 2026-11-26. Probe: [`cc-chat-mode-probe/probe.py`](cc-chat-mode-probe/probe.py).
Purpose: ground the launch contract of [`.design/chat-mode.md`](../.design/chat-mode.md)
§7 (U1–U3, U8, U17) — the real `claude` binary, an isolated `HOME` /
`CLAUDE_CONFIG_DIR`, and a scripted Messages API that logs every request.
**Nothing leaves 127.0.0.1.**

Run:

```bash
python3 .research/cc-chat-mode-probe/probe.py <outdir>
```

The probe writes `requests.jsonl` (tools/system/blocks per request), every
request body under `bodies/`, and `result.json`. The gateway is
`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN=sk-ant-fake-probe`, `ANTHROPIC_API_KEY=""`.

## Findings

| # | Check | Result |
|---|---|---|
| U2 | `--tools "Read,Glob,Grep"` | The request `tools` are **exactly** `Glob, Grep, Read`. No Bash/Write/Edit/WebFetch/Task/MCP. |
| U2 | `--tools ""` | `tools` is **empty** (all built-ins disabled). |
| U1 | `--restricted` + `Read` of an absolute path outside cwd | Refused by the CLI before the file is read: `... is outside <cwd>; --restricted confines the file tools to the working directory.` |
| U1 | `--restricted` + `Read ../escape.txt` | Refused, same message (resolves to outside cwd). |
| U1 | `--restricted` + `Read link.txt` (symlink → outside file) | Refused, same message (resolves the target, not the link name). |
| U1 | `--restricted` + `Read notes.txt` (inside cwd) | Allowed; the file content reaches the model. |
| U3 | `--safe-mode` with a project `CLAUDE.md`, a user `CLAUDE.md`, and a `PreToolUse` hook in user settings | **No** `CLAUDE.md` marker in any request body; the hook did **not** run. |
| U3 control | same run **without** `--safe-mode` (no `--restricted`) | Both `CLAUDE.md` markers reach the system/messages and the hook **ran** (`plain_control_hook_ran=true`). |
| U3 | `--restricted` alone (no `--safe-mode`) | Also suppresses settings hooks and `CLAUDE.md` (`--restricted` "ignores user, project and local settings files"); the two are independent layers. |
| U3 | auth through `ANTHROPIC_AUTH_TOKEN` under `--safe-mode` | Every scenario authenticated to the mock with the token and an empty `ANTHROPIC_API_KEY`; `--safe-mode` does not break it. (`--bare` is **not** used: it only accepts `ANTHROPIC_API_KEY`.) |
| U17 | user message with a `content` array `[{text}, {image/source/base64/png}]` | The API request carries the `image` block unchanged (`type=image, media_type=image/png, data_len=100`). The CLI accepts an array `content`, not only a string. |
| U8 | `CLAUDE_CONFIG_DIR` | Transcripts (`projects/<cwd-slug>/<session>.jsonl`) and `sessions/` land under the given config dir; the real `~/.claude` is untouched. |
| — | `--system-prompt` | Replaces the built-in prompt. The request's `system` is `[billing header, "You are a Claude agent, built on Anthropic's Claude Agent SDK.", <our text>]` — the big CC tool/workspace prompt is gone; a tiny identity block and the billing line remain. |

## Notes for the implementation

- `--restricted` confines **file tools** to the working directory; the probe
  exercised `Read`. `Glob`/`Grep` share the same confinement path in the CLI but
  were not individually exercised here.
- The system prompt still carries a ~one-line identity block plus the billing
  header; `--system-prompt` is not a byte-for-byte replacement of `system`.
- `--restricted` and `--safe-mode` each suppress settings hooks and `CLAUDE.md`;
  chat uses **both**, so a future change to one still leaves the other.
- `--tools` is an allowlist: any tool added by a later CLI release stays out
  unless named. Re-run this probe on every CLI bump (U14).

## Raw output

`result.json` from the run used for this note:

```
tools             rc=0  SEE-TOOLS ['Glob','Grep','Read']
tools_empty       rc=0  SEE-TOOLS []
restrict_abs      "…outside …/chat; --restricted confines the file tools to the working directory."
restrict_dotdot   same refusal
restrict_symlink  same refusal
restrict_inside   content of notes.txt read
safe              hook=False  CLAUDE.md absent
safe_control      hook=False  CLAUDE.md absent   (no --safe-mode, still --restricted)
plain_control     hook=True   CLAUDE.md present  (no --safe-mode, no --restricted)
image             image block reaches API
```
