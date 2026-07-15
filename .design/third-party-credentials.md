# Third-Party Cloud Credentials (AWS Bedrock / GCP Vertex / Azure OpenAI)

Activates the multi-field credential path that was reserved in the domain model +
DB but never wired to the request path, API, or UI.

> Status: implemented (Phases 1–4). Follow-ups listed in §11.

## 1. Goal

Connect AI to a model behind a cloud provider's own auth instead of a bearer key:

| Cloud | Model family | Inbound protocol | Cloud auth |
|---|---|---|---|
| AWS Bedrock | Claude | Anthropic Messages | SigV4 (access key/secret/region) or Bedrock bearer token |
| GCP Vertex AI | Claude | Anthropic Messages | GCP service account → OAuth2 |
| GCP Vertex AI | Gemini | Google GenAI | GCP service account → OAuth2 |
| Azure OpenAI | GPT / o-series | OpenAI Chat/Responses | Azure `api-key` |

Only the *outbound* auth+endpoint shape is new; inbound protocol handling,
streaming, and error shapes are unchanged.

## 2. Starting point

Reserved and left untouched (worked as-is): `ai.AuthType{AWSSigV4,AzureKey,GCPVertex}`,
`ai.CredentialBundle`, `Provider.Credential`, `IsMultiFieldCredential()`
(`ai/provider.go`); DB persistence (`internal/data/db/provider_store.go`, incl.
`UpdateCredentialBundle`); store test at `provider_store_test.go:650-699`.

The gap was everything above the store: `GetAccessToken()` returned `""` for the
three types (`ai/provider.go:344`) and nothing in `internal/client/`,
`provider/handler.go`, the API schema, the frontend, or `providers.json`
referenced them.

## 3. Key decision — reuse the SDKs' first-party cloud adapters

No hand-rolled SigV4 / URL rewriting. The pinned submodules each ship an adapter
expressed as a `RequestOption`/middleware (or, for genai, a config):

| Cloud + protocol | Adapter | Mechanism |
|---|---|---|
| Bedrock + Anthropic | `anthropic-sdk-go/bedrock.WithConfig(aws.Config)` | SigV4 or bearer; URL/body rewrite; eventstream→SSE |
| Vertex + Anthropic | `anthropic-sdk-go/vertex.WithCredentials(ctx, loc, project, creds)` | SA OAuth2 + `/v1/messages`→Vertex URL |
| Vertex + Gemini | `go-genai` `ClientConfig{Backend: BackendVertexAI, Project, Location, Credentials}` | native Vertex backend |
| Azure OpenAI | `openai-go/azure.WithEndpoint(endpoint, apiVersion)` + `.WithAPIKey(key)` | `api-key` header, `api-version` query, deployment URL |

`NewOpenAIClient`/`NewAnthropicClient` already take `extraOptions ...RequestOption`,
so the option just gets appended. `NewGoogleClient` mutates the genai config
directly. Importing these pulled `aws-sdk-go-v2` (+config/credentials/eventstream),
`smithy-go`, `golang.org/x/oauth2/google`, `cloud.google.com/go/auth` as direct
deps (`go mod tidy`). Azure `WithAPIKey` needs no `azcore` (Entra would).

## 4. Routing matrix

`auth_type` alone doesn't pick the GCP path — model family does, and the
provider's `api_style` already carries it:

| `auth_type` | `api_style` | Base client | Adapter |
|---|---|---|---|
| `aws_sigv4` | `anthropic` | `AnthropicClient` | `bedrock.WithConfig` |
| `gcp_sa` | `anthropic` | `AnthropicClient` | `vertex.WithCredentials` |
| `gcp_sa` | `google` | `GoogleClient` | genai `BackendVertexAI` |
| `azure_key` | `openai` | `OpenAIClient` | `azure.WithEndpoint`+`WithAPIKey` |

## 5. Credential schema (`ai/credential.go`)

`CredentialBundle.Fields` is a `map[string]string`, so shapes are data. Canonical
keys + validation live in `ai/credential.go` (`CredentialSchema`,
`ValidateCredential`, `IsSecretCredentialField`), shared by client + handler.

```
aws_sigv4: region (req) · access_key_id + secret_access_key  OR  bearer_token · session_token (opt)
gcp_sa:    project_id (req) · location (req) · service_account_json (req, secret)
azure_key: endpoint (req) · api_version (req) · api_key (req, secret)
```

(No Azure `deployment` field: the azure adapter derives the deployment URL
segment from the request's model name, so name the model after the deployment.
A stored override the client never applies would be a dead knob.)

