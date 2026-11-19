# Claude Code 2.1.328 — request construction and OAuth, compared with tingly-box

Analysed **`@anthropic-ai/claude-code` 2.1.328** (npm `latest` and `next`; `stable` is 2.1.311;
build time 2026-11-18T21:07Z, git `e96a84ee`), diffed against **2.1.280**, the profile
`.design/claude-code-oauth-compat.md` currently pins. Static analysis only: the platform binary
(`@anthropic-ai/claude-code-linux-x64`) was unpacked and its embedded JS bundle read. The binary was
**not executed**, nothing was logged in, no real token or `~/.claude` / `~/.tingly-box` was touched. Everything
below is "read from the bundle" unless marked *(inferred)*; wire-level claims that need a live capture are
listed in §7.

Companion docs: [`.design/claude-code-oauth-compat.md`](../.design/claude-code-oauth-compat.md) (wire identity, upgrade checklist §B6),
[`.design/oauth.md`](../.design/oauth.md), [`.design/quota-semantics.md`](../.design/quota-semantics.md).

## 0. Summary

1. **The 2.1.280 profile is still structurally right.** The things that make a request look like Claude Code
   are unchanged: preamble sentences, fingerprint salt/indices, `metadata.user_id` shape, `cch` seed,
   Zig xxHash constant, OAuth endpoints / client id. The 2.1.280 → 2.1.328 bump is a *version, SDK, runtime and beta-order*
   update, not a redesign.
2. **Concrete drift to apply on the next bump** (§3): UA `2.1.328`, `X-Stainless-Package-Version` 0.112.1 → **0.128.0**,
   `X-Stainless-Runtime-Version` v26.3.0 → **v26.12.1** *(inferred from the embedded Bun 1.4.4 `node/v26.12.1`)*,
   `thinking-token-count` **moved out of the baseline** (changes beta order), several new betas / billing-header
   fields / request headers.
3. **OAuth: our endpoints are the old ones.** CC uses `claude.com/cai/oauth/authorize` and
   `platform.claude.com/v1/oauth/token`; tingly uses `claude.ai/oauth/authorize` and `api.anthropic.com/v1/oauth/token`,
   omits `org:create_api_key` / `user:plugins` scopes on login, sends no `scope` on refresh, sends an empty `client_secret`,
   and fakes a Chrome UA + `Origin: claude.ai` on token calls. It works today; it is the part most likely to be
   cut off or behave differently (§4).
4. **The bundle embeds Anthropic's own "Claude Code gateway protocol" document** (26 KB markdown). It states
   what a gateway must do for Claude Code: pass unknown body fields / `anthropic-beta` values through, never reject on
   unknown headers, `x-should-retry: false` on terminal 401/429, `anthropic-ratelimit-unified-*` response headers
   for usage notices, `GET /api/oauth/usage` shape, `/v1/models`, count_tokens behaviour (§5). tingly-box implements
   none of the error / usage-header conventions.
5. **Biggest untapped signal for quota:** every Claude subscription response carries
   `anthropic-ratelimit-unified-{5h,7d,overage}-{status,utilization,reset,surpassed-threshold}` headers. tingly only
   polls `/api/oauth/usage`; it never reads these headers (§3.9).

## 1. Method (reproducible, read-only)

```bash
export npm_config_cache=<shared cache>
npm view @anthropic-ai/claude-code version dist-tags time.modified      # 2.1.328, latest/next; stable 2.1.311
npm pack @anthropic-ai/claude-code@2.1.328                              # wrapper: install.cjs + cli-wrapper.cjs only
npm pack @anthropic-ai/claude-code-linux-x64@2.1.328                    # 373 MB Bun single-file binary
# JS source is plaintext in the ELF (headers "// @bun @bytecode ... // Version: 2.1.328" per chunk).
# Slice it with the script in claude-code-oauth-compat.md §B2.2 (62 MB for 2.1.328, 38 MB for 2.1.280).
```

Bundle layout changed slightly: several `// @bun @bytecode` chunks (`/$bunfs/root/chunk-*.js`) rather than one
file, but the source text is still greppable. Symbol names are minified and change every release; anchor on string
literals (`"x-app"`, `x-anthropic-billing-header: cc_version`, `CONSOLE_AUTHORIZE_URL`, `DISABLE_INTERLEAVED_THINKING`).
A string-set diff (beta names, `x-*` headers, URLs, API paths, env vars) between 2.1.280 and 2.1.328 found 8 new dated betas,
~100 new `x-*`/`anthropic-*` literals (most are CCR/memory/runner internals), 34 new API paths, ~357 new env vars.

Pitfall: running the downloaded binary (to capture real requests as §B2.4 describes) was refused by the
sandbox classifier in this session; that is why §7 lists captures as open.

## 2. Our current assumptions (from `.design/`)

