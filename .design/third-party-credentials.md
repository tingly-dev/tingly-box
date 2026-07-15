# Third-Party Cloud Credentials (AWS Bedrock / GCP Vertex / Azure OpenAI)

Design for activating the multi-field credential path that is already reserved in
the domain model and DB but never wired to the request path, API, or UI.

> Status: design. No behavior change yet. This doc is the plan to close the seam.

---

## 1. Goal

Let a user "Connect AI" to a model that lives behind a cloud provider's own auth,
not a bearer API key:

| Cloud | Model family | Inbound protocol the gateway must speak | Cloud auth |
|---|---|---|---|
| **AWS Bedrock** | Claude | Anthropic Messages | AWS SigV4 (access key / secret / region), or Bedrock bearer token |
| **GCP Vertex AI** | Claude | Anthropic Messages | GCP service account → OAuth2 |
| **GCP Vertex AI** | Gemini | Google GenAI | GCP service account → OAuth2 |
| **Azure OpenAI** | GPT / o-series | OpenAI Chat/Responses | Azure `api-key` (or Entra token) |

The gateway already normalizes inbound protocols; this feature only adds new
*outbound* auth+endpoint shapes for an upstream provider. Everything the client
sees (streaming SSE, error shapes, model routing) stays identical.

---

## 2. Current state — what is reserved vs missing

The multi-field credential concept is **fully built at the domain-model and DB
layer** and **completely absent above the store**. There is a hard cut at the
`internal/server/module/provider` handler boundary.

| Layer | Multi-field creds today | Verdict |
|---|---|---|
| Domain model `ai/provider.go` | `AuthTypeAWSSigV4` / `AuthTypeAzureKey` / `AuthTypeGCPVertex`, `CredentialBundle`, `Provider.Credential`, `IsMultiFieldCredential()` | **Reserved, active** |
| `typ` re-exports `internal/typ/type.go:314-316,337` | present | **Reserved, active** |
| DB store `internal/data/db/provider_store.go:63,121,188,255,514` | full persist + `UpdateCredentialBundle` | **Reserved, active** |
| Store test `provider_store_test.go:650-699` | Bedrock round-trip proven | **Reserved, active** |
| **Outbound client `internal/client/*`** | zero references; `GetAccessToken()` returns `""` for all three | **Missing** |
| HTTP handler `provider/handler.go` | no credential field in create/update/mask | **Missing** |
| API types `provider/types.go` + `openapi.json` | no `credential`, `auth_type` free-form string | **Missing** |
| Generated client + frontend `frontend/src/**` | nothing (`AuthTypeBadge` stops at vmodel) | **Missing** |
| Provider templates `internal/data/providers.json` | no cloud entries; `region` here means `cn/intl/global`, **not** `us-east-1` | **Missing** |

So the storage/domain layer needs **no change**. The work is the transport seam,
the API seam, and the UI seam.

### 2.1 Where `GetAccessToken()` dead-ends

`ai/provider.go:344` switches on `AuthType`; the three multi-field types are not
cases, so it returns `""`. No client constructor, pool selector, or round-tripper
in `internal/client/` (43 files) references them. That is the entire runtime gap.

---

## 3. Key insight — the vendor SDKs already ship the cloud adapters

We do **not** hand-roll SigV4, Vertex URL rewriting, or Azure versioning. The
pinned submodules under `libs/` each carry a first-party cloud adapter expressed
as a `RequestOption` / middleware that runs closest to the wire:

| Cloud + protocol | Adapter entry point | Mechanism |
|---|---|---|
| Bedrock + Anthropic | `libs/anthropic-sdk-go/bedrock` → `bedrock.WithConfig(aws.Config)` (`bedrock.go:217`), `WithLoadDefaultConfig` (`:186`) | SigV4 signer + URL/body rewrite + eventstream→SSE translation; also honors `AWS_BEARER_TOKEN_BEDROCK` / `cfg.BearerAuthTokenProvider` |
| Vertex + Anthropic | `libs/anthropic-sdk-go/vertex` → `vertex.WithCredentials(ctx, region, projectID, *google.Credentials)` (`vertex.go:57`), `WithGoogleAuth` (`:31`) | google OAuth2 token source + `/v1/messages`→Vertex URL rewrite |
| Vertex + Gemini | `libs/go-genai` → `genai.ClientConfig{Backend: BackendVertexAI, Project, Location, Credentials}` (`client.go:64-114`) | native Vertex backend |
| Azure OpenAI | `libs/openai-go/azure` → `azure.WithEndpoint(endpoint, apiVersion)` (`azure.go:52`) + `azure.WithAPIKey(key)` (`:153`) or `WithTokenCredential` (`:114`) | `api-key` header, `api-version` query, deployment URL shape |

