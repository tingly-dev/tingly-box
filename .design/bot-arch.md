# Bot Architecture — resource, channel, consumers (+ naming)

The design record for the bot subsystem after the capability-decoupling work:
the three-layer model, the wiring diagrams, the mount semantics, and the
naming decisions (including known debts and the agreed target vocabulary).

Contents:

1. Problem
2. The model — resource, surface, purposes (big map)
3. The switch ladder — enabled → mounted → attached → running
4. Mount semantics + decision table
5. Scenarios column anatomy
6. Inbound dispatch
7. Flows — notify/prompt and chat control round trips
8. The three "Manager"s
9. Naming — decoder ring, debts, target vocabulary
10. Frontend IA — mirror the model
11. Notes / trade-offs
12. Tests

## 1. Problem

A bot used to be hard-wired to one purpose: remote control of Claude Code.
`bot.Manager` required the agent service + session manager, and the
`remote.channel.Channel` serving `/tingly/:scenario/notify` scenario plugins
was a side effect of the remote-agent handler: registered in its `Attach`,
powered by *its* IMPrompter, with reply routing buried inside its
`HandleMessage`. Consequences:

- A bot could not exist as a pure notification/interaction surface. Turning
  the remote_agent mount off took the whole bot — and the channel — offline.
- The channel's lifetime and the agent machinery were welded together even
  though they are orthogonal (UX principle: separate orthogonal axes).

An intermediate design modeled the channel as a consumer/mount of its own,
which read as "channel is one of the bot's purposes" — wrong layer. The final
model below fixes that: the channel is the bot's usage surface; purposes are
users of the channel.

## 2. The model — resource, surface, purposes

Three layers, strictly ordered. A **bot** is a connection resource. Its
**channel** is the one usage surface a running bot exposes — host-owned
infrastructure, not a purpose. **Consumers** are the channel's users; they
mount onto the bot and point in opposite directions:

```
                 ┌──────────────────────────────────────────┐
                 │              BOT (resource)               │
                 │   imbot conn · chat store · pairing ·     │
                 │   audit · supervised lifecycle            │
                 └────────────────────┬─────────────────────┘
                                      │ exists iff running
                                      ▼
                 ┌──────────────────────────────────────────┐
                 │            CHANNEL (surface)              │   host-owned
                 │  · remote.channel.Channel in Registry     │   (runBotWithSettings)
                 │  · ONE shared IMPrompter                  │
                 │  · promptReplyRouter (replies come home)  │
                 └──────┬─────────────────────────┬─────────┘
                        │ used by                 │ used by
                        ▼                         ▼
        ┌──────────────────────────┐  ┌──────────────────────────┐
        │ consumer: notify          │  │ consumer: remote_agent   │
        │ app ➜ human               │  │ human ➜ app              │
        │ scenario notifications /  │  │ control @cc / @tb        │
        │ interactive prompts       │  │ BotHandler · agent router│
        │ (no wiring of its own —   │  │ · SmartGuide · sessions  │
        │  it is a REASON TO RUN)   │  │ prompts via shared       │
        └────────────┬─────────────┘  │ prompter like everyone   │
                     ▲                └────────────┬─────────────┘
        routed to by │                             ▼
        ┌────────────┴─────────────┐  ┌──────────────────────────┐
        │ /tingly/:scenario/notify  │  │ agentboot (Claude exec)  │
        │ plugin ── binding.Resolver│  │ approval asks ──► shared │
        │ ── channel.Registry.Get   │  │ prompter                 │
        └──────────────────────────┘  └──────────────────────────┘
```

Key consequences:

- The channel has **no mount of its own** — every running bot registers
  exactly one (on start) and unregisters it (on stop). One prompter per bot
  means one reply namespace and one "Always Allow" whitelist; no
  claim-ordering tricks between prompters.
- `notify` (`consumer_notify.go`) carries no wiring at all; its mount is
  purely the *reason to run* for a bot whose only job is scenario traffic.
- `remote_agent` (`consumer_remote_agent.go`) owns the agent service,
  sessions, and SmartGuide; its `BotHandler` is the inbound catch-all and
  sends approval/ask prompts through the shared prompter like everyone else.
  In standalone mode (CLI `remote`, test harness — no host) it creates a
  private prompter and routes replies itself via the same shared mechanics in
  `prompt_reply.go`.

## 3. The switch ladder — enabled → mounted → attached → running

Two config switches and two runtime states, in strict order. Each level only
matters if the one above is true.