| Area | Assumption (source) |
|---|---|
| Identity | Sign Claude OAuth provider requests as the official CLI: pinned UA `claude-cli/<v> (external, cli)`, "interactive terminal on api.anthropic.com" persona, preamble `You are Claude Code…`, billing header `x-anthropic-billing-header: cc_version=<v>.<fp>; cc_entrypoint=cli; cch=<xxh64>;` as `system[0]` (compat §B0–B5). |
| Versions | Only Legacy (2.1.86) + one native version (2.1.280); upgrade in place when Anthropic raises the minimum (`claude_code_version_too_old`, compat §B6). |
| Betas | Composed per request: baseline by model + body-derived + whitelisted replay of inbound flags; single header value in official push order (compat §B3.2). |
| `cch` | xxHash64 (Zig `PRIME64_4`), seed `0x4D659218E32A3268`, over final wire bytes with model blanked and `max_tokens` removed (compat §B3.3.4). Server-side verification unproven. |
| Not synthesized | `cc_prev_req`, `cc_prompt_id`, `cc_turn_origin` (needs cross-request state); only replayed if inbound. |
| OAuth | Authorization-code + PKCE against `claude.ai/oauth/authorize` / `api.anthropic.com/v1/oauth/token`, JSON token requests, scopes `user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload`, refresh 5 min buffer, ticker 2 min, window 15 min (`oauth.md`, `ai/oauth/registry.go`). |
| Quota | Poll `GET /api/oauth/usage` (+ `anthropic-beta: oauth-2025-04-20`), both `limits[]` and legacy shapes decoded (`ai/quota/fetcher/anthropic.go`); hourly nudge request to start windows (`quotawindow`). |
| Models | `internal/catalog/claude.models.json` lists 24 ids incl. fable-5/5-1/5-2, opus-5/5-5, sonnet-5/5-5, haiku-5-5. |

## 3. Request construction in 2.1.328

### 3.1 Client identity — unchanged / bumped

| Item | 2.1.280 (pinned) | 2.1.328 | Note |
|---|---|---|---|
| `User-Agent` | `claude-cli/2.1.280 (external, cli)` | `claude-cli/2.1.328 (external, <entrypoint>[, agent-sdk/…][, client-app/…][, workload/…])` | `-p` = `sdk-cli`; extra segments only when env set. |
| SDK (`X-Stainless-Package-Version`, SDK UA `Anthropic/JS`) | 0.112.1 | **0.128.0** | the only `"0.1xx.x"` version literal in the bundle. |
| `X-Stainless-Runtime-Version` | v26.3.0 (Bun 1.4.3) | **v26.12.1** *(inferred)* | Bun 1.4.4's embedded `npm_config_user_agent` is `bun/1.4.4 … node/v26.12.1`; same mapping held for 2.1.280 (`bun/1.4.3 … node/v26.3.0`). Confirm with a capture. |
| `X-Stainless-Lang/OS/Arch/Runtime/Retry-Count/Timeout` | same | same | OS names `MacOS/Linux/Windows`, arch `x64/arm64/x32`; timeout header = `trunc(timeout/1000)` = 600 default. |
| `anthropic-version` / `anthropic-dangerous-direct-browser-access` / `Accept` / `x-app` | `2023-06-01` / `true` / `application/json` / `cli` (`cli-bg` for bg sessions) | same | `dangerouslyAllowBrowser:true` still set. |
| `X-Claude-Code-Session-Id` | uuid | uuid | |
| Auth | subscriber → `authToken` (Bearer) + oauth beta; else `x-api-key` | same (`Ttl`: `isSubscriber ? {authToken} : {apiKey}`) | |
| `metadata.user_id` | JSON string `{device_id, account_uuid, session_id[, parent_session_id][, ti][, tk]}` | **unchanged** (`CLAUDE_CODE_EXTRA_METADATA` merged, trimmed if too long; `tk` stripped from `CLAUDE_CODE_EXTRA_BODY`) | |
| Preamble | 3 sentences | **unchanged** (`vertex` always sentence 1; non-interactive + append-system-prompt → sentence 2; else non-interactive → sentence 3) | |
| Fingerprint | `sha256("59cf53e54c78"+text[4,7,20]+version)[:3]` | **unchanged** (first non-meta user message, first text block) | |
| `cch` seed | `0x4D659218E32A3268` | **unchanged** (single occurrence in the binary; no old seed; Zig `PRIME64_4` 14×, standard 0×) | preimage rules live in native code → not verifiable statically. |

### 3.2 New / changed request headers

Built in the client factory (`d0`), merged with `ANTHROPIC_CUSTOM_HEADERS`. "Direct" = `firstParty && base URL is api.anthropic.com`
(or `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL`).

