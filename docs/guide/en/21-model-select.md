# Model Select

The Model Select dialog assigns a target provider and model to a forwarding rule — the primary interaction point for configuring routing rules. The same picker is used by the Image Playground's *Choose another model…*, Bench's target, and the [Custom](./06-scenario-special.md) and [Team](./07-team.md) rule editors.

---

![Model Select Dialog](../images/model-select.png)

## How to Open

In any agent's **Model Rules** section:

- **Click a model card** in the routing graph (e.g. `claude-sonnet-5`) — opens in *edit* mode, titled **Choose Model**, with the current provider and model already ticked
- **Click ＋ Add model** (the dashed card at the end of a tier or smart rule) — adds another model to that rule
- **New Model** (toolbar) — *create-rule* mode: *Select a model for your new rule*; one click makes a rule for that model. Switch on **Select multiple** and the title becomes *Create a rule for each model* — see [Codex § New Model](./04-scenario-codex.md#new-model-several-models-at-once)
- **Quick Start → Choose Model** — scrolls to Model Rules and spotlights **＋ Add model** for you

---

## Dialog Structure

### Left: provider rail

- **Search credentials…** filters providers by name
- Providers are grouped by credential type — **OAUTH** and **API KEY** — each section with its count
- Each row shows the provider's **logo**, its name and **protocol badges** (`O` OpenAI-style, `A` Anthropic-style; both for fusion providers), and a ✓ beside the provider that holds the current selection

### Right: models

A toolbar along the top:

- **Refresh models** — fetch the latest model list from the provider
- **Edit Provider** — edit that provider in place (rename, URL, key, proxy) without leaving the picker
- **Search models…** — filter by model name. Pressing Enter remembers the term **per provider, on this device** (browser local storage); with the box empty the remembered terms show as **Recent searches** chips, one click to reuse (× to forget)
- **Select multiple** — in create-rule mode only

Under it: a model count (`4 models`) and the cards. The **first card is always `＋ Custom Model — Enter any model ID`**, for models the list doesn't show (custom IDs you add are remembered per provider and can be edited or deleted from their cards). After it come, when applicable, the **New Models** (freshly appeared in the provider's list; marked `NEW`, dismissable), **Recent** and **Starred Models** sections, then every model. While models load, a spinner only appears if the fetch is slow, so quick fetches don't blink.

A footer shows the provider's **Provider Quota** (the same ring/`% left` cells as [Credentials](./08-credentials.md#quota-column)) with **Details** (the raw upstream response) and a refresh button.

### Selecting

Click a model card to select it. In *edit* mode that writes it into the routing rule and closes; in *add* mode it adds a new service to the rule; in *create-rule* mode it creates the rule (or, with **Select multiple**, toggles the model — a tick on each pick — and the footer's `N models selected` → **Add N models** creates them all, **Clear** resets).

### Per-card corners

| Corner | Content |
|--------|---------|
| Top-left | `NEW` badge, or a marker for a custom model |
| Top-right | A check on the currently selected model |
| Bottom-left | A pass/fail dot from the last test run, if any — click it to reopen that result |
| Bottom-right | Hover-only actions: **Copy** (the exact model id) and **Test** (bolt); custom-model cards also have **Edit** and **Delete** |

**Test** opens the [probe panel](./20-routing-rules.md#probe-panel) aimed at that single model — pick a request shape, **Run Test**, read the journey, token usage and response — without first selecting the model into a rule. The result stays as the status dot, so testing several models leaves a quick scorecard.

---

## Related Pages

- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Routing Rules & Plugins](./20-routing-rules.md)
- [Credentials](./08-credentials.md)
