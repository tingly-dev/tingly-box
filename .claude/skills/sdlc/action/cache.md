# Cache Skill

The `/cache` skill manages SDLC workflow state and cached architecture documentation.

## Usage

```
/cache [action] [key] [value?]
```

**Actions:**
- `get` - Retrieve cached value by key
- `set` - Store a value with associated key
- `clear` - Remove cached value by key (or clear all if no key)
- `list` - List all cached keys and metadata
- `invalidate` - Mark cache entry as stale
- `refresh` - Update cache with fresh data

## Architecture Cache

### Overview

Architecture documentation is cached in `.sdlc/docs/arch/` to avoid repeated code analysis during SDLC workflow. See [ARCH_CACHE_SYSTEM.md](../../.sdlc/docs/arch/ARCH_CACHE_SYSTEM.md) for full documentation.

### Cache Structure

```
.sdlc/docs/arch/
├── main/                         # Branch-aware caches
│   ├── overview-arch.md
│   ├── [module]-arch.md
│   └── [module]/[sub]-arch.md
├── feature-a/
│   └── [module]-arch.md
└── cache-metadata.json           # Cache metadata
```

### Reading Architecture Cache

**Priority Order (most specific first):**

1. **Component**: `.sdlc/docs/arch/{branch}/[module]/[sub]-arch.md` (12h TTL)
2. **Module**: `.sdlc/docs/arch/{branch}/[module]-arch.md` (3d TTL)
3. **Project**: `.sdlc/docs/arch/{branch}/overview-arch.md` (7d TTL)
4. **Fallback**: `.sdlc/docs/arch/main/[file].md` (if branch cache missing)

**Examples:**

```bash
# For auth module work
.sdlc/docs/arch/feature-a/auth/login/oauth-arch.md  # Most specific
.sdlc/docs/arch/feature-a/auth/login-arch.md
.sdlc/docs/arch/feature-a/auth-arch.md
.sdlc/docs/arch/main/auth-arch.md                  # Fallback to main

# Use glob to find relevant cache
.sdlc/docs/arch/**/*-arch.md
.sdlc/docs/arch/auth-arch.md
.sdlc/docs/arch/auth/**/*-arch.md
.sdlc/docs/arch/main/*-arch.md                     # Explicit main branch
```

### Cache Levels

| Level         | Pattern                           | TTL    | Example                                        | Use Case                 |
| ------------- | --------------------------------- | ------ | ---------------------------------------------- | ------------------------ |
| **Project**   | `overview-arch.md`                | 7 days | `.sdlc/docs/arch/overview-arch.md`             | Whole project context    |
| **Module**    | `[module]-arch.md`                | 3 days | `.sdlc/docs/arch/auth-arch.md`                 | Feature work on module   |
| **Component** | `[module]/[sub]-arch.md`          | 1 day  | `.sdlc/docs/arch/auth/login-arch.md`           | Deep dive into component |
| **Detailed**  | `[module]/[sub]/[detail]-arch.md` | 12h    | `.sdlc/docs/arch/auth/providers/oauth-arch.md` | Detailed analysis        |

### Cache File Format

Each cache file includes:

```markdown
# [Scope] Architecture

**Last Updated**: YYYY-MM-DD
**Cache Level**: [Project|Module|Component|Detailed]
**Expires**: YYYY-MM-DD
**Branch**: [branch-name]
**Hash**: [git commit hash]

## Overview
[High-level description]

## Components
[Component breakdown]

## Dependencies
[What this depends on]

## Data Models
[Relevant data structures]

## Integration Points
[How it connects to other parts]
```

### Writing Architecture Cache

When generating cache:

1. **Determine appropriate level** based on scope
2. **Generate architecture documentation**
3. **Save with naming convention**: `[YYYYMMDD]-[scope]-arch.md`
4. **Update metadata** in `cache-metadata.json`
5. **Include hash** for change detection

### Cache Freshness

Check if cache is valid:

```bash
# Compare last updated date with TTL
if [ $(($(date +%s) - cache_timestamp)) -gt $((ttl_seconds)) ]; then
    echo "Cache expired, regenerate"
fi

# Compare git hash within branch context
cached_hash=$(grep "Hash:" .sdlc/docs/arch/{branch}/module-arch.md)
current_hash=$(git rev-parse HEAD)
current_branch=$(git branch --show-current)
if [ "$cached_hash" != "$current_hash" ]; then
    echo "Code changed, invalidate cache"
fi
```