Two of our constructors already take variadic SDK options, so injecting these is
nearly free:

- `NewOpenAIClient(provider, model, sessionID, extraOptions ...option.RequestOption)` — `internal/client/openai.go:56`
- `NewAnthropicClient(provider, model, sessionID, extraOptions ...anthropicOption.RequestOption)` — `internal/client/anthropic.go:56`

`NewGoogleClient` (`google.go:35`) builds a `genai.ClientConfig` directly, so it
needs an explicit Vertex branch rather than an option.

### 3.1 Dependencies

The adapters pull deps already present in the submodule `go.mod`s but not yet in
the root module (they become direct once imported):

- `github.com/aws/aws-sdk-go-v2`, `.../config`, `.../credentials`, `.../aws/protocol/eventstream`, `github.com/aws/smithy-go` (Bedrock)
- `golang.org/x/oauth2/google`, `google.golang.org/api` (Vertex-Anthropic; go-genai already vendors its own)
- Azure `WithAPIKey` needs no `azcore`; only `WithTokenCredential` (Entra) would pull `azidentity`. **Start with `WithAPIKey` only.**

Action: after adding the imports, run `go mod tidy`. No `go.work` change (all under root module).

---

## 4. The credential matrix (the routing question)

`auth_type` alone does not pick the SDK path for GCP — the model family does.
The provider's existing `APIStyle` already carries that discriminator, so we reuse
it rather than inventing a new field:

| `auth_type` | `api_style` | Base client | Cloud adapter applied |
|---|---|---|---|
| `aws_sigv4` | `anthropic` | `AnthropicClient` | `bedrock.WithConfig` |
| `gcp_sa` | `anthropic` | `AnthropicClient` | `vertex.WithCredentials` |
| `gcp_sa` | `google` | `GoogleClient` | genai `BackendVertexAI` |
| `azure_key` | `openai` | `OpenAIClient` | `azure.WithEndpoint` + `azure.WithAPIKey` |

Out of scope for v1 (documented, not built): non-Claude Bedrock models (Llama,
Titan) which speak the Bedrock-native API not Anthropic Messages; Azure via Entra
token credential; Bedrock cross-region inference profiles beyond a single region.

---

## 5. Credential bundle schema

`CredentialBundle.Fields` is a free `map[string]string` (`ai/provider.go:60`), so
new shapes are data, not columns. We define canonical, validated keys per
`auth_type`. The store test already uses `access_key_id` / `secret_access_key` /
`region` (`provider_store_test.go:656-658`) — keep those.

```
aws_sigv4:
  access_key_id       (required unless bearer_token)
  secret_access_key   (required unless bearer_token)
  session_token       (optional; for STS/temp creds)
  region              (required, e.g. "us-east-1")  → base URL
  bearer_token        (optional alt: Bedrock API key; sets cfg.BearerAuthTokenProvider)

gcp_sa:
  service_account_json  (required; the full SA key JSON — secret)
  project_id            (required)
  location              (required, e.g. "us-east5" / "global")

azure_key:
  api_key       (required)
  endpoint      (required, e.g. "https://my-res.openai.azure.com")
  api_version   (required, e.g. "2024-10-21")
  deployment    (optional; when model name ≠ deployment name)
```

Rules:
- **Region / project / endpoint are config, not secrets** — they may appear
  unmasked in `ProviderResponse`. Only `secret_access_key`, `session_token`,
  `bearer_token`, `api_key`, `service_account_json` are secrets → masked.
- Validation lives in one place, `CredentialBundle`-aware, keyed by `auth_type`
  (a `Validate(authType)` helper next to the bundle). The handler calls it.
- `region` in the bundle is the **cloud region** and is orthogonal to the
  template's geographic `region` (`cn/intl/global`) — do not conflate the two.
  See §9.

