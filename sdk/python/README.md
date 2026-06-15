# tingly — Python SDK for tingly-box

Write an LLM experiment or plugin in a handful of lines and reuse the
tingly-box gateway's power: provider routing, fallback, guard rails, quota and
logging. You write the idea; the box handles the plumbing.

## Install

```bash
pip install tingly            # core
pip install "tingly[all]"     # + openai and anthropic SDKs
```

## Experiment ASAP

```python
import tingly

tb = tingly.connect(scenario="experiment")   # auto-discovers your local tb

# One-shot, transport picked for you, model routed by tb:
print(tb.ask("Summarize tingly-box in one line", model="auto"))

# Or use the SDK objects directly — already pointed at the gateway:
resp = tb.openai.chat.completions.create(
    model="auto",
    messages=[{"role": "user", "content": "hi"}],
)
resp = tb.anthropic.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "hi"}],
)
```

Every call above flows through tingly-box, so guard rails, quota, logging and
fallback apply automatically — your experiment never has to know.

## How `connect()` finds your box

In order: explicit args → `TINGLY_BOX_URL` / `TINGLY_BOX_TOKEN` env →
`~/.tingly-box/sdk.json` → `~/.tingly-box/config.json` + localhost probe.

The token is your **admin** token (tb's `UserToken`); the SDK uses it once to
mint a session, then uses the returned model token for the LLM calls.

## Diagnose

```bash
tingly doctor            # traverses the real path and prints what works
tingly doctor --link     # save gateway URL + token to ~/.tingly-box/sdk.json
```

A green `tingly doctor` is a guarantee your code will run.

## Status

This is **Layer 1** (client-side library). Layer 2 (tb-hosted plugins with a
manifest and lifecycle UI) and Layer 3 (plugin-as-virtual-model) build on the
same module. See `.design/python-sdk.md` in the repo for the full design.
