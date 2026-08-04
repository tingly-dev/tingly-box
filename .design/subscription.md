# Subscription — custom remote for external tools (双端收发)

> Status: **spec + phase 1-2 implementation** · Date: 2026-08-04
> Builds on: [`.design/bot-arch.md`](bot-arch.md) (resource → channel → consumers),
> [`.design/bot-interaction-api.md`](bot-interaction-api.md) (general notify/interact API),
> [`.design/bot-capability-access-control.md`](bot-capability-access-control.md) (target access model),
> [`.design/security.md`](security.md) (no arbitrary access, no silent fallback).

## 1. Problem

The remote pillar supports exactly one external party well: Claude Code. Its
hooks post to `/tingly/claude_code/notify`, its approvals ride the shared
prompter, and `@cc` gives it an inbound identity in chat. Any *other*
independent tool an operator runs (a cron report, a CI gate, an on-call
script) has only half a story:

- **Outbound is solved.** `POST /api/v1/bots/{bot}/notify|interact` +
  long-poll (bot-interaction-api.md) lets any caller push a notification or
  ask a question. But the caller is anonymous — every tool authenticates as
  the *operator* (full `UserToken`, which can also restart the server and
  edit providers), messages from different tools are indistinguishable in
  chat, and nothing scopes a tool to "its" chat.
- **Inbound does not exist.** There is no way for a human in chat to address
  a message *to* an external tool. The dispatch pyramid ends at the
  remote_agent catch-all; a tool cannot be `@`-ed, cannot receive a reply to
  its own report, and has no delivery surface.

The product need (from real use): *"I have an independent tool that wants
periodic two-way interaction through my bot — like remote does for Claude
Code, but implemented outside tingly-box."*

## 2. Mental model — tb is a switchboard, not a bot platform

A **Subscription** is a named association between an external tool and a
bot + chat. It gives the tool:

