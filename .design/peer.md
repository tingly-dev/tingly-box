# Peer — tingly-box as a pseudo-IM platform for external tools

> Status: **spec + implementation** · Date: 2026-08-13
> Supersedes the earlier "Subscription" draft (same problem, simpler protocol).
> Builds on: [`.design/bot-arch.md`](bot-arch.md) (resource → channel → consumers),
> [`.design/bot-capability-access-control.md`](bot-capability-access-control.md) (target access model),
> [`.design/security.md`](security.md) (no arbitrary access, no silent fallback).

## 1. Problem

The remote pillar supports exactly one external party well: Claude Code. Its
hooks post to `/tingly/claude_code/notify`, its approvals ride the shared
prompter, and `@cc` gives it an inbound identity in chat. Any *other*
independent tool an operator runs (a cron report, a CI gate, an on-call
script) has only half a story:

- **Outbound is possible but wrong.** The bot notify API lets any caller push
  a message, but the caller authenticates as the *operator* (full `UserToken`
  — the whole control plane), messages from different tools are
  indistinguishable in chat, and nothing scopes a tool to "its" chat.
- **Inbound does not exist.** A human in chat cannot address a message *to*
  an external tool; the dispatch pyramid ends at the remote_agent catch-all.

The product need (from real use): *"I have an independent tool that wants
periodic two-way interaction through my bot — like remote does for Claude
Code, but implemented outside tingly-box."*

## 2. Mental model — tingly-box is itself a (tiny) IM platform

Every real IM platform solves this exact problem with the same shape:
register a bot, hand it a token, and give it **two verbs** — *send a
message* and *poll updates* (Telegram's `sendMessage` / `getUpdates` is the
canonical form). tingly-box adopts that shape wholesale:

> An external tool registers a **Peer** on tingly-box, gets a scoped token,
> and from then on talks to tingly-box exactly as if tingly-box were an IM
> platform. tingly-box bridges those messages to the *real* IM: on the real
> platform it is a bot; toward the tool it is the platform.

What a Peer gets:

- an **identity** (name shown in chat, `@name` addressable),
- a **scoped credential** (`tb-peer-…` token, valid only for this peer's
  data plane, never the control plane),
- **two verbs**: `send` (outbound, attributed, chat-scoped) and `updates`
  (inbound long-poll with a confirm-on-next-poll cursor).

The protocol is deliberately the *familiar* one. A tool author who has ever
written a Telegram/Slack bot already knows how to integrate; nothing here
needs to be learned. Less is more applies to **concepts, not just code**:
there is no notify-vs-interact-vs-reply taxonomy, no second reply channel,
no request/response matrix. One outbound verb, one inbound stream.

The hard line that keeps this from becoming "a bot platform on top of bot
platforms": **tingly-box never hosts, schedules, or triggers the tool.**
When the tool runs is the tool's business (cron, CI, systemd). tb only
answers: *may this message pass, as whom, into which chat, and how does the
answer come home.*

## 3. Resource

```
Peer {
  uuid           string   // stable identity ("peer:<uuid>" in CurrentAgent)
  name           string   // mention word: [a-z0-9_-]{2,32}, globally unique,
                          // reserved words rejected (cc, tb, mock, peers)
  bot_uuid       string   // the bot whose channel it uses
  chat_id        string   // bound external chat id (the SAME identifier the
                          // channel layer speaks; see note below)
  exclusive      bool     // true = every plain message in the bound chat is
                          // for this peer (dedicated-chat mode)
  enabled        bool
  token_hash     string   // sha256 of the tb-peer- token; plaintext shown once
  acked_update_id int64   // server-side updates cursor
  created/updated_at
}
```

Deliberately **its own small table**, not a row in the Scenarios JSON and
not a premature merge into the BotCapability schema. The capability model
can absorb peers later with one small migration; coupling the MVP to that
target-state migration would be over-design. The mount question is answered
the same way notify answers it: *an enabled peer is a reason for its bot to
run* (§7).

**Why `chat_id` (external id) and not a `TargetRef`.** Inbound claim (§6)
matches messages by the chat id the platform delivers; outbound delivery
speaks the same id to `channel.Channel`. Binding the peer to the external id
keeps both directions one string comparison and zero resolver dependencies.
Authorization is not weakened by this: **the binding itself is the
authorization** — the operator explicitly chose this chat (the
"authorization carried by the prompt's own targeting" rationale of
bot-arch.md §11). A peer can never reach, or be reached from, any other
chat.

## 4. Credential

An external tool must not hold the operator UserToken — that token is the
whole control plane. So:

- Format `tb-peer-<48 hex>` (crypto/rand), following the `tb-user-` /
  `tb-share-` naming family.
- Stored as SHA-256 hash; plaintext returned exactly once (create / rotate).
- Valid **only** on `/api/v1/peers/{id}/send|updates`, and only when the
  token belongs to `{id}`. Everything else keeps requiring UserToken. The
  operator token also works on the data plane (so a human can test with the
  credential they already have).
- Disabled peer ⇒ token rejected (401), same body for wrong-token and
  disabled so the data plane doesn't leak state.

