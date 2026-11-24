# MCP: paste a command or config

Companion to `mcp-profiles.md` (the model) and `mcp-tool-groups.md` (tool
lists). This document is about how a connection's settings get into the form.

## Problem

MCP READMEs hand out a ready command (`npx -y @scope/server …`), a client
registration command (`claude mcp add …`) or a config block (`mcpServers`
JSON, Codex `config.toml`). The connection form asked for command, each
argument, each env var and the URL field by field, so the user retyped what
they had just copied — the worst case being a STDIO command with many
arguments.

## Design

A **"Paste a command or config"** box at the top of the connection form, in
the Connect tools dialog (Any MCP) and in the connection drawer's settings
(hidden for a preset such as GitHub, whose fields are fixed). Pasting reads the
text right away; typed text is read with **Fill in fields**.

- **The form is the preview.** Reading fills the fields below; nothing is
  saved until the user presses the dialog's own Connect / Save. A green line
  says what was read ("Read as command line: npx · 3 arguments").
- **Only what the paste states is replaced.** A JSON argument list replaces the
  arguments and keeps the command; a URL replaces the endpoint. A name the user
  typed is kept; an empty name (or one the previous fill put there) takes the
  pasted one.
- **Several servers** in one config (`mcpServers` with three entries): the
  line says "Found 3 servers" and a toggle row picks the one to fill. One
  connection per dialog keeps the review-then-connect step; paste again for
  the next.
- **Failure** keeps the pasted text and the form untouched, and the box's
  helper text names the problem (unclosed quote, unreadable JSON, no server
  found, no command).
- `${KEY}` env values become "inherit" entries, the form's existing convention.

Pure frontend (`features/mcp/pasteImport.ts`, UI `pages/mcp/MCPPasteImport.tsx`).
The form value already holds every field these formats carry, so no API or
stored-config change.

## Formats

| Paste | Becomes |
| --- | --- |
| Command line `A=1 npx -y pkg "/a b"` | STDIO: command, args (POSIX quoting: `'…'`, `"…"` with `\"`, `\` escapes, `\`-newline joins lines), leading `VAR=value` → env; a leading `$ ` prompt is dropped |
| `claude` / `gemini mcp add [-t http] [-e K=V…] [-H "K: V"] <name> <cmd-or-url> [args…]`, `claude mcp add-json <name> '<json>'` | name, transport, URL or command + args, env, headers |
| `codex mcp add <name> [--env K=V] [--url U] -- <cmd…>` | the same |
| JSON: `mcpServers` (Claude Desktop, Cursor, Windsurf, Gemini), `servers` (VS Code), `mcp` (opencode, `command` as array, `environment`), `context_servers` (Zed), a bare `"name": {…}` fragment, or one server object | per server: `command`/`args`/`env`/`cwd`, or `url`/`serverUrl`/`httpUrl` + `headers`; `type` `sse` → SSE, `http`/`streamable-http`/`remote` → Streamable HTTP, `stdio`/`local` → STDIO. Trailing commas are tolerated; a `command` string with spaces and no `args` is split like a command line |
| Codex `config.toml`: `[mcp_servers.<name>]` with `command`, `args` (one or several lines), `env` inline table or `.env` sub-table, `url`, `http_headers` | the same |
| JSON array of strings `["-y","pkg"]` | the arguments |
| A URL | Streamable HTTP endpoint (SSE when the path ends in `/sse`), name from the host |

The name suggestion for `npx`/`uvx`/`bunx`/`pipx` commands is the package
name without scope or version; other commands get none rather than a guess.

**Not handled, on purpose:** pipes, `&&`, variable expansion, globbing,
Windows `^` continuations, full TOML. The box is a convenience, not a shell;
anything it cannot read stays editable by hand.