| Header | Value / when | tingly today |
|---|---|---|
| `x-claude-code-request-class` | `main` / `subagent` / `auxiliary` / `compaction` / `workflow`; direct or `CLAUDE_CODE_GATEWAY_HINT_HEADERS` | default `main`, replays inbound ✅ |
| `x-claude-code-agent-type` | builtin agent name / `custom` / `teammate` (agent query sources only) | replays inbound ✅ |
| `x-claude-1p-request-class` | **new vs 2.1.280.** `main` / `blocking-subagent` / `background-task` / `summary-extraction` / `followup-generation` (same gate) | ❌ not sent |
| `x-claude-code-auxiliary-type` | **new.** per-source label for auxiliary calls (same gate) | ❌ |
| `x-claude-code-prompt-id` | **new as header.** the prompt uuid (same value as `cc_prompt_id`) when valid uuid | ❌ |
| `x-claude-code-agent-id`, `-parent-agent-id` | percent-encoded ids | replays inbound ✅ |
| `x-client-request-id` | per-request uuid, direct only (present in 2.1.280 too) | ❌ never sent |
| `traceparent` | only when tracing enabled | n/a |
| `anthropic-usage-limit` | `extended` after a "usage limit" rejection grace-resend, `slow` for the slow lane | ❌ (new semantics, see §3.9) |
| `x-cc-fallback-from-model`, `-fallback-category`, `-fallback-trigger`, `x-cc-original-request-id` | server-side model-fallback stamping, direct only | ❌ |
| `x-cc-context-compacted` / `x-claude-code-context-compacted`, `x-cc-compaction-request` / `x-claude-code-compaction`, `x-claude-code-prev-tool-durations` | compaction / tool-latency hints (direct, or hint-header mode) | ❌ |
| `x-claude-code-managed-settings-checksum` | only with `CLAUDE_CODE_GATEWAY_HINT_HEADERS` | n/a |
| `x-client-app`, `x-claude-remote-*`, `x-cowork-enrollment`, `anthropic-client-platform/version` (desktop), `x-anthropic-additional-protection` | embedder / CCR / desktop only | n/a |

Gate for the hint headers (`HJr`): `CLAUDE_CODE_GATEWAY_HINT_HEADERS` if set; else direct first-party; else GrowthBook `tengu_splendid_sutton`
(default false). So a user pointing real CC at tingly sends **none** of the direct-only headers and **no `cch`/`cc_prompt_id`** — consistent with
the existing "re-sign as direct persona" design.

### 3.3 `anthropic-beta`

Registry (`rPi`) now has 48 entries. Dated betas added since 2.1.280: `cache-keepalive-2026-09-03`, `claude-code-side-query-2026-09-29`,
`program-use-case-ids-2026-09-17`, `request-tags-2026-08-21`, `resumption-2026-10-08`, `tool-provenance-2026-10-26`,
`user-profiles-2026-08-18` (replaces `-2026-03-24`), `web-search-results-only-2026-11-15`; removed: `skills-2025-10-02`.
Others present (undated or older): `advisor-tool-2026-03-01`, `server-side-fallback-2026-06-01/-07-01`, `fallback-credit-2026-06-01`,
`auto-mode-classifier-2026-07-16`, `mcp-servers-2025-12-04`, `files-api-2025-04-14`, `environments-2025-11-01`, `ccr-byoc-2025-07-29`.

**Baseline (`W1`) in 2.1.328**, in push order:

1. `claude-code-20250219` — non-haiku
2. `oauth-2025-04-20` — subscriber login
3. `context-1m-2025-08-07` — **gate changed**: model supports 1M **and** not "1M by its own name" (`$y(model) && !CP(model,{byItsOwnName:true})`). `VXo` later drops it again when `iS()` and re-appends it at the **end** when the model selection needs it.
4. `interleaved-thinking-2025-05-14`
5. `redact-thinking-2026-02-12` — first-party, interleaved-capable, interactive
6. `context-management-2025-06-27`
7. `structured-outputs-2025-12-15` — GrowthBook `tengu_tool_pear` (unchanged)
8. `web-search-2025-03-05` — vertex/foundry
9. `prompt-caching-scope-2026-01-05` — first-party **direct** only
10. `mid-conversation-system-2026-04-07` — registered with a null header (`hh`) → not sent as a header any more

Then per-turn (`z1`): `per-turn-control`, `timing`, `mid-conversation-tool-changes`, `inline-tools`, `tool-provenance`; then `ANTHROPIC_BETAS`; then in the main loop:
`program-use-case-ids`, **`advisor-tool`**, **`thinking-token-count-2026-05-13`**, sticky betas, tool-search (`advanced-tool-use-2025-11-20`), `prompt-caching-scope` (if not yet),
then in `VXo`: `claude-code-side-query`, `web-search-results-only`, `effort-2025-11-24`, `task-budgets`, `structured-outputs` (from `format`), `dangerous-tool-use`,
`thinking-binding-controls`, `thinking-display-updates`, `resumption-2026-10-08` or `thinking-resumption`, `fast-mode`, `afk-mode`, `extended-cache-ttl`,
`prompt-caching-evict`, `cache-diagnosis`, `cache-keepalive`, `mid-conversation-system-clear-at`.

**Order change that matters for our synthesized header:** in 2.1.280 `thinking-token-count` sat in the baseline right after `interleaved-thinking`
(`...interleaved, thinking-token-count, context-management, prompt-caching-scope, effort...`). In 2.1.328 it is **no longer in `W1`**; it is pushed per query when
the model's capability table says thinking is supported, i.e. *after* `prompt-caching-scope` (predicted `claude-code, oauth, interleaved, context-management, prompt-caching-scope, thinking-token-count, effort, …` *(inferred, needs capture)*).
tingly's `claudeCodeBetaEmissionOrder` still encodes the 2.1.280 order.