- an **identity** (name shown in chat, `@name` addressable),
- a **scoped credential** (`tb-sub-…` token that can only drive this
  subscription's endpoints, never the control plane),
- an **outbound path** (notify / interact, attributed and chat-scoped),
- an **inbound mailbox** (messages addressed to it, pulled via long-poll).

The hard line that keeps this from becoming "a bot platform on top of bot
platforms": **tingly-box never hosts, schedules, or triggers the tool.**
When the tool runs is the tool's business (cron, CI, systemd). tb only
answers: *may this message pass, as whom, into which chat, and how does the
answer come home.* The capability surface is three verbs — notify, interact,
receive — and it does not grow.

## 3. Resource

```
Subscription {
  uuid          string   // stable identity ("sub:<uuid>" in CurrentAgent)
  name          string   // mention word: [a-z0-9_-]{2,32}, globally unique,
                         // reserved words rejected (cc, tb, mock)
  bot_uuid      string   // the bot whose channel it uses
  chat_id       string   // bound external chat id (the SAME identifier the
                         // channel layer speaks; see note below)
  exclusive     bool     // true = every plain message in the bound chat is
                         // for this subscription (dedicated-chat mode)
  enabled       bool
  token_hash    string   // sha256 of the tb-sub- token; plaintext shown once
  acked_event_id int64   // server-side mailbox cursor
  created/updated_at
}
```

Deliberately **its own small table**, not a row in the Scenarios JSON and
not a premature merge into the BotCapability schema. The capability model
(bot-capability-access-control.md) can absorb subscriptions later with one
small migration; coupling the MVP to that target-state migration would be
over-design. Correspondingly, the mount question is answered the same way
notify answers it: *an enabled subscription is a reason for its bot to run*
(§7).

**Why `chat_id` (external id) and not a `TargetRef`.** Inbound claim (§6)
matches messages by the chat id the platform delivers; outbound delivery
speaks the same id to `channel.Channel`. Binding the subscription to the
external id keeps both directions one string comparison and zero resolver
dependencies in the consumer. When subscriptions are absorbed into the
access model, the binding migrates to `TargetRef` together with everything
else. Authorization is not weakened by this: **the binding itself is the
authorization** — the operator explicitly chose this chat, exactly the
"authorization carried by the prompt's own targeting" rationale of
bot-arch.md §11. A subscription can never reach, or be reached from, any
other chat.

## 4. Credential — the scoped token, and why now

`bot-interaction-api.md` §3.1 deliberately did not mint a third token and
said a scoped token must arrive "as its own proposal" when a real
requirement exists. This is that proposal: an **external tool must not hold
the operator UserToken** — that token is the whole control plane. So:

- Format `tb-sub-<48 hex>` (crypto/rand), following the `tb-user-` /
  `tb-share-` naming family.
- Stored as SHA-256 hash; plaintext returned exactly once (create / rotate).
- Valid **only** on `/api/v1/subscriptions/{id}/…` data-plane endpoints, and
  only when the token belongs to `{id}`. Everything else keeps requiring
  UserToken. The operator token also works on the data plane (so a human can
  test with the credential they already have — no new distribution step for
  debugging).
- Disabled subscription ⇒ token rejected (401), same body for wrong-token
  and disabled so the data plane doesn't leak state.

## 5. HTTP surface

Control plane (UserToken, existing `apiV1` group):

```
GET    /api/v1/subscriptions              list (no token hashes)
POST   /api/v1/subscriptions              create → {subscription, token}   token shown once
GET    /api/v1/subscriptions/{id}
PUT    /api/v1/subscriptions/{id}         name/chat/exclusive/enabled
DELETE /api/v1/subscriptions/{id}
POST   /api/v1/subscriptions/{id}/token   rotate → {token}                 shown once
```

Data plane (subscription token *or* UserToken; separate route group with its
own middleware):

```
POST /api/v1/subscriptions/{id}/notify        one-way push into the bound chat
POST /api/v1/subscriptions/{id}/interact      prompt in the bound chat → 202 + request_id
GET  /api/v1/subscriptions/{id}/interact/{rid}  long-poll the reply (same 200/410/504/404
                                                 matrix as the bot API)
GET  /api/v1/subscriptions/{id}/events?timeout=30s   long-poll inbound mailbox
POST /api/v1/subscriptions/{id}/events/ack    {up_to: <event id>} advance cursor
POST /api/v1/subscriptions/{id}/reply         {text, event_id?} send into chat,
                                              threaded to the event when given
```

The notify/interact bodies are the bot-API bodies minus `target`/`chat_id` —
the subscription *is* the target. Delivery reuses the same
`channel.Registry` + `interaction.Registry` the bot API drives; no new
runtime. Requests 404 with the uniform "bot not running" body when the
bound bot has no registered channel.

**Attribution.** Every outbound message a subscription sends is prefixed
with its name — `【report】…` — on the title (or body when there is no
title). Two tools sharing a chat must be distinguishable; this is required
UX, not decoration. The same prefix marks `reply` sends.

## 6. Inbound — mailbox + three addressing tiers

### Mailbox semantics

- Events are **persisted** (`subscription_events` table): at-least-once,
  ordered by autoincrement id, delivered via long-poll (`GET /events`
  returns events with `id > acked_event_id`, waits up to `timeout` when
  empty), acknowledged by advancing the server-side cursor
  (`POST /events/ack`). A crashed tool re-reads unacked events on
  reconnect. Bounded: oldest unacked events beyond a cap (1000/subscription)
  are dropped oldest-first, and the drop is logged.
- **Offline notice** (ux-principles: diagnostics must traverse the real
  path). When an event is enqueued and no poller is currently waiting, the
  human gets one in-chat notice — `📥 @report is not connected; message
  queued` — once per offline episode (flag resets when a poller connects).
  A periodic tool is *usually* offline; silently swallowing messages would
  make the feature feel broken, and a notice per message would be spam.

### Addressing — how a human reaches the subscription without typing `@` every time

Three tiers, lowest cognition first. `@name` is a **handoff moment, not a
per-message prefix** — the same model `@cc`/`@tb` already established
(`CurrentAgent` persisted per chat; subsequent plain messages follow it).

1. **Exclusive chat binding** (zero addressing): `exclusive: true` — every
   plain message in the bound chat goes to the subscription. The right shape
   for the single-tool dedicated-DM/topic case.
2. **Reply-to** (contextual): replying to a message the subscription sent
   routes that one message to it, without touching the sticky state. The
   tool posts its morning report; the human replies to that bubble at noon;
   no `@`, and the ongoing `@cc` conversation is undisturbed. Implemented by
   tracking recently sent message ids per subscription (in-memory, bounded;
   lost on restart — the other two tiers still work, documented).
3. **Sticky handoff** (explicit): `@report` (or `@report ask text…`)
   switches the chat's `CurrentAgent` to `sub:<uuid>`; plain messages then
   flow to the subscription until `@cc`/`@tb`/another `@sub` switches away.
   Confirmation message teaches the way back ("send @tb to return").

### Dispatch — where the consumer sits

The subscription consumer implements `bot.Consumer` and is injected
**between** the host router and the remote_agent catch-all:

```
[0] DisabledChatGate · AuthorizationGate · promptReplyRouter   (host)
[1] notify                      (no OnMessage)
[2] subscription                claims per the rules below
[3] remote_agent                terminal catch-all (unchanged)
```

Claim rules, evaluated only when the message's chat is some enabled
subscription's bound chat (otherwise pass — rule 0 is the security gate:
binding = authorization, unbound chats never reach a subscription):

