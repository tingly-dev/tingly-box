# Fusion Provider

A **fusion provider** is a single `Provider` record that exposes two base URLs —
one for the OpenAI-compatible protocol and one for the Anthropic-compatible
protocol — under the same API credential.  The dispatcher routes each inbound
request to whichever URL matches the client's protocol natively, eliminating
protocol translation overhead for providers that support both natively (e.g.
Vertex AI, Bedrock, many inference platforms).

Fusion mode is always active (graduated from experimental in May 2026).

---

## Data model

```
Provider {
    APIBase:          "https://api.example.com/openai/v1"   // primary / legacy fallback
    APIStyle:         "openai"                              // primary style (openai by convention)
    APIBaseOpenAI:    "https://api.example.com/openai/v1"  // fusion: OpenAI-side URL
    APIBaseAnthropic: "https://api.example.com/anthropic"  // fusion: Anthropic-side URL
    AuthType:         "api_key"                             // fusion requires api_key
    Token:            "..."
}
```

`APIBaseOpenAI` and `APIBaseAnthropic` are optional.  A provider is considered
"fusion" when **both** are non-empty.  `APIBase`/`APIStyle` are always populated
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

`internal/server/fusion.go` — called at the top of every inbound handler before
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

When the user selects a provider template that has both `baseUrlOpenAI` and
`baseUrlAnthropic`, the form lets them pick both protocol checkboxes.  Once both
are checked, a **Fusion mode** toggle appears below the protocol selector.

| Fusion toggle | Outcome |
|---|---|
| Off (default) | Two separate `Provider` records are created — one `openai`, one `anthropic` — sharing the same credential. |
| On | One fusion `Provider` record is created with both URLs and `APIStyle: openai` as primary. |

A topology hint below the toggle tells the user which outcome is selected
("merged into one" vs "saved as two separate providers").

### Custom endpoint (no template)

A free-form **Custom endpoint** can also be a fusion provider — the second URL is
entered manually instead of inferred from a template.

- The single Base URL field is the OpenAI / primary URL. When **both** protocols
  are ticked, the Fusion toggle and topology hint appear just as for templates.
- By default both protocols share that one URL (same address ⇒
  `api_base_openai == api_base_anthropic`). An **"Anthropic endpoint differs?"**
  link reveals a second labelled field (*OpenAI Base URL* / *Anthropic Base URL*)
  for providers that serve each protocol at a distinct address; **"Use the same
  URL"** collapses it back.
- Fusion toggle **on** ⇒ one fusion record (URLs may be equal or distinct);
  **off** ⇒ two split single-protocol records, each carrying its own URL.

The URL source in `buildAddProviderPayload` is therefore template-driven
(`providerBaseUrls`) **or** form-driven (`apiBaseOpenAI`/`apiBaseAnthropic`,
falling back to `apiBase`) — the backend payload is identical either way.

> **Known gap:** upgrading an *existing* single-protocol custom provider to fusion
> via the edit dialog is not yet wired (the edit-mode upgrade toggle still keys off
> a matched template). Downgrade of an existing custom fusion provider works, since
> `isExistingFusion` derives from the stored URLs. Add-mode custom fusion is fully
> supported.

---

## Edit flow

Edit mode enforces three rules that differ from add mode.

### Rule 1 — Protocol lock for non-fusion providers

If the provider being edited is **not** a fusion provider (only one of
`APIBaseOpenAI`/`APIBaseAnthropic` is set, or neither), the protocol checkboxes
are **disabled**.  The user cannot change the protocol by clicking them.

*Rationale:* switching an existing OpenAI provider to Anthropic would silently
change its routing behavior for all rules that reference it.  Creating a new
entry is explicit and leaves the old one intact.

### Rule 2 — Fusion upgrade via toggle

If the template matched to the provider's `APIBase` supports **both** protocols,
a **Fusion mode** toggle appears in edit mode even for non-fusion providers.