```
   Bot.Enabled                      config: the resource may run
        │ true
        ▼
   Consumer.Mounted(setting)        config: a purpose is switched on
        │  remote_agent: ScenarioMounted(scenarios,"remote_agent")
        │  notify:       OutboundScenarioMounted(scenarios)
        │ ≥1 consumer mounted            ("no mount, no bot")
        ▼
   Consumer.Attach(...) → Attached  runtime: consumer wired to the live bot
        ▼
   running                          runtime: goroutine up, imbot connected
        └─ CHANNEL comes with this level — registered on start,
           unregistered on stop; it has no mount of its own
```

## 4. Mount semantics + decision table

Each consumer decides for itself whether it is mounted, from the bot's
`Scenarios` list (`remote/binding`):

| Consumer     | Predicate | Absent binding | Malformed blob |
|--------------|-----------|----------------|----------------|
| remote_agent | `ScenarioMounted(s, "remote_agent")` | mounted (legacy default-on) | mounted (fail open) |
| notify       | `OutboundScenarioMounted(s)` — any binding named ≠ remote_agent, not disabled | NOT mounted (nothing to route) | not mounted |

A bot runs iff `Enabled && at least one consumer mounted`, applied uniformly
in `Manager.Start`, `Manager.Sync`, and the imbot `UpdateSettings` reconcile.
The channel is registered in every non-OFFLINE cell:

```
                          notify mount (outbound bindings: claude_code…)
                          none / disabled          present + active
                        ┌───────────────────────┬───────────────────────────┐
   remote_agent   ON    │ agent bot              │ dual-purpose bot          │
   (absent = ON,        │ BotHandler attached;   │ both consumers attached;  │
    legacy default)     │ channel registered but │ one prompter serves agent │
                        │ nothing routes to it   │ asks AND scenario prompts │
                        ├───────────────────────┼───────────────────────────┤
   remote_agent   OFF   │ OFFLINE                │ notify-only bot  ★        │
   (explicit            │ resource idle,         │ notifications + prompts,  │
    enabled:false)      │ "no mount, no bot"     │ zero agent commands       │
                        └───────────────────────┴───────────────────────────┘
```

★ = the capability this design unlocked: `remote_agent: false` + an active
outbound binding = a bot that delivers notifications and interactive prompts
and routes the answers back, with no agent commands exposed.

The asymmetric defaults (remote_agent fails open, notify fails closed) are a
known semantic debt — see §9.

## 5. Scenarios column anatomy — one list, two kinds of rows

```
db.Settings.Scenarios  (JSON string, per bot)
│
│   [
│     {"name":"remote_agent","enabled":false},          ─┐  MOUNT ROW
│                                                        │  no chat_id, no plugin;
│                                                        │  read by ScenarioMounted
│                                                        ─┘
│     {"name":"claude_code",                            ─┐  ROUTE ROW (real scenario)
│      "chat_id":"dm:ops",                               │  read by binding.Resolver
│      "events":["Stop"],                                │  to route (scenario,event)
│      "permission_policy":{…}}                          │  → this bot + chat
│   ]                                                   ─┘
│
└── same list, two species — "scenario" only truly describes the second
```

## 6. Inbound dispatch — host router first, then consumers

Every reply to a prompt comes home to the ONE shared prompter, so the host
claims the whole "perm" namespace before any consumer runs. Fixed order:

```
 imbot.Manager.OnMessage(msg)
        │
        ▼
 [0] promptReplyRouter (HOST) ────────── claims:
        │ false                            · any "perm" callback (unknown ID
        │                                    → "expired", still claimed)
        │                                  · text that answers a pending
        ▼                                    request in this chat
 [1] notify consumer ─────────────────── no OnMessage (nothing to claim)
        │
        ▼
 [2] remote_agent consumer ───────────── always true (terminal catch-all):
                                           gates · commands · agent routing
```

Standalone mode (CLI `remote`, test harness): no host, no router — the
BotHandler creates a private prompter and routes replies itself through the
same `prompt_reply.go` mechanics.

## 7. Flows

### Flow A: notify/prompt round trip

Works with **zero remote-agent machinery** (remote_agent mount off):

