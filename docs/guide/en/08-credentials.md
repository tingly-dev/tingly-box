# Credentials

Path: `/credentials`

![Credentials](../images/credentials.png)

The Credentials page is the core of Tingly-Box's configuration chain. All provider API keys and OAuth credentials are centrally managed here.

---

## Page Overview

In the sidebar, this page's group is labeled **Credential** and contains three sub-pages: **Model Key** (this page), **Sharing** (see [API Tokens](./10-api-tokens.md)), and **Virtual Models**.

The page header shows the total credential count (e.g. `Managing 5 credentials`). The top action bar includes:

| Button | Function |
|--------|----------|
| **Connect AI** | Opens the unified provider picker to add new credentials (same flow as Onboarding) |
| **Import** | Bulk-import provider configurations (JSON/YAML format) |
| **Providers** | Navigates to the Onboarding page (browse all providers) |

---

## Credential Types

### API Keys Table

Lists all providers connected via API Key:

| Column | Description |
|--------|-------------|
| Provider | Provider name and icon |
| Base URL | API endpoint address |
| Token | API Key (masked by default) |
| Status | Enabled/disabled |
| Quota | Known quota info (click to refresh) |
| Actions | Edit, delete, enable/disable |

### OAuth Table

Lists all providers connected via OAuth (e.g. Claude.ai):

| Column | Description |
|--------|-------------|
| Provider | Provider name |
| Status | Authorization status |
| Expiry | Token expiration time |
| Actions | Refresh token, edit, delete, enable/disable |

---

## Adding a Provider (the Connect AI flow)

Click **Connect AI** to open the provider picker. This is the single entry point for connecting any AI service, and it works in two steps: **pick a type, then fill in the config**.

### Step 1: Pick a provider

![Connect AI Picker](../images/connect-ai.png)

A search box (filters by name) sits at the top; below it providers are grouped by connection type, and each card carries a coloured badge marking its kind:

| Section | What's in it | What happens on click |
|---------|--------------|------------------------|
| **Custom** | `Custom endpoint` (bring your own Base URL), `Import` (from file/clipboard) | Opens a blank config form / the import dialog |
| **OAuth sign-in** | Providers that support OAuth (Claude Code, Google Gemini CLI, Codex, …) | **Launches the OAuth flow directly** — no API key needed |
| **Self-hosted** | Locally hosted services (e.g. Ollama); the card shows `localhost:port` | Opens the config form with the Base URL pre-filled but **editable** (adjust to your host/port) |
| **API key providers** | Cloud providers accessed via API key, grouped by region (CN / Global); each card shows its protocol (OpenAI · Anthropic) | Opens the config form with name and Base URL pre-filled |

> Most providers are pre-configured, so you'll only be asked for what they need. Not listed? Pick **Custom endpoint** to enter any base URL yourself.

### Step 2: Fill in the config form

![Provider Config Form](../images/connect-ai-form.png)

Choosing any non-OAuth provider opens the config form:

- **Base URL** (required): the API endpoint. Pre-filled for known providers; freely editable for Custom / Self-hosted
- **API Key** (required): the access token. For a local service with no auth, flip the **No API Key Required** toggle to skip it
- **API Style (protocol)**:
  - **OpenAI Compatible** (recommended): most endpoints speak the OpenAI API — start here unless you know otherwise
  - **Anthropic**: native Anthropic protocol
  - Both can be enabled at once (a fusion provider), letting one credential serve both OpenAI and Anthropic inbound protocols
- **Proxy URL** (optional, under advanced): route this provider through a dedicated HTTP proxy
- **User Agent** (optional, under advanced): custom request header

Click **Test** to verify connectivity, then **Save**.

> **OAuth providers are the exception**: selecting an OAuth card in step 1 jumps straight to the authorization page — there's no step-2 form, and the token is saved automatically once you authorize.

---

## Bulk Import

Click the **Import** button:

1. Select a file (JSON or YAML) or paste configuration content directly
2. Supported format example:
   ```yaml
   providers:
     - name: "My OpenAI"
       api_base: "https://api.openai.com/v1"
       api_style: "openai"
       token: "sk-..."
   ```
3. Click **Import** to confirm
4. If a provider already exists, the system prompts whether to force-overwrite (Force Add)

---

## Editing a Provider

Click the edit icon on the right of a provider row to open the edit form. You can modify:
- Name
- API Base URL
- API Key/Token
- Proxy settings
- Enabled/disabled status

---

## Enable / Disable a Provider

Each provider row has a toggle for quick enable/disable. Disabled providers will not receive new routing requests, but their configuration is retained.

---

## Related Pages

- [Virtual Models](./09-virtual-models.md)
- [API Tokens](./10-api-tokens.md)
- [Getting Started](./01-getting-started.md)
