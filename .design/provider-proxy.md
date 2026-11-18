# Per-credential proxy switch

A credential (provider) can route its outbound requests through an HTTP(S)/SOCKS5 proxy (`proxy_url`). The credential list exposes a one-click on/off switch for it.

## Model

- `ai.Provider.ProxyURL` is the stored URL. `ai.Provider.ProxyDisabled` (`proxy_disabled`, DB column `proxy_disabled`) turns it off **without forgetting the URL**, so the switch is lossless.
- The zero value of `ProxyDisabled` is "on", so every existing credential keeps its proxy after upgrade; no data migration (GORM adds the column).
- Outbound code must read `Provider.EffectiveProxyURL()` (empty when off), never `ProxyURL`. That covers the request clients (OpenAI/Anthropic/Google/Gemini/Antigravity/OpenCode transport, model listing), every quota fetcher, OAuth token refresh (worker and handler), and the request log's proxy field. Reads that show configuration (API responses, edit forms, CLI) still use `ProxyURL`.
- "Off" means the same as a credential with no proxy: direct, unless the server-wide `respect_env_proxy` opts into the environment proxy.

## API

- `POST /api/v2/providers/{uuid}/proxy` `{enabled, proxy_url?}`. Enabling needs a stored URL (or one in the body, validated as http/https/socks5); disabling never needs one. Builtin and virtual-model providers are rejected.
- It saves through `Config.UpdateProvider`, so the provider-update hooks run and the client pool drops that provider's pooled clients and transports; the next request dials the new route with no restart.
- `PUT /providers/{uuid}` accepts `proxy_disabled`. Without it, sending a *changed* non-empty `proxy_url` switches the proxy on, while re-sending the unchanged URL (what the edit form does on every save) leaves a switched-off proxy off.
- No bulk endpoint: updates are per row with no cross-row transaction, and "turn on for all" has no meaning for credentials without a stored URL. Add one only with per-item results.

## UI

- `ProxySwitch` (credential list: table, list and card views). Route icon = in use, route-off icon = stored but off, faint route = nothing stored (click opens the editor). It is a `role="switch"`, 40px touch target on phones, state also carried by icon shape and tooltip, not colour alone. Tooltip URLs hide proxy credentials.
- `useProviderProxySwitch` flips the row optimistically, ignores clicks while that row's request is in flight, reconciles with the server's answer, and on failure restores the previous value and toasts the error.
- Edit dialogs (API key and OAuth) show a "Use this proxy" switch under the URL; typing or applying a URL switches it on. Both views read the same server field, so they agree after any change.

## Not detected

There is no proxy reachability probe, so the list cannot say "configured but failing". A failed request still logs the proxy it used (`direct` or `scheme://host`).
