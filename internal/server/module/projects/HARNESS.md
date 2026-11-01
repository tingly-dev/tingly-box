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

This is not a replacement for platform-specific Telegram/Feishu/Weixin tests,
real Claude Code compatibility tests, or end-to-end validation of file uploads,
network failures, and every possible session command.
