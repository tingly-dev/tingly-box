# Bot Interaction Interface — auth + open two-kind API (notify / interactive)

> Status: **spec** · Date: 2026-07-25
> Builds on: [`.design/bot-arch.md`](bot-arch.md) (resource → channel → consumers),
> [`.design/security.md`](security.md) (random default tokens, no silent fallback).
> Scope: the *inbound* HTTP surface that lets external callers drive a bot's
> `channel` — i.e. post a one-way notification or start an interactive prompt —
> **behind authentication**, and generalize it beyond Claude Code hooks so it
> can serve future custom interactions.

## 1. Problem

`bot-arch.md` decoupled the bot into **resource → channel → consumers** and
unlocked the notify-only bot. The HTTP front end for that channel,
`POST /tingly/:scenario/notify` + `GET /tingly/:scenario/wait/:id`
([`internal/server/module/notify`](../../internal/server/module/notify)), is
the only public path to a bot's surface today. It has **two gaps**:

1. **No authentication.** The route group is registered directly on the engine
   with no auth middleware
   ([`routes.go:6`](../../internal/server/module/notify/routes.go#L6)):
   ```go
   ccGroup := engine.Group("/tingly/:scenario")
   ccGroup.POST("/notify", handler.Notify)     // open
   ccGroup.GET("/wait/:request_id", handler.Wait) // open
   ```
   Anyone who can reach the port can drive a configured bot — push arbitrary
   messages into operator chats, or fire interactive prompts that block and
   poll. The security policy in `security.md` exists precisely to avoid this
   shape ("no arbitrary access"), yet the one surface that actually reaches a
   human bypasses it entirely.

2. **Hard-wired to one consumer.** The HTTP body is a free-form `map[string]any`
   parsed *by the scenario plugin* (`claudecode.New`), which classifies events
   by `hook_event_name`. There is no general "send a notification" / "ask a
   question" entrypoint. Custom integrations (a CI pipeline wanting to notify
   "build failed", an on-call tool wanting a yes/no confirm) have no clean path
   — they must impersonate a Claude Code hook payload, which is brittle and
   undocumented.

**Route-family mismatch.** The open `/tingly/:scenario/notify` sits under
`/tingly/*`, which everywhere else means *AI-gateway traffic*
(`POST /chat/completions`, `/messages`, …) gated by `ModelAuthMiddleware`
([`server_routes.go:51`](../../internal/server/server_routes.go#L51)). But
bot interaction is a *control-plane* action — drive the operator's own bot —
identical in trust level to `imbot`/`provider`/`usage` management, which live
under `/api/v1/*` behind `UserAuthMiddleware`
([`server_routes.go:146`](../../internal/server/server_routes.go#L146)). A
control-plane action parked on the gateway prefix is the layer collision
`bot-arch.md` §9 warns against ("one word, one layer"). The general API
belongs under `/api/v1/`.

The user's framing maps directly onto the existing domain model
([`remote/interaction/types.go`](../../remote/interaction/types.go)):

| User's term            | Domain type                | Channel method        | HTTP shape today          |
|------------------------|----------------------------|-----------------------|---------------------------|
| 单向消息 (one-way)      | `interaction.Notification` | `Channel.Send`        | hidden inside `notify`    |
| 交互消息 (interactive)  | `interaction.Interaction`  | `Channel.Prompt`      | `notify` + `wait` long-poll |

So **the two interface kinds already exist as types**; the work is to
(a) gate them behind auth and (b) expose them as a general, caller-facing API
rather than a Claude-Code-only hook shim.

## 2. Goals / non-goals

**Goals**

- G1. Every request that reaches a bot's channel is authenticated. No anonymous
  notify/prompt, ever.
- G2. Expose exactly two interaction interfaces — **notify** (one-way) and
  **interact** (request → reply) — as first-class, documented, caller-facing
  endpoints, usable by any integration, not just Claude Code hooks.
- G3. Preserve the Claude Code hook flow unchanged (its `hook_event_name`
  classification stays a plugin; the new general API sits *alongside* it).
- G4. Distinguish the two kinds **explicitly in the URL/method**, not by
  sniffing a payload field. (UX principle: eliminate mode pickers; the request
  shape is the mode.)

**Non-goals**

- Out-of-process scenario plugins / RPC plugin transport (kept domain-neutral
  for the future, per `interaction` package doc, but not built here).
- A frontend UI to create outbound *routes* (the Notify-page write-path gap
  noted in `bot-arch.md` §10 — separate project).
- AuthN *of the human answering* a prompt. Reply authorization is already
  carried by prompt targeting (`bot-arch.md` §11); this spec concerns callers,
  not repliers.
- Changing the model: resource/channel/consumer is untouched.

## 3. Design

### 3.1 One token model, reused — do not invent a third

The codebase already has two operative credentials
([`security.md`](security.md)):

| Existing token   | Purpose                          | Source                   |
|------------------|----------------------------------|--------------------------|
| `UserToken`      | control plane / web UI           | `tb-user-<64 hex>`       |
| `ModelToken`     | model proxy                      | JWT-signed               |

A third, "notify token" is **tempting but wrong**: it multiplies rotation
surfaces and forces operators to mint/distribute yet another secret before any
integration works. Instead, **bot interaction auth reuses `UserToken`** via the
existing `UserAuthMiddleware`
([`middleware/auth.go:251`](../../internal/server/middleware/auth.go#L251)).

Rationale:

- Bot interaction is a **control-plane action** (drive the operator's own bot),
  identical in trust level to "restart the server" or "edit a provider". It is
  *not* model-proxy traffic and should not use `ModelToken`.
- Reusing `UserToken` means an operator's existing token (the one they already
  use for the web UI / CLI) works immediately, with no new distribution step.
- The middleware already exists, is tested, and emits the standard error shape.

We do **not** spec a third, scoped "notify token" here. Per the project's UX
rule (don't over-design; follow existing conventions), the only credential a
caller needs is the one that already exists. If a scoped, per-bot, revocable
token ever becomes a real requirement, it is a separate capability proposal —
not a prerequisite for this API.

### 3.2 URL shape — control-plane route family

Per the "eliminate mode pickers" UX principle, the kind is in the route, not
the body. The general API lives under `/api/v1/bots/{bot}/...` — the same
control-plane family as `imbot`/`provider`/`usage`, reusing the existing
`apiV1` group that already applies `getUserAuthMiddleware`
([`server_control.go:154`](../../internal/server/server_control.go#L154)):

```
POST   /api/v1/bots/{bot}/notify        one-way push           → 200
POST   /api/v1/bots/{bot}/interact      start interactive      → 202 + request_id
GET    /api/v1/bots/{bot}/interact/{id} long-poll for reply   → 200/410/504
GET    /api/v1/bots/{bot}/chats         discover chat_id       → 200
```

`{bot}` is the bot UUID (the `channel.Registry` key — see
[`runtime_default.go:47`](../../remote/scenario/runtime_default.go#L47)).
A bot is a connection resource; addressing it directly matches the resource
model and avoids the indirect "scenario → binding → bot" resolution that the
Claude Code hook path still needs.

Why `/api/v1/bots/*` and not under `/tingly/`:

- `/tingly/:scenario` is *scenario-scoped AI-gateway traffic* — its first
  segment is a plugin name (`claude_code`), resolution goes through
  `binding.Resolver`, and the whole family is `ModelAuthMiddleware`-gated
  gateway traffic. The general API is *control-plane*, *bot-scoped*, and
  `UserAuthMiddleware`-gated. Two different trust levels → two prefixes.
  Collapsing them resurrects the "is this segment a scenario or a bot id?"
  name collision `bot-arch.md` §9 just spent effort splitting.
- `/api/v1/` is where every other "drive the operator's own system" endpoint
  already lives (`imbot.RegisterRoutes(apiV1, …)`,
  [`server_control.go:191`](../../internal/server/server_control.go#L191)).
  Following that convention is the lowest-surprise choice — a new control-plane
  endpoint does not invent a new route family.

### 3.3 Discovering `chat_id`

Both request bodies below require a `chat_id`, and until this endpoint existed
that value was undiscoverable: it is not in `/help`, not on the bots table, and
not returned by any other API. An endpoint whose required input cannot be
obtained is not usable, so the route family carries a third, read-only member:

```http
GET /api/v1/bots/{bot}/chats
→ 200 {"chats": [{"chat_id": "...", "platform": "...", "is_paired": true,
                  "project_path": "...", "updated_at": "..."}],
       "running": true}
```

Two scoping rules keep the shared chat store from leaking:

- **Platform.** The store is keyed by `chatID` alone, with no platform
  dimension, so a record whose `Platform` is empty or does not match the bot's
  own channel platform cannot be proven to belong to this bot and is dropped at
  the source (`ChatStoreJSON.ListChats`). The same reasoning makes
  `GetOrCreateChat` refuse a cross-platform `chatID` collision instead of
  silently handing back — and later re-stamping — another platform's chat.
- **Chat-id lock.** When the bot has a `chat_id_lock` set it can reach exactly
  that one chat, so the list collapses to it.

A bot that is not running has zero reachable chats. That is an empty state, not
an error, so this endpoint returns `200` with an empty list and `running:false`
rather than the `404` `/notify` and `/interact` use — the caller can then say
"start the bot" instead of "listing failed" (ux-principles #11).

`/help` also prints the chat's own id in-conversation, so the value is
obtainable from whichever surface the operator happens to be on.

### 3.4 The two interfaces

Both are thin handlers over the **same** `channel.Channel` the notify consumer
already uses. No new runtime; `DefaultRuntime.Notify` / `.Ask`
([`runtime_default.go:67`](../../remote/scenario/runtime_default.go#L67)) are
exactly the two operations we need. The new module calls the registry's channel
directly, so it does not even need the runtime — but reusing the runtime keeps
audit (`RuntimeAuditSink`) consistent. **Decision: call the channel directly
from the handler**, and emit audit through the same `audit.Logger` already wired
in [`server_control.go:139`](../../internal/server/server_control.go#L139).
This keeps the handler self-contained and avoids coupling the open API to the
scenario runtime's event-parsing assumptions.

#### Notify (one-way)

```http
POST /api/v1/bots/{bot}/notify
Authorization: Bearer <UserToken>
Content-Type: application/json

{
  "chat_id": "dm:ops",          // required: target conversation
  "title": "Build #412 failed", // optional
  "body":  "...",               // required
  "level": "info"               // optional: info|warn|error
}
```

→ `200 {"ok":true}` on delivery; `404` if the bot isn't running (no channel in
registry); `403` if authenticated but the bot is not in notify-capable state.

Maps to `channel.Channel.Send(ctx, Target{ChatID: chat_id},
Notification{Title, Body})`. Fire-and-forget semantics, identical to
`DefaultRuntime.Notify`.

#### Interact (request → reply)

```http
POST /api/v1/bots/{bot}/interact
Authorization: Bearer <UserToken>
Content-Type: application/json

{
  "chat_id": "dm:ops",
  "kind": "confirm",            // confirm | choose | ask
  "title": "Deploy to prod?",
  "body":  "commit a1b2c3",
  "options": [{"value":"yes","label":"Yes","style":"primary"},
              {"value":"no","label":"No","style":"danger"}],  // confirm/choose
  "timeout_seconds": 120
}
```

→ `202 {"request_id":"<uuid>","expires_at":"..."}`. Client then long-polls:

```http
GET /api/v1/bots/{bot}/interact/{request_id}?timeout=45s
```

Response status mapping is **identical** to today's `/wait` endpoint
([`handler.go:118`](../../internal/server/module/notify/handler.go#L118)):
`200 answered/cancelled`, `410 timeout`, `504 pending`, `404 expired`. This is
not a new contract — it reuses `interaction.Registry[Result]` and its
`Await/Resolve/Cancel` lifecycle verbatim.

"如果能合并执行也行" (could be merged): the two *interfaces* stay distinct
(URL/method is the mode — G4), but they share **one handler module, one auth
middleware, one registry, one channel lookup**. That is the merge that matters;
merging the *HTTP verbs* would reintroduce a mode picker.

### 3.5 Auth wiring — reuse the `apiV1` group

The control-plane `apiV1` group already exists and already applies
`getUserAuthMiddleware`
([`server_control.go:154-155`](../../internal/server/server_control.go#L154)).
We register the new module onto it the same way `imbot`, `usage`, and `oauth`
do — no new group, no new middleware application:

```go
// server_control.go — alongside the existing imbot/usage registrations on apiV1
botAPI := notifymodule.NewBotAPIHandler(s.channelRegistry, s.interactionRegistry, auditLog)
notifymodule.RegisterBotRoutes(apiV1, botAPI)   // routes under /api/v1/bots/{bot}/*
```

Routes are registered as `bots/:bot/{notify,interact,interact/:id}` inside that
group, so the full paths are `/api/v1/bots/{bot}/...` as in §3.2. Auth is
inherited from the group — nothing extra to wire.

The Claude Code hook path (`/tingly/:scenario/*`) is left **unchanged** for now
(see §5 — it gets auth in a follow-up to avoid breaking hook scripts in the
same PR).

The handler must still defend in depth: even past auth, resolve the bot from
the registry and 404 when absent — an authenticated caller should not be able
to probe which bots exist via timing/detail differences (return the same
"bot not running" body shape for unknown and stopped bots).

### 3.6 What about `/tingly/:scenario/notify` itself?

It is the **same unauthenticated hole**. Closing the new general API while
leaving the Claude Code hook path open would be half a fix. Plan:

- **This spec / first PR:** ship the authenticated `/api/v1/bots/*` general API.
- **Immediate follow-up (same design):** add `getUserAuthMiddleware()` to the
  `/tingly/:scenario` group and thread the token into Claude Code's hook
  script config (the hook runs locally, so it can read the operator token from
  the same place the gateway already writes it). This is called out separately
  because it touches the Claude Code hook *client* (env / config) and the
  scenarios' stored bindings, not just the server.
- **Long-term (not committed):** the hook path is itself a layer oddity — a
  control-plane action parked on the gateway prefix. Migrating it to
  `/api/v1/...` would be the clean end state, but it is a breaking change to
  every hook script's URL and is explicitly **not** in scope here.

## 4. Module layout

New file alongside the existing thin handler, not a replacement:

```
internal/server/module/notify/
  handler.go          # existing: /tingly/:scenario/{notify,wait} — unchanged
  bot_api.go          # NEW: BotAPIHandler (notify + interact + wait)
  bot_routes.go       # NEW: RegisterBotRoutes(group, handler)
  bot_api_test.go     # NEW
```

Why keep both `handler.go` and `bot_api.go`: the scenario path's value is
*plugin classification* (hook_event_name → push vs interactive); the bot path's
value is *direct, classified-by-caller* delivery. They share types
(`interaction.*`) and dependencies (`channel.Registry`, `interaction.Registry`)
but not control flow. Forcing them together would make the handler read as two
modes behind one entrypoint — exactly the picker we're avoiding.

## 5. Migration / sequencing

1. **PR 1 (this spec):** `/api/v1/bots/*` general API, registered on the
   existing `apiV1` group (inheriting `getUserAuthMiddleware`). Claude Code
   path untouched → zero behavior change for existing users.
2. **PR 2:** gate `/tingly/:scenario/*` behind `getUserAuthMiddleware()`; update
   the Claude Code hook helper to send the token. Document the requirement.

No third step is committed. A scoped per-bot token for third-party integrations
is **not** pre-built; if it is ever needed it arrives as its own proposal, and
the handler is already auth-mechanism-agnostic (auth comes from the group, not
the handler).

## 6. Open questions (need a decision before build)

- **Q1 — `/tingly/:scenario` auth in the same PR or a follow-up (PR 2)?**
  Bundling is more secure sooner but couples server + hook-client changes and
  risks breaking existing hook scripts that don't yet send a token. Default:
  follow-up, so PR 1 is purely additive.
- **Q2 — audit detail.** Record `chat_id` and `kind` in the audit log (useful)
  vs. only the fact-of-call (less PII in logs). Default: log caller + bot +
  kind + outcome, omit `body` text.

## 7. Tests

- `bot_api_test.go`: chats lists only same-platform records and collapses to a
  chat-id lock when set, and reports `running:false` for a stopped bot;
  notify delivers via a fake channel; interact returns 202 +
  resolves via the shared registry; wait reuses the existing 200/410/504
  matrix; 401 without token; 404 for unknown/stopped bot (same body shape);
  403 for non-notify-capable bot state.
- Reuse `interaction.Registry` tests — no new lifecycle behavior introduced.

## 8. UX-principle check (`.design/ux-principles.md`)

- *Eliminate mode pickers*: the kind is in the URL (`/notify` vs `/interact`),
  not a `{"mode":...}` field. ✓
- *Split name collisions*: the control-plane API lives under `/api/v1/bots/*`
  (`UserAuthMiddleware`), the Claude Code gateway path under `/tingly/:scenario/*`
  (`ModelAuthMiddleware`) — different trust levels stay in different route
  families, no "is this segment a scenario or a bot?" ambiguity. ✓
- *Smart defaults over toggles*: no new config flag, no new token; auth reuses
  the existing control-plane credential and group. ✓
- *Diagnostics traverse the real path*: the handler resolves the real channel
  from the registry and returns the actual delivery error, not a generic 500. ✓
