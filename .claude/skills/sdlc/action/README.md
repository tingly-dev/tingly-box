# Action Skills

This directory contains action and utility skills that support the SDLC workflow. These skills can be called at any time during the development process.

## Available Skills

| Skill | Purpose | Documentation |
|-------|---------|---------------|
| **discuss** | Interactive technical discussion | [discuss.md](./discuss.md) |
| **doc** | Documentation generation and management | [doc.md](./doc.md) |
| **pencil** | Wireframe and UI/UX design | [pencil.md](./pencil.md) |
| **cache** | Architecture knowledge caching | [cache.md](./cache.md) |
| **archive** | Archive old documentation | [archive.md](./archive.md) |
| **git** | Git operations assistance | [git.md](./git.md) |
| **git-resolve** | Git conflict resolution | [git-resolve.md](./git-resolve.md) |
| **handoff** | Subagent delegation with context | [handoff.md](./handoff.md) |

## Usage

Utility skills are available as standalone commands:

```bash
/discuss [topic description]
/doc [type] [target]
/pencil [design description]
/cache [scope] [action]
/archive [scope] [pattern]
/git [action] [options]
/git-resolve [strategy]
/handoff [task] [options]
```

## Integration with SDLC

These utility skills are designed to work seamlessly with the SDLC workflow:

- **During research**: Use `/cache` to read architecture knowledge
- **During spec**: Use `/pencil` to create wireframes, `/doc` to generate API docs
- **During coding**: Use `/git` for branch management, `/git-resolve` for conflict resolution, `/handoff` for code analysis
- **During any phase**: Use `/doc` to update documentation
- **For delegation**: Use `/handoff` to delegate complex tasks with full context

**Note**: `/discuss` can be used independently anytime for technical discussions, exploration, and decision-making - it's not tied to any specific phase.

**Note**: `/handoff` is useful for delegating time-consuming analysis or exploration to subagents while maintaining SDLC context.

## Design Reference

These skills were created based on the SDLC v3.2 design document:
`/Users/yz/Project/feng-project/vibely/.sdlc/docs/pencil/2026-03-08-sdlc-v3.2-flow.md`

## File Structure

```
action/
├── README.md           # This file
├── discuss.md          # Technical discussion skill
├── doc.md              # Documentation skill
├── pencil.md           # Wireframe design skill
├── cache.md            # Architecture caching skill
├── archive.md          # Archive skill
├── git.md              # Git operations skill
├── git-resolve.md      # Git conflict resolution skill
└── handoff.md          # Subagent delegation skill
```

## Dependencies

Utility skills may depend on each other:
- **discuss** depends on: cache, doc, git
- **doc** depends on: cache, pencil
- **pencil** depends on: doc, cache
- **cache** depends on: doc, git
- **git** depends on: cache, doc
- **git-resolve** depends on: git
- **handoff** depends on: cache, Agent, sdlc state

## Version

**Created**: 2026-03-08
**SDLC Version**: 3.2 (Flow-Based)
**Updated**: 2026-03-09 (added handoff skill)
