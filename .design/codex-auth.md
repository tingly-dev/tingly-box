# Codex Auth Modes: gateway, direct ChatGPT, and hybrid

> Audience: contributors touching the Codex "Auto Config" flow
> (`~/.codex/config.toml` + `~/.codex/auth.json`).
> This documents the three ways tingly-box wires Codex authentication —
> **gateway** (`apikey`), **direct ChatGPT** (`chatgpt`), and **hybrid** — with
> the emphasis on why hybrid was added: it lets a user route requests through
> the tingly-box gateway *and* keep a native ChatGPT login in `auth.json`, and
> maps onto two orthogonal UI axes instead of a growing mode picker.

---

## 1. Background — two mutually-exclusive modes, and the conflict

Before this change, Codex apply had two auth modes (`CodexAuthMode`,
`internal/server/config/apply_config.go`), surfaced as a single "Auth source"
radio in `CodexConfigModal`:

| Mode (`authMode`) | `config.toml` | `auth.json` | Requests |
|---|---|---|---|
| gateway (`apikey`) | `model_provider="tingly-box"` + `[model_providers.tingly-box]` + profiles + catalog | `OPENAI_API_KEY = <tingly JWT>` | codex → tingly-box |
| official (`chatgpt`) | tingly keys **cleared** (`ClearCodexGatewayConfig`) | `{ auth_mode:"chatgpt", tokens:{…} }` | codex → OpenAI direct |

Both modes **own `auth.json`**: gateway writes `OPENAI_API_KEY`, official writes
the OAuth `tokens` block and clears the key. Switching one way wipes the other.
This is the exact conflict `cc-switch` documents: Codex App needs the official
login in `auth.json` to identify the account and enable remote control / plugins,
but pointing Codex at a third-party provider historically overwrote it.

**Key realization:** the conflict lives *entirely* in `auth.json`, and our
gateway mode doesn't actually need `auth.json` — Codex sends whatever key it has
for an `apikey` provider as `Authorization: Bearer …`, and tingly-box sees an
identical request whether that key came from `OPENAI_API_KEY` or from a
provider-scoped token in `config.toml`.

## 2. Hybrid mode

Hybrid moves the gateway credential out of `auth.json` and into the provider
stanza, freeing `auth.json` to keep the official login:

```toml
model_provider = "tingly-box"

[model_providers.tingly-box]
name = "OpenAI using Tingly Box"
base_url = "http://127.0.0.1:12580/tingly/codex"
wire_api = "responses"
experimental_bearer_token = "tingly-box-…"   # gateway token, provider-scoped
requires_openai_auth = true                   # provider still uses the OpenAI auth path
```

> Gateway (`apikey`) mode also emits `requires_openai_auth = true` (no bearer
> token) so Codex sources the provider credential from `auth.json`'s
> `OPENAI_API_KEY`. Neither mode writes `preferred_auth_method` — it is **not** in
> Codex's `config-schema.json` (which is `additionalProperties: false`), so
> writing it fails schema validation ("Additional properties are not allowed").

`auth.json` keeps `{ auth_mode:"chatgpt", tokens:{…} }` (or is left untouched).
Result: **Codex App still sees the official account; `codex` requests still
route through tingly-box.** This mirrors `cc-switch`'s "Codex App Enhancements /
keep official login" toggle (v3.16.1).

### `requires_openai_auth = true`

`experimental_bearer_token` is an OpenAI-labeled *experimental* field.
`requires_openai_auth = true` keeps this provider on the OpenAI auth path so the
provider-scoped bearer token is honored. Both keys are written together and only
when a bearer token is supplied.

### auth.json: materialize vs leave untouched

Hybrid takes the OAuth provider UUID **optionally**:

- **UUID supplied** → materialize that stored Codex login into `auth.json`
  (same writer as `chatgpt` mode) — useful to (re)establish a valid login.