Replay whitelist gaps (`claude_betas.go`): tingly knows 30 flags; missing vs registry: `cache-keepalive`, `resumption`, `advisor-tool`, `server-side-fallback*`,
`fallback-credit`, `program-use-case-ids`, `claude-code-side-query`, `request-tags`, `tool-provenance`, `web-search-results-only`, `auto-mode-classifier`.
Inbound real-CC traffic carrying these would have the beta dropped while the matching **body field** (§3.5) may still be forwarded.

### 3.4 Billing header (`system[0]`)

`x-anthropic-billing-header: cc_version=<v>.<fp>; cc_entrypoint=<ep>;` then, in order, each only when its gate holds:
` cch=00000;` (direct first-party or vertex), `cc_workload`, `cc_is_subagent=true`, `cc_prev_req` (direct), `cc_prompt_id` (direct),
`cc_turn_origin` (direct, `^[a-z][a-z_]{0,31}$`), **`cc_prompt_index=<n>; cc_turn_index=<n>;`** *(new)*, **`cc_gateway=1|env;`** *(new, gateway mode)*,
**`cc_project=<id>; cc_project_src=<src>;`** *(new, `claude_proj_…`/`chan_…` ids, subscriber)*, **`cc_prev_msg=msg_…;`**, **`cc_prev_prompt_index; cc_prev_turn_index`** *(new, after compaction)*,
**`cc_dispatch_entrypoint=<x>;`** *(new)*. `CLAUDE_CODE_ATTRIBUTION_HEADER`-style opt-out unchanged (`GM()`).
tingly's `billingHeaderPreservedFields` only knows the 2.1.280 set; the new keys would be dropped from an inbound header (harmless — not synthesized either way).

A side note: the bundle has `function Atl(e){return e.replaceAll("cch=","cch\\u003d")}` used by the runner's nested-call forwarder, i.e. a client that re-signs a
body escapes literal `cch=` inside user content so it cannot be mistaken for the placeholder. tingly's `claude_cch.go` instead locates the placeholder only inside the top-level `system` billing block; equivalent for our purpose.

### 3.5 Body fields seen in the request builder

Same core as before (`model`, `messages`, `system` blocks with `cache_control {type:"ephemeral"[,ttl]}`, `tools`, `metadata`, `max_tokens`, `stream:true`, `thinking`, `context_management.edits[clear_thinking_20251015]`,
`output_config{effort,format,task_budget}`, `speed:"fast"`, `diagnostics`). New or newly visible:

- `thinking.display`: `omitted`, `summarized`, **`highlights`**, `updates`; `thinking.block_binding {prefix_mismatch_behavior}` (with `thinking-binding-controls`).
- `resumption:{type:"enabled"}` (+ `resumption-2026-10-08`), `fallback_credit_token`, `program_use_case_id`, `tags` (billing group), `safeguards` (auto mode; reply `safeguard_results`).
- `cache_control.evict_on_complete:true` (+ `prompt-caching-evict`), `cache_control.scope:"global"`.
- Thinking default: adaptive for adaptive-capable models, else `enabled` with `budget_tokens` clamped to `[1024, max_tokens-1]`; `disabled` only on first-party when allowed.
- Non-streaming fallback exists (`CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK`); count_tokens answered by a normal minimal-output Haiku request when signed into a gateway (§5).

### 3.6 Models

`claude-{fable-5,5-1,5-2}`, `claude-{mythos-5,5-1,5-2}`, `claude-{opus-4…4-8,5,5-5}`, `claude-{sonnet-4…4-6,5,5-5}`, `claude-{haiku-4-5,5-5}` plus legacy 3.x.
`claude.models.json` covers all except `mythos-*` (restricted tier). Only new-vs-2.1.280 ids: `fable-5-2`, `mythos-5-2`, `haiku-5-5`, `sonnet-5-5` — already in our catalog.

### 3.7 Retries, timeouts, streaming

- SDK layer: retry on 408/409/429/≥500, `x-should-retry` honoured, `retry-after(-ms)` honoured, default backoff `min(0.5·2^n, 8s)` with −25% jitter; a **401 triggers one token-cache invalidate + retry**.
- App layer (`$ze`): default `maxRetries` **10** (300 in watchdog mode), clamp 15 via `CLAUDE_CODE_MAX_RETRIES`; 401 → force OAuth refresh and rebuild the client (bounded consecutive attempts; refresh-lock timeout / disk-full / on-hold are distinct terminal errors); 429 `unified-status: rejected` → "usage-limit grace resend" with `anthropic-usage-limit: extended`; 529/overload capacity-wait and model fallback logic.
- Timeouts: request timeout 600 s default (`API_TIMEOUT_MS`), stream idle ≥ 300 s (`CLAUDE_STREAM_IDLE_TIMEOUT_MS`, cap 1800 s), `CLAUDE_STREAM_FIRST_BYTE_TIMEOUT_MS`.