Enabling the toggle:
- auto-selects both protocol checkboxes
- populates `APIBaseOpenAI` and `APIBaseAnthropic` from the template
- sets `APIBase` to the OpenAI URL (by convention), `APIStyle` to `openai`
- on submit, `updateProvider` stores the new fusion fields

Disabling the toggle (reverting):
- restores the original single-protocol state from a snapshot taken when the dialog opened (`initialFusionRef`)
- clears both fusion URL fields
- restores original `APIBase`/`APIStyle`

The snapshot is captured on dialog open so stale React state is never read
during the revert operation.

### Rule 3 — Fusion downgrade

If the provider being edited **is** a fusion provider, both protocol checkboxes
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
| `isExistingFusion` | True when the dialog opens on a provider with both fusion URLs set |
| `initialFusionRef` | Snapshot of `{ openAI, anthropic, apiBase, apiStyle }` taken on open |
| `createFusionProvider` | Tracks the fusion toggle state (also written to parent as `createFusionProvider` field) |
| `effectiveLocked` | `fusionLocked \|\| protocolLocked`; passed to `ProtocolSelector` as `fusionLocked` |
| `showFusionToggle` | add: `protocolOpenAI && protocolAnthropic`; edit: `!isExistingFusion && hasBothBaseUrls` |
| `showSecondUrl` / `anthropicUrlValue` | custom mode only: progressive-disclosure of a second (Anthropic) URL field and its local value. Committed to `data.apiBaseAnthropic` on blur/submit. |

In custom mode `syncProtocolsToParent` does **not** clear the fusion URL fields
(the URL text inputs own them); it only clears the unused side when the user drops
back to a single protocol. `showTopologyHint` also fires in custom mode.

`handleFusionDowngrade(nextOpenAI, nextAnthropic)` — called from protocol toggle
handlers when `isExistingFusion` is true.  Reads from `initialFusionRef` and
fires `onChange` to push the updated fields to the parent.

### `CredentialPage.tsx` — `buildAddProviderPayload`

When `bothProtocols && shouldCreateFusion` (fusion toggle on):

```ts
{
    api_base: openaiUrl,
    api_style: 'openai',
    api_base_openai: openaiUrl,
    api_base_anthropic: anthropicUrl,
    ...
}
```

When `bothProtocols && !shouldCreateFusion` (toggle off): returns an **array**
of two single-protocol payloads, one per protocol.

### `CredentialPage.tsx` — `buildEditProviderPayload`

Always includes `api_base_openai` and `api_base_anthropic`.  Sending empty
strings clears the fields on the backend, enabling the downgrade path.

### `internal/server/provider_handler.go`

`CreateProvider`: validates that fusion URLs (`api_base_openai` /
`api_base_anthropic`) are only supplied for `api_key` auth and non-Google style.

`UpdateProvider`: applies `api_base_openai` / `api_base_anthropic` unconditionally
(no flag gate since fusion is stable).

---

## Key files

| File | Role |
|---|---|
| `ai/provider.go` | `Provider` type; `IsFusion()`, `ResolveEndpoint()`, `HasFusionURL()` |
| `ai/provider_test.go` | Unit tests for `ResolveEndpoint` and `IsFusion` |
| `internal/server/fusion.go` | `resolveProviderForClient` — dispatch-time endpoint resolution |
| `internal/server/provider_handler.go` | `CreateProvider` / `UpdateProvider` — fusion field validation and persistence |
| `frontend/src/components/ProviderFormDialog.tsx` | Add + edit form; fusion toggle, protocol lock, upgrade/downgrade logic |
| `frontend/src/components/providerFormDialog/FusionToggle.tsx` | Fusion checkbox with tooltip |
| `frontend/src/components/providerFormDialog/ProtocolSelector.tsx` | Protocol checkboxes; respects `fusionLocked` (covers both OAuth and edit-mode lock) |
| `frontend/src/pages/CredentialPage.tsx` | `buildAddProviderPayload` (split vs fusion), `buildEditProviderPayload` |
| `frontend/src/i18n/locales/en.ts` | `providerDialog.fusion.*` strings |
