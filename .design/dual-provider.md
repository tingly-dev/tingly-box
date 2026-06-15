# Dual Provider

A **dual provider** is a single `Provider` record that exposes two base URLs —
one for the OpenAI-compatible protocol and one for the Anthropic-compatible
protocol — under the same API credential.  The dispatcher routes each inbound
request to whichever URL matches the client's protocol natively, eliminating
protocol translation overhead for providers that support both natively (e.g.
Vertex AI, Bedrock, many inference platforms).

Dual mode is always active (graduated from experimental in May 2026).

---

## Data model

```
Provider {
    APIBase:          "https://api.example.com/openai/v1"   // primary / legacy fallback
    APIStyle:         "openai"                              // primary style (openai by convention)
    APIBaseOpenAI:    "https://api.example.com/openai/v1"  // dual: OpenAI-side URL
    APIBaseAnthropic: "https://api.example.com/anthropic"  // dual: Anthropic-side URL
    AuthType:         "api_key"                             // dual requires api_key
    Token:            "..."
}
```

`APIBaseOpenAI` and `APIBaseAnthropic` are optional.  A provider is considered
"dual" when **both** are non-empty.  `APIBase`/`APIStyle` are always populated
for backward compatibility with model-probe and model-list code that reads them
directly.  By convention, `APIBase` is set to the OpenAI URL.

### Constraints

| Constraint | Reason |
|---|---|
| `AuthType` must be `api_key` | OAuth tokens are issuer-specific; the token's scope is tied to one protocol endpoint |
| `APIStyle` must not be `google` | Google auth is per-project, not per-endpoint |
| Template **optional** | A template pre-fills both URLs; custom endpoints supply the second URL manually (see Add flow). |

---

## Dispatch (`resolveProviderForClient`)

`internal/server/dual.go` — called at the top of every inbound handler before
any protocol translation.

```
resolveProviderForClient(p, clientStyle):
    (baseURL, style) = p.ResolveEndpoint(clientStyle)
    if unchanged → return p as-is
    clone p; set clone.APIBase = baseURL, clone.APIStyle = style
    return clone
```

`Provider.ResolveEndpoint` (`ai/provider.go`):

| Inbound client style | APIBaseOpenAI set? | APIBaseAnthropic set? | Result |
|---|---|---|---|
| `openai` | ✓ | any | `(APIBaseOpenAI, openai)` |
| `anthropic` | any | ✓ | `(APIBaseAnthropic, anthropic)` |
| any | ✗ | ✗ | `(APIBase, APIStyle)` — legacy single-protocol |

The returned shallow clone is used by downstream HTTP clients and protocol
transformers.  The stored provider record is never mutated.

---

## Add flow

See also: [connect-provider-flow.md](connect-provider-flow.md) for the full
picker → form sequence.

Dual is reached two ways, kept on separate surfaces so each form answers one
question:

### Preset template (both protocols)

When the user selects a provider template that has both `baseUrlOpenAI` and
`baseUrlAnthropic`, the form lets them pick both protocol checkboxes.  Once both
are checked, a **Dual mode** toggle appears below the protocol selector.

| Dual toggle | Outcome |
|---|---|
| Off (default) | Two separate `Provider` records are created — one `openai`, one `anthropic` — sharing the same credential. |
| On | One dual `Provider` record is created with both URLs and `APIStyle: openai` as primary. |

A topology hint below the toggle tells the user which outcome is selected
("merged into one" vs "saved as two separate providers").  Presets keep this
split-vs-merge choice.

### "Dual endpoint" picker card (custom)

The Connect AI picker has a dedicated **Dual endpoint** card (next to "Custom
endpoint").  It opens a purpose-built form (`ProviderFormDialog` with
`dualMode`) that is *born dual*:

- two free-text URL fields — *OpenAI Base URL* + *Anthropic Base URL*;
- one shared API key;
- no protocol selector, no topology toggle, no progressive disclosure;
- **Test Connection probes both endpoints** (parallel, per-side results) — a
  fused provider answers both protocols, so verifying only one URL would leave
  the other untested.

It **always** produces a single fused record (`api_base_openai` +
`api_base_anthropic` both set, `api_base = openai URL`, `api_style = openai`,
`auth_type = api_key`).  The two URLs may be identical (degenerate but allowed).

Plain **Custom endpoint** is therefore strictly single-protocol (a single URL +
an OpenAI/Anthropic radio).  To get two independent records instead of a fused
one, add two Custom entries — "split" is no longer a modeled mode on the custom
side.

`buildAddProviderPayload` sources the two URLs template-driven (`providerBaseUrls`)
**or** form-driven (`apiBaseOpenAI`/`apiBaseAnthropic`, falling back to `apiBase`),
so the backend payload is identical for both surfaces.

---

## Edit flow

How an existing provider opens depends on its stored shape:

- **Both dual URLs set** → opens the dedicated **Dual endpoint** form
  (`dualMode`).  A *"Convert to a single endpoint"* link downgrades it (keeps
  the OpenAI URL, clears both dual fields, `APIStyle = openai`).
- **Single protocol** (or matched preset) → opens the standard form.  A
  *"Add an Anthropic endpoint (make it a Dual provider)"* link upgrades it: the
  current URL becomes the OpenAI side and the dialog switches to `dualMode`
  (`CredentialPage.handleConvertToDual` / `handleConvertToSingle` flip the
  parent's mode flags and patch `providerFormData`; the open-effect, keyed on
  `[open, dualMode]`, re-initialises the form for the new shape).

