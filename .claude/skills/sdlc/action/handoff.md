# Handoff Skill

The `/handoff` skill delegates tasks to subagent(s) with shared SDLC context, documentation, and project state.

## Quick Start

```bash
# During SDLC - auto-infer task from current phase (no args needed!)
/sdlc coding
/handoff
# → Infers: "Implement based on spec and current state"

# With explicit task
/handoff Analyze the authentication flow

# Hand off with specific subagent type
/handoff Review the login form --to review

# Hand off with full context
/handoff Research caching solutions --context all

# Parallel subagents
/handoff Analyze all services --parallel 3
```

## Overview

`/handoff` is an **optional auxiliary tool** for task delegation:

1. **Auto-Inference** - During SDLC, infers task from current phase (no args needed!)
2. **Gathers Context** - Collects SDLC state, architecture cache, specs
3. **Selects Subagent** - Chooses appropriate type (explore, code, test, review)
4. **Delegates** - Passes task with full context to subagent
5. **Presents Results** - Formats and returns findings

## Key Design

**Two Modes of Operation:**

1. **Direct Execution** - SDLC phases execute directly (default)
2. **Delegated Execution** - Use `/handoff` to offload (optional)

**Auto-Inference During SDLC:**

When called without arguments during an active SDLC workflow, `/handoff` automatically infers what to do based on:
- **Current phase** - Determines appropriate task
- **SDLC state** - Workflow, title, branch, history
- **Spec documents** - Requirements to implement/verify
- **Architecture cache** - Context about relevant modules
- **Git status** - Current changes

## Phase-Specific Auto-Inference

| Current Phase | Inferred Task | Subagent |
|---------------|---------------|----------|
| `research` | Research the current topic | `explore` |
| `understand` | Understand the code/architecture | `explore` |
| `spec` | Help write specification | `explore` |
| `coding` | Implement based on spec | `code` |
| `test` | Analyze test coverage | `test` |
| `verify` | Verify vs spec requirements | `review` |
| `secure` | Security analysis | `review` |
| `cr` | Code review | `review` |

**Example:**
```bash
/sdlc start feature "User Auth"
/sdlc coding
/handoff  # No args needed!
# → Infers: "Implement user authentication based on spec"
# → Uses code subagent with spec + arch context
```

## Usage

```
/handoff [task] [--to <type>] [--context <sources>] [--parallel <n>]
```

**Arguments:**
- `task` - Task description (OPTIONAL during SDLC - auto-inferred)

**Flags:**
- `--to <type>` - Subagent: `explore`|`code`|`test`|`review`|`auto`
- `--context <sources>` - Context: `sdlc`,`arch`,`spec`,`all` (default: sdlc,arch)
- `--parallel <n>` - Number of subagents (default: 1)
- `--output <path>` - Save output to file
- `--merge` - Merge parallel outputs

## Context Sources

| Source | Description | Default |
|--------|-------------|---------|
| `sdlc` | Workflow state from `.sdlc/state.json` | ✓ |
| `arch` | Architecture cache from `.sdlc/docs/arch/` | ✓ |
| `spec` | Spec documents from `.sdlc/docs/spec/` | ✗ |
| `all` | All sources + research | ✗ |

**Note**: Works even without SDLC workflow (requires explicit task).

## Subagent Types

| Type | Best For |
|------|----------|
| `explore` | Code analysis, architecture |
| `code` | Implementation |
| `test` | Testing tasks |
| `review` | Quality checks |
| `auto` | Auto-select (by phase or keywords) |

## When to Use

**Good for:**
- Large codebase analysis
- Time-consuming exploration
- Parallel investigation
- Quick phase delegation (`/handoff` with no args)

**Not needed for:**
- Quick file reads (use Read)
- Simple edits (use Edit)
- Focused tasks (main agent is faster)

## Examples

### Auto-Inference (No Args)
```bash
/sdlc coding
/handoff
# → Infers task from phase, delegates to code subagent
```

### Explicit Task
```bash
/handoff Explore the auth module --to explore --context arch
```

### Parallel Analysis
```bash
/handoff Review all services --parallel 4 --to review
```

### Standalone (No SDLC)
```bash
/handoff Analyze codebase structure --context arch
```

## Integration with SDLC

`/handoff` works **with or without** active SDLC:

- **Without SDLC**: Requires explicit task
- **With SDLC**: Auto-infers from phase if no task provided

See [full documentation](../../commands/handoff.md) for complete details.

## Completion Criteria

- [ ] Task inferred from phase if no args
- [ ] Delegated with Agent tool
- [ ] Context gathered (if available)
- [ ] Appropriate subagent selected
- [ ] Results formatted and presented

## Dependencies

- **Agent**: For subagent delegation
- **cache**: Architecture context (optional)
- **sdlc state**: Workflow context (optional)

---

**Version**: 1.0.0 | **Created**: 2026-03-09
