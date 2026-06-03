# Getting Started

This chapter guides you through the first-time startup and provider setup so that all agent scenarios are ready to use.

---

## First Launch

When you access the Tingly-Box Web UI for the first time, the system detects that no providers are configured and automatically redirects to the **Onboarding** page at `/onboarding`.

---

## Onboarding Page

Page title: **Welcome to Tingly Box**

Two methods are available to add your first AI provider:

### Method 1: Browse and Select a Provider

Switch to the **Browse providers** tab:

1. Use the search box to filter the provider list
2. Click the target provider (e.g. Anthropic, OpenAI, DeepSeek)
3. Fill in the configuration form:
   - **Name**: Display name for the provider
   - **API Base**: API endpoint (usually pre-filled)
   - **API Style**: Interface style (`openai` or `anthropic`)
   - **Token**: Your API Key
   - **Proxy URL** (optional): HTTP/HTTPS proxy address
4. Confirm — the system saves the credentials

For OAuth-enabled providers (e.g. Claude.ai), the system automatically initiates the OAuth authorization flow.

### Method 2: Paste Config for Auto-Detection

Switch to the **Paste & detect** tab:

1. Paste a provider configuration snippet (JSON or YAML) into the input area
2. The system automatically parses and identifies the provider type and credentials
3. Confirm to save

### Completing Onboarding

After successfully adding a provider, a success dialog appears with two options:
- **Go to Agents** — Navigate to the scenario overview and start using agents
- **Stay Here** — Continue adding more providers

---

## Existing Installations: Adding Providers via Credentials

If you have already completed onboarding and need to add a new provider, go to the [Credentials](./08-credentials.md) page (`/credentials`) and click **Connect AI** — the flow is identical to Onboarding.

---

## Next Steps

- Go to [Scenario Overview](./02-scenario-overview.md) to see all available agents
- See [Claude Code Configuration](./03-scenario-claude-code.md) to start with the primary scenario
