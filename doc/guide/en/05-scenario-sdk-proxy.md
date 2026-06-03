# OpenAI / Anthropic SDK Proxy

This chapter covers the OpenAI-compatible interface proxy and the Anthropic native interface proxy — both designed for applications that call AI APIs directly in code.

---

## OpenAI Scenario

Path: `/agent/openai`

Transparently proxies requests from any application using the OpenAI SDK to the providers managed by Tingly-Box.

### Use Cases

- Your own Python/Node.js/Go applications using the `openai` official SDK
- Third-party tools configured with an OpenAI-compatible endpoint (LangChain, LlamaIndex, etc.)
- Unified management of API credentials across multiple OpenAI-compatible providers

### Page Structure

1. **OpenAI Configuration Card**: Shows proxy Base URL and API Key
2. **Models and Forwarding Rules** (collapsible): Configure which provider requests are routed to

### Integration

Point the OpenAI SDK's `baseURL` to the proxy address provided by Tingly-Box:

```python
from openai import OpenAI
client = OpenAI(
    base_url="<tingly-box-base-url>",
    api_key="<tingly-box-api-key>",
)
```

```javascript
import OpenAI from 'openai';
const client = new OpenAI({
  baseURL: '<tingly-box-base-url>',
  apiKey: '<tingly-box-api-key>',
});
```

---

## Anthropic Scenario

Path: `/agent/anthropic`

Proxies requests from applications using the Anthropic official SDK to providers managed by Tingly-Box (including non-Anthropic providers that support the Anthropic protocol).

### Use Cases

- Applications using the `anthropic` official SDK to call the Claude API
- Switching underlying providers without code changes
- Auditing and tracking Anthropic API usage

### Integration

Point the Anthropic SDK's `base_url` to the Tingly-Box proxy address:

```python
import anthropic
client = anthropic.Anthropic(
    base_url="<tingly-box-base-url>",
    api_key="<tingly-box-api-key>",
)
```

---

## Zen Mode

| Scenario | Zen Path |
|----------|---------|
| OpenAI | `/zen/openai` |
| Anthropic | `/zen/anthropic` |

---

## Relationship to Credentials

The forwarding rules for these scenarios determine which provider requests are ultimately sent to. If no providers have been added, go to [Credentials](./08-credentials.md) first.

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Credentials](./08-credentials.md)