- Secret keys (masked, never logged): `secret_access_key`, `session_token`,
  `bearer_token`, `api_key`, `service_account_json`. Config keys (region/project/
  endpoint/api_version/deployment) are not secret. Unknown keys fail closed (secret).
- The bundle's `region` is the **cloud region** (`us-east-1`), orthogonal to the
  template's geographic `region` (`cn/intl/global`).

## 6. Client layer (`internal/client/`)

Per-cloud files, mirroring `codex_client.go`/`kimi_client.go`. The generic
clients stay the SDK-call surface; cloud files only translate the bundle and wrap
the base constructor via `extraOptions` (no interface-method duplication).

- `bedrock_client.go` — `NewBedrockClient` → `NewAnthropicClient(…, bedrockOption)`; `awsConfigFromBundle` (static creds, or `BearerAuthTokenProvider` when `bearer_token` set).
- `vertex_client.go` — `NewVertexAnthropicClient` (via `vertex.WithCredentials`) + `applyVertexToGenaiConfig` for the Gemini path.
- `azure_client.go` — `NewAzureClient` → `NewOpenAIClient(…, azureOptions…)`.

Wiring / gotchas:
- `pool.go` dispatches by auth type like the OAuth-issuer switches
  (`aws_sigv4→Bedrock`, `gcp_sa`+anthropic→VertexAnthropic, `azure_key→Azure`;
  `gcp_sa`+google falls into `NewGoogleClient`).
- `anthropic.go`/`openai.go` skip the base `WithAPIKey(GetAccessToken())` for
  multi-field auth (it would plant an empty header); the adapter option is applied
  last so it wins on base URL + auth.
- go-genai only auto-installs SA auth when it builds its own HTTP client; we pass
  our proxy/logging client, so `applyVertexToGenaiConfig` calls
  `httptransport.AddAuthorizationMiddleware` — otherwise Vertex goes out unauthed
  (fails closed if no HTTP client is set).
- For gcp_sa the genai `HTTPOptions.BaseURL` is left **empty**: genai derives the
  correct Vertex host from Location — incl. `global` (aiplatform.googleapis.com)
  and multi-regional `us`/`eu` (aiplatform.<loc>.rep.googleapis.com) — only when
  BaseURL is unset; forcing the stored APIBase would break those locations.
- GCP credentials are cached by sha256(SA JSON) (`vertex_client.go`): clients are
  rebuilt per request, so without the cache every request re-parses the SA key
  and mints a fresh OAuth token (blocking round-trip to Google).
- `ListModels` returns `ErrModelsEndpointNotSupported` for cloud → template fallback.
- Known limit: `vertex.WithCredentials` installs its own HTTP client, so
  `provider.ProxyURL` is not honored on the Vertex-Anthropic path. Bedrock/Azure
  keep our transport (proxy works).

## 7. Backend API (`provider/`)

- `types.go`: `credential map[string]string` on Create/Update/Response
  (flat map = `CredentialBundle.Fields`).
- `handler.go`: CreateProvider validates the bundle via `ai.ValidateCredential`
  for multi-field auth (token-required only for `api_key`), sets `p.Credential`,
  clears `Token`; rejects unknown `auth_type` (whitelist). UpdateProvider replaces
  the bundle when a non-empty map is sent. `maskForResponse` returns the fields in
  full — matching the existing Token behavior for the local admin UI (real masking
  of both is a follow-up; see §11).
- `openapi.json` regenerated (`go run ./cli/tingly-box swagger`).

## 8. Frontend (Connect AI)

- Picker: a **Cloud** section (`ConnectProviderDialog.tsx`), not a mode toggle.
  Cards are data-driven from templates via `serviceProviders.useCloudProviders()`;
  cloud templates are excluded from the API-key list (like OAuth). Selection kind
  `{kind:'cloud', presetId}` → `useProviderDialog.onCloud`. Every surface that
  renders the picker must handle the cloud kind — wired on `CredentialPage`,
  `ConnectProviderFlow`, `Onboarding`, and scenario `TemplatePage`.
