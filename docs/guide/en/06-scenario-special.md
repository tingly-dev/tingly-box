# Custom / Embed

This chapter covers two catch-all scenarios: the Custom catch-all scenario (formerly "OpenClaw") and the Embedding API proxy.

> Team and Image used to be covered here too. Both are now top-level rail items with their own pages — see [Team](./07-team.md) and [Image](./07-image.md).

---

## Custom

Path: `/agent/custom`

![Custom Scenario](../images/custom-scenario.png)

The Custom scenario (previously labeled "OpenClaw" / "Claw Agent" in the sidebar) is a generic catch-all: bring your own request model name and get a standardized API endpoint for any custom agent framework to connect to. Shown in the sidebar by default (hide it in the sidebar's [edit mode](./02-scenario-overview.md#managing-visibility-sidebar-edit-mode) if you don't need it).

### Page Structure

The standard agent page ([Claude Code § Page Structure](./03-scenario-claude-code.md#page-structure)) with no client-specific setup — no Auto Config / Setup Guide button and no Quick Start:

1. **Header**: status line plus **Requests / Usage / Log**
2. **Connection card**: **Base URL** (`/tingly/custom`) and **API Key**, folding once connected
3. **Plugins card**: Thinking / Vision Proxy / Record
4. **Model Rules** (collapsible): **New Model** adds routing rules for the request model names your agent sends

### Use Cases

- Custom agent frameworks needing a unified API endpoint
- Multiple agents sharing the same set of provider credentials
- Independent routing rules for agent access

---

## Embed (Embedding API)

Path: `/agent/embed`

Proxies Embedding API requests, for text vectorization applications.

### Page Structure

Same agent page, titled **Embed API**:

1. **Header** and **connection card** with the proxy address (`/tingly/embed`) and key
2. **Plugins card** showing **Record** only (thinking and vision don't apply to embeddings)
3. **Embedding Model Rules** (collapsible): routing rules specifically for embedding models

### Use Cases

- Text vectorization for RAG (Retrieval-Augmented Generation) applications
- Semantic search systems
- Text similarity computation

### Integration

```python
from openai import OpenAI
client = OpenAI(
    base_url="<tingly-box-embed-url>",
    api_key="<tingly-box-api-key>",
)
response = client.embeddings.create(
    model="text-embedding-3-small",
    input="your text here",
)
```

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Team](./07-team.md)
- [Image](./07-image.md)
- [Credentials](./08-credentials.md)