- **UUID omitted** → `ApplyCodexAuth` is a no-op; whatever `codex login` already
  wrote survives. This is the smart default (ux-principles #6): most hybrid users
  already have a working login and don't want the file touched.

## 3. Wire shape

```jsonc
// POST /config/apply/codex  and  /config/preview/codex
{
  "preferences": { … },
  "writeCatalog": true,
  "authMode": "hybrid",              // "" | "apikey" | "chatgpt" | "hybrid"
  "oauthProviderUuid": ""            // optional for hybrid; required for chatgpt
}
```

- Apply (hybrid): `config.toml` = full gateway rewrite **with**
  `experimental_bearer_token = <model token>`; `auth.json` = materialize-or-skip.
  Catalog is still written (unlike `chatgpt`).
- Preview (hybrid): `configToml` carries the bearer token; `authJson` is empty
  (no token to show — the UI renders a note instead of leaking OAuth tokens).

## 4. UI — one 3-way select, not two axes

The modal (`CodexConfigModal.tsx`) picks the mode with a **single 3-way radio**:

| option | → authMode | OAuth picker |
|---|---|---|
| Tingly Box gateway | `apikey` | — |
| Tingly Box gateway + keep official ChatGPT login | **`hybrid`** | optional (default *"Keep existing"*) |
| Direct to OpenAI | `chatgpt` | required |

**Why not two axes (routing × keep-login).** That framing was tried first
(ux-principles #4 — split a knob that controls two things). But the two axes
aren't actually orthogonal: *direct routing without keeping the official login*
is an invalid combination, so the 2×2 grid has a dead cell. Representing it with
a routing radio + a "keep login" checkbox forced the checkbox to be
checked-and-disabled in direct mode — a disabled control pinned to a fixed value,
which is the classic smell of N valid states crammed into a grid with a hole.
There are really **three** valid states, so a 3-way select models them honestly
and removes the awkward disabled control. ux-principles #2 (avoid mode pickers)
is only a mild concern here because the Quick Config work surface stays visible
below the selector regardless — this is a property selector, not an entry gate.
The cost — the two gateway options read similarly — is paid down with a concrete
consequence caption under each option (ux-principles #5/#8).

The OAuth provider picker appears for `hybrid` (optional; default *"Keep existing
auth.json (don't modify)"*) and `chatgpt` (required). In the Manual tab, `hybrid`
replaces the `OPENAI_API_KEY` Step 2 with a note (the token lives in
`config.toml`, so there is nothing to write to `auth.json`).

## 5. Caveats

- `experimental_bearer_token` is OpenAI-discouraged and could change in a future
  Codex release. `env_key` is the stabler alternative but isn't self-contained
  (needs a shell env var), so it wasn't chosen for the one-click flow. Revisit if
  Codex removes the field.
- The tingly JWT is written in plaintext into `config.toml`. It is no more
  exposed than today's `auth.json` `OPENAI_API_KEY`, but `config.toml` is more
  often shared/screenshotted — worth a mention in support.
- If the tingly model token rotates, `config.toml` goes stale and the user must
  re-apply — same as the existing `auth.json` gateway path.
- tingly-box does **not** refresh the official OAuth tokens after a hybrid apply;
  `codex` CLI owns their lifecycle (same contract as `chatgpt` mode).

## 6. Key files

| Layer | File | Role |
|---|---|---|
| Backend | `internal/server/config/apply_config.go` | `CodexAuthHybrid`; `bearerToken` threaded through `mergeCodexConfig` / `ApplyCodexConfigWithContextWindows` / `RenderCodexConfigTOML`; provider stanza gets `experimental_bearer_token` + `requires_openai_auth`; `ApplyCodexAuth` materialize-or-skip |
| Backend | `internal/server/module/configapply/{handler,types}.go` | `authMode="hybrid"` branch; optional `oauthProviderUuid`; preview embeds bearer token, omits `authJson` |
| Frontend | `frontend/src/pages/scenario/components/CodexConfigModal.tsx` | routing radio + keep-login checkbox → derived `authMode`; hybrid preview + Manual-tab note |
| Frontend | `frontend/src/services/api.ts` | `applyCodexConfig` / `getCodexConfigPreview` accept `'hybrid'` |
| Tests | `internal/server/config/apply_config_hybrid_test.go` | bearer token present in hybrid / absent in gateway; hybrid auth materialize-or-skip; no `OPENAI_API_KEY` leak |

## 7. Config schema reference

Codex publishes a machine-readable schema for `config.toml`. Validate any change
to what we emit against it (editors with the *Even Better TOML* extension check
it live):

- Reference (prose): <https://developers.openai.com/codex/config-reference>
- JSON Schema: <https://developers.openai.com/codex/config-schema.json>
  (both 308-redirect to `learn.chatgpt.com/docs/...`)

Facts that constrain our output (JSON Schema draft-07):

- The **root** object and the **`ModelProviderInfo`** (a `[model_providers.*]`
  entry) both set `additionalProperties: false` — an unknown key is a hard
  validation error, not a warning.
- Valid `[model_providers.*]` fields include `name`, `base_url`, `wire_api`
  (default `"responses"`), `env_key`, `experimental_bearer_token`,
  `requires_openai_auth`, `http_headers`, `query_params`, `auth`, … There is
  **no `preferred_auth_method`** — we used to emit it and it failed validation
  ("Additional properties are not allowed"). It is gone from both the provider
  object and the root.
- `requires_openai_auth` defaults to **`false`** = *"key comes from the `env_key`
  env var"*; **`true`** = *"provider uses the OpenAI auth path"*. Gateway (apikey)
  mode sets `true` (sources `OPENAI_API_KEY` from `auth.json`); hybrid also sets
  `true` while supplying the provider-scoped `experimental_bearer_token`.
- `model_reasoning_effort` is schema-typed as any non-empty string
  (`ReasoningEffort`, `minLength: 1`) — values are model-advertised, so our
  whitelist stays a superset rather than a fixed enum.
- Still valid (contrary to some doc summaries): top-level `profiles`,
  `model_supports_reasoning_summaries`, `model_catalog_json`.

To re-check after a Codex release:
`curl -sL https://developers.openai.com/codex/config-schema.json | jq '.properties | keys'`
and `jq '.definitions.ModelProviderInfo.properties | keys'`.

## 8. Prior art

- cc-switch — *Codex official auth preservation guide* and Issue #2850 /
  release v3.16.1 ("Codex App Enhancements"): the `experimental_bearer_token` in
  `config.toml` + untouched `auth.json` pattern this mirrors.
