# ai — Provider Types

The `ai` package defines the core `Provider` type used throughout tingly-box.
A provider is the unit of AI access: it names an upstream (or in-process) service,
records how to authenticate against it, and carries any protocol metadata needed
by the dispatcher.

Three `AuthType` values are in production use:

| `auth_type` | Constant | Upstream | Credential on wire |
|---|---|---|---|
| `api_key` | `AuthTypeAPIKey` | Any OpenAI- or Anthropic-compatible HTTP API | `Authorization: Bearer <token>` |
| `oauth` | `AuthTypeOAuth` | Issuer-specific (Claude Code, Codex, Copilot, …) | Short-lived OAuth access token, auto-refreshed |
| `vmodel` | `AuthTypeVirtual` | None — served in-process by the `vmodel` package | — |

---

## `api_key` providers

Standard key-based access. The user supplies a base URL and an API key;
`Token` is forwarded verbatim as the `Bearer` credential on every request.

```
Provider {
    APIBase:  "https://api.openai.com/v1"
    APIStyle: "openai"
    AuthType: "api_key"
    Token:    "sk-..."
}
```

`APIStyle` controls request/response translation: `openai` (default) or
`anthropic`.  For providers that speak both natively, the optional dual
fields `APIBaseOpenAI` / `APIBaseAnthropic` let the dispatcher pick the
matching endpoint without protocol conversion.  See
[`.design/dual-provider.md`](../.design/dual-provider.md) for the full
design, dispatch logic, and edit-mode rules.

---

## `oauth` providers

OAuth providers carry a short-lived `AccessToken` inside `OAuthDetail`.
The `Issuer` field (e.g. `claude_code`, `codex`, `copilot`, `gemini`, …)
selects the token-manager that knows how to refresh the credential before
it expires.

```
Provider {
    APIBase:  "https://api.anthropic.com"
    APIStyle: "anthropic"
    AuthType: "oauth"
    OAuthDetail: {
        Issuer:       "claude_code"
        AccessToken:  "sk-ant-oat01-..."
        RefreshToken: "..."
        ExpiresAt:    "2025-06-01T12:00:00Z"
    }
}
```

The dispatcher detects OAuth expiry before forwarding and silently refreshes
the token. `IsOAuthToken()` provides a runtime check based on the
`sk-ant-oat` prefix, independent of the `AuthType` field.

---

## `vmodel` providers

Virtual-model providers are backed entirely in-process by the `vmodel`
package — no outbound HTTP is ever made. The dispatcher short-circuits to
the in-process handler immediately after rule resolution, so all middleware
(guardrails, recording, usage tracking) still executes normally.

```
Provider {
    UUID:     "vmodel-builtin-anthropic"
    APIBase:  "vmodel://local"        // sentinel; never dialed
    APIStyle: "anthropic"
    AuthType: "vmodel"
    Source:   "builtin"
    VModelDetail: {
        Models: ["claude-instant", "claude-echo", ...]
    }
    // Token is implicitly "EMPTY" — see GetAccessToken
}
```

### Credential sentinel (`VModelSentinelToken`)

The Anthropic and OpenAI SDKs install a lazy credential check that fires
**at request time**: if `APIKey` is empty the SDK returns `ErrNoCredentials`
before the HTTP call. Because vmodel requests are short-circuited to the
in-process handler (see below) no real HTTP call is ever made, but the SDK
client may still be constructed on code paths shared with real providers.

To satisfy the check without requiring every vmodel provider to carry a real
token, `GetAccessToken()` returns the sentinel string `"EMPTY"` for all
`AuthTypeVirtual` providers. The sentinel is exported as `VModelSentinelToken`
and is never transmitted to a real upstream.

### How the dispatcher routes a vmodel request

1. The routing selector resolves the rule to a `(provider, model)` pair as
   usual.
2. `provider.IsVirtual()` is true → the dispatcher rewrites the request body
   with the resolved model ID and calls the in-process handler directly
   (`virtualModelService.GetHandler().ChatCompletions(c)` or `.Messages(c)`).
3. The vmodel handler looks up the model ID in its per-protocol registry and
   streams a response.

### In-process endpoints

Two independent route groups are mounted at startup:

| Prefix | Protocol | Endpoints |
|---|---|---|
| `/virtual/openai/v1` | OpenAI Chat | `GET /models`, `POST /chat/completions` |
| `/virtual/anthropic/v1` | Anthropic Messages | `GET /models`, `POST /messages` |

These can be used directly (e.g. for local testing with an OpenAI SDK pointed
at `http://localhost:12580/virtual/openai/v1`).

### Model list

The model list for a vmodel provider lives on the provider record itself
(`VModelDetail.Models`), not in the upstream-model cache.  Both
`GET /api/v1/provider-models/{uuid}` and `POST /api/v1/provider-models/{uuid}`
return `VModelDetail.Models` directly, bypassing the normal upstream-fetch
path.

### Probe

`APIBase` is the sentinel `vmodel://local`, which no HTTP client can dial.
The probe resolution layer (`probe_v2_handler.resolveTargetToProviderModel`)
detects `provider.IsVirtual()` after the initial resolve and re-routes
through `resolveProviderConfigTarget` with a synthetic inline config:
`APIBase = http://127.0.0.1:<port>/virtual/anthropic` (Anthropic SDK appends
`/v1`) or `…/virtual/openai/v1` (OpenAI SDK appends nothing), and
`Token = cfg.GetModelToken()`. From there the SDK probe is identical to a
user-supplied provider_config target — round-tripping through HTTP loopback
into the in-process vmodel handler, exercising route, auth middleware,
registry lookup, and streaming response without leaving the process. The
stored provider record is never mutated.

### Builtin providers

Two builtin providers — `vmodel-builtin-anthropic` and `vmodel-builtin-openai`
— are seeded idempotently on every server startup via
`virtualserver.EnsureBuiltinProviders`.  They are created with `Source: "builtin"`,
which makes them undeletable and non-mutable through the API; only their
`Enabled` flag may be toggled by the user.  On subsequent startups the
`Enabled` flag is preserved while the model list is refreshed from the
in-process registry.

---

## `Source` field

Independent of `AuthType`, `Source` records who created the provider:

| `source` | Meaning |
|---|---|
| `user` (default) | Created by a user through the UI or API; can be deleted and mutated freely. |
| `builtin` | Seeded by the server at startup; only `Enabled` may be changed. |

All builtin providers today are also `vmodel` providers, but the two
concepts are separate — a future builtin could carry a real API key.

---

## Model ordering in the UI

When the model-select dialog lists providers, `auth_type` determines sort
order: OAuth first (0), then api_key (1), then vmodel (2).  This keeps
virtual models available but visually de-emphasised, at the end of the list.
