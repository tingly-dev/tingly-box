# WorkBuddy OAuth (Tencent WorkBuddy / CodeBuddy)

How Tingly-Box signs a user into **WorkBuddy** (Tencent; CodeBuddy is the same
CN account behind another entry point), turns the login into a provider, keeps
it fresh, and serves it to any OpenAI / Anthropic / Responses client. Generic
OAuth mechanics (sessions, re-auth, refresher) are in `oauth.md`; this file
covers only what WorkBuddy does differently. Proposal: TEP-2097.

## Status of the protocol: third-party, cross-checked

This integration uses WorkBuddy's undocumented desktop login routes, rather
than the OAuth 2.1 application flow of its Open Platform. The routes first came from
integrations that agree on them (`elavita/dsh-codearts-auth`,
`pilciw/codebuddy2api`, OmniRoute's CodeBuddy CN gateway). The client
identification and the billing meter were then aligned with
`@magpie-community/opencode-workbuddy-auth` (MIT, npm), which runs against real
accounts on both builds and records what WorkBuddy refuses, and with CodexBar's
WorkBuddy provider (steipete/CodexBar#4227), which reads the same meter with
website cookies. Tingly-Box itself has still not been run against a real
account. To keep a correction cheap:

- every value the upstream polices (host, `platform`, `User-Agent`, client
  headers) lives in one table, `ai.WorkBuddySiteFor` / `ai.WorkBuddy*`
  constants (`ai/issuer.go`), read by login, inference and quota;
- response fields are parsed tolerantly (camelCase and snake_case, expiry as a
  lifetime, unix s/ms, RFC 3339, or the JWT `exp`), so a spelling difference
  does not fail a login.

When a real capture disagrees, fix the table / parser and record it here.

### Upstream refusals (2026-10-21)

The WorkBuddy transport normalizes `{code,msg,displayMsg}` into an OpenAI
error envelope, preserving the HTTP status, vendor code and original body.
Only `11128` with an `unapproved channel` message, or client-identification
code `10085`, marks a channel refusal. Such a 400 may use the existing fallback
before response commitment, without counting as a transient service failure.
Other 400s remain terminal; `403/11140` includes an account-restriction message
and follows the existing 403 handling. No new health state or cooldown is added.

References: [channel refusals](https://github.com/magpie-community/plugins/issues/279),
[account restrictions](https://github.com/yetone/magpie/issues/1849).

## Login: server-mediated poll (`OAuthMethodServerPoll`)

Same shape as ZCode (`zcode-oauth.md`): nothing returns to localhost, so the
login URL works on a phone and the flow works headless.

```
POST {origin}/v2/plugin/auth/state?platform=<platform>   → {state, authUrl}
open authUrl (any browser)
GET  {origin}/v2/plugin/auth/token?state=…   1s polls, 5 min
     code 11217 = pending … → {accessToken, refreshToken, expiresIn|expiresAt}
GET  {origin}/v2/plugin/login/account?state=…   best-effort, 15 s
     code 12151 = pending … → {uid, nickname, enterprise domain}
```

Every login request carries `X-Domain`, `X-Product: SaaS` and the client
`User-Agent`; the state request also carries the desktop client's
`X-No-Authorization` / `X-No-User-Id` / `X-No-Enterprise-Id` /
`X-No-Department-Info: true`. The sign-in URL is opened with `version=2.0.0`
and a fresh 32-hex `loginSessionId`, as the desktop client opens it.

- **Poll semantics** match ZCode: the pending code, transport errors, 5xx,
  408/429 and a non-envelope body are one more round; any other code ends the
  login with the server's message (it reaches the login dialog).
- **Account info is best-effort.** It only names the provider (nickname, else
  the issuer's display name) and fills `X-User-Id`; a login that has tokens
  never fails on it.
- Web and CLI share one entry point, `Manager.InitiateServerPollFlow` /
  `CompleteServerPollFlow` (`ai/oauth/serverpoll.go`), which dispatches to the
  ZCode or WorkBuddy flow. The handler's background poller
  (`pollForServerPollToken`) and the CLI no longer name an issuer.

## Issuers: one per account region

| issuer | origin / `X-Domain` | `platform` | `X-Agent-Type` |
|---|---|---|---|
| `workbuddy` | `https://www.workbuddy.ai` | `workbuddy-ai` | `main` |
| `workbuddy_cn` | `https://copilot.tencent.com` | `workbuddy` | — |

The CN `platform` was `ide` (CodeBuddy IDE's) until the cross-check above;
`workbuddy` is what the WorkBuddy desktop client asks for. `X-Product-Code` was
dropped for the same reason: the desktop client does not send it.

Two issuers, not a region switch: they are different accounts on different
hosts, and re-auth's issuer check must never let one overwrite the other
(`ux-principles.md` §2, §3). CodeBuddy is not a third issuer — it is an entry
point onto the CN accounts, so the CN card says "WorkBuddy / CodeBuddy CN".

## Token lifecycle

Unlike ZCode the login yields a real access/refresh pair with an expiry, so the
generic machinery applies unchanged:

- `ExpiresAt` comes from the response, else the JWT `exp`; unknown stays `""`.
- **Refresh** is `POST {origin}/v2/plugin/auth/token/refresh` with the refresh
  token in `X-Refresh-Token` — not an OAuth grant. `Manager.refreshToken`
  dispatches WorkBuddy issuers to it, so the background refresher and
  `POST /oauth/refresh` both work with no WorkBuddy branch of their own. A
  rejected refresh reaches the "Token refresh failed → Reauthorize" dialog.
- `ExtraFields`: `workbuddy_uid`, `workbuddy_nickname`,
  `workbuddy_enterprise_domain`; the uid is also the provider's `UserID`
  (`account_id` metadata).

## Serving requests

- Provider: `APIStyle openai`, `APIBase = {origin}/v2`, so the SDK posts to
  `{origin}/v2/chat/completions`. Anthropic and Responses clients reach it
  through the existing protocol conversion (endpoint mode chat); there is no
  known native Anthropic endpoint, so the provider is not dual.
- `WorkBuddyClient` (`internal/client/workbuddy_client.go`) is the OpenAI
  client plus `workbuddyRoundTripper`, which adds `WorkBuddySite.ClientHeaders`
  and `X-User-Id`.

### Client identification

- **`User-Agent: WorkBuddy/5.5.6` is required on chats**: any other is refused
  with error 10085. The first version sent `CLI/2.108.1 CodeBuddy/2.108.1`,
  which would have had every chat refused.
- The desktop client also sends `X-IDE-Type` / `X-IDE-Name: WorkBuddy`,
  `X-IDE-Version`, `X-Requested-With: XMLHttpRequest`, `X-Agent-Intent: craft`,
  `X-Agent-Purpose: conversation` (and `X-Agent-Type: main` on the
  international build). WorkBuddy's usage list attributes a request to a
  client by them; without them it shows under no client.
- `GET /v3/config` answers per client: `CLI/2.0.0 WorkBuddy/5.5.6` gets the
  config with a `cli` agent, a bare `WorkBuddy/…` the IDE's.

### Streaming only

WorkBuddy refuses a chat that is not streamed. `WorkBuddyClient.ChatCompletionsNew`
sends every complete call as a stream (with `include_usage`) and assembles the
chunks into one `ChatCompletion` — the same fallback `CodexClient.ResponsesNew`
makes for the ChatGPT backend. It sits in the client, not in the stage
endpoint's `StreamOnly`, because complete chat calls also come from the
passthrough, cross-protocol and tool-engine paths, which never reach a stage.
An upstream refusal keeps its HTTP status; a stream that breaks before any
answer is an `EmptyResponseError` (502, failover may retry).

### Request shapes

`workbuddyRoundTripper` rewrites each `/chat/completions` body
(`internal/client/workbuddy_request.go`) into shapes WorkBuddy takes, meaning
the same. A body that needs nothing is sent byte for byte.

- `max_completion_tokens` → `max_tokens`; role `developer` → `system`.
- `tool_choice` objects → WorkBuddy's string form (`auto`, `required`, or the
  tool's name); `none` drops the tools.
- Tool names outside `[A-Za-z0-9_-]` are refused: they go out with `_` (kept
  apart from existing names), the chat's history and `tool_choice` follow, and
  the model's calls are named back in the response stream, so the client only
  ever sees its own names.
- Tool parameter schemas (400, code 11133 otherwise, per model): the root is
  made an object (allOf merged; anyOf / oneOf object branches merged, a field
  stays required only if every branch requires it); types are lowercased and
  unknown ones dropped; a non-list `required` is dropped; an enum gets its
  values' type, or becomes an anyOf of one typed enum per type, and an empty
  enum is dropped; boolean sub-schemas become `{}` / `{"not": {}}`; a
  pattern's `\0` becomes `\u0000`; tuple `items` become one schema.

### Models

`GET {origin}/v3/config`, not `/models`: the `cli` agent's `models` when
present, else every `data.models[].id`. There is no embedded catalog entry:
model ids are account- and region-specific, so a failed fetch shows an empty
list rather than invented ids.

## Quota

`ProviderTypeWorkBuddy` (`ai/quota/fetcher/workbuddy.go`), one fetcher for both
issuers. The billing meter lives on the same origin as the rest of the API —
`copilot.tencent.com` / `www.workbuddy.ai` — under `/billing/meter/...` with no
`/v2`, and takes the plugin bearer plus the client headers. (CodexBar reads it
on `www.workbuddy.cn` with website cookies, which is the same backend behind
the website.) Every reply is `{code, msg, data}`; a non-zero code is an error
with WorkBuddy's message.

- **`POST get-user-resource-summary`** `{}` → `data.Packages[]` with
  `CycleTotalCapacity` / `CycleRemainCapacity` / `CycleUsedCapacity` /
  `CycleFrozenCapacity` (decimal strings) and `CapacityUnit`, plus
  `SubscriptionPackageName` and `IsPaidUser`. The `credits` packages are summed
  into one **Credits** window, as WorkBuddy's own usage page shows them; frozen
  credits (held by running requests) go in the description. Plan: the
  subscription name, else Pro / Free from `IsPaidUser`.
- **Reset time** (optional, 5 s budget): `get-user-resource-paid-packages` and
  `get-user-resource-free-packages` list `Accounts[].CycleEndTime`. They refuse
  a request without `PackageCodes` (code 10001), so the desktop client's codes
  are sent. The earliest future end among the packages the summary counted,
  read as UTC+8 and plus one second (the end is inclusive), makes the window a
  monthly limit. The daily slice package (`…001…`, `CapacityType` 4) refills
  every midnight and is skipped. A failed listing leaves the credits as a
  balance with no reset; it never drops the balance.
- **Enterprise accounts** have no packages in the summary; their credits are
  `get-enterprise-user-usage` (`limitNum`, `credit`, `cycleStartTime`,
  `cycleResetTime`). A personal account's refusal of it is ignored.

## Not done (follow-ups)

- **A run against a real account** (above).
- **Conversation ids** (`X-Conversation-ID`, `X-Conversation-Request-ID`, …):
  WorkBuddy counts one user turn as one request by them; without them each
  step of a tool loop is counted separately.
- Enterprise (team) accounts: the desktop client sends `X-Enterprise-Id` /
  `X-Tenant-Id` (the account's `enterpriseId`), without which WorkBuddy
  answers as the personal account. Dedicated / self-hosted domains
  (`*.copilot.qq.com`) are recorded but requests still go to the public origin.
- Importing the desktop app's existing login state; API-key CodeBuddy.