```
 Claude Code hook (Stop / PermissionAsk …)
        │  HTTP POST /tingly/claude_code/notify
        ▼
 scenario plugin (claudecode) ── runtime ── binding.Resolver
        │                                     scans enabled bots' Scenarios,
        │                                     matches (scenario, event)
        │                                     → {botUUID, chat_id, options}
        ▼
 channel.Registry.Get(botUUID) ──► imchannel.Channel (the bot's channel)
        │
        │ Send(target, notification)          one-way: bot.SendMessage, done
        │
        │ Prompt(target, interaction)         interactive:
        ▼
 shared IMPrompter
        │  registers pending[reqID] · sends prompt msg (+keyboard)
        ▼
 human sees prompt in chat ──► taps button / types "y"
        │  inbound message
        ▼
 [0] promptReplyRouter claims it ──► SubmitDecision
        ▼
 Prompt(...) returns Reply ──► plugin ──► hook HTTP response
```

### Flow B: chat control round trip

```
 human types "@cc fix the bug" / "/help" / media
        │ inbound message
        ▼
 [0] promptReplyRouter: nothing pending → PASS
        ▼
 [2] remote_agent (catch-all) → BotHandler.HandleMessage:
        │    gates: chat-id lock · pairing (TOFU) · group whitelist
        │    commands: /stop /bind /cd … · handoff @cc @tb
        ▼
 agentRouter ──► agentboot executor (Claude Code)
        │              │
        │              └─ approval ask? → the SAME shared prompter
        │                   prompt msg → user's reply claimed by [0],
        │                   SubmitDecision resolves the ask
        ▼
 streamed responses → bot.SendMessage → chat
```

## 8. The three "Manager"s

```
 module/imbot.BotManager          HTTP facade (server module)
   │  Start/Stop/Restart/Sync API, status, pairing endpoints
   ▼
 bot.Manager                      lifecycle supervisor (this doc's subject)
   │  mount gate · goroutine-per-bot · panic isolation ·
   │  channel ownership · dispatch (router + consumers)
   ▼
 imbot.Manager                    platform connection layer (one per bot run)
      AddBot/Start/GetBot · reconnect · OnMessage plumbing
```

## 9. Naming — decoder ring, debts, target vocabulary

### Decoder ring (current code)

```
"channel"
  ├─ remote/channel.Channel ........ interface: a human-facing message surface
  │                                   (Send one-way, Prompt interactive)
  ├─ remote/channel.Registry ....... uuid → Channel lookup used by scenarios
  ├─ remote/channel/imchannel ...... the imbot-backed implementation of ↑
  ├─ "the bot's channel" ........... this doc's layer-2: registry entry +
  │                                   shared prompter + reply routing, one
  │                                   per RUNNING bot, owned by the host
  └─ (unrelated) Go chan / tingly test transport Channel(chatID)

"scenario"
  ├─ real scenario ................. outbound plugin, e.g. "claude_code";
  │                                   has a Plugin, routed via /tingly/:scenario
  ├─ db.Settings.Scenarios ......... the per-bot JSON list (config column)
  └─ "remote_agent" entry in ↑ ..... NOT a scenario — a mount switch that
                                      happens to live in the same list

"binding"
  ├─ remote/binding.Binding ........ one row of the Scenarios list
  │                                   (scenario name → chat_id/events/options)
  ├─ /bind <code> .................. TOFU pairing command   (unrelated)
  └─ project bind (/cd, PendingBind) chat → project dir     (unrelated)

"agent"
  ├─ remote agent .................. the bot purpose (control CC from chat)
  ├─ agentboot Agent / AgentService  the executor framework (@cc runs here)
  └─ SmartGuide (@tb) .............. the built-in navigator agent

"enabled"
  ├─ Bot.Enabled ................... resource switch: "may run at all"
  └─ Binding.Enabled ............... mount switch: "this purpose is on"
```

### Known debts and target vocabulary

Decisions already taken: the layer fix (channel = surface, not a mount) and
the `notify` consumer name. Remaining debts, with the agreed direction:

| Debt | Today | Target | Cost |
|------|-------|--------|------|
| scenario routes vs mounts share one word | `binding.Binding` in `Scenarios` | **Route** / package `remote/routing`; mount rows stop pretending to be scenarios | code-only rename, cheap |
| server-centric direction words | `OutboundScenarioMounted` | `HasActiveRoutes` (notify's mount predicate) | cheap |
| "Consumer" describes code, not product | `Consumer` | consider **Capability** (matches product framing: a bot's capabilities) | cheap but wide |
| three `Manager`s | `bot.Manager` | `bot.Supervisor` | mechanical |
| two `Enabled`s | `Binding.Enabled` | `active` (or `mounted` after schema work) | schema-touching |
| asymmetric mount defaults | absent → on (remote_agent) / off (notify) | migrate to explicit rows + one uniform default rule | data migration |
| Scenarios column holds two species | one JSON list | split capabilities vs routes in schema + API | expensive, separate project |
| package name vs UI name | `internal/remote_control` | `remote_agent` (UI already renamed) | mechanical, wide |

Rules of thumb going forward:

- Name by **product concept**, not by code relationship (UX-first repo).
- One word, one layer: "channel" is reserved for the bot's surface
  (`remote/channel.*`); purposes get purpose names (`notify`, not
  "channel"); routes get "route", pairing gets "pair".
- New direction words are role words (control / notify), never
  inbound/outbound.

## 10. Frontend IA — mirror the model

The frontend follows the resource/purpose split (UX principle: separate
orthogonal axes; organize IA around user questions) as **two twin sections
with identical per-platform pagination** — same platform subitems, same
filter — differing only in what they show for each bot:

```
 Bots (/bots/:platform)              resource section, per-platform pages
   "is my telegram bot connected?"    platform · auth · alias · proxy ·
   NAV: HIDDEN for now (see below)    enable/restart · pairing · delete
   card shows purpose STATUS only     ("Remote Agent" chip → click-through
                                       to /remote-agent/:platform)

 Remote (/remote-agent/:platform)    purpose section, SAME pagination
   nav label: "Remote" / "远程"       mount switch (enable cascade) ·
   "which bot drives Claude Code,     SmartGuide model graph · chat ID lock ·
    with what model and limits?"      bash allowlist · platform setup guide ·
                                      Add Bot (shared dialog, in place)
```

**Shared add/edit interaction.** The bot-resource form lives in one component
(`components/bot/BotConfigDialog`: platform/auth/alias/proxy, validation, QR
binding + orphan reuse). Both sections open it in place — adding a bot from a
Remote page never navigates away; `?add=1` on `/bots/:platform` deep-links
into it for external links.

**Bots nav is hidden, not removed** (`showBotsSection` flag in
`useActivityItems`). Bot has a single purpose today, so a standalone resource
section answers no independent user question. While hidden, the Remote card
is fully self-sufficient — it additionally hosts the RESOURCE operations:
edit (shared dialog), restart/delete (⋮ overflow + confirm), and the pairing
code panel (the artifact the user needs next: `/bind` in chat). No bot-level
enable switch there: with one purpose, the mount switch is the effective
on/off ("no mount, no bot"). Ownership is unchanged — only the rendering
surface moved.

**Graduation plan.** When a second purpose (notify UI) ships: flip
`showBotsSection` back on, move the resource operations back to the Bots
cards, and slim the Remote card to purpose-only. Notify becomes a third
mirrored purpose section, not more toggles on the bot card.

Route history: `/remote-control/*` (original combined) → `/remote-agent/*`
(rename) → the split above. Legacy `/remote-control/*` redirects to
`/remote-agent/*`.

## 11. Notes / trade-offs

- The host router answers prompt replies without the remote-agent pairing
  gate. Prompts only go to operator-configured chats (scenario `chat_id`, or
  chats where the agent is already interacting), and text replies are only
  consumed from the chat holding the pending request, so authorization is
  carried by the prompt's own targeting.
- The host claims the whole "perm" namespace, so `BotHandler`'s own
  perm-reply paths are only reachable in standalone mode. Both delegate to
  `prompt_reply.go`, so behavior is identical either way.
- Because agent approvals and scenario prompts share one prompter, an
  "Always Allow" granted in one flow applies to the other — same semantics
  as before the split, when one prompter served everything.
- A bot with remote_agent mounted but no routes registers a channel nothing
  routes to; harmless, and it keeps "channel comes with running" simple.

## 12. Tests

- `manager_channel_test.go` — notify-only lifecycle + end-to-end prompt
  answer over the tingly in-process transport; coexistence dispatch (`/help`
  falls through the router to the agent while a scenario prompt is claimed);
  channel-registration-is-host-infra (agent-only bot registers/unregisters).
- `manager_lifecycle_test.go` — mount gate (start suppressed / Sync starts /
  Sync stops without touching `Enabled`); restart/goroutine-leak;
  independence between bots.
- `remote/binding/mount_test.go` — `ScenarioMounted` /
  `OutboundScenarioMounted` / `SetScenarioEnabled` truth tables.
