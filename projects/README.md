# tingly-project

**English** · [简体中文](README.zh-CN.md)

A local, multi-thread workspace for coding agents. You talk to one
**coordinator** in a project conversation; it splits the work into parallel
**threads**, each a Claude Code session on its own git worktree and branch, and
keeps you posted on what is ready for review and what is waiting on you.

The workflow is inspired by the project-and-thread style of working with AI
agents. tingly-project is an independent open-source implementation built on
[Tingly Box](https://github.com/tingly-dev/tingly-box), and it runs entirely on
your machine.

![A project conversation with two threads running](showcase/e2e-demo/screenshots/1-conversation-waiting.png)

## Features

- **One place to send work.** Paste a task list, a bug report or a stack trace
  into the conversation. The coordinator answers quick questions itself, starts
  one thread per task, or routes a follow-up to the thread already working on
  it.
- **Every thread is a real Claude Code session** with its own worktree and
  branch, so threads never step on each other. Permission requests and
  questions wait for you inside the thread.
- **Overview** groups threads by state: Waiting on you, Ready for review,
  Working, Idle and Resolved. Review a thread's diff and merge it into its base
  branch from there.
- **Standing context** for every thread: a one-line goal, project
  instructions, memory files (`MEMORY.md` plus notes Claude writes and you can
  edit) and a Library of shared files.
- **Local and multi-model.** Threads run on your machine, and the coordinator
  and threads can use different models through Tingly Box.

## Quick start

You need `git` and the [Claude Code CLI](https://docs.claude.com/en/docs/claude-code)
(`claude` on `PATH`).

```bash
task build                 # builds the web UI and the binary
./bin/tingly-project       # opens on http://127.0.0.1:12590
```

By default the coordinator and threads talk to a local Tingly Box at
`http://localhost:12580/tingly/anthropic`. Open **Settings** to paste a Tingly
Box model token, pick a default model and send a test message. You can also
point it at any Anthropic-compatible endpoint. OpenAI-style endpoints work for
the coordinator and chats; threads then fall back to Claude Code's own login.

Want to see the whole loop without a model key? Run the scripted
[end-to-end demo](showcase/e2e-demo/README.md): one message in, two parallel
threads, review a diff, merge.

## Where your data lives

Everything is stored in `~/.tingly-project` (override with `--data-dir` or
`$TINGLY_PROJECT_HOME`). Each project's memory, library and thread worktrees
sit under `projects/<id>/`. Embedded in Tingly Box it keeps the same layout in `<config dir>-project` (or `--projects-dir`, remembered for later starts)
(`~/.tingly-box-project` by default), beside the config directory and never inside
it, so threads do not start next to its credentials and instances do not share data.

The server binds to `127.0.0.1` and has no login, so keep it on a machine you
trust and do not expose the port.

## Development

```bash
task dev:backend    # API on :12590
task dev:frontend   # Vite on :3010, proxies /api
task test           # go vet + go test + typecheck + lint + vitest
task codegen        # after changing API models or routes
```

| Layer | Stack |
|---|---|
| Backend | Go, gin, [`tingly-box/swagger`](https://github.com/tingly-dev/tingly-box/tree/main/swagger) route groups, gorm + SQLite, [`tingly-box/agentboot`](https://github.com/tingly-dev/tingly-box/tree/main/agentboot) for Claude Code, Anthropic / OpenAI Go SDKs |
| Frontend | React 19, MUI, Vite, `openapi-typescript` + `openapi-fetch`, Tabler icons, i18next (en / zh) |

Background notes: [`.research/claude-projects.md`](.research/claude-projects.md) ·
Design: [`.design/projects.md`](.design/projects.md)

## Disclaimer

tingly-project is an independent community project. It is not affiliated with,
endorsed by or sponsored by Anthropic. Claude and Claude Code are trademarks of
Anthropic, PBC, and are named here only to describe the tools this project
works with.

## License

[Mozilla Public License 2.0](LICENSE)