### 3.8 Telemetry / auxiliary endpoints (all `api.anthropic.com`)

`POST /api/event_logging/v2/batch` (batch ≤200, 10 s timeout, backoff 0.5→30 s), `POST /api/claude_code/metrics`, `GET /api/claude_cli/bootstrap`,
`GET /api/claude_code/policy_limits`, `GET /api/claude_code/settings`, `GET /api/oauth/profile`, `POST /api/oauth/org`, `POST /api/oauth/validate`,
`POST /api/eval/sdk-zAZezfDKGoZuXXKe` (GrowthBook now served by Anthropic; `cdn.growthbook.io` is gone), `/v1/messages/cache_touch` (new, cache keep-alive),
`/api/claude_code_shared_session_transcripts`, `/api/oauth/files/*`, `/api/oauth/file_upload`, `/v1/code/*` (CCR/remote). `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` skips bootstrap/GrowthBook.

### 3.9 Rate-limit response headers (subscription)

`anthropic-ratelimit-unified-`: `status` (`allowed|allowed_warning|rejected`), `reset` (unix s), `representative-claim` (`5h|7d|overage|…`), `fallback` (`available`),
`5h-{status,utilization,reset,surpassed-threshold}`, `7d-{…}` (**`5h-status` / `7d-status` are new**), `overage-{status,reset,utilization,disabled-reason,in-use,period,scope,daily-cap-paused,surpassed-threshold,period-monthly-utilization,period-channel-utilization}`,
`grace-{status,5h-utilization,7d-utilization}`, `upgrade-paths`, `slow-{status,offer,budget-reset,budget-utilization,max-wait,retry-after}`.
The CC "quota check" is just `max_tokens:1` with content `"quota"` whose response headers are parsed — the same trick our hourly `quotawindow` nudge uses, but we throw the headers away.

Usage endpoint variants called by CC: `/api/oauth/usage`, `?skip_spend=1`, `?cedar_ember=1&skip_spend=1`, `?at_wall=1…`, `?starry_waterfall=<surface>&skip_spend=1`
(`limits[]` rows `{kind: session|weekly_all|weekly_scoped, group, percent, resets_at, scope{model|surface{display_name}}, severity, is_active}` + `extra_usage{is_enabled, monthly_limit, used_credits, utilization, currency}`).
Our decoder already matches this schema; `spend` block and legacy shape are handled, `seven_day_{opus,sonnet,cowork,oauth_apps,omelette,overage_included}` legacy keys exist in the bundle.

## 4. OAuth in 2.1.328 vs tingly-box

### 4.1 What CC does (read from the bundle; endpoints identical to 2.1.280)

