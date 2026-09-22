# ZCode OAuth (GLM Coding Plan)

How Tingly-Box signs a user into a **GLM Coding Plan** through the ZCode login
(BigModel / 智谱 in China, Z.ai internationally), turns that login into a
provider, and serves the plan to Claude Code, Codex and any OpenAI / Anthropic /
Responses client. Generic OAuth mechanics (sessions, re-auth, refresher) are in
`oauth.md`; this file covers only what ZCode does differently.

## Why this exists

A coding plan is a subscription, not an API key. The official way to use it is
the ZCode desktop client; the plan's API key exists (`api_key.secret`, visible in
the plan console) but nothing in the sign-in hands it to a third-party gateway.
This flow gets it the same way the desktop client does, so a user can **log in
once and then point every client at Tingly-Box**.

Protocol was cross-checked against three sources that agree byte-for-byte:
the ZCode desktop client (3.12.3 / 3.14.0), TriDefender/zcode-api
(`src/auth/oauth.ts`, `src/auth/resolver.ts`) and router-for-me/CLIProxyAPI
PR #5993 (`internal/auth/zcode`).

## What ZCode's "OAuth" actually is

Two stages, neither of which is OAuth 2.0.

### 1. Server-mediated login (`OAuthMethodServerPoll`)

```
POST zcode.z.ai/api/v1/oauth/cli/init   {"provider":"bigmodel"|"zai"}
     Authorization: Bearer <32 random bytes, hex — minted by us>
  → {flow_id, authorize_url, expires_at, poll_interval_sec}

open authorize_url + interstitial          (browser, ANY device)
     ?redirect=…    (bigmodel)  |  ?redirect_uri=…  (zai)
     = https://zcode.z.ai/app/oauth/login?redirect=zcode://oauth/callback&app_version=3.14.0

GET  zcode.z.ai/api/v1/oauth/cli/poll/{flow_id}   (same Bearer)
  → {status:"pending"} … {status:"ready", token:<plan jwt>, user:{user_id},
                          bigmodel|zai:{access_token}}
```

- **No client id, no secret, no redirect of ours.** zcode.z.ai owns the callback;
  the interstitial records the authorization server-side (that is what flips the
  poll to `ready`) before bouncing the browser to the client's custom scheme.
  Building a direct authorize URL with a localhost redirect is rejected upstream
  ("Redirect URI not registered for this client"), so there is no auth-code
  variant to fall back to.
- Because nothing returns to this machine, the URL can be opened on a phone —
  a headless server logs in without port forwarding.
- Poll error semantics mirror the desktop client: a 4xx (other than 408/429) or
  an envelope `code != 0` is fatal; a transport error, 5xx, or a non-envelope
  body is one more `pending` round. The server's `poll_interval_sec` is clamped
  to [1s, 30s].

### 2. Credential resolution (the part that makes it usable)

The `access_token` above is an **account token for the business API**, not a
key the model endpoints accept. The plan's real credential is resolved from it:

| step | BigModel (`bigmodel.cn`) | Z.ai (`api.z.ai`) |
|---|---|---|
| authorization | account token, bare | `POST /api/auth/z/login {token}` → `Bearer <biz token>` |
| `GET /api/biz/customer/getCustomerInfo` | default organization ("默认机构") → default project ("默认项目"), else first of each | same |
| `GET/POST …/api_keys` | reuse the key named `zcode-api-key`, else create it | same |
| `GET …/api_keys/copy/{key}` | `secretKey` — tolerated if unreadable (bare key, like the desktop client) | required; a missing secret fails the login |
| credential | `api_key.secret` | `api_key.secret` |

That credential is **static**: no refresh token, no expiry. It is the plan's API
key and works on both plan endpoints.

## How it lands in Tingly-Box

```
AuthorizeOAuth ─── OAuthMethodServerPoll ──▶ InitiateZCodeFlow   (init + interstitial)
      │                                            │
      ▼ auth_url + session_id                      ▼ go pollForZCodeToken
frontend opens URL, polls /oauth/status     CompleteZCodeFlow = Poll + ResolveCredential
                                                   │  → oauth.Token{AccessToken: api_key.secret,
                                                   │                RefreshToken: account token}
                                                   ▼
                                          createProviderFromToken   (create or re-auth, unchanged)
```

- **Issuers**: `zcode_cn` (BigModel) and `zcode` (Z.ai). Two issuers, not one
  with a platform switch: they are different accounts on different hosts, and
  re-auth's issuer check must never let one overwrite the other
  (`ux-principles.md` §2, §3).
- **The token is shaped as an ordinary `oauth.Token`** so the terminal step,
  re-authentication and the session lifecycle need no ZCode branch:
  - `AccessToken` = `api_key.secret`. `GetAccessToken()` returns it, so the
    Anthropic client sends it as `x-api-key` and the OpenAI client as
    `Authorization: Bearer` — exactly how a hand-pasted plan key is sent today.
  - `RefreshToken` = the account token. ZCode has no refresh grant; the field
    holds the thing a new credential can be derived *from*.
  - `Expiry` zero → `ExpiresAt = ""` → the background refresher skips it (there
    is nothing to refresh).
  - `Metadata` → `ExtraFields`: `zcode_platform`, `zcode_api_key` (key id, no
    secret — what the UI may show), `zcode_user_id`, `zcode_plan_jwt`.
