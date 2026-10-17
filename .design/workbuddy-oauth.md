# WorkBuddy OAuth (Tencent WorkBuddy / CodeBuddy)

How Tingly-Box signs a user into **WorkBuddy** (Tencent; CodeBuddy is the same
CN account behind another entry point), turns the login into a provider, keeps
it fresh, and serves it to any OpenAI / Anthropic / Responses client. Generic
OAuth mechanics (sessions, re-auth, refresher) are in `oauth.md`; this file
covers only what WorkBuddy does differently. Proposal: TEP-2097.

## Status of the protocol: unverified

WorkBuddy publishes no API for third-party login. Everything below follows
public third-party integrations that agree on routes and codes
(`elavita/dsh-codearts-auth`, `pilciw/codebuddy2api`, OmniRoute's CodeBuddy CN
gateway). It has **not** been checked against a real round trip from the
official client. The code is written to make that check cheap:

- every value the upstream may police (host, `platform`, `X-Product-Code`,
  `User-Agent`) lives in one table, `ai.WorkBuddySiteFor` / `ai.WorkBuddy*`
  constants (`ai/issuer.go`), read by both login and inference;
- response fields are parsed tolerantly (camelCase and snake_case, expiry as a
  lifetime, unix s/ms, RFC 3339, or the JWT `exp`), so a spelling difference
  does not fail a login.

When a real capture disagrees, fix the table / parser and record it here.

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

Every request carries `X-Domain`, `X-Product: SaaS`, `X-Product-Code` and the
client `User-Agent`.

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

| issuer | origin / `X-Domain` | `platform` | `X-Product-Code` |
|---|---|---|---|
| `workbuddy` | `https://www.workbuddy.ai` | `workbuddy-ai` | `workbuddy` |
| `workbuddy_cn` | `https://copilot.tencent.com` | `ide` | `codebuddy` |

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
  client plus `workbuddyRoundTripper`, which adds the same identification
  headers as the login and `X-User-Id`.
- **Models** come from `GET {origin}/v3/config` (`data.models[].id`), not
  `/models`. There is no embedded catalog entry: model ids are account- and
  region-specific and unverified, so a failed fetch shows an empty list rather
  than invented ids.

## Not done (follow-ups)

- **Verification against the official client** (above). Open points: the
  international chat path, token lifetime, how strictly `User-Agent` /
  `X-Product-Code` are enforced, and the reported streaming-only behaviour of
  chat completions (a non-stream request may need stream-and-aggregate).
- Quota / credits panel.
- Enterprise (dedicated / self-hosted, `*.copilot.qq.com`) accounts: the domain
  is recorded but requests still go to the public origin.
- Importing the desktop app's existing login state; API-key CodeBuddy.
