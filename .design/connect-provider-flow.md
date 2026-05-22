# Connect AI Flow

Unified two-step flow for adding any AI credential — API key, OAuth sign-in, or
self-hosted inference server — replacing the old separate "Add OAuth" + "Add API Key"
buttons with a single entry point.

## Entry point

Button label: **Connect AI** (previously "Connect Provider").  
Lives in the Credential → Model Key page header.

![Credentials page](images/connect-ai-credentials.png)

---

## Step 1 — Picker ("Connect AI" dialog)

A scrollable picker grouped into four sections.  
Title, description, and search bar are **locked** (don't scroll with the card list).  
Scrollbar is always visible on the card area.

![Picker](images/connect-ai-picker.png)

**Section order:** Custom → OAuth sign-in → Self-hosted → API key providers

| Section | Grid | Badge | Notes |
|---|---|---|---|
| Custom | 1-column | Key | Single "Custom endpoint" card |
| OAuth sign-in | 2-column | OAuth (green) | Short names fit side-by-side |
| Self-hosted | 2-column | Self-hosted (amber) | Ollama, LM Studio, LocalAI, Jan, vLLM, SGLang |
| API key providers | 1-column | Key | Long localized names need full width |

**Routing on card click:**
- **Custom / Key provider / Self-hosted** → opens the form dialog (pre-filled)
- **OAuth** → opens OAuthDialog in direct mode (skips the provider grid, straight to auth)

---

## Step 2 — API key form

Layout top → bottom:

1. **Base URL \*** — required; inline error if submitted empty
2. **API Key** — password field with show/hide toggle
3. **No API Key Required** — right-aligned checkbox; disables the key field
4. **API Style** — OpenAI / Anthropic checkboxes; topology hint appears when both are checked on a dual-URL template
5. **Advanced accordion** — collapsed by default in add mode, auto-expanded in edit mode

When opened from the picker, the form shows a **← Back** button (bottom-left) that
closes the form and re-opens the picker. "Test Connection" and the submit button stay
grouped on the bottom-right.

![Form — add mode](images/connect-ai-form-add.png)

![Form — Advanced expanded](images/connect-ai-form-advanced.png)

The dialog is capped at `88vh`. The content area scrolls independently; action buttons are always pinned at the bottom.

### Self-hosted provider pre-fill

Self-hosted cards pre-fill the form based on ecosystem conventions:

| Provider | Default URL | Pre-filled API Key | Notes |
|---|---|---|---|
| Ollama | `http://localhost:11434/v1` | `ollama` | Server ignores auth by default; clients pass this as placeholder |
| LM Studio | `http://localhost:1234/v1` | `lm-studio` | LiteLLM / aider convention |
| LocalAI | `http://localhost:8080/v1` | _(empty)_ | No convention; marked Optional |
| Jan | `http://localhost:1337/v1` | _(empty)_ | No convention; marked Optional |
| vLLM | `http://localhost:8000/v1` | `EMPTY` | Official docs placeholder |
| SGLang | `http://localhost:30000/v1` | `EMPTY` | Official docs placeholder |

All six ship with auth **disabled** by default. Pre-filled keys are client-side
conventions only — the server accepts them (or nothing) when auth is off. Users
running a server with real auth configured can overwrite the field.

The `optionalEditable` prop on `ApiKeyField` keeps the field editable even when
`noKeyRequired` is true (LocalAI / Jan), so users who do configure auth can still
enter their key without unchecking a separate toggle.

---

## Design decisions

| Decision | Rationale |
|---|---|
| "Connect AI" not "Connect Provider" | Friendlier; covers OAuth, API key, and self-hosted under one verb |
| Self-hosted as separate section, not mixed into API key providers | Different auth model (optional/placeholder key, URL is the main config); amber accent distinguishes it visually |
| Static list, no auto-detection | Auto-probing localhost ports adds latency and complexity; users know what they're running |
| Pre-fill default API key per provider | Reduces friction: users with default setup click through without typing; users with real auth just overwrite |
| ← Back button when opened from picker | Users can reconsider provider choice without dismissing the whole flow |
| Back + [Test | Submit] layout via `ml:auto` | Avoids `justifyContent: space-between` + empty placeholder span |
| Header locked in picker | Description + search don't scroll away when the list is long |
| Single-column for API key providers | Localized CN names (e.g. "百度千帆 Coding Plan") overflow a 2-column grid |
| "No API Key Required" as right-aligned checkbox | Checkbox placement matches the key field it modifies; chips felt too prominent for a rare toggle |
| Advanced accordion | Proxy, user-agent, name are rarely needed; hiding them shortens the common add-key path |
| Base URL required validation | Blocks submit and shows an inline field error; clears on first keystroke |
| Empty `!` fix in test panel | Splitting an empty `details` string produced a dangling warning icon with no text |
| OAuth direct mode | Picker already chose the provider; OAuthDialog skips its own grid via `autoStartProviderId` |

---

## Key files

| File | Role |
|---|---|
| `frontend/src/components/ConnectProviderDialog.tsx` | Step 1 — unified picker; `SELF_HOSTED_PROVIDERS` constant with default keys |
| `frontend/src/components/ProviderFormDialog.tsx` | Step 2 — API key / custom form; `onBack` prop for picker navigation |
| `frontend/src/components/OAuthDialog.tsx` | Step 2 — OAuth flow; `autoStartProviderId` for direct mode |
| `frontend/src/pages/CredentialPage.tsx` | Wires picker → form routing; `fromConnectPicker` + `isLocalProvider` state |
| `frontend/src/components/providerFormDialog/ApiKeyField.tsx` | Key field; `hideCheckbox` + `optionalEditable` props |
| `frontend/src/components/providerFormDialog/ProviderAutocomplete.tsx` | Base URL field; `required`/`error`/`helperText` props |
| `frontend/src/components/providerFormDialog/VerificationResultPanel.tsx` | Test result panel; filters empty detail rows |
| `internal/data/providers.json` | Provider templates; self-hosted entries use `type: "self-hosted"` |