The template-driven preset edit additionally enforces the three rules below.

### Rule 1 — Protocol lock for non-dual providers

If the provider being edited is **not** a dual provider (only one of
`APIBaseOpenAI`/`APIBaseAnthropic` is set, or neither), the protocol checkboxes
are **disabled**.  The user cannot change the protocol by clicking them.

*Rationale:* switching an existing OpenAI provider to Anthropic would silently
change its routing behavior for all rules that reference it.  Creating a new
entry is explicit and leaves the old one intact.

### Rule 2 — Dual upgrade via toggle

If the template matched to the provider's `APIBase` supports **both** protocols,
a **Dual mode** toggle appears in edit mode even for non-dual providers.

Enabling the toggle:
- auto-selects both protocol checkboxes
- populates `APIBaseOpenAI` and `APIBaseAnthropic` from the template
- sets `APIBase` to the OpenAI URL (by convention), `APIStyle` to `openai`
- on submit, `updateProvider` stores the new dual fields

Disabling the toggle (reverting):
- restores the original single-protocol state from a snapshot taken when the dialog opened (`initialDualRef`)
- clears both dual URL fields
- restores original `APIBase`/`APIStyle`

The snapshot is captured on dialog open so stale React state is never read
during the revert operation.

### Rule 3 — Dual downgrade

If the provider being edited **is** a dual provider, both protocol checkboxes
are enabled and either can be deselected.

Deselecting one side:
- retains the remaining protocol's URL as `APIBase`
- sets `APIStyle` to match the remaining protocol
- clears both `APIBaseOpenAI` and `APIBaseAnthropic` (the provider reverts to single-protocol)

The URL for the remaining side is read from the snapshot (not from React state)
to avoid async-update races.

Deselecting the **last** protocol is blocked — at least one must remain.

---

## State & implementation

### `ProviderFormDialog.tsx`

| State / ref | Purpose |
|---|---|
| `isExistingDual` | True when the dialog opens on a provider with both dual URLs set |
| `initialDualRef` | Snapshot of `{ openAI, anthropic, apiBase, apiStyle }` taken on open |
| `createDualProvider` | Tracks the dual toggle state (also written to parent as `createDualProvider` field) |
| `effectiveLocked` | `dualLocked \|\| protocolLocked`; passed to `ProtocolSelector` as `dualLocked` |
| `showDualToggle` | preset-only (`!customMode && !dualMode`): add `protocolOpenAI && protocolAnthropic`; edit `!isExistingDual && hasBothBaseUrls` |
| `fusOpenAIUrl` / `fusAnthropicUrl` | `dualMode` local mirrors for the two URL fields. Seeded from `data.apiBaseOpenAI`/`apiBaseAnthropic` on `[open, dualMode]`; committed to parent on blur and in `handleSubmit`. |

`customMode` is strictly single-protocol: `ProtocolSelector` renders radios
(`singleSelect`) and the toggle handlers are mutually exclusive. The dual toggle
and topology hint are preset-only.

`handleDualDowngrade(nextOpenAI, nextAnthropic)` — called from protocol toggle
handlers when `isExistingDual` is true.  Reads from `initialDualRef` and
fires `onChange` to push the updated fields to the parent.

### `CredentialPage.tsx` — `buildAddProviderPayload`

When `bothProtocols && shouldCreateDual` (dual toggle on):

```ts
{
    api_base: openaiUrl,
    api_style: 'openai',
    api_base_openai: openaiUrl,
    api_base_anthropic: anthropicUrl,
    ...
}
```

When `bothProtocols && !shouldCreateDual` (toggle off): returns an **array**
of two single-protocol payloads, one per protocol.

### `CredentialPage.tsx` — `buildEditProviderPayload`

Always includes `api_base_openai` and `api_base_anthropic`.  Sending empty
strings clears the fields on the backend, enabling the downgrade path.

### `internal/server/provider_handler.go`

`CreateProvider`: validates that dual URLs (`api_base_openai` /
`api_base_anthropic`) are only supplied for `api_key` auth and non-Google style.

`UpdateProvider`: applies `api_base_openai` / `api_base_anthropic` unconditionally
(no flag gate since dual is stable).

---

## Key files

| File | Role |
|---|---|
| `ai/provider.go` | `Provider` type; `IsDual()`, `ResolveEndpoint()`, `HasDualURL()` |
| `ai/provider_test.go` | Unit tests for `ResolveEndpoint` and `IsDual` |
| `internal/server/dual.go` | `resolveProviderForClient` — dispatch-time endpoint resolution |
| `internal/server/provider_handler.go` | `CreateProvider` / `UpdateProvider` — dual field validation and persistence |
| `frontend/src/components/ProviderFormDialog.tsx` | Add + edit form; dual toggle, protocol lock, upgrade/downgrade logic |
| `frontend/src/components/providerFormDialog/DualToggle.tsx` | Dual checkbox with tooltip |
| `frontend/src/components/providerFormDialog/ProtocolSelector.tsx` | Protocol checkboxes; respects `dualLocked` (covers both OAuth and edit-mode lock) |
| `frontend/src/pages/CredentialPage.tsx` | `buildAddProviderPayload` (split vs dual), `buildEditProviderPayload` |
| `frontend/src/i18n/locales/en.ts` | `providerDialog.dual.*` strings |