## 5. HTTP surface

Control plane (UserToken, existing `apiV1` group):

```
GET    /api/v1/peers               list (no token hashes)
POST   /api/v1/peers               create → {peer, token}    token shown once
GET    /api/v1/peers/{id}
PUT    /api/v1/peers/{id}          name/chat/exclusive/enabled
DELETE /api/v1/peers/{id}
POST   /api/v1/peers/{id}/token    rotate → {token}          shown once
```

Data plane (peer token *or* UserToken; separate route group with its own
middleware) — **two verbs, the whole protocol**:

```
POST /api/v1/peers/{id}/send
     {text, reply_to_update_id?}         → {ok, message_id}
GET  /api/v1/peers/{id}/updates?offset=&timeout=&limit=
     → {updates: [...]}                  long-poll, offset confirms
```

### send

One outbound verb. `text` is markdown; tingly-box prefixes it with the
peer's attribution mark (`【name】…`) — two tools sharing a chat must be
distinguishable, this is required UX, not decoration. `reply_to_update_id`
threads the message to the update's original platform message (and forwards
the platform reply-context token where required — Weixin/WeCom); an
already-pruned update degrades to an unthreaded send rather than an error.
Requests 404 with the uniform "bot not running" body when the bound bot has
no registered channel.

There is no title/level/kind taxonomy: a message is a message. Whether it
is a report, a question, or an answer is the tool's concern, expressed in
text — exactly like on a real IM platform.

### updates — the inbound stream

Telegram `getUpdates` semantics:

- Updates are **persisted** (`peer_updates` table): at-least-once, ordered
  by autoincrement id, capped (1000/peer, oldest dropped first, drop
  logged — no silent caps).
- `offset` is the confirm cursor: passing `offset=N` acknowledges every
  update with `id < N` (advances the server-side cursor, prunes) and
  returns updates with `id ≥ N`. Omitting `offset` re-reads unconfirmed
  updates — so a tool that crashes mid-batch sees the batch again. The
  idiom is: process a batch, next poll passes `last_update_id + 1`.
- Empty queue ⇒ the request parks up to `timeout` (default 25s, cap 60s)
  and returns `{updates: []}` on expiry. A parked poller is what "the tool
  is online" means (§6 offline notice, `/peers` status).

Each update is a **typed envelope**, extensible without breaking the
stream:

```
{ update_id, type: "message",
  chat_id, sender_id, message_id, text, created_at }
```

v1 ships exactly one type: `message` (a human's chat message routed to
this peer). Future interaction affordances (inline buttons → a `callback`
type, membership events, …) arrive as **new update types in the same
stream**, never as a second channel — this is the guardrail learned from
the superseded draft, which had grown a parallel in-memory interact/wait
path whose answers (unlike the mailbox) did not survive a restart.

## 6. Inbound — how a human reaches a peer

### Addressing — three tiers, lowest cognition first

`@name` is a **handoff moment, not a per-message prefix** — the same model
`@cc`/`@tb` already established (`CurrentAgent` persisted per chat;
subsequent plain messages follow it).

1. **Exclusive chat binding** (zero addressing): `exclusive: true` — every
   plain message in the bound chat goes to the peer. The right shape for
   the single-tool dedicated-DM/topic case.
2. **Reply-to** (contextual): replying to a message the peer sent routes
   that one message to it, without touching the sticky state. The tool
   posts its morning report; the human replies to that bubble at noon; no
   `@`, and the ongoing `@cc` conversation is undisturbed. Implemented by
   tracking recently sent message ids per peer (in-memory, bounded; lost on
   restart — the other two tiers still work).
3. **Sticky handoff** (explicit): `@report` (or `@report ask text…`)
   switches the chat's `CurrentAgent` to `peer:<uuid>`; plain messages then
   flow to the peer until `@cc`/`@tb`/another `@peer` switches away. The
   confirmation teaches the way back ("send @tb to return").

### Offline notice

(ux-principles: diagnostics must traverse the real path.) When an update is
enqueued and no poller is parked, the human gets one in-chat notice —
`📥 @report is not connected; message queued` — once per offline episode
(flag resets when a poller connects). A periodic tool is *usually* offline;
silently swallowing messages would make the feature feel broken, and a
notice per message would be spam.

### Dispatch — where the consumer sits

The peer consumer implements `bot.Consumer` and is injected **between** the
host router and the remote_agent catch-all:

```
[0] DisabledChatGate · AuthorizationGate · promptReplyRouter   (host)
[1] notify                      (no OnMessage)
[2] peer                        claims per the rules below
[3] remote_agent                terminal catch-all (unchanged)
```

Claim rules, evaluated only when the message's chat is some enabled peer's
bound chat (otherwise pass — rule 0 is the security gate: binding =
authorization, unbound chats never reach a peer):

- Callbacks, `/`-commands (except `/peers`), and `@cc`/`@tb` handoffs
  always **pass** — remote_agent keeps owning commands and its own handoff
  even in a sticky-peer chat, so `/stop`, `/help`, and switching away all
  keep working.
- `/peers` is claimed: lists this chat's peers, their online state, and the
  current target.
