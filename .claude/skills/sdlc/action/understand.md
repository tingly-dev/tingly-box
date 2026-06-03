# Understand Phase Skill

Understands the current codebase, architecture, and implementation patterns to build context before making changes.

## Usage

```
/sdlc understand [scope]
```

## Description

Builds comprehensive knowledge of the existing codebase by analyzing architecture, mapping components, and creating cached documentation. Unlike `research` (which explores external technologies), `understand` focuses on **internal code comprehension**.

### When to Use

- **Onboarding**: When joining a project or working with unfamiliar code
- **Before changes**: Before implementing features, fixing bugs, or refactoring
- **Context building**: When you need to understand "how this works currently"
- **Architecture discovery**: When exploring how components are connected
- **Before writing specs**: To cache architecture knowledge for reuse

### What It Does

- Maps project structure and key components
- Identifies tech stack, frameworks, and dependencies
- Analyzes code patterns and conventions
- Documents module relationships and data flow
- **Creates architecture cache in `.sdlc/docs/arch/`**
- Identifies potential issues or technical debt

## Architecture Cache

### Cache Levels

Understand generates architecture cache at different levels based on scope:

| Level          | Scope Pattern                     | TTL Reference | Output Path                                |
| -------------- | --------------------------------- | ------------- | ------------------------------------------ |
| **Project**    | No scope (entire project)         | ~30 days      | `.sdlc/arch/overview-arch.md`              |
| **Module**     | `src/[module]` or `[module]/`     | ~14 days      | `.sdlc/arch/[module]-arch.md`              |
| **Sub-module** | `src/[module]/[sub]`              | ~7 days       | `.sdlc/arch/[module]-[sub]-arch.md`        |
| **Component**  | Deep dive into specific component | ~3 days       | `.sdlc/arch/[module]-[sub]-[comp]-arch.md` |

> **Note**: Cache files are stored directly in `.sdlc/arch/` with a flat structure. The scope is encoded in the filename rather than using subdirectories.

### Cache File Format

Each cache file includes:

```markdown
# [Scope] Architecture

**Last Updated**: YYYY-MM-DD
**Cache Level**: Project|Module|Sub-module|Component
**Expires**: YYYY-MM-DD (~X days)
**Branch**: [branch-name]
**Hash**: [git commit hash]

## Overview
[High-level description]

## Components
[Component breakdown]

## Dependencies
[What this depends on]

## Integration Points
[How it connects to other parts]
```

### Reading Existing Cache

Before generating new cache, understand checks for existing cache:

```bash
# Priority order (most specific first)
.sdlc/arch/[module]-[sub]-[comp]-arch.md  # Component level
.sdlc/arch/[module]-[sub]-arch.md          # Sub-module level
.sdlc/arch/[module]-arch.md                # Module level
.sdlc/arch/overview-arch.md                # Project level
```

If cache exists and is fresh (within TTL, no code changes), understand reuses it instead of regenerating.

### Cache Invalidation

Cache is invalidated when:
- TTL has expired (reference only, check actual code changes)
- Git hash doesn't match current HEAD
- Files in scope have been modified

```bash
# Check if cache is stale
cache_hash=$(grep "Hash:" .sdlc/arch/overview-arch.md)
current_hash=$(git rev-parse HEAD)

if [[ "$cache_hash" != *"$current_hash"* ]]; then
    echo "Cache stale, regenerating"
fi
```

## Process

1. **Check Existing Cache**
   - Look for existing architecture cache in `.sdlc/docs/arch-*`
   - Check if cache is fresh (hash comparison, file modification time)
   - Reuse if fresh, otherwise proceed to analysis

2. **Project Mapping**
   - Explore directory structure and organization
   - Identify entry points and key modules
   - Map component hierarchies and relationships

3. **Tech Stack Analysis**
   - Identify frameworks, libraries, and tools
   - Note dependencies and versions
   - Understand build tools and development setup

4. **Code Pattern Discovery**
   - Identify coding conventions and patterns
   - Note architectural patterns (MVC, microservices, etc.)
   - Understand state management and data flow

5. **Generate Architecture Cache**
   - Save to `.sdlc/arch/[scope]-arch.md` with date
   - Include hash for change detection
   - Set appropriate TTL based on level
   - Update `.sdlc/arch/cache-metadata.json` if needed

## Scope Options

```bash
/sdlc understand                # Entire project → overview-arch.md
/sdlc understand src/auth       # Auth module → auth-arch.md
/sdlc understand auth/login     # Login sub-module → auth/login-arch.md
/sdlc understand --deep         # Deeper analysis → detailed component cache
```

