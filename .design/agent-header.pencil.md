# Agent page header — fold the connection once it is proven (pencil)

Scope: how much of the Agent page's header is shown, and when. Context and decisions: [ui-flow-analysis.md](./ui-flow-analysis.md); page template: [agent-page-redesign.md](./agent-page-redesign.md).

Legend: `●` = state dot · `⧉` = copy · `⌄ ⌃` = expand / collapse · `[ ]` = button.

## 1. Rule

Show the connection rows (Base URL, API Key, Plugins) while they are the page's job; fold them to one line once the client is **proven** to use this gateway.

```
proof of "connected"                          default
─────────────────────────────────────────     ───────────
config file reads back as `applied`           folded
(not applied / outdated / unreadable)         open  (Reapply is the next action)
SDK pages (no config to read back)            open  (the values ARE the page)
user chose by hand                            their choice, remembered per agent
```

## 2. Before / after (Codex-like agent, applied)

```
BEFORE  (header ≈ 215px)
┌ DeepSeek ⓘ  ● Applied ──────── [Open Web UI] [Requests & usage] [Auto Config] ┐
│ Base URL   http://localhost:3000/tingly/dsh        ⧉   Local | Docker          │
│ API Key    mock-model-token                        👁 ⧉                        │
│ Plugins    Thinking By Client | Vision Proxy Off ⌄                             │
└────────────────────────────────────────────────────────────────────────────┘

AFTER   (header ≈ 135px, model rules move up ~100px)
┌ DeepSeek ⓘ  ● Applied ──────── [Open Web UI] [Requests & usage] [Auto Config] ┐
│ Base URL  http://localhost:3000/tingly/dsh ⧉                 Connection details ⌄ │
└────────────────────────────────────────────────────────────────────────────┘
```

## 3. Not done in this prototype

- Plugins summary on the folded line (`Thinking: By Client · Vision: Off`, click to open a popover).
- Merging Quick Start into the same card (one card, two states).
- Turning "Requests & usage" into a live chip (`● 12 today · 3m ago`) inside the folded line, which would take a button out of the header.