### Auto-Refresh Rules

| Trigger                   | Paths Affected | Cache Levels to Invalidate |
| ------------------------- | -------------- | -------------------------- |
| Git commit to `src/auth/` | `src/auth/**`  | auth-arch.md, auth/**/*    |
| Config file change        | `config/**`    | overview-arch.md           |
| Package.json change       | `package.json` | overview-arch.md           |

## Workflow State Cache

### State Storage

```
.sdlc/
├── state.json            # Current workflow state
├── history.json          # Phase execution history
└── config.json           # Configuration (optional)
```

### State File Example

```json
{
  "workflow": "feature",
  "current_phase": "spec",
  "completed": ["research"],
  "branch": "feature/user-auth",
  "description": "User Authentication System",
  "started_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T14:30:00Z",
  "phase_history": [
    { "phase": "research", "completed_at": "2026-03-08T10:30:00Z" }
  ]
}
```

## Integration with SDLC Phases

### Spec Phase

1. **Before writing spec**:
   ```bash
   # Check for relevant arch cache
   glob(".sdlc/docs/arch/*-arch.md")

   # Read most specific cache available
   # Use cached info for design decisions
   ```

2. **After understanding**:
   ```bash
   # Generate or update arch cache
   # Save to .sdlc/docs/arch/[timestamp]-[scope]-arch.md
   ```

### Understand Phase

- Automatically generates architecture cache
- Saves to appropriate level based on scope
- Updates cache metadata

### Research Phase

- Reads module-level cache for context
- Focuses research on specific areas
- Updates cache with findings

## Examples

### Example 1: Feature Development

```bash
# 1. Check for auth module cache
cat .sdlc/docs/arch/auth-arch.md

# 2. If fresh (within 3 days), use it
# 3. If stale or missing, generate new
/sdlc understand src/auth

# 4. Write spec using cached architecture
/sdlc spec "Add OAuth login"
```

### Example 2: Bug Fix

```bash
# 1. Check for specific component cache
cat .sdlc/docs/arch/auth/login-arch.md

# 2. Understand flow from cache
# 3. Debug the issue
/sdlc debug "Login fails on Safari"

# 4. Update cache if architecture changed
```

### Example 3: Reading Cache Hierarchy

```bash
# Most specific first
if [ -f ".sdlc/docs/arch/auth/login/oauth-arch.md" ]; then
    cat .sdlc/docs/arch/auth/login/oauth-arch.md
elif [ -f ".sdlc/docs/arch/auth/login-arch.md" ]; then
    cat .sdlc/docs/arch/auth/login-arch.md
elif [ -f ".sdlc/docs/arch/auth-arch.md" ]; then
    cat .sdlc/docs/arch/auth-arch.md
else
    cat .sdlc/docs/arch/overview-arch.md
fi
```

## Best Practices

### When to Create Cache

- **Project overview**: First time working on project
- **Module cache**: Before starting feature work on a module
- **Component cache**: When diving deep into implementation
- **Detailed cache**: For complex, critical components

### When to Update Cache

- After significant code changes
- When architecture patterns change
- Before starting major refactoring
- When cache expires (based on TTL)

### When to Skip Cache

- For trivial code changes
- When working with well-known modules
- When cache is very recent (< 1 hour old)
- For quick one-off tasks

## Completion Criteria

- [ ] Architecture cache directory created (`.sdlc/docs/arch/`)
- [ ] Multi-level cache format defined
- [ ] TTL-based freshness control working
- [ ] Hash-based invalidation working
- [ ] Fallback strategy between cache levels
- [ ] Metadata tracking implemented
- [ ] Integration with spec phase working
- [ ] Integration with understand phase working
- [ ] Documentation provided
- [ ] Examples provided

## Dependencies

- **doc**: Create architecture documentation from cached data
- **git**: Track commits for hash-based invalidation
- **understand**: Generate architecture understanding
- **spec**: Read architecture cache for context

## References

- [ARCH_CACHE_SYSTEM.md](../../.sdlc/docs/arch/ARCH_CACHE_SYSTEM.md) - Full cache system documentation
- [spec.md](../../commands/spec.md) - Integration with spec command
