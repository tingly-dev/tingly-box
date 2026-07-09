# Virtual Models

Path: `/credentials/virtual-models`

![Virtual Models](../images/virtual-models.png)

Virtual Models are Tingly-Box's built-in synthetic model providers. They require no real API keys and are suitable for demos, development debugging, and dry-run scenarios.

---

## Page Overview

Page subtitle: `Built-in synthetic model providers for onboarding, demos, and dry-runs.`

### Virtual Models Table

Lists all built-in virtual model providers:

| Column | Description |
|--------|-------------|
| Provider | Virtual provider name |
| Status | Enable/disable toggle |

---

## Use Cases

| Scenario | Description |
|----------|-------------|
| **Onboarding demo** | Walk through the full UI flow without configuring a real provider |
| **Development debugging** | Test forwarding rule configurations without consuming real API quota |
| **Feature demos** | Demonstrate Tingly-Box features to your team without exposing real API keys |

---

## Differences from Real Providers

- Virtual providers are **built-in** and cannot be added or deleted via the UI
- Responses contain **simulated content** — no real AI model is called
- Individual virtual providers can be **enabled/disabled** via the toggle
- Compatible with **all scenarios** (Claude Code, OpenAI, Anthropic, etc.)

---

## How to Enable

1. Go to `/credentials/virtual-models`
2. Find the target virtual provider
3. Switch the toggle to **enabled**

Once enabled, the virtual provider appears in each scenario's model routing options and can be configured as a forwarding target just like a real provider.

---

## Related Pages

- [Credentials](./08-credentials.md)
- [API Tokens](./10-api-tokens.md)
