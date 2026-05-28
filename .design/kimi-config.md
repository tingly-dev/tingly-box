# Kimi Code OAuth & Client Round Trip: Design and Decisions

> Audience: tingly-box contributors touching the Kimi Code provider path,
> or anyone adding a new CLI-impersonation OAuth provider.
> This document records the OAuth device-code flow, the `kimiRoundTripper`
> impersonation headers, request body normalization, and how device ID is
> bound to a credential — verified against the
> [kimi-cli](https://github.com/MoonshotAI/kimi-cli) reference implementation.

---

## 1. Background

Kimi Code (`kimi.com/code`) is Moonshot AI's coding assistant. It does not
expose a plain API-key endpoint to third parties; access requires a user
account authenticated via OAuth. The OAuth flow follows
**RFC 8628 Device Authorization Grant** (device code flow): the user visits a
verification URL on their phone/browser, and the server polls until the user
approves.

After authentication, all inference requests must carry a set of
`X-Msh-*` headers that identify the client as `kimi_cli` — the same headers
kimi-cli sends. Without them the API rejects the request.

The backend target for Kimi Code OAuth tokens is `https://api.kimi.com/coding/v1`,
**not** the standard Moonshot API (`api.moonshot.cn/v1`). These are separate
products with separate auth systems.

---

## 2. OAuth device-code flow

### 2.1 Provider configuration

```go
// ai/oauth/provider.go
registry.Register(&ProviderConfig{
    Type:               ai.IssuerKimiCode,
    GrantType:          "urn:ietf:params:oauth:grant-type:device_code",
    ClientID:           "17e5f671-d194-4dfb-9706-5516cb48c098",
    DeviceCodeURL:      "https://auth.kimi.com/api/oauth/device_authorization",
    TokenURL:           "https://auth.kimi.com/api/oauth/token",
    Scopes:             nil,            // kimi-cli sends no scope parameter
    AuthStyle:          AuthStyleInNone,
    OAuthMethod:        OAuthMethodDeviceCode,
    TokenRequestFormat: TokenRequestFormatForm,
})
```

Key points:
- **No client secret** — public device-code client, per kimi-cli.
- **No scopes** — kimi-cli does not send a `scope` parameter; sending one
  causes the device_authorization request to fail.
- **Form encoding** — token requests use `application/x-www-form-urlencoded`,
  not JSON.
- **Client ID verified** against `KIMI_CODE_CLIENT_ID` in kimi-cli's
  `auth/oauth.py`.

### 2.2 `KimiHook` — per-request headers on auth calls

Every token-related HTTP call (device authorization, token polling, refresh)
goes through `KimiHook.BeforeToken`, which sets the same fingerprint headers
that kimi-cli sends:

```go
// ai/oauth/hook.go
func (h *KimiHook) BeforeToken(body map[string]string, header http.Header) error {
    header.Set("X-Msh-Platform",   "kimi_cli")
    header.Set("X-Msh-Version",    "1.10.6")
    header.Set("X-Msh-Device-Name", KimiDeviceName())   // os.Hostname()
    header.Set("X-Msh-Device-Model", KimiDeviceModel()) // "macOS arm64"
    header.Set("X-Msh-Os-Version",  KimiOsVersion())    // see §4
    return nil
}
```

`X-Msh-Device-Id` is **not** set here — see §3 for why.

### 2.3 Device ID binding per credential

kimi-cli generates one UUID on first launch and stores it at
`~/.kimi-cli/share/device_id`, reusing the same ID for all sessions and all
API calls.

tingly-box takes a different approach: a fresh UUID is generated **per OAuth
flow** and stored in `OAuthDetail.DeviceID`. Every subsequent refresh and
inference request for that credential reuses the same ID. Multiple
credentials therefore appear as different devices to Kimi — which is
functionally correct (each credential IS a separate account/session).

The device ID is injected via `oauth.WithExtraHeader`:

```go
// internal/server/module/oauth/handler.go
if issuer == ai.IssuerKimiCode {
    kimiDeviceID = uuid.New().String()
    deviceOpts = append(deviceOpts, WithKimiDeviceID(kimiDeviceID))
}
```

`WithKimiDeviceID` wraps `oauth.WithExtraHeader("X-Msh-Device-Id", id)`, which
the OAuth manager applies to every token request via `applyExtraHeaders`.
After the flow succeeds, the device ID is persisted in `OAuthDetail.DeviceID`
and reattached on refresh (`oauth_refresher.go`, `handler.go:RefreshOAuthToken`).

### 2.4 Polling and refresh

Token polling and refresh both go through `oauth.Manager.PollForToken` /
`Manager.RefreshToken`. Both paths call `BeforeToken` (including the
`X-Msh-Device-Id` extra header), so the device ID is consistent across the
entire credential lifetime.

---

## 3. Client round tripper — inference impersonation

Every inference request to `https://api.kimi.com/coding/v1` is wrapped by
`kimiRoundTripper`, which adds the CLI fingerprint headers and normalizes
the request body:

```go
// internal/client/kimi_round_tripper.go
const (
    kimiCLIUserAgent = "KimiCLI/1.10.6"
    kimiCLIPlatform  = "kimi_cli"
    kimiCLIVersion   = "1.10.6"
)

func (t *kimiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("User-Agent",        kimiCLIUserAgent)
    req.Header.Set("X-Msh-Platform",    kimiCLIPlatform)
    req.Header.Set("X-Msh-Version",     kimiCLIVersion)
    req.Header.Set("X-Msh-Device-Name", t.deviceName)
    req.Header.Set("X-Msh-Device-Model", t.deviceModel)
    req.Header.Set("X-Msh-Os-Version",  t.osVersion)
    if t.deviceID != "" {
        req.Header.Set("X-Msh-Device-Id", t.deviceID)
    }
    // body normalization (see §5)
    ...
}
```

The `deviceName`, `deviceModel`, `osVersion` are resolved once at construction
(no per-request syscall). `deviceID` comes from `provider.OAuthDetail.DeviceID`.

**Header consistency**: `KimiHook` (auth) and `kimiRoundTripper` (inference)
must send the same values. The shared `oauth.KimiDeviceName()`,
`KimiDeviceModel()`, `KimiOsVersion()` helpers in `ai/oauth/hook.go` ensure
both paths use identical logic.

---

## 4. OS version headers

`X-Msh-Os-Version` is set to a hardcoded representative value per platform,
matching the style of `platform.version()` output from kimi-cli:

```go
// ai/oauth/hook.go
func KimiOsVersion() string {
    switch runtime.GOOS {
    case "darwin":
        return "14.6.1"     // macOS Sonoma latest stable
    case "windows":
        return "10.0.22631" // Windows 11 23H2
    default:                // linux and others
        return "6.8.0"      // Ubuntu 24.04 LTS kernel
    }
}
```

Hardcoded rather than dynamic (`/proc/sys/kernel/osrelease`, `sw_vers`) to
avoid I/O on every request and to stay consistent regardless of the host the
server runs on.

---

## 5. Request body normalization

kimi-cli is a Python CLI client; it does no body rewriting. tingly-box acts as
a proxy, so it must normalize OpenAI-style payloads before forwarding. The
normalization logic is based on the CLIProxyAPI Go reference:

### 5.1 Model name prefix stripping

Kimi's API does not accept the `"kimi-"` prefix:

```
"kimi-k2"  →  "k2"
"kimi-K2"  →  "K2"    (prefix check is case-insensitive, remainder preserved)
"k2"       →  "k2"    (no-op)
```

### 5.2 Empty assistant message filtering

Assistant messages with no content, no tool calls, no function call, and no
`reasoning_content` are dropped. These arise from multi-turn traces where a
turn produced only a tool call and the assistant content field was left empty.
Kimi's API rejects such messages.

### 5.3 Tool message normalization

Two fixups applied to every `messages` array:

1. **`call_id` → `tool_call_id`**: Some clients send `call_id` instead of the
   OpenAI-standard `tool_call_id`. When `tool_call_id` is absent but `call_id`
   is present, `tool_call_id` is added (both fields coexist; no data removed).

2. **Infer missing `tool_call_id`**: If a `tool` message has no ID at all and
   exactly one assistant tool call is still pending, the pending call's ID is
   used. Ambiguous cases (multiple pending) are left untouched.

### 5.4 `reasoning_content` on tool-calling assistant messages

Kimi requires `reasoning_content` on every assistant message that has
`tool_calls`. When the field is absent or empty:

- If a prior assistant message in the same conversation had non-empty
  `reasoning_content`, that value is reused (carries the last known reasoning
  forward).
- Otherwise, the content text of the current message is used as a fallback.
- If nothing is available, an empty string `""` is written.

No sentinel string is injected — empty string signals "no reasoning available"
without introducing a non-standard marker.

---

## 6. API base and style

```go
case ai.IssuerKimiCode:
    apiBase  = "https://api.kimi.com/coding/v1"
    apiStyle = protocol.APIStyleOpenAI
```

Kimi Code exposes an OpenAI-compatible chat completions endpoint, so no
protocol translation is required. The `kimiRoundTripper` layers on top of the
standard OpenAI SDK transport.

---

## 7. Reference verification

All constants and endpoint URLs were verified against
`src/kimi_cli/auth/oauth.py` and `src/kimi_cli/auth/platforms.py` in the
[MoonshotAI/kimi-cli](https://github.com/MoonshotAI/kimi-cli) repository:

| Item | kimi-cli | tingly-box |
|------|----------|-----------|
| Client ID | `17e5f671-d194-4dfb-9706-5516cb48c098` | same |
| Device auth URL | `https://auth.kimi.com/api/oauth/device_authorization` | same |
| Token URL | `https://auth.kimi.com/api/oauth/token` | same |
| Grant type | `urn:ietf:params:oauth:grant-type:device_code` | same |
| API base | `https://api.kimi.com/coding/v1` | same |
| Scopes | none (no `scope` param) | `nil` |
| `X-Msh-Platform` | `"kimi_cli"` | `"kimi_cli"` |
| `X-Msh-Version` | `VERSION` (≈ `1.10.6`) | `"1.10.6"` |
| `X-Msh-Device-Name` | `os.hostname()` | `os.Hostname()` |
| `X-Msh-Device-Model` | `"<OS> <arch>"` | `KimiDeviceModel()` |
| `X-Msh-Os-Version` | `platform.version()` | hardcoded per platform |
| `X-Msh-Device-Id` | persistent per-device UUID | persistent per-credential UUID |
| Token format | form-encoded | `TokenRequestFormatForm` |

---

## 8. Key files

| Layer | File | Role |
|---|---|---|
| OAuth config | `ai/oauth/provider.go` | `IssuerKimiCode` provider registration |
| OAuth hook | `ai/oauth/hook.go` | `KimiHook`, `KimiDeviceName`, `KimiDeviceModel`, `KimiOsVersion` |
| OAuth helper | `internal/server/module/oauth/kimi.go` | `WithKimiDeviceID` — injects `X-Msh-Device-Id` into token requests |
| Auth handler | `internal/server/module/oauth/handler.go` | device ID generation, `pollForDeviceCodeToken`, `createProviderFromToken` |
| Background refresh | `internal/server/background/oauth_refresher.go` | reattaches device ID on token refresh |
| Round tripper | `internal/client/kimi_round_tripper.go` | inference headers + body normalization |
| Round tripper test | `internal/client/kimi_round_tripper_test.go` | normalization unit tests |
