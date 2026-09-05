# Custom / Embed / Image Playground / Team

This chapter covers four specialized scenarios: the Custom catch-all scenario (formerly "OpenClaw"), Embedding API proxy, Image Playground (generation + editing), and multi-team workspaces.

---

## Custom

Path: `/agent/custom`

![Custom Scenario](../images/custom-scenario.png)

The Custom scenario (previously labeled "OpenClaw" / "Claw Agent" in the sidebar) is a generic catch-all: bring your own request model name and get a standardized API endpoint for any custom agent framework to connect to. Hidden from the sidebar by default (unhide it from [Scenario Overview](./02-scenario-overview.md) if you need it).

### Page Structure

1. **Provider Configuration Card**:
   - **Base URL**: Agent interface address (with copy button)
   - **API Key**: Access credentials (with copy button)
2. **Model Rules** (collapsible): Configure routing rules for agent requests

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

## Image Playground

Path: `/agent/image` (both the old `/agent/imagegen` and the old standalone `/agent/playground` now redirect here — image generation config and the interactive test bench live on one page)

![Image Playground Scenario](../images/image-playground.png)

Proxies image generation/editing API requests (DALL-E compatible interface), with an inline playground for trying prompts without writing code.

### Page Structure

1. **Image API Configuration Card**: Shows proxy address and key, plus a **Quick Start** button (curl example, one-click copy)
2. **Image Playground card**: The interactive test bench, described below
3. **Image Model Rules** (collapsible): Routing rules for image generation models

### Image Playground Card

A **Generate / Edit** toggle switches the card between two modes:

**Generate mode**
- **Model** dropdown (populated from configured Image Model Rules), **Prompt** text box, **Size** / **Quality** / **N** (count) controls
- **Generate** button; while running, a **Generate another · N running** button lets you queue more without waiting

**Edit mode**

![Image Playground — Edit Mode](../images/image-playground-edit.png)

- **Reference images** dropzone — drag & drop, click to browse, or **paste** an image directly (up to 5, PNG/JPEG/WebP)
- Same Model/Prompt/Size/Quality/N controls, with an **Edit Image** button in place of Generate
- Each result thumbnail has an **Edit this image** action that feeds it back in as a new reference — chaining edits without leaving the page

**Results panel** (right side)
- Generated images appear in a session-scoped gallery (not persisted across reloads)
- Each result carries: **View original image** (for edited results, compares against its source), **Copy prompt**, and **Download**
- **Split into tiles**: cuts a result into an evenly divided grid (rows × columns, adjustable outer margin and gap) — for sticker sheets, contact sheets, or spritesheets. Click a tile to exclude it, then **Download this tile** or **Download N PNGs (ZIP)**

### Integration

```bash
curl <tingly-box-imagegen-url>/images/generations \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "dall-e-3", "prompt": "a cute cat", "n": 1, "size": "1024x1024"}'
```

---

## Team

Path: `/agent/team` (default team) or `/agent/team/:slug` (additional teams). Hidden from the sidebar by default.

![Team Workspace](../images/team-workspace.png)

Multi-team workspaces give each team its own isolated routing configuration and sharing keys, so a shared Tingly-Box instance can serve several teams without their model rules or API keys leaking into each other.

### Workspace Navigation

Teams get their own **profile-style** navigation block in the sidebar, mirroring how Claude Code Profiles work:
- Each team is a separate nav item, subtitled `slug - name`; the built-in **Default** team is always first
- **Add Team** at the bottom of the block opens an inline popover to name and create a new team

### Page Structure

Same shape as any other scenario page:
1. **Provider Configuration Card**, titled `Team - <name>`:
   - An info icon shows the sharing-key access scope tooltip (keys work only against `/tingly/team` and `/tingly/team/v1` — they cannot reach other teams, scenario endpoints, or management APIs)
   - An edit icon opens **Team settings** to rename the team; a delete icon (non-default teams only) removes it — you must move or delete its sharing keys first
   - **Enabled** toggle (top-right): disabling a team blocks its sharing keys from reaching model endpoints, without deleting the team or its configuration
   - **Sharing Keys** button opens the per-team key management dialog
   - Same **Plugins** row as other scenarios (Thinking / Smart Compact / Vision Proxy / Record)
2. **Model Rules** (collapsible): routing rules scoped to this team, independent of every other team's

### Sharing Keys Dialog

Manage the API tokens that give access to this team's `/tingly/team` endpoint:
- **Create Token**: name a new sharing key
- Table columns: name, key (reveal/copy), enabled toggle, last-used, and **Move** — reassign a key to a different active team **without rotating it**
- Deleting a key is immediate and irreversible

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Credentials](./08-credentials.md)
- [Usage Dashboard](./11-dashboard.md)
