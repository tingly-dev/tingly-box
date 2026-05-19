# Security: Design Decisions

> Audience: tingly-box backend contributors. This document records
> security-relevant policy decisions, one section per decision.

---

## Random default authentication tokens

### Problem

tingly-box gates its Web UI and control-plane API behind a `UserToken`,
and its model proxy behind a `ModelToken`. Historically both had
well-known compile-time defaults — `tingly-box-user-token` and
`tingly-box-model-token` — defined in `internal/constant/constant.go`.

A fresh install would write the literal default string into
`config.json` and use it as the operative credential. Because the value
is public (in the source tree, in every release binary), anyone who can
reach the loopback port on a brand-new box can log in as the operator.
For a gateway that proxies developer LLM traffic and stores upstream
provider keys on the operator's behalf, that is the wrong default.

An earlier mitigation added secure-random generation
(`config.GenerateUserToken`, 32 bytes from `crypto/rand`, hex encoded,
`tb-user-` prefix → 256 bits of entropy) and a startup warning when the
default string was detected — but **three code paths still wrote the
well-known constant**:

1. `NewConfig`: on `crypto/rand` failure, fell back to `DefaultUserToken`.
2. `NewConfig`: on JWT-generation failure for the ModelToken, fell back
   to `DefaultModelToken` (a separate bug then immediately overwrote
   that with an empty value, corrupting rather than weakening the
   config).
3. `CreateDefaultConfig` (the actual first-run path most installs took):
   unconditionally assigned `c.UserToken = constant.DefaultUserToken`
   without ever calling the random generator.

### Policy

A configuration that does not already contain a `user_token` or
`model_token` MUST receive a fresh cryptographically random value before
it is persisted. There is no "safe default" fallback string.

If `crypto/rand.Read` fails, bootstrap returns an error and the process
exits. Booting with a known-weak token is a worse failure mode than
refusing to boot.

| Path                                            | UserToken                                 | ModelToken                                  | On generator error |
|-------------------------------------------------|-------------------------------------------|---------------------------------------------|--------------------|
| `NewConfig` — no token on an existing config    | `GenerateUserToken()` → `tb-user-<64 hex>` | JWT signed with `JWTSecret`                 | Return error       |
| `CreateDefaultConfig` — true first run          | `GenerateUserToken()` → `tb-user-<64 hex>` | `"tingly-box-" + JWT` (legacy prefix kept)  | Return error       |

### Legacy installs

The two `Default*Token` constants are retained in
`internal/constant/constant.go`, but their **only** remaining purpose is
detection. On startup, if `cfg.UserToken == constant.DefaultUserToken`
we emit a multi-line `SECURITY WARNING` pointing the operator at the
rotation flows:

1. Web UI → System page → Access Control → "Reset user token".
2. CLI: `tingly-box auth token --reset` (planned).

We deliberately do **not** silently rotate at startup:

- The operator may have automation, bookmarks, or other clients pinned
  to the existing value. Rotating without consent would lock them out
  of their own box on an unrelated upgrade.
- A persistent warning is more likely to drive rotation to completion
  than a one-shot silent rewrite the operator never sees.

Rotation is already wired server-side in
`internal/server/webui_auth.go` (`ResetUserToken`, `ResetModelToken`);
both routes require an authenticated session, so a legacy-default
holder can rotate from inside the Web UI without any out-of-band trust
bootstrap.

### Why keep the default constants at all

Deleting them would silently break:

- The detection branch in `NewConfig` and the `is_default` flag exposed
  by `GetUserToken` to the Web UI.
- The "real token vs test token" assertion in
  `internal/server_test/server_test.go`.

The right read of these constants today is *"the string we look for in
older configs"*, not *"the value we hand out to new configs"*. Renaming
to make that clearer is a follow-up.

### Out of scope (follow-ups)

- `JWTSecret` is currently `fmt.Sprintf("%d", time.Now().UnixNano())` —
  64 bits of guessable timing data, not a secret. Should be upgraded
  to 32 bytes from `crypto/rand`.
- No rate-limiting on the Web UI login endpoint. Brute-force protection
  rests entirely on token entropy. Acceptable at 256 bits; if we ever
  shorten the token, rate-limiting must come in first.

### Files involved

| File                                       | Role                                                                                  |
|--------------------------------------------|---------------------------------------------------------------------------------------|
| `internal/constant/constant.go`            | Holds `DefaultUserToken` / `DefaultModelToken` — detection-only, not fallback.        |
| `internal/server/config/util.go`           | `GenerateUserToken`, `GenerateModelToken`, `GenerateSecureToken`, `IsDefaultToken`.   |
| `internal/server/config/config.go`         | `NewConfig` bootstrap and `CreateDefaultConfig`. Both refuse legacy defaults.         |
| `internal/server/webui_auth.go`            | `GetUserToken` (exposes `is_default`), `ResetUserToken`, `ResetModelToken`.           |
| `internal/server_test/server_test.go`      | Distinguishes real vs test tokens via the legacy default string.                      |