- Dialog: `components/cloud/CloudProviderDialog.tsx` (separate from the
  protocol-slot `ProviderFormDialog`, like OAuth's `OAuthDialog`). Fields come from
  `cloudCredentialSchema.ts` (per-`auth_type` field schema + `buildCloudApiBase`,
  the code-side mirror of `ai.CredentialSchema`); the card identity (name/icon/
  api_style/models) comes from the template. Secrets are password fields with a
  reveal toggle; optional fields under an Advanced divider. Submits `{name,
  api_base, api_style, auth_type, credential}`.
- Vertex is two cards (Claude `api_style=anthropic`, Gemini `api_style=google`).
- `AuthTypeBadge` renders Bedrock/Vertex/Azure labels.

## 9. Templates (`internal/data/providers.json`)

Four templates: `aws-bedrock`, `gcp-vertex-claude`, `gcp-vertex-gemini`,
`azure-openai`. Each carries `auth_type`, explicit `api_style`,
`canonical_domain`, and a seeded model list (no live `/models`). Model IDs
verified against AWS/Google/Anthropic/Microsoft docs (Jul 2026): Bedrock
`anthropic.claude-opus-4-8` etc.; Vertex `claude-opus-4-8` /
`claude-haiku-4-5@20251001`; Gemini `gemini-3-flash` / `gemini-2.5-pro`; Azure
`gpt-5` / `o4-mini`.

- `ProviderTemplate.APIStyle` added; `findTemplateByProvider` now matches
  `canonical_domain` **and** `api_style`, so the two Vertex templates (same
  `aiplatform.googleapis.com`) resolve to the right model family.
- `ValidateTemplate` exempts cloud (and OAuth) templates from the base-URL rule.
- `GetProviderTemplates` serializes the full template map → new fields reach the
  frontend with no codegen change.
- The dialog's computed `api_base` (`bedrock-runtime.<region>…`,
  `<location>-aiplatform.googleapis.com`, the Azure endpoint) matches each
  template's `canonical_domain`, so the model list resolves after connect.
- Covered by `internal/data/cloud_template_test.go`.

## 10. Cross-cutting

| Concern | Behavior |
|---|---|
| Secrets at rest | SQLite `credential` text column; same posture as tokens. Encrypt-at-rest = follow-up. |
| GCP token refresh | Inside the SDK's google token source; no manual loop. |
| AWS creds | v1 = static keys or Bedrock bearer. STS/assume-role later. |
| Model listing | Template-seeded; no live fetch. |
| Timeouts/retries | Constructor defaults; adapters don't override. |
| Dual-URL | N/A for cloud (single derived endpoint). |

## 11. Follow-ups (not built)

- Cloud-aware **edit** flow + masked-secret round-trip (edit currently opens the
  generic form; responses return credentials in full).
- **Test Connection** through the signed client path.
- `GetAccessToken()` returns `""` for cloud types; the manual-header call sites
  outside the client constructors (`internal/probe/sdkprobe.go` lightweight
  probe, `internal/tbclient`) send unauthenticated requests for cloud providers.
  A central "apply credential" seam would fix all of them at once.
- Backend derivation of `api_base` from the bundle (today the frontend computes
  it; a raw-API caller must replicate the URL convention).
- Azure Entra token; AWS STS/assume-role/instance-profile; non-Claude Bedrock
  (Bedrock-native API); encrypt the credential column.

Resolved decisions: Vertex split into two cards (unambiguous `api_style`); Bedrock
leads with access-key/secret, bearer optional; encryption deferred.

## 12. Key files

| File | Role |
|---|---|
| `ai/provider.go`, `internal/data/db/provider_store.go` | Auth types, bundle, persistence — reused as-is |
| `ai/credential.go` | Field keys, `CredentialSchema`, `ValidateCredential`, `IsSecretCredentialField` |
| `internal/client/{bedrock,vertex,azure}_client.go` | Per-cloud constructors + bundle→SDK translation |
| `internal/client/{openai,anthropic,google}.go`, `pool.go` | Skip empty key / Vertex config branch / dispatch |
| `internal/server/module/provider/{types,handler}.go` | `credential` field; validate/mask; auth_type whitelist |
| `internal/data/providers.json`, `provider_template.go` | Cloud templates; `APIStyle` + api_style-aware matching |
| `frontend/src/components/cloud/{CloudProviderDialog.tsx,cloudCredentialSchema.ts}` | Cloud add dialog + field schema |
| `frontend/src/components/ConnectProviderDialog.tsx`, `services/serviceProviders.ts`, `hooks/useProviderDialog.tsx`, `components/AuthTypeBadge.tsx` | Picker section, cloud accessors, routing, badges |
| `libs/anthropic-sdk-go/{bedrock,vertex}`, `libs/openai-go/azure`, `libs/go-genai` | Vendored cloud adapters |
</content>