| Item | Claude Code | tingly-box (`ai/oauth/registry.go`, `hook.go`, `manager.go`) |
|---|---|---|
| Client id | `9d1c250a-e61b-44d9-88ed-5944d1962f5e` (public, no secret) | same ✅ |
| Authorize URL | claude.ai accounts: **`https://claude.com/cai/oauth/authorize`**; Console accounts: `https://platform.claude.com/oauth/authorize` | `https://claude.ai/oauth/authorize` ⚠ old host |
| Authorize params | `code=true`, `client_id`, `response_type=code`, `redirect_uri`, `scope` (space separated), `code_challenge`, `code_challenge_method=S256`, `state`, optional `orgUUID`, `login_hint`, `login_method` | `code=true`, `response_type=code` + PKCE ✅; no `orgUUID` / `login_hint` / `login_method` |
| Redirect | `http://localhost:<free port>/callback`; manual copy-paste fallback `https://platform.claude.com/oauth/code/callback` | fixed preferred port 54545 `/callback`; no manual-code fallback |
| Scopes (login) | `org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload` + `user:plugins` + `user:privacy_consents:write` (the last two behind per-env "SCOPE_REGISTERED" flags, both `true` in prod); `claude setup-token` asks **only `user:inference`** with `expires_in=31536000` (1 year); extra scopes `user:projects:read/write` only for specific apps | `user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload` ⚠ missing `org:create_api_key`, `user:plugins` |
| Token URL | **`https://platform.claude.com/v1/oauth/token`**, JSON body, no browser headers (axios defaults) | `https://api.anthropic.com/v1/oauth/token` ⚠ different host; JSON ✅; sends Chrome UA, `Origin`/`Referer: claude.ai` ⚠ |
| Exchange body | `{grant_type:"authorization_code", code, redirect_uri, client_id, code_verifier, state[, expires_in]}` | equivalent (generic builder) |
| Refresh body | `{grant_type:"refresh_token", refresh_token, client_id, scope: <user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload user:plugins>[, expires_in]}`; on `400 invalid_scope` retries once with defaults | `{grant_type, refresh_token, client_id, client_secret:""}` ⚠ no `scope`; empty secret key sent |
| Token response | `access_token`, `refresh_token` (kept if absent), `expires_in`, **`refresh_token_expires_in`** (new; refresh path stores it, absent ⇒ unknown), `scope`, `account{uuid,email_address}`, `organization{uuid}` | `AnthropicTokenResponse` has access/refresh/expires_in/org/account; **no refresh-token expiry** |
| Refresh timing | refresh when `now + 5 min >= expiresAt` (`DD`) — on demand at request time + after any 401; file-lock + CAS save (`claudeAiOauth` in `~/.claude/.credentials.json` / keychain) so concurrent CC processes don't double-refresh | background ticker 2 min, window 15 min, buffer 5 min ✅ (stronger than CC's on-demand); no cross-process lock needed |
| 401 handling | one forced refresh + rebuilt client (SDK `tokenCache.invalidate()` retry, app-level up to 2); `invalid_grant` ⇒ "OAuth refresh token is no longer valid; run /login"; a `400/401/403` token/refresh reply carrying a specific `error_uri` body ⇒ **"OAuth account is on hold"** (typed error holding a URL; the exact marker string was not extracted) | 401/403 ⇒ failover trigger ✅; "on hold" not recognised |
| Post-login | token response already carries account/org uuid; `GET /api/oauth/profile` (Bearer; plan: `organization_type` `claude_max/pro/team/enterprise` → max/pro/team/enterprise, `rate_limit_tier`, `billing_type`, seat tier, `plan_display_name`); `GET /api/oauth/claude_cli/roles`; `POST /api/oauth/org`; API-key path `POST /api/oauth/claude_cli/create_api_key` → `raw_key` (needs `org:create_api_key`) | `AfterToken` calls `GET https://api.anthropic.com/v1/account` (not an endpoint CC uses; silently ignored on failure) ⚠; account id/email otherwise from token response ✅ |
| Storage | `{claudeAiOauth:{accessToken,refreshToken,expiresAt,refreshTokenExpiresAt,scopes,subscriptionType(+Source,VerifiedAt),rateLimitTier,clientId}}`; env `CLAUDE_CODE_OAUTH_TOKEN` (setup-token), `…_FILE_DESCRIPTOR`, `CCR_OAUTH_TOKEN_FILE` | `OAuthDetail` in provider store |
| Sending the token | `Authorization: Bearer <access>` + `anthropic-beta: oauth-2025-04-20` (first in beta list after `claude-code-…`), **no `x-api-key`** | same ✅ |
| Local/staging override | `CLAUDE_CODE_CUSTOM_OAUTH_URL` (allow-list only), `CLAUDE_CODE_OAUTH_CLIENT_ID` | n/a |

### 4.2 Usage / plan endpoints

`GET /api/oauth/usage` (Bearer; `Content-Type: application/json`; **5 s timeout**; with `refreshOAuth` on 401) is what `/usage` and the status line use. It accepts query variants (`skip_spend=1`, `at_wall=1`, `starry_waterfall=<surface>`, `cedar_ember=1`)
that change which parts of the body are computed — `skip_spend=1` is the cheap "windows only" read. tingly sends no query params, so it always pays for the spend computation and any
`spend`/`extra_usage` failures hit the whole call. The 2.1.328 `/usage` schema matches our decoder (`limits[]`, `extra_usage`, `spend`).

### 4.3 What this means

- No evidence in the bundle that the old `claude.ai` / `api.anthropic.com` OAuth hosts were retired, but CC itself has moved off them (both 2.1.280 and 2.1.328 use the new hosts), so
  they are no longer exercised by the official client; behaviour (rate limits, scopes granted, `invalid_scope`, future removal) will track the official hosts first.
- Because refresh sends `scope` in CC and the server can answer `invalid_scope`, any change in the granted-scope set on Anthropic's side first shows up on CC's path. tingly's scope-less refresh keeps the original grant, which is fine, but it means tingly-issued tokens will never gain `org:create_api_key` / `user:plugins` until re-login.
- `refresh_token_expires_in` is new information we discard; the refresher's "give up after ~72 h expired" rule and the `ExpiresAt=""` handling in `oauth.md` are compatible with it.

## 5. The embedded "Claude Code gateway protocol" (Anthropic's own spec for gateways)

The bundle ships `# Claude Code gateway protocol` (also published at `code.claude.com/docs/en/llm-gateway-protocol`; the page could not be fetched from the sandbox, the text above comes from the binary).
It is written for a gateway in front of Claude Code, which is exactly tingly-box's position. Key normative points and tingly status:

