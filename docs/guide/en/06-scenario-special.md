# Claw Agent / Embed / ImageGen

This chapter covers three specialized scenarios: OpenClaw universal agent, Embedding API proxy, and Image Generation API proxy.

---

## Claw Agent (OpenClaw)

Path: `/agent/agent`

OpenClaw is a universal agent interface providing a standardized API endpoint for custom agent frameworks to connect to.

### Page Structure

1. **Provider Configuration Card**:
   - **Base URL**: Agent interface address (with copy button)
   - **API Key**: Access credentials (with copy button)
2. **Models and Forwarding Rules** (collapsible): Configure routing rules for agent requests

### Use Cases

- Custom agent frameworks needing a unified API endpoint
- Multiple agents sharing the same set of provider credentials
- Independent routing rules for agent access

---

## Embed (Embedding API)

Path: `/agent/embed`

Proxies Embedding API requests, for text vectorization applications.

### Page Structure

1. **Embed API Configuration Card**: Shows proxy address and key
2. **Embedding Models and Forwarding Rules** (collapsible): Routing rules specifically for embedding models

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

## ImageGen (Image Generation)

Path: `/agent/imagegen`

![ImageGen Scenario](../images/imagegen.png)

Proxies image generation API requests (DALL-E compatible interface).

### Page Structure

1. **ImageGen API Configuration Card**: Shows proxy address and key
2. **Quick Start Example**: Provides a curl example request with one-click copy
3. **Image Generation Models and Forwarding Rules** (collapsible)
4. **Open Playground** button: Navigates to the [Playground](./07-scenario-playground.md) for interactive testing

### Integration

```bash
curl <tingly-box-imagegen-url>/images/generations \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "dall-e-3", "prompt": "a cute cat", "n": 1, "size": "1024x1024"}'
```

---

## Related Pages

- [Playground](./07-scenario-playground.md)
- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
