# Getting Started

This chapter guides you through the first-time startup and provider setup so that all agent scenarios are ready to use.

---

## First Launch

There is no longer a dedicated, full-screen onboarding wizard. When you access the Tingly-Box Web UI for the first time, the system detects that no providers are configured and automatically redirects to the **Tips & Help** page at `/help` instead — the same page documented in [Tips & Help](./13-help.md), just with its **Provider Catalog** card expanded by default so it's the first thing you see. (The old `/onboarding` URL still works — it just redirects to `/help` now.) Everyone else lands on the agent page they were last on.

---

## Provider Catalog: Adding Your First Provider

Page title: **Tips & Help**. Subtitle: *"A few easy-to-miss but useful things."* The **Provider Catalog** card (one of four accordion cards on this page — see [Tips & Help](./13-help.md) for the others) is expanded by default for a brand-new install, with the subtitle *"Browse the catalog or paste a config snippet — we'll figure out the rest."*

![Provider Catalog card on the Help page](../images/onboarding.png)

It embeds the exact same browsable list as the **Connect AI** button elsewhere in the app (Credentials, scenario pages, etc.) — there's no separate onboarding-only flow to learn. A search box sits at the top, followed by:

**Custom** section — three cards:
- **Custom endpoint**: manually specify any OpenAI/Anthropic-compatible API endpoint
- **Import**: import a provider config from a file or the clipboard
- **Paste & detect**: paste a `.env` file, curl command, or JSON snippet — Tingly-Box extracts the URL and credentials automatically

**OAuth sign-in** section — providers that support OAuth authorization (Claude Code, Google Gemini CLI, Antigravity, Codex, Kimi Code, etc.). Clicking one launches the OAuth flow directly — no API Key required, and the token is saved automatically once you authorize.

Scroll down for more providers that use API Keys, grouped by region and protocol (OpenAI / Anthropic).

Choosing any non-OAuth provider opens a config form inline on the same card — see [Credentials · Adding a Provider](./08-credentials.md#adding-a-provider-the-connect-ai-flow) for the full field-by-field breakdown (Base URL, API Key, API Style, Proxy URL, etc.).

### Completing Setup

After successfully adding a provider, a success dialog appears with two options:
- **Go to Agents** — Navigate to your agent scenarios and start using them
- **Stay Here** — Continue adding more providers

---

## Existing Installations: Adding Providers via Credentials

If you've already connected a provider and need to add another, go to the [Credentials](./08-credentials.md) page (`/credentials`) and click **Connect AI**. It opens the same unified picker as the Provider Catalog card above; the full two-step "pick a type, then fill in the config" flow is documented in [Credentials · Adding a Provider](./08-credentials.md#adding-a-provider-the-connect-ai-flow).

![Connect AI Picker](../images/connect-ai.png)

---

## Next Steps

- Go to [Scenario Overview](./02-scenario-overview.md) to see all available agents
- See [Claude Code Configuration](./03-scenario-claude-code.md) to start with the primary scenario