| Spec point | Detail | tingly status |
|---|---|---|
| Don't reject the unknown | "New optional request parameters, body fields and headers can appear in any Claude Code release"; on `/v1/messages` pass body fields **and `anthropic-beta` values to the upstream unchanged** (except request tags); forward or drop other headers but never fail over them | We **filter** `anthropic-beta` through a whitelist for the Claude OAuth upstream and rebuild body via typed SDK structs → a new CC feature can be silently stripped or half-forwarded. Needs a per-field audit (§6 G4). |
| Errors | Anthropic envelope `{"type":"error","error":{type,message}}`; `x-should-retry: false` with 401 for a revoked/expired bearer (otherwise CC retries with backoff for **~3 minutes** before prompting) and with 501; 429 needs `Retry-After`; 429 `billing_error` for caps | gateway-generated errors never set `x-should-retry` (grep: no occurrences) → CC users hitting a disabled rule / bad token wait through retries. |
| Usage-limit headers | `anthropic-ratelimit-unified-{status,representative-claim,overage-status,overage-utilization,overage-surpassed-threshold,reset,overage-reset,overage-period,overage-disabled-reason,overage-scope}`; strip the upstream's own `anthropic-ratelimit-*` first so org-wide quota doesn't look like the user's; CC shows "You've used NN% … resets …" past 75 % / 95 % | none emitted; we also currently forward upstream `anthropic-ratelimit-*` verbatim *(not verified in code, worth checking)* |
| `GET /api/oauth/usage` on the gateway | lets CC show dollars: `extra_usage` (cents) + `limits[]` row per cap, `utilization/percent` in whole percents, `resets_at` ISO-8601 identical (to the second) to the `-overage-reset` header | not served |
| `GET /v1/models` | Anthropic list shape, ids `claude-{family}-{major}-{minor}`; used only when `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` | served in Anthropic shape ✅ (check id style for routed aliases) |
| `/v1/messages/count_tokens` | answer or `501 not_supported` + `x-should-retry: false`; CC falls back to a billed minimal Haiku request | we proxy count_tokens (`anthropic_count_tokens.go`) ✅ |
| Auto mode | adds a beta and top-level `safeguards`; reply `safeguard_results`; pass all three or answer `400` naming `safeguards` so CC falls back to its own check | untested; risk of a half-forwarded request |
| Request tags | `tags:{<name>:<group>}` body + `request-tags-2026-08-21` beta; Bedrock/Mantle/Vertex reject both → strip before them | not handled (only matters for gateway-`/login` sessions, i.e. not tingly today) |
| Bedrock/Vertex/Foundry | translate model ids, move the betas Bedrock rejects into the body as `"anthropic_beta": [...]`, emit own `ping` events during silent gaps, `count_tokens` via CountTokens, relay `prompt is too long` / exact `Input is too long.` messages (CC's auto-compact keys on them) | partially covered by `protocol-transform.md`; **error-message relay for compaction** is worth a test |
| Telemetry | `POST /v1/{metrics,logs,traces}` return 200 even if discarded (404 makes the exporter log errors every flush) | not served; irrelevant while users point `ANTHROPIC_BASE_URL` only |
| Managed settings, device-flow OAuth | `/.well-known/oauth-authorization-server`, `/oauth/device_authorization`, `/oauth/token`, `/managed/settings` (+ checksum hint header) | not implemented; optional, only for `/login`-to-gateway mode (CC sends `User-Agent: claude-code/<ver>`, `surface=claude_code`) |
| TLS | CC pins the SHA-256 of a gateway's leaf cert per host after first confirmation | n/a unless we ever implement `/login`-to-gateway |

## 6. Gap list (priority = risk of breakage × user visibility)

| # | Pri | Gap / outdated assumption | Evidence | Suggested change |
|---|---|---|---|---|
| G1 | **P1** | Native profile is two releases behind (2.1.280 vs latest 2.1.328; `stable` is 2.1.311). No rejection observed here, but the server gates on UA version (`claude_code_version_too_old`) and the lag grows every release. | §3.1, §3.3 | Bump per compat §B6: UA 2.1.328, SDK **0.128.0**, runtime **v26.12.1** *(confirm by capture)*, beta emission order (`thinking-token-count` after `prompt-caching-scope`, `context-1m` placement/gate), new replay-able betas, new billing-header keys preserved, replay `x-claude-1p-request-class` / `x-claude-code-auxiliary-type` / `x-claude-code-prompt-id`, synthesize `x-client-request-id` (uuid per request, direct-persona only). Keep "Legacy + latest" rule. |
| G2 | **P1** | Whitelist-based `anthropic-beta` filtering plus typed-struct body rebuild can strip a beta but keep its body field (or the reverse) for real 2.1.328 clients routed through a Claude OAuth provider: `resumption`, `fallback_credit_token`, `program_use_case_id`, `safeguards`, `tags`, `thinking.display:"highlights"`, `thinking.block_binding`, `cache_control.evict_on_complete`. Anthropic's spec says to pass unknown body fields and betas through unchanged. | §3.3, §3.5, §5 | Audit with the replay harness (`go run ./cli/harness agent claude --upstream real`/fake) using a real 2.1.328 client; decide per field: forward both (flag + field) or drop both. Add the missing betas to `claudeCodeClientReplayableBetas` in the same change. |
| G3 | **P1** | OAuth uses superseded hosts and a thinner scope set than the official client; refresh differs (no `scope`, empty `client_secret`, no `invalid_scope` retry); token calls spoof a browser (`Origin: claude.ai`, Chrome UA) instead of the CLI's plain JSON POST. | §4.1 | Switch `AuthURL` → `https://claude.com/cai/oauth/authorize`, `TokenURL` → `https://platform.claude.com/v1/oauth/token`; add `org:create_api_key` (+ optionally `user:plugins`); send `scope` on refresh and retry once without/with defaults on `invalid_scope`; drop empty `client_secret` for Claude; drop the browser header spoof. **Gate behind an experiment/flag and keep old values as fallback** — cannot be verified without a real login (not done here). |
| G4 | P2 | Gateway-generated errors lack the conventions CC expects: no `x-should-retry: false` on terminal 401/403/501/400s, no `Retry-After` on our 429s, Anthropic envelope not guaranteed on every error path. A CC user with a revoked/disabled rule waits ~3 min of retries. | §5 | Central error writer for the Anthropic-protocol routes (`internal/protocolserver`): envelope + `x-should-retry` + `Retry-After`; test with CC `-p`. |
| G5 | P2 | Quota only by polling `/api/oauth/usage` (and without `skip_spend=1`). Subscription responses already carry `anthropic-ratelimit-unified-*` for free; CC's own quota check is a 1-token request whose headers are parsed. | §3.9, §4.2 | Passive quota update from response headers (5h/7d utilization+reset+status, overage status) in the Claude OAuth round-tripper; use `?skip_spend=1` for the cheap poll; read `rejected` status to open the circuit immediately instead of waiting for a 429 loop. |
| G6 | P3 | CC-facing usage UX: we emit none of the unified headers and don't serve `/api/oauth/usage`, so CC shows nothing about tingly's own limits. | §5 | Optional: when a tingly limit/budget exists, emit `…-unified-status/overage-*` per the spec and `GET /api/oauth/usage`. |
| G7 | P3 | `AfterToken` calls `https://api.anthropic.com/v1/account` (not an endpoint CC uses; failure is swallowed). Plan/tier is not captured. | §4.1 | Use token-response `account`/`organization` (already present) and `GET /api/oauth/profile` for plan (`organization_type`, `rate_limit_tier`) to show Pro/Max/Team in the UI and to tune quota display. |
| G8 | P3 | `refresh_token_expires_in` and "account on hold" error are not modelled. | §4.1 | Store refresh expiry when present; map hold errors to a distinct re-auth state. |
| G9 | P3 | Docs: `.design/claude-code-oauth-compat.md` §B0/§B8 stop at 2.1.280; `.design/oauth.md` says nothing about the official endpoints. | — | Add a §B9 (2.1.280 → 2.1.328 diff) after G1 lands; fold §4 of this note into `oauth.md`. |

### Risks worth keeping in view

- **`cch` server-side verification is still unproven** (compat §B7): seed and Zig constant unchanged in 2.1.328 (seed at one offset, no old seed, Zig `PRIME64_4` 14×), but the preimage rules live in native code and cannot be re-verified statically.
- **Beta order is part of the fingerprint if the server compares strings**; the `thinking-token-count` relocation is inferred from the JS push order, not captured.
- **Fake browser headers on token calls** (G3) are the opposite of what the official client sends; if Anthropic tightens bot checks on the OAuth host this is the first thing that fails.
- Whitelisted betas / typed bodies (G2) fail *quietly* when Anthropic adds a field: the request still 200s but a feature (resumption, fallback credit, safeguards) silently stops working.

## 7. Not verified / needs a live capture or real credentials

1. Real wire capture of 2.1.328 (interactive + `-p`, OAuth + API key, proxy + forced direct) per compat §B2.4 — blocked here because executing the downloaded binary was denied by the sandbox. Needed to confirm: runtime version, beta string/order, billing header keys, `x-claude-1p-request-class` etc., `cch` preimage.
2. Whether the server still accepts `claude.ai/oauth/authorize` + `api.anthropic.com/v1/oauth/token` for new logins (G3) and what scopes a login without `org:create_api_key` yields.
3. The published `llm-gateway-protocol` page (fetch blocked) — compare with the embedded copy; the embedded one is the authoritative version for 2.1.328.
4. Whether tingly forwards upstream `anthropic-ratelimit-*` response headers to clients (not checked in code).
5. GrowthBook-gated behaviour (`tengu_*`) — values per user are unknowable from the bundle.

## 8. Suggested follow-up tasks (small, independently shippable)

1. **Profile bump 2.1.328** (G1): constants, beta order, preserved billing keys, hint headers, `x-client-request-id`, tests/captures, design doc §B9. Needs permission to run the downloaded binary against a fake API.
2. **Pass-through audit for new CC fields/betas** (G2): harness replay with a 2.1.328 client; fix whitelist/typed-body gaps.
3. **OAuth endpoint/scope/refresh alignment behind a flag** (G3, G7, G8).
4. **Gateway error conventions** (G4): envelope, `x-should-retry`, `Retry-After`; CC `-p` test.
5. **Passive quota from `anthropic-ratelimit-unified-*` + `skip_spend`** (G5).
6. *(Optional)* **Serve usage headers / `/api/oauth/usage` to CC clients** (G6).
