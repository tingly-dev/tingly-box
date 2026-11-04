# Tingly IM / Projects integration harness

Run from the repository root:

```sh
go test ./internal/server/module/projects -run '^TestTingly' -count=1
```

`newTinglyProjectsHarness` starts the real TB bot manager, channel registry,
access and chat stores, Projects host and HTTP handlers, and session engine.
Tingly's in-process IM transport drives bot messages and callbacks. The
coordinator uses an in-process Anthropic-compatible gateway, while a temporary
script emulates the Claude Code stream-json protocol (including approval and
question requests). Neither real IM credentials nor a model account are needed.
The script requires Python 3 and a POSIX executable; tests skip if unavailable.

The suite covers web/IM round trips for a folder session and coordinator,
selected-bot routing and rebinding, first-message session creation, `/new` and
`/verbose`, permission approvals and denials, text answers to questions, feature
switching, blocked-chat delivery, `/dir` folder selection, `/project` coordinator selection,
and privileged-access enforcement (including directory buttons). Unit tests alongside it cover individual
relay and persistence failure paths. Add integration cases here when behavior
crosses the bot/Projects boundary.

Slash-command scenarios (`tingly_slash_commands_test.go`) drive the real
authorization gate and chat handler and grant permissions through the access
API the admin UI calls (the harness mounts it next to Projects): picking
registered folders and projects at start level, free paths and files denied
with a notice until `remote_control.privileged` is granted (and denied again when
it is revoked), plain commands with extra whitespace or a full-width slash, and
in a group the decorated forms (`/dir@bot`, `@bot /dir`, `@bot/dir`, `<@U1> /dir`,
`<@!1> /dir`, `@_user_1 /dir`) needing the grant. Platform mention formats are
simulated by sending that text through the Tingly transport; the harness cannot
show whether the Telegram, Slack, Discord, Feishu, WeCom, WhatsApp or DingTalk
adapters deliver exactly that text, because those adapters do not run here.

This is not a replacement for platform-specific Telegram/Feishu/Weixin tests,
real Claude Code compatibility tests, or end-to-end validation of file uploads,
network failures, and every possible session command.
