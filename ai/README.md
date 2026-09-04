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

Virtual-model providers are backed by the `vmodel` package running inside the
gateway process, but they are **dispatched exactly like real providers**: the
same official SDK, the same transport chain (rule flags, logging, probe
headers, timeouts). The only vmodel-specific step is the dialer — a provider
whose `APIBase` carries the `vmodel://` scheme is connected to a private
in-memory HTTP listener served by `virtualserver.Serve` instead of the
network. See `.design/vmodel-transport.md`.

```
Provider {
    UUID:     "vmodel-builtin-anthropic"
    APIBase:  "vmodel://anthropic"     // protocol root on the private listener
    APIStyle: "anthropic"
    AuthType: "vmodel"
    Source:   "builtin"
    VModelDetail: {
        Models: ["virtual-claude-3", "echo-model", ...]
    }
    // Token is implicitly "EMPTY" — see GetAccessToken
}
```

### `vmodel://` base URLs

| APIBase | SDK base URL | Serves |
|---|---|---|
| `vmodel://openai` | `http://vmodel.internal/openai/v1` | `/models`, `/chat/completions`, `/responses` |
| `vmodel://anthropic` | `http://vmodel.internal/anthropic/v1` | `/models`, `/messages` |

`virtualserver.APIBase`, `virtualserver.IsAPIBase` and `virtualserver.HTTPBase`
are the helpers — everything vmodel-specific lives in `vmodel/virtualserver`.
`vmodel.internal` never resolves on any network; the transport's dialer
ignores it and connects to the listener directly. Rows that
still carry the legacy `vmodel://local` sentinel fall back to the provider's
`APIStyle` and are rewritten by the builtin seed on the next start.

### Credential sentinel (`VModelSentinelToken`)

The Anthropic and OpenAI SDKs install a lazy credential check that fires
**at request time**: if `APIKey` is empty the SDK returns `ErrNoCredentials`
before the HTTP call. To satisfy it without requiring every vmodel provider to
carry a real token, `GetAccessToken()` returns the sentinel string `"EMPTY"`
for all `AuthTypeVirtual` providers. The private listener ignores
credentials; the sentinel never reaches a network upstream.

### How the dispatcher routes a vmodel request

1. The routing selector resolves the rule to a `(provider, model)` pair as
   usual.
2. `ClientPool` builds the ordinary SDK client for the provider. Because
   `APIBase` is `vmodel://…`, the constructor rewrites the base URL with
   `virtualserver.HTTPBase` and uses `virtualserver.Transport()` as the base of
   the otherwise unchanged transport chain. `server.NewServer` started the
   target with `virtualserver.Serve`.
3. The request travels as real HTTP — status codes, headers, SSE framing —
   to the virtualserver, which looks the model up in its per-protocol
   registry and streams a response.

### Public endpoints

Independently of provider dispatch, two authenticated route groups are
mounted on the main engine for clients that want a fixed URL:

| Prefix | Protocol | Endpoints |
|---|---|---|
| `/virtual/openai/v1` | OpenAI Chat | `GET /models`, `POST /chat/completions`, `POST /responses` |
| `/virtual/anthropic/v1` | Anthropic Messages | `GET /models`, `POST /messages` |

### Model list

The model list for a vmodel provider lives on the provider record itself
(`VModelDetail.Models`), not in the upstream-model cache.  Both
`GET /api/v1/provider-models/{uuid}` and `POST /api/v1/provider-models/{uuid}`
return `VModelDetail.Models` directly, bypassing the normal upstream-fetch
path.

### Probe

Nothing special: a vmodel provider is probed through the same SDK client as
any other provider, so the probe traverses the real dispatch path.

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
