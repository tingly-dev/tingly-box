# IM chats in Projects — Pencil Graph

Visual companion to [`projects-im.md`](./projects-im.md), which stays the
authority on decisions and status. Everything here is a **proposal** (TEP
#2143): the diagrams show the target, not what exists today.

Contents:

- Before / after (map)
- What a chat binds to
- Inbound: from a message to a thread
- Outbound: from thread events to chats
- Addressing a thread from a project-bound chat
- Approval with two surfaces open
- UI: where each Remote piece goes
- UI: bind from either side
- Data: what leaves Remote, what Projects owns
- Phases and what runs in parallel

## Before / after

```
BEFORE                                                  AFTER
──────                                                  ─────
rail ─┬─ Projects                                       rail ── Projects
      │    projects ── coordinator ── threads                   ├─ Projects         coordinator + threads
      │    working folders ── sessions                          ├─ Working folders  sessions
      └─ Remote (page)                                          └─ IM (sidebar entry, settings only)
           bots · access control · @tb/@cc routing

IM chat ─▶ Remote                                       IM chat ─▶ Remote (access only)
            ├─ @tb  SmartGuide executor                            │  pairing · whitelist · disabled
            ├─ @cc  Claude executor ─▶ own engine                  ▼
            └─ remote/session store  ◀── second store            ChannelBinding (in Projects)
                                                                   │
Projects ── threads ── engine ── threads store                     ▼
            (never sees an IM chat)                              Projects: threads + coordinator
                                                                 one runtime, one store
```

## What a chat binds to

```
                    ┌────────────────────────── Projects ───────────────────────────┐
IM chat             │                                                                │
(platform, chat_id) │   ChannelBinding                                               │
   ┌──────────┐     │   ┌────────────────────────────┐                               │
   │ Telegram │─────┼──▶│ target_kind = project      │──▶ Project ──▶ Coordinator    │
   │ DM       │     │   │ project_id                 │       └──▶ threads (by coord.) │
   └──────────┘     │   └────────────────────────────┘                               │
   ┌──────────┐     │   ┌────────────────────────────┐                               │
   │ Feishu   │─────┼──▶│ target_kind = workdir      │──▶ Working folder              │
   │ group    │     │   │ workdir_id, thread_id ─────┼──▶ current session (a thread)  │
   └──────────┘     │   └────────────────────────────┘      older sessions kept       │
   ┌──────────┐     │                                                                │
   │ new chat │─ ─ ─┼─ ─ (no row) ─ ─▶ Coordinator, no project; replies with the     │
   └──────────┘     │                  recent projects/folders to pick from          │
                    └────────────────────────────────────────────────────────────────┘

one chat  → one place at a time (unique on platform + chat_id)
one place → any number of chats (phone DM and a team group, say)
```

## Inbound: from a message to a thread

```
IM message
   │
   ▼
Remote: access control ── not paired / disabled ──▶ dropped (unchanged)
   │ ok
   ▼
Lookup ChannelBinding(platform, chat_id)
   │
   ├─ none ─────────▶ coordinator (no project) ──▶ reply + "pick where to work"
   │                                                  [recent projects] [folders]
   │                                                  user taps one ─▶ binding created
   │
   ├─ project ──────▶ append project message (source=im, platform, chat_id)
   │                      │
   │                      ▼
   │                  Coordinator (serial queue, as for Web)
   │                      └─▶ answers here / starts threads / hands to a thread
   │
   └─ workdir ──────▶ thread_id set?
                         ├─ yes ─▶ Threads.Send(thread_id, text)
                         └─ no ──▶ Threads.StartSession(folder, text)
                                      └─ binding.thread_id = new thread

slash commands that stay:   /bind   pick a place        /new   fresh session in this folder
                            /stop   interrupt            /unbind  leave the place
```

## Outbound: from thread events to chats

```
thread / coordinator events  (transcript, approval, ask, report, error)
   │
   ▼
Projects event hub ── subscribers: Web (SSE)   ┐
                                    IM bridge   ├─ one stream, filtered per subscriber
                                                ┘
IM bridge (TB host capability)
   │  for each ChannelBinding of the place
   ▼
render (remote/control/render, imbot-output.md: stream, fold, send files)
   │  prefix "[tingly-box] "  ← place name, so the chat always says where it is
   ▼
chat

coordinator thread_update  ──▶  one folded line in IM ("2 thread updates")
approval / ask             ──▶  an interactive message (buttons) in every bound chat
```

## Addressing a thread from a project-bound chat

```
chat message
   │
   ▼
 quoted a thread message? ── yes ──▶ that thread                      (stateless)
   │ no
   ▼
 starts with "@<number>"?  ── yes ──▶ thread #<number>, rest is text  (stateless)
   │ no                                (@name, not a number → coordinator resolves; ambiguous → asks back)
   ▼
 binding.focus_thread_id set? ─ yes ─▶ that thread                    (sticky, /focus #2 … /back)
   │ no
   ▼
 coordinator

every thread message in the chat is prefixed  [#2 退款测试补幂等校验]
approvals / asks always go to their own thread, whatever the focus is

/threads  ─▶  #1 docs 链接检查         运行中        [聚焦] [停止]
              #2 退款测试补幂等校验    等你处理 ●    [聚焦] [停止]
              #3 清理旧 migration      待审查        [聚焦] [停止]
```

## Approval with two surfaces open

```
thread asks to run `rm -rf build/`
        │ EvApproval (pending)
        ├──────────────▶ Web: card with [Allow once] [Allow always] [Deny]
        └──────────────▶ IM:  message with the same buttons

        first answer wins (Threads.Respond is idempotent on request id)

   Web taps Allow ─────▶ Respond(id, allow) ─▶ ok
   IM  taps Deny  ─────▶ Respond(id, deny)  ─▶ "already answered"
                          │
                          ▼
                IM edits its message: "Allowed on the web" (buttons removed)
                Web card closes the same way if IM answered first
```

## UI: where each Remote piece goes

```
Remote page (today)                      Projects (after)
┌────────────────────────────┐           ┌─────────────┬──────────────────────────────┐
│ Remote Control             │           │ Projects    │ tingly-box        [IM: 2 ▾]  │
│ [Connect a bot]            │           │  tingly-box │ Goal …                        │
│ ┌ Telegram bot ──────────┐ │           │  docs       │ Conversation …                │
│ │ who can control        │ │           │ Folders     │                               │
│ │ @cc profile / model    │ │  ───────▶ │  ~/work/a   │                               │
│ │ @tb model              │ │           │  ~/work/b   │                               │
│ └────────────────────────┘ │           │             │                               │
└────────────────────────────┘           │ ─────────── │                               │
                                         │ ⚙ IM        │  ← bots, access control        │
                                         └─────────────┴──────────────────────────────┘

bot connection, pairing, whitelist ──▶ IM settings (sidebar bottom)      config, not content
@cc profile / model / permission    ──▶ per project / per folder session  already Thread fields
@tb model                           ──▶ removed (coordinator model)
chat transcript                     ──▶ the normal session view, with a platform icon
```

## UI: bind from either side

```
Project / folder header                         IM settings
┌──────────────────────────────────────────┐    ┌───────────────────────────────────────┐
│ tingly-box              [✈ Telegram ×]   │    │ Telegram bot            ● connected    │
│                         [🐦 Feishu team ×]│    │  chats                                 │
│                         [+ Bind chat]     │    │  ┌───────────────────────────────────┐ │
└───────────────┬──────────────────────────┘    │  │ @yz (DM)      → tingly-box   [Change]│ │
                │ [+ Bind chat]                 │  │ Team group    → docs         [Change]│ │
                ▼                               │  │ new chat      → (not bound)  [Bind]  │ │
   ┌───────────────────────────────┐            │  └───────────────────────────────────┘ │
   │ Bind a chat to tingly-box      │            │  who can control: paired · whitelist   │
   │  ( ) @yz (DM)                  │            └───────────────────────────────────────┘
   │  ( ) Team group                │
   │  ( ) a chat not yet seen       │   both write the same ChannelBinding rows
   │  [Cancel]            [Bind]    │
   └───────────────────────────────┘

session view, message from IM:
   ┌──────────────────────────────────────┐
   │ ✈ yz  fix the flaky test in billing  │   small platform icon, no other change
   └──────────────────────────────────────┘
```

## Data: what leaves Remote, what Projects owns

```
Remote keeps (access to the bot)         Projects owns (what the chat is doing)
─────────────────────────────────         ────────────────────────────────────
bot settings (token, platform)            ChannelBinding (platform, chat_id → place)
Chat: is_paired, paired_*, owner_id       Thread / project message source fields
      is_whitelisted, disabled            Thread: profile, model, permission_mode
      context_token, verbose              
audit, rate limits                        gone
                                          ──────────────────────────────────
                                          remote/session store   → threads
                                          Chat.ProjectPath/History → binding (one-time convert)
                                          Chat.CurrentAgent, BashCwd → dropped (no @tb)
                                          SmartGuide history     → dropped
```

## Milestones and what runs in parallel

```
M1 coordinator only ──▶ M2 working folder ──▶ M3 approvals in IM ──▶ M4 address a thread ──▶ M5 retire legacy ──▶ M6 @route
 │                                                                                           (unbound → coordinator,
 ├─ backend PR   ChannelBinding (project), source fields, routes, IM→coordinator, coordinator→IM     Remote page, remote/session)
 └─ frontend PR  IM sidebar entry, header tags, bind dialog, source icon   (placeholder API → task codegen)

rule for every milestone: a chat WITH a binding uses the new path; a chat WITHOUT one keeps today's @cc/@tb
until M5, so nothing breaks while the milestones land one by one.

M1 deliberately leaves out: folder binding, approvals in IM (IM gets one line "a thread needs you, open the web"),
thread addressing, removing the old path.
```