- Callbacks, `/`-commands (except `/subs`), and `@cc`/`@tb` handoffs always
  **pass** — remote_agent keeps owning commands and its own handoff even in
  a sticky-subscription chat, so `/stop`, `/help`, and switching away all
  keep working.
- `/subs` is claimed: lists this chat's subscriptions and the current
  target.
- `@<name>` of a subscription bound to this chat → sticky handoff (+ enqueue
  trailing text), confirm.
- reply-to a tracked subscription message → enqueue to that subscription.
- sticky (`CurrentAgent == "sub:<uuid>"`, still bound + enabled) → enqueue.
  If the target is gone (deleted/disabled), reset `CurrentAgent` and pass —
  self-healing, no dead-letter chat.
- exclusive subscription bound to this chat → enqueue.
- otherwise pass.

Prompt replies never collide: the host's `promptReplyRouter` runs earlier
([0]) and `HandlePromptTextReply` already refuses to consume `@`-prefixed
handoff words.

## 7. Reason to run

A bot runs iff `Enabled && ≥1 consumer mounted` (bot-arch.md §3). The
subscription consumer's mount predicate is "the store has ≥1 enabled
subscription for this bot" — exactly notify's shape (implicit, derived from
data, no new toggle). In the capability-store lifecycle path
(`Manager.mountedConsumers`), consumer names that are not stored
capabilities (`notify`/`remote_control`) fall back to the consumer's own
`Mounted` — the generic rule that lets data-derived purposes coexist with
explicit capability rows.

## 8. What this deliberately does not do

**Deferred** (build when a real need arrives, seams already in place):

- Webhook push delivery (pull via long-poll fits local-first; push needs the
  tool to be reachable + retry/signature machinery).
- Group-actor granularity for who may talk to a subscription (arrives with
  the access-model absorption).
- Declarative event templates / custom scenario names on the hook path.
- Frontend management page beyond a placeholder (API-first per repo
  convention; run `task codegen` to surface the client SDK).

**Never** (guardrails — crossing these turns the switchboard into a
platform): scheduling/triggering user flows, a workflow DSL, hosting user
code, an in-tb plugin runtime.

## 9. UX-principle check

- *Eliminate mode pickers*: direction is the endpoint (notify / interact /
  events / reply), never a body field. ✓
- *Separate orthogonal axes*: identity+scope (subscription) vs delivery
  mechanics (channel/interaction registries) vs lifecycle (consumer mount)
  stay in separate layers; none is rebuilt. ✓
- *Smart defaults over toggles*: no new global switch; `exclusive` is the
  only per-subscription toggle and defaults off; mount is derived from data. ✓
- *Diagnostics traverse the real path*: offline notice comes from the actual
  enqueue path; delivery errors surface the channel's real error. ✓
- *Embed education*: handoff confirmations teach the way back; `/subs` shows
  live state in-chat. ✓
- *Name by product concept*: "Subscription" everywhere; `scenario` is not
  reused (bot-arch.md §9 debt not compounded). ✓

## 10. Tests

- Domain: name validation (reserved/format/uniqueness), token
  generate/hash/verify, mailbox enqueue/poll/ack/cap/offline-notice edges.
- Consumer: claim-rule truth table (pass rules, mention, sticky,
  reply-to, exclusive, self-heal on dead sticky target), CurrentAgent
  round-trip through the real chat store.
- HTTP: CRUD + token-shown-once + rotate; data-plane auth matrix (sub token
  on own id / foreign id / disabled sub / user token); notify/interact
  against a fake channel; events long-poll + ack cursor; reply threading.
- Lifecycle: subscription-only bot runs; deleting the last subscription
  stops it on next sync (mirrors notify's mount tests).