- `@<name>` of a peer bound to this chat → sticky handoff (+ enqueue
  trailing text), confirm.
- reply-to a tracked peer message → enqueue to that peer.
- sticky (`CurrentAgent == "peer:<uuid>"`, still bound + enabled) →
  enqueue. If the target is gone (deleted/disabled), reset `CurrentAgent`
  and pass — self-healing, no dead-letter chat.
- exclusive peer bound to this chat → enqueue.
- otherwise pass.

Every claimed message is acknowledged with the platform's "received"
reaction — the sender sees it landed even when the tool answers much later.

Prompt replies never collide: the host's `promptReplyRouter` runs earlier
([0]) and `HandlePromptTextReply` already refuses to consume `@`-prefixed
handoff words.

## 7. Reason to run

A bot runs iff `Enabled && ≥1 consumer mounted` (bot-arch.md §3). The peer
consumer's mount predicate is "the store has ≥1 enabled peer for this bot" —
exactly notify's shape (implicit, derived from data, no new toggle). In the
capability-store lifecycle path (`Manager.mountedConsumers`), consumer names
that are not stored capabilities fall back to the consumer's own `Mounted` —
the generic rule that lets data-derived purposes coexist with explicit
capability rows.

## 8. What this deliberately does not do

**Deferred** (build when a real need arrives, seams already in place):

- Inline buttons: `send` grows an `options` field; the press arrives as a
  `callback`-type update in the same stream. Never a second channel.
- Webhook push delivery (pull via long-poll fits local-first; push needs
  the tool to be reachable + retry/signature machinery).
- Group-actor granularity for who may talk to a peer (arrives with the
  access-model absorption).
- Frontend management page beyond a placeholder (API-first per repo
  convention; run `task codegen` to surface the client SDK).

**Never** (guardrails — crossing these turns the platform into a
bot-hosting platform): scheduling/triggering user flows, a workflow DSL,
hosting user code, an in-tb plugin runtime.

## 9. UX-principle check

- *Eliminate mode pickers*: direction is the endpoint (send / updates);
  message "kinds" don't exist. ✓
- *Separate orthogonal axes*: identity+scope (peer) vs delivery mechanics
  (channel registry) vs lifecycle (consumer mount) stay in separate
  layers. ✓
- *Smart defaults over toggles*: no new global switch; `exclusive` is the
  only per-peer toggle and defaults off; mount is derived from data. ✓
- *Diagnostics traverse the real path*: offline notice comes from the
  actual enqueue path; delivery errors surface the channel's real error. ✓
- *Embed education*: handoff confirmations teach the way back; `/peers`
  shows live state in-chat. ✓
- *Name by product concept*: "Peer" — the other end of the wire on
  tingly-box's own platform; not "subscription" (nothing is subscribed
  to), not "bot" (taken by real IM bots). ✓

## 10. Verification

Four layers, all reproducible with `go test` (no external services — the
tingly in-process platform stands in for the real IM):

- **Domain** (`remote/peer`): name validation (reserved/format/uniqueness),
  token generate/hash/verify, updates enqueue/poll/offset-confirm/cap/
  offline-notice edges, crash-replay (poll without offset re-reads).
- **Consumer** (`remote/control/peerconsumer`): claim-rule truth table
  (pass rules, mention, sticky, reply-to, exclusive, self-heal on dead
  sticky target).
- **HTTP** (`internal/server/module/peerapi/handler_test.go`): CRUD +
  token-shown-once + rotate; data-plane auth matrix (peer token on own id /
  foreign id / disabled peer / user token); send against a fake channel
  incl. reply threading; updates long-poll + offset cursor.
  (`internal/db`): SQLite store CRUD/cursor/cap.
- **End-to-end** (`internal/server/module/peerapi/e2e_test.go`,
  `TestPeerEndToEnd`): the full production stack with only the ends
  simulated — a synthetic human on a tingly `InProcessTransport` and a tool
  speaking real HTTP against `httptest`. Everything between is production
  code: SQLite stores, `bot.Manager` lifecycle (Sync mounts on the first
  peer, stops on the last delete), the real dispatch chain (host gates →
  peer consumer), `channel.Registry` + `imchannel` outbound, the
  scoped-token middleware, and the two-verb protocol. The script walks the
  §2 product story: register → bot runs → `@report` handoff → sticky
  message → tool receives typed update via parked long-poll → threaded
  answer lands as a chat reply → offset confirm + crash replay → offline
  notice → `/peers` overview → forged-token 401 → delete → bot stops.

```bash
go test ./remote/peer/ ./remote/control/peerconsumer/ \
        ./internal/server/module/peerapi/ ./internal/db/ -race
# just the end-to-end story, verbose:
go test ./internal/server/module/peerapi/ -run TestPeerEndToEnd -v
```

A reference tool lives at `remote/peer/examples/echo-tool` — the complete
protocol in one std-lib-only Go file (announce, getUpdates loop with
offset confirm, threaded echo replies). It doubles as a manual smoke test
against a live server:

```bash
go run ./remote/peer/examples/echo-tool -peer <uuid> -token tb-peer-…
```
