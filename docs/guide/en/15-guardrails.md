# Guardrails

Paths: `/guardrails`, `/guardrails/groups`, `/guardrails/rules`, `/guardrails/credentials`, `/guardrails/history`

Guardrails enforce rule-based safety checks on AI agent tool calls and tool results, preventing dangerous operations, protecting private data, and controlling resource access.

> **Note**: Guardrails is an opt-in power-up (*Guardrails*, tagged *Exp.*): switch it on in the [Power-ups menu](./02-scenario-overview.md#power-ups-the-menu-behind-the-gear) or [Experimental Features](./19-experimental.md). Its pages are then the **Guardrails** group (Guardrails · Policy Groups · Policies · Secrets · History) in the sidebar of the **Power-ups** rail item.

---

## Guardrails Overview (`/guardrails`)

![Guardrails Overview](../images/guardrails.png)

### Statistics Dashboard

**Policy Breakdown (left card)** — summarized by policy type:

| Type | Description |
|------|-------------|
| Resource Access | File read/write/delete and network access policy count (enabled/total) |
| Command Execution | Shell command execution control policy count |
| Privacy (Content) | Content privacy filter policy count |

**Event Summary (right card)** — guardrail event statistics:

| Metric | Color |
|--------|-------|
| Total Events | — |
| Allow | Green |
| Review | Yellow |
| Blocked | Red |
| Masked | Purple |

### Policy Import/Export

**Import Policies** button → opens import dialog:
- Select a file (YAML/JSON policy fragment)
- Or paste policy content directly (multiple formats supported)
- Confirm **Import**

**Export Imports** button → opens export dialog:
- Checkbox list to select which policy fragment files to export
- **Select All** / **Clear** bulk operations
- Click **Export** to download selected policies as YAML files

---

## Policy Groups (`/guardrails/groups`)

![Policy Groups](../images/guardrails-groups.png)

Policy groups organize multiple policies for collective management, with group-level enable/disable support.

> Page note: *Groups are collections of policies: a policy can appear in several groups, and a group only takes effect when it is enabled. Built-in is a policy label, not a group type.*

### Group List

The **Groups (N)** card lists each policy group with its name, severity (`high severity`), policy count (`3 policies assigned`), an **Enabled** switch and **Edit** (a delete action for custom groups). The group whose policies are shown below is marked **Selected**.

> The `Default` group is built-in and cannot be deleted (lock icon shown).

### Create/Edit a Group

Click **New Group** or **Edit**:
- **Name**: Group name
- **Severity**: Low / Medium / High
- **Enabled**: Toggle

### Assign Policies

The lower card, **Policies in <group>**, is the assignable policy list (*Check a policy to include it in this group. Only enabled policies appear here.*). Each policy entry has:
- Policy name and type badge (e.g. `Privacy`)
- Description (e.g. `No patterns configured`)
- An individual toggle

Toggle a policy on to include it in the current group; toggle off to remove it. A single policy can belong to multiple groups.

---

## Policies (`/guardrails/rules`)

![Guardrails Policies](../images/guardrails-rules.png)

A **Manage Groups** button in the top-right jumps to [Policy Groups](#policy-groups-guardrailsgroups).

### Three Tabs

| Tab | Description |
|-----|-------------|
| **Resource Access** | File read/write/delete and network access rules |
| **Command Execution** | Shell command execution pattern-matching rules |
| **Privacy** | Content regex/keyword filtering rules |

### Bulk Actions

Each tab provides:
- **Enable All**: Enable all policies in the current tab
- **Disable All**: Disable all policies in the current tab

### Policy List

Each policy shows:
- Policy ID (auto-generated)
- Policy name
- Status (Enabled / Disabled / No active group)
- Assigned group
- Actions: edit (all policies), delete (custom policies only)

### Creating a Policy

Click **New Policy** (the dialog is titled *New Policy*, or *Edit Policy · <id>* when editing) or a policy row to open the policy editor:

**Basic fields:**
- ID (auto-generated, customizable)
- Name
- Assigned policy group

**Resource Access specific:**
- Actions: read / write / delete / network (multi-select)
- Resources: resource path list (glob patterns)
- Tools: applicable tool name list

**Command Execution specific:**
- Terms/Patterns: command keyword or regex list
- Actions: execute / install (multi-select)

**Privacy specific:**
- Patterns: keyword or regex list
- Pattern Mode: substring / regex
- Case Sensitive: toggle

**Common fields:**
- Scenario Scope: applicable scenario (Anthropic / Claude Code / OpenAI, etc.)
- Verdict: block / allow / mask / review
- Reason: explanation shown to the agent when blocked

### Download Management

The **Download Management** card at the bottom lets you install additional policy fragments from the curated remote registry; once installed, enable or disable them from the policy list above. A **Retry** button re-fetches the registry if it fails to load.

---

## Secrets (`/guardrails/credentials`)

![Guardrails Secrets](../images/guardrails-credential.png)

Nav label and page title: **Secrets** (previously "Protected Credentials" / "Credential" — the sidebar row and the page itself were renamed together). Subtitle: *"Keep real secrets local and replace them with alias tokens before content reaches the model."* Keep real secrets (API keys, tokens, passwords, etc.) local and replace them with alias tokens before content reaches the model — protected secrets are always masked in model-visible content, and Tingly restores the real value locally only when a tool actually needs it at execution time. The list card itself is titled **Protected secrets**.

- **Add Credential**: Manually register a secret and its alias
- **Import from Credentials**: Pull existing values from [Credentials](./08-credentials.md) instead of re-entering them
- **Reload** (top right): Refresh the list

---

## Audit History (`/guardrails/history`)

![Guardrails History](../images/guardrails-history.png)

Path: `/guardrails/history`

The page is titled **Event History** — *Recent Guardrails activity. Expand a row only when you need the full context.* — and lists **Events (N)**.

### Filters

- **Verdict**: All / Allow / Review / Block / Mask
- **Time**: All Time / 1 Hour / 24 Hours / 7 Days

### Event List

Expandable table rows (Time · Verdict · Phase · Scenario · Policy Input · Summary) — expand a row to view:
- Provider and model
- Request direction (request/response)
- Triggered policy list
- Block/review message

### Actions

- **Refresh**: Manually refresh the event list
- **Clear**: Clear all history (requires confirmation)

---

## Related Pages

- [Experimental Features](./19-experimental.md)
- [MCP & Tools](./16-mcp-tools.md)
