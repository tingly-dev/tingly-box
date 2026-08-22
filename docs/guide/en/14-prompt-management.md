# Prompt Management

Paths: `/prompt/user`, `/prompt/skill`, `/prompt/command` (Full Edition)

Prompt Management provides IDE recording browsing and Skills management, helping teams accumulate and reuse AI coding knowledge.

> **Note**: Prompt Management is available in **Full Edition** only, and the corresponding toggles must be enabled in [Experimental Features](./19-experimental.md).

---

## User Requests

Path: `/prompt/user`

### Function

The User Requests page browses and manages interaction recordings captured from Claude Code IDE sessions. Use cases:
- Reviewing past AI-assisted decision processes
- Extracting successful prompt templates
- Team knowledge sharing

### Three-Column Layout

**Left column: Calendar**
- Calendar view showing recording counts per date
- Range filter buttons (Today / This Week / This Month / All)

**Center column: Recording List**
- Search box: filter by content
- User filter: filter by recording user
- Project filter: filter by project
- Type filter: code-review / debug / refactor / test
- Each recording shows title, type badge, timestamp

**Right column: Recording Details**
- Summary
- Metadata: user, project, type, duration, model, timestamp
- Full conversation content

---

## Skills Management

Path: `/prompt/skill`

![Skills Management](../images/prompt-skills.png)

### Function

The Skills page manages reusable prompt snippets synced from IDE configurations (e.g. `.claude/skills/` directories), supporting:
- Auto-discovery of skills from multiple IDE sources
- Grouped browsing and search
- Viewing skill content in Markdown or raw format

### Three-Column Layout

**Left column: Skill Locations**

Each location corresponds to a directory source:
- Location name
- Path
- IDE source badge (e.g. claude_code)
- Skill count
- Actions: refresh, edit, delete

Top buttons:
- **Add Location**: Manually add a skill directory
- **Auto Discovery**: Scan all configured IDEs for skill directories

**Center column: Skills List**

After selecting a location, shows all skills in that location:
- Toggle between **Grouped** and **Flat** view
- Grouping strategies: Auto / Pattern / Flat
- Search filter

**Right column: Skill Content**

After selecting a skill:
- File metadata (path, size, modification time)
- **Markdown** rendered view (default)
- **Raw** plaintext view
- Copy button

---

## Commands

Path: `/prompt/command`

Current status: **Coming Soon** (feature under development)

---

## Related Pages

- [Experimental Features](./19-experimental.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
