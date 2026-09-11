# Model Select

The Model Select dialog is used to assign a target provider and model to a forwarding rule — it is the primary interaction point for configuring routing rules.

---

![Model Select Dialog](../images/model-select.png)

## How to Open

In any scenario's (Claude Code, Codex, etc.) **Model Rules** section, you can open the dialog in several ways:

- **Click an existing provider node**: Click the card showing a model name (e.g. `claude-sonnet-4-6`) in the routing graph — opens in "edit" mode
- **Click the "+ Add" button**: Click the add button at the end of a rule row — opens in "add" mode
- **AgentSetupCard Quick Start**: In Claude Code's Quick Start step 2, expand it and click **Choose Model**

---

## Dialog Structure

### Left Panel: Provider List

- Lists all configured providers (from [Credentials](./08-credentials.md))
- Click a provider name to expand/collapse its model list
- Each provider shows all supported models

### Top: Search & Filter

- **Search box**: Filter by model name or provider name
- Quickly locate a specific model

### Selection

- Click a model row to select it; confirming writes it to the routing rule
- In "add" mode, selecting a model automatically creates a new forwarding rule

### Per-Card Test Action

Each model card follows a consistent corner convention:

| Corner | Content |
|--------|---------|
| Top-left | Category marker — `NEW` badge, or a triangle marking a custom model |
| Top-right | A checkmark on the currently selected model |
| Bottom-left | A persistent pass/fail status dot from the last test run, if any — click it to reopen the dialog with that result |
| Bottom-right | Hover-only action icons: **Edit**, **Delete**, and **Test** (bolt icon) |

Clicking the bolt icon opens a **Test** dialog to run a model directly from the card — pick a request shape, click **Run Test**, and see the request journey, token usage, and response — without first selecting the model into a rule. The result persists as the status dot after the dialog closes, so testing several models leaves a quick visual scorecard.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Routing Rules & Plugins](./20-routing-rules.md)
- [Credentials](./08-credentials.md)