---

## 6. Architecture — request lifecycle and insertion points

Established lifecycle (from the outbound-path audit):

```
gin handler
 → ProtocolHandler.DispatchChainResult        internal/server/protocol_dispatch.go:64  (switch TargetAPI)
   → ClientPool.Get{OpenAI,Anthropic,Google}Client   internal/client/pool.go:63/124/169
       └─ selects base client by AuthType/issuer      pool.go:75,136,199   ← ADD multi-field branch
   → New{OpenAI,Anthropic,Google}Client(...)          builds SDK client + transport chain
       └─ WithAPIKey(GetAccessToken()) today           ← REPLACE with cloud adapter option
   → forwarding.Forward*                              unchanged
```

### 6.1 Per-cloud client files (mirrors `codex_client.go` / `kimi_client.go`)

Each cloud gets its own `xx_client.go`, consistent with the existing
per-provider client convention. Each file owns exactly one cloud's credential
translation + thin constructor; the generic `AnthropicClient` / `OpenAIClient` /
`GoogleClient` remain the SDK-call surface, so there is no interface-method
duplication (the cloud files wrap the base constructors via `extraOptions`).

- `internal/client/bedrock_client.go` — `NewBedrockClient` → `NewAnthropicClient(…, bedrockOption(p))`; `awsConfigFromBundle` builds `aws.Config` with static creds (or `BearerAuthTokenProvider` when `bearer_token` set) → `bedrock.WithConfig`.
- `internal/client/vertex_client.go` — `NewVertexAnthropicClient` → `NewAnthropicClient(…, vertexAnthropicOption(p))` via `vertex.WithCredentials`; **and** `applyVertexToGenaiConfig` for the Gemini path (go-genai has no request-option seam, so it mutates the config).
- `internal/client/azure_client.go` — `NewAzureClient` → `NewOpenAIClient(…, azureOptions(p)…)` via `azure.WithEndpoint` + `azure.WithAPIKey`.
- Shared field-key constants + `ValidateCredential` live in `ai/credential.go` (reused by the future handler for validation + masking).

### 6.2 Constructor changes (generic clients stay generic)

- `openai.go:56` / `anthropic.go:56`: **skip the base `WithAPIKey(GetAccessToken())`
  when `p.AuthType.IsMultiFieldCredential()`** (it would only plant an empty
  bearer/x-api-key header). The cloud adapter option — passed in by the per-cloud
  constructor through the existing `extraOptions ...RequestOption` param — is
  applied last, so it wins on base URL + auth. The SigV4 caveat holds: we register
  no *body-mutating* SDK middleware after the adapter, so the signature stays valid
  (our UA/logging/proxy live on the `http.Client` transport, not SDK middleware).
- `google.go:35`: add a `p.AuthType == AuthTypeGCPVertex` branch calling
  `applyVertexToGenaiConfig` (in `vertex_client.go`) before `genai.NewClient`.
  go-genai only auto-installs SA auth when it builds the HTTP client itself, so
  the Vertex path calls `httptransport.AddAuthorizationMiddleware` to layer the
  OAuth2 bearer onto our proxy/logging client.

### 6.3 Pool wiring (parallels the OAuth-issuer switches)

`pool.go` dispatches by auth type exactly like the existing Codex/Kimi/ClaudeCode
issuer switches:

- `GetAnthropicClient`: `aws_sigv4 → NewBedrockClient`, `gcp_sa → NewVertexAnthropicClient`, else `NewAnthropicClient`.
- `GetOpenAIClient`: `azure_key → NewAzureClient`, else `NewOpenAIClient`.
- `newGoogleClientForProvider`: `gcp_sa` falls through to `NewGoogleClient`, which applies the Vertex config internally.

### 6.4 Proxy / transport interaction

`provider.ProxyURL` still flows through the pooled `*http.Transport`
(`transport_pool.go:275`). SigV4 signs headers+body, not the network path, so a
proxy is transparent. Vertex/Azure likewise unaffected. Keep the existing
session-bound transport as the base `http.Client` under all cloud adapters.

---

## 7. Backend API + handler

### 7.1 Types — `internal/server/module/provider/types.go`

Add to `CreateProviderRequest`, `UpdateProviderRequest`, `ProviderResponse`:

```go
// Credential carries multi-field cloud credentials for auth types
// aws_sigv4 / gcp_sa / azure_key. Ignored for api_key/oauth/vmodel.
Credential map[string]string `json:"credential,omitempty" description:"Cloud credential fields (aws_sigv4/gcp_sa/azure_key)"`
```

Keep it a flat `map[string]string` on the wire (matches `CredentialBundle.Fields`)
so the frontend form is generic and the OpenAPI schema stays simple.

### 7.2 Handler — `internal/server/module/provider/handler.go`

- **CreateProvider** (`:111`): the `Token` required-check (`:119`) must not fire
  for multi-field auth. Branch: when `req.AuthType.IsMultiFieldCredential()`,
  validate `req.Credential` via the per-type validator instead, and set
  `provider.Credential = &typ.CredentialBundle{Fields: req.Credential}` (no Token).
- **UpdateProvider** (`:248`): merge `Credential` when present; a nil map leaves it
  unchanged, an empty-value field is treated as "keep existing secret" so the UI
  can round-trip masked values (see §8.2).
- **maskForResponse** (`:39`): add cases for the three types — echo config keys
  (region/project/endpoint/api_version) verbatim, replace secret keys with a
  masked sentinel (`sk-***…***` style, matching token masking).
- **Reject unknown `auth_type`**: today it is copied verbatim (`:135,179`). Add a
  whitelist so a typo can't create an inert provider.

### 7.3 Codegen

Backend-first per CLAUDE.md: define models, add the swagger annotations, then
`task codegen` to regenerate `openapi.json` + the frontend client SDK. Until then
leave a frontend placeholder and tell the user codegen must run. Also give
`auth_type` an enum in the swagger annotation so the generated client is typed.

---

## 8. Frontend — Connect AI flow

Ref: `.design/connect-ai-flow.md`, `.design/ux-principles.md`.

### 8.1 Picker — a new section, not a new mode (implemented)

The picker groups Custom / OAuth / **Cloud** / Self-hosted / API-key
(`ConnectProviderDialog.tsx`). The Cloud section lists Bedrock / Vertex-Claude /
Vertex-Gemini / Azure cards with an info-tinted badge. Selecting a card carries a
new `ConnectSelection` kind `{kind:'cloud', presetId}`. No top-level mode toggle
(UX principle: eliminate mode pickers). Per decision #1, Vertex is split into two
cards ("Vertex — Claude" `api_style=anthropic`, "Vertex — Gemini"
`api_style=google`) so routing is unambiguous.

### 8.2 Form — a dedicated dialog, not the protocol-slot form (implemented)

Cloud credentials do not fit `ProviderFormDialog` (built around protocol slots +
a single bearer token), so — mirroring how OAuth uses its own `OAuthDialog` —
cloud providers get **`components/cloud/CloudProviderDialog.tsx`**, driven by a
per-cloud field schema in **`components/cloud/cloudCredentialSchema.ts`** (the
frontend mirror of `ai.CredentialSchema`; field keys must match). Concrete-value
inputs, not aliases; secrets are password fields with a reveal toggle; optional
fields (session/bearer token, deployment, proxy) sit under an Advanced divider.
The dialog computes a meaningful `api_base` from the fields (adapter ignores it,
but the create API requires it) and POSTs `{name, api_style, auth_type,
credential, ...}` via `api.addProvider`.

Wiring: `useProviderDialog` routes the `cloud` kind to an `onCloud(presetId)`
callback (like `onOAuth`/`onImport`); `CredentialPage` opens the dialog.
`AuthTypeBadge` renders Bedrock/Vertex/Azure labels for the list.

The picker cloud cards are **data-driven from the backend templates** (Phase 4):
`serviceProviders.getCloudProviders()` / `useCloudProviders()` surface templates
whose `auth_type` is a cloud type; `cloudCredentialSchema.ts` keeps only the
per-`auth_type` field schema + `buildCloudApiBase`, and `CloudProviderDialog`
resolves the template by id and the fields by its `auth_type`.

**Not yet done (frontend):** editing an existing cloud provider still opens the
generic edit form; a cloud-aware edit path (and the masked-secret round-trip)
plus a signed Test-Connection are the remaining follow-ups.

### 8.3 Badges + types