## Output Format

### Architecture Cache Template

```markdown
# [Scope] Architecture

**Last Updated**: YYYY-MM-DD
**Cache Level**: Project|Module|Sub-module|Component
**Expires**: YYYY-MM-DD (~X days)
**Branch**: [branch-name]
**Hash**: [git commit hash]
**Parent**: [parent-cache-file.md] (for sub-levels)

## Overview
[Purpose and responsibilities]

## Components

| Component   | Location | Purpose        |
| ----------- | -------- | -------------- |
| [Component] | [path]   | [what it does] |

## Dependencies
- [Dependency 1] - [how it's used]
- [Dependency 2] - [how it's used]

## Data Flow
[How data flows through the system]

## Key Patterns
- [Pattern 1]: [description]
- [Pattern 2]: [description]

## Integration Points
- [Integration 1]: [how it connects]
- [Integration 2]: [how it connects]

## Related Areas
- [Related module/component]: [relationship]
```

## Output Locations

### Architecture Cache (Primary Output)

Cache files are stored directly in `.sdlc/arch/` with a flat structure:

```
.sdlc/arch/
├── overview-YYYYMMDD-arch.md          # Project level (~30 days)
├── auth-YYYYMMDD-arch.md              # Module level (~14 days)
├── auth-login-YYYYMMDD-arch.md        # Sub-module level (~7 days)
└── cache-metadata.json                # Cache metadata
```

**Note**: The date is included in the filename to support multiple cache versions and freshness tracking.

### Understanding Reports (Secondary Output)

```
.sdlc/docs/category-feature-date.understand.md
```

Examples:
- `project-overview-20260320.understand.md`
- `auth-module-20260320.understand.md`

## Completion Checklist

- [ ] Checked for existing architecture cache
- [ ] Project structure mapped
- [ ] Key components identified
- [ ] Tech stack documented
- [ ] Code patterns noted
- [ ] Module relationships understood
- [ ] Dependencies mapped
- [ ] Architecture cache saved to `.sdlc/arch/`
- [ ] Hash included for change detection
- [ ] Understanding report saved to `.sdlc/docs/`

## Examples

### Example 1: Project Understanding (Creates overview cache)

```bash
/sdlc understand
```

Generates:
- `.sdlc/arch/overview-YYYYMMDD.arch.md` - Project architecture cache (~30 days)
- `.sdlc/docs/project-overview-YYYYMMDD.understand.md` - Full understanding report

Covers:
- Overall architecture and structure
- All major components and relationships
- Complete tech stack
- Code patterns and conventions

### Example 2: Module Understanding (Creates module cache)

```bash
/sdlc understand src/auth
```

Generates:
- `.sdlc/arch/auth-YYYYMMDD.arch.md` - Auth module cache (~14 days)
- `.sdlc/docs/auth-module-YYYYMMDD.understand.md` - Module understanding report

Focuses on:
- Auth module structure
- Authentication flow
- Integration with rest of app
- Dependencies and data flow

### Example 3: Sub-module Understanding (Creates sub-module cache)

```bash
/sdlc understand auth/login
```

Generates:
- `.sdlc/arch/auth-login-YYYYMMDD.arch.md` - Login component cache (~7 days)
- `.sdlc/docs/auth-login-YYYYMMDD.understand.md` - Detailed understanding report

Focuses on:
- Login flow details
- Component structure
- Error handling
- Integration points

## Integration in workflow

### Feature Development
```
understand → research → spec → coding → test → verify → commit → pr
```

### Bug Fix
```
understand → debug → coding → test → verify → commit → pr
```

### Refactor
```
understand → cr → spec → coding → test → verify → cr → commit → pr
```

### Spec Writing
```
# spec uses understand's cache
/sdlc understand src/auth    # Creates auth-YYYYMMDD-arch.md
/sdlc spec "Add OAuth"       # Reads auth-arch.md for context
```

## Related Skills

- **/research** - External technology and solution research
- **/spec** - Uses architecture cache to write specifications
- **/debug** - Problem diagnosis (after understanding context)
- **/cr** - Code review and quality assessment
- **/pencil** - Create diagrams to visualize architecture
- **/doc** - Generate documentation from understanding

## Tips

- Use `understand` first when working with unfamiliar code
- Check existing cache before regenerating
- Architecture cache speeds up future spec writing
- Combine with `/pencil` for visual diagrams
- Re-run after significant changes to update cache
- Use specific scope for large codebases
- Notes on technical debt help prioritize refactor work

**See also**: `.sdlc/arch/ARCH_CACHE_SYSTEM.md` for full cache documentation
