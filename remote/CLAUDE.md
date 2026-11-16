# Remote

A library that connects IM chats to agents: bot lifecycle, pairing and access
control (`access`, `bot`), the chat handler and its commands (`chat`), and the
channel / interaction layer for agent-initiated notifications and questions
(`channel`, `interaction`, `scenario`, `ask`). It is its own Go module
(`github.com/tingly-dev/tingly-box/remote`), embedded by Tingly Box (the root
module) and by Projects.

# Boundary

- Remote imports `imbot` and `agentboot` only. It never imports the root module
  or Projects (`task remote:boundary`, also run in CI).
- What a chat is handed to is the port in `agent` (`agent.Chat`, the session
  command set). The embedder implements it; Remote never names a concrete agent.
- Persistence is injected (`bot.ChatStoreInterface`, access stores). The host
  supplies the database-backed ones; `bot.MemoryChatStore` is for tests and for
  embedders that do not need chats to survive a restart.
- The approval words a human answers with live in `interaction`
  (`ApprovalChoice`), one vocabulary for every host.

Design: `../.design/remote-layering.pencil.md`, `../.design/remote-refactor.md`.
Run `task remote:test` from the repo root.