- **Refresh = re-resolve.** `POST /oauth/refresh` on a ZCode provider re-runs
  stage 2 from the stored account token (`ReResolveZCodeCredential`): a rotated
  or re-created plan key is picked up without a browser round trip. When the
  account token itself has expired the error reaches the "Token refresh failed"
  dialog, which offers Reauthorize — the same recovery path as every issuer.
- **Provider name** defaults to the plan ("BigModel Coding Plan" / "Z.ai Coding
  Plan"): ZCode returns no email to name the provider after, and the account id
  is deliberately not shown.

## Dual endpoints: both protocols, natively

The plan is served behind two protocols on the same host, and the resolved key
is accepted on both:

| | BigModel | Z.ai |
|---|---|---|
| Anthropic (`APIBase`, primary) | `https://open.bigmodel.cn/api/anthropic` | `https://api.z.ai/api/anthropic` |
| OpenAI (`APIBaseOpenAI`) | `https://open.bigmodel.cn/api/coding/paas/v4` | `https://api.z.ai/api/coding/paas/v4` |

So the provider is created **dual** (`dual-provider.md`): Claude Code reaches
the plan on `/api/anthropic`, Codex / OpenAI-compatible clients on
`/api/coding/paas/v4`, and Responses requests are downgraded per
`openai-endpoint-routing.md` — no protocol translation in either direction.

Dual mode was `api_key`-only because an OAuth bearer is scoped to one endpoint.
ZCode is the sanctioned exception: `Provider.dualEligible()` admits OAuth
providers whose issuer passes `ai.IssuerSupportsDual` (only the two ZCode
issuers). `IsDual`, `ResolveEndpoint` and the provider PATCH validation all go
through it; every other OAuth issuer is unchanged. `ClientPool.GetOpenAIClient`
maps the ZCode issuers to the generic bearer client, since that is all their
OpenAI endpoint needs.

## Quota

`ai/quota` infers the fetcher from the API host. `open.bigmodel.cn` already
mapped to the GLM fetcher; `api.z.ai` did not map at all (only the older
`zai.app` alias did), so a Z.ai plan had no quota panel even with a pasted key.
`z.ai` is now matched. Both fetchers take the plan key as a bearer — the same
value `GetAccessToken()` returns for a ZCode provider — so the plan's windows
show up with no ZCode-specific code, and the hourly `quotawindow` nudge keeps
them moving like any other OAuth provider.

## Rollout

BigModel (China) ships first: its card is enabled in Connect AI, the Z.ai card
is present but `enabled: false` in `OAuthDialog.tsx` until the international
flow has been verified against a live account. The backend and CLI accept both
issuers already — enabling Z.ai is that one flag.

## Not mirrored (on purpose)

- **ZCode client identity headers / request signing.** The desktop client sends
  `X-ZCode-*` identity headers and, on some paths, a signed request. The plan
  endpoints accept the plain key without them — that is how every pasted-key
  user reaches them today — and Tingly-Box has no provider-level header layer
  by design (`user-agent.md`). Revisit only if the plan endpoints start gating
  on client identity.
- **Start-plan / trial plans** (`zcode.z.ai/api/v1/zcode-plan/...`, JWT auth),
  the off-peak `/async/*` channel and trial-plan auto-claim. The plan JWT is
  stored (`zcode_plan_jwt`) so a future start-plan path needs no new login.
- **Endpoint routing** (`zcode.z.ai/api/v1/agent/configs` "ultra gateway"
  mapping). Unverified against an entitled account; the pinned plan endpoint is
  always used.

## Key files

| File | Role |
|---|---|
| `ai/oauth/zcode.go` | `ZCodeClient` (init / poll / resolve), `Manager.InitiateZCodeFlow`, `CompleteZCodeFlow`, `ReResolveZCodeCredential` |
| `ai/oauth/zcode_test.go` | protocol tests against an httptest server (both platforms) |
| `ai/oauth/registry.go` | `zcode` / `zcode_cn` entries, `OAuthMethodServerPoll` |
| `ai/issuer.go` | issuers, `ZCodeEndpoints` |
| `ai/provider.go` | `dualEligible` / `IssuerSupportsDual` |
| `internal/server/module/oauth/handler.go` | server-poll branch of `AuthorizeOAuth`, `pollForZCodeToken`, refresh-as-re-resolve, dual endpoints on create |
| `internal/command/oauth.go` | CLI `runServerPollFlow` |
| `internal/client/pool.go` | OpenAI client for the ZCode issuers |
| `ai/quota/manager.go` | `z.ai` host → Zai fetcher |
| `frontend/src/components/OAuthDialog.tsx` | Connect AI cards |
