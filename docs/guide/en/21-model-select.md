# Model Select

The Model Select dialog is used to assign a target provider and model to a forwarding rule — it is the primary interaction point for configuring routing rules.

---

![Model Select Dialog](../images/model-select.png)

## How to Open

In any scenario's (Claude Code, Codex, etc.) **Models and Forwarding Rules** section, you can open the dialog in several ways:

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

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Routing Rules & Extensions](./20-routing-rules.md)
- [Credentials](./08-credentials.md)
