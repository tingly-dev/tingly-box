# Re-OAuth In Place

Recovery flow for an OAuth credential that has gone permanently invalid (refresh
token revoked, account re-consent required). Re-runs the interactive OAuth
authorization and **overwrites the credential on the existing provider** instead of
creating a new one — preserving the provider UUID and every config that references it.

## Why

A provider's UUID is its identity. Load-balancing services inside rules, smart-routing
services, `CurrentServiceID`, the vision-proxy service, advisor config, and cached models
all reference a provider by UUID. Deleting a provider runs
`removeProviderServicesFromRules` (`internal/server/config/provider.go`), which strips
those services — so the old "delete + recreate" recovery silently dismantles the user's
routing setup and mints a fresh, unreferenced UUID.

`/api/v1/oauth/refresh` already overwrites credentials in place, but it needs a still-valid
refresh token. When OAuth is fully dead, only a fresh interactive flow can recover — and
that flow must write back onto the same UUID.

## How

A target UUID rides the existing OAuth session lifecycle; the terminal step updates
instead of creates.

```
authorize(provider_uuid) ──▶ SessionState.TargetProviderUUID
        │                              │
        ▼                              ▼
  validate up front            createProviderFromToken
  (exists, OAuth, issuer        ├─ empty  → AddProvider (new UUID)
   matches)                     └─ set    → UpdateProvider (same UUID)
```

- **Request** — `OAuthAuthorizeRequest.provider_uuid` (optional). When set, re-auth.
- **AuthorizeOAuth** validates up front: target must exist, be an OAuth provider, and its
  issuer must equal the requested provider type (can't re-auth a Claude provider with a
  Codex login). Stored on `SessionState.TargetProviderUUID`.
- **createProviderFromToken** — the single terminal path for both the authorization-code
  callback and device-code polling. With a target set: re-validate issuer, overwrite only
  the credential surface (`OAuthDetail`, re-enable), preserve UUID / name / endpoints /
  endpoint-mode, honor a proxy chosen during re-auth, then `UpdateProvider`. Model list is
  re-fetched. Returns the same UUID; session completes exactly as the create path.

Both flows are covered with no extra wiring because they already funnel through
`createProviderFromToken`.

## UX

The OAuth provider table already scaffolded a **Reauthorize** overflow action
(`OAuthTable.tsx`), warning-colored when the token is expired. It is wired in
`CredentialPage` to open the OAuth dialog in direct mode for the provider's issuer, passing
`reauthProviderUuid`. The dialog shows an info banner — *the provider keeps its existing
name and UUID, so all routing rules and model keys stay intact* — and labels the action
"Reauthorize". This satisfies the ux-principles "'done' ≠ locked", "surface the next
action", and "scope side effects to the current surface": an expired credential is
recoverable in place, beside the related Refresh Token action, touching only that one
provider.

## Out of scope

- Auto-triggering re-auth from a request-time 401 (future: surface a CTA).
- Changing a provider's issuer/type via re-auth — explicitly rejected.

## Key files

| File | Role |
|---|---|
| `internal/server/module/oauth/types.go` | `OAuthAuthorizeRequest.provider_uuid` |
| `ai/oauth/manager.go` | `SessionState.TargetProviderUUID` |
| `internal/server/module/oauth/handler.go` | `AuthorizeOAuth` validation; `createProviderFromToken` update-in-place branch |
| `frontend/src/components/OAuthDialog.tsx` | `reauthProviderUuid` prop + re-auth copy |
| `frontend/src/pages/CredentialPage.tsx` | `handleReauthorize` wiring |
| `frontend/src/services/api.ts` | `oauthAuthorize` accepts `provider_uuid` |