- `AuthTypeBadge.tsx:15` add `aws_sigv4` / `gcp_sa` / `azure_key` labels + colors.
- `frontend/src/types/provider.ts` extend `Provider` with the generated
  `credential` field (placeholder until codegen, like the existing `vmodel`
  `TODO(codegen)`).
- Provider/brand logos via `BrandIcons.tsx` / `ProviderIcon.tsx` (AWS, GCP, Azure).

### 8.4 Test Connection

The probe (`provider-form-dialog/probe.ts`) must traverse the **real** signed
path (UX principle: diagnostics use the real path) — i.e. hit the actual client
with the cloud adapter, e.g. a minimal `messages`/`generateContent` or a
models-list call, not a bare URL ping that skips SigV4.

---

## 9. Provider templates — `internal/data/providers.json`

Add cloud entries. Two schema notes:

1. The template's `region` field is geographic grouping (`cn/intl/global/self-hosted`,
   `provider_template.go:83`). The **cloud region** (`us-east-1`, `us-east5`) is a
   *credential* field, entered by the user, not a template constant. Keep them
   separate; a template may set the geographic `region` to `global`.
2. Templates currently only carry `auth_type: oauth|key` (`provider_template.go:103`).
   Extend the doc/enum to include the three cloud types and, for cloud entries, a
   `credential_schema` hint (which fields to render) so the form is template-driven
   rather than hard-coded per card.

Cloud templates also can't fetch a live model list the usual way (no public
`/models`): seed `models` from the template (Bedrock Claude model IDs, Vertex
Claude/Gemini IDs, Azure deployments) with `ModelCacheSourceTemplate`
(`provider/types.go:99`). Quota fetch stays "no public API" like
`vertexai.go:46`.

**Implemented (Phase 4):** four cloud templates in `providers.json`
(`aws-bedrock`, `gcp-vertex-claude`, `gcp-vertex-gemini`, `azure-openai`) carry
`auth_type`, an explicit `api_style`, a `canonical_domain`, and seeded model
lists (IDs verified against AWS/Google/Anthropic/Microsoft official docs, July
2026). `ProviderTemplate` gains an `APIStyle` field, and
`findTemplateByProvider` now matches `canonical_domain` **and** `api_style` — so
the two Vertex templates (same `aiplatform.googleapis.com` domain) resolve to
the right model family by the provider's style. `ValidateTemplate` no longer
requires a base URL for cloud (or OAuth) templates. Since `GetProviderTemplates`
serializes the full template map, the new fields reach the frontend with no
schema/codegen change. The computed `api_base` the dialog sends
(`bedrock-runtime.<region>…`, `<location>-aiplatform.googleapis.com`, the Azure
endpoint) matches each template's `canonical_domain`, so the model list resolves
after connect. Covered by `internal/data/cloud_template_test.go`.

---

## 10. Cross-cutting concerns

| Concern | Decision |
|---|---|
| **Secrets at rest** | Same store as tokens (SQLite `credential` text column). Inherits existing at-rest posture (`.design/security.md`); no new plaintext surface. Consider a follow-up to encrypt the bundle column. |
| **Secrets in responses** | Masked in `maskForResponse`; config keys echoed. Never log `service_account_json` / secret / api_key. |
| **GCP token refresh** | Handled *inside* the SDK's google token source — no manual refresh loop (unlike our OAuth path). Build `*google.Credentials` once per client. |
| **AWS creds** | v1 = static keys or Bedrock bearer token. STS/assume-role/instance-profile is a later `WithLoadDefaultConfig` variant. |
| **Model listing** | Template-seeded; no live fetch v1. |
| **Error mapping** | SDK adapters surface cloud errors (403 SigV4, 401 Vertex) through the same SDK error types → existing error normalization applies. Add friendly hints for the common misconfig (bad region, expired SA). |
| **Timeouts / retries** | Reuse constructor defaults (`constant.DefaultRequestTimeout`, `WithMaxRetries(0)`). Cloud adapters do not override. |
| **Dual-URL** | Not applicable to cloud auth (single endpoint derived from region/endpoint). `IsDual()` already excludes non-api_key; keep multi-field out of dual routing. |

---

## 11. UX-principles check (`.design/ux-principles.md`)

- **Eliminate mode pickers** → Cloud is a picker *section*, not a mode toggle. ✅
- **Show concrete values, not aliases** → real Region / Endpoint / Project inputs. ✅
- **Separate orthogonal axes** → cloud region (credential) vs geographic region
  (template grouping) kept distinct. ✅
- **Diagnostics traverse the real path** → Test Connection uses the signed client. ✅
- **Surface the artifact for the next action** → after connect, land on the new
  provider's model list (template-seeded), same as api_key flow. ✅
- **Smart defaults** → template pre-fills api_style, api_version, known regions. ✅

---

## 12. Phased plan

1. **Backend runtime (no UI):** `cloud_credential.go` translator + constructor
   branches + pool wiring; `go mod tidy` for aws/google deps. Unit-test each cloud
   path with a recorded/stubbed upstream. *Ships value: providers created directly
   in DB (as the store test already does) now actually route.*
2. **API seam:** `types.go` + `handler.go` create/update/mask + swagger enum;
   `task codegen`. Validation helper on `CredentialBundle`.
3. **Frontend:** picker Cloud section, per-cloud form schema, badges, masking
   round-trip, Test Connection, brand icons.
4. **Templates (done):** Bedrock/Vertex-Claude/Vertex-Gemini/Azure entries in
   `providers.json` with `api_style` + seeded model lists; `api_style`-aware
   template matching; frontend cloud cards data-driven from these templates.
5. **Follow-ups:** cloud-aware edit path + masked-secret round-trip; signed
   Test-Connection; Azure Entra token; AWS STS/assume-role; non-Claude Bedrock;
   encrypt-at-rest for the credential column.

---

## 13. Open decisions (need a call before build)

1. **Vertex model-family routing** — infer `api_style` from the chosen model
   (Claude vs Gemini) at connect time, or make the user pick Claude/Gemini on the
   Vertex card? (Recommend: two Vertex cards, "Vertex — Claude" and
   "Vertex — Gemini", so `api_style` is unambiguous and the model list is right.)
2. **Bedrock auth default** — lead with access-key/secret, or with the newer
   Bedrock API key (bearer)? (Recommend: access-key primary, bearer as a toggle.)
3. **Encrypt-at-rest now or later** — service-account JSON is higher-value than a
   bearer token; do we gate v1 on column encryption?

---

## 14. Key files

| File | Role in this feature |
|---|---|
| `ai/provider.go:13-70,205` | Auth types, `CredentialBundle`, `Provider.Credential` — reused as-is |
| `internal/data/db/provider_store.go:121,188,255,514` | Persist bundle — reused as-is |
| `ai/credential.go` | **New** — canonical field keys, `CredentialSchema`, `ValidateCredential`, `IsSecretCredentialField` (shared by client + handler) |
| `internal/client/bedrock_client.go` | **New** — `NewBedrockClient` + `awsConfigFromBundle` (SigV4 / bearer) |
| `internal/client/vertex_client.go` | **New** — `NewVertexAnthropicClient` + `applyVertexToGenaiConfig` (Claude & Gemini on Vertex) |
| `internal/client/azure_client.go` | **New** — `NewAzureClient` + `azureOptions` |
| `internal/client/openai.go:56` | Skip empty `WithAPIKey` for multi-field |
| `internal/client/anthropic.go:56` | Skip empty `WithAPIKey` for multi-field |
| `internal/client/google.go:35` | Vertex `BackendVertexAI` branch |
| `internal/client/pool.go:63,124,169` | Dispatch to per-cloud clients by auth type |
| `internal/server/module/provider/types.go` | Add `credential` to req/resp |
| `internal/server/module/provider/handler.go:39,111,248` | Mask/create/update credential; auth_type whitelist |
| `internal/data/providers.json` + `provider_template.go:83,103` | Cloud templates; `credential_schema`; region semantics |
| `frontend/src/components/ConnectProviderDialog.tsx:24` | Cloud picker section + `cloud` kind |
| `frontend/src/components/ProviderFormDialog.tsx` | Per-cloud credential fields |
| `frontend/src/components/AuthTypeBadge.tsx:15` | Cloud auth badges |
| `libs/anthropic-sdk-go/bedrock`, `.../vertex`, `libs/openai-go/azure`, `libs/go-genai` | Vendored cloud adapters — the engines we wire to |
</content>
