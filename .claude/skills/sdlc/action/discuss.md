# Discuss Skill

Interactive technical discussion: present brief approaches, ask for needs, then provide detailed recommendations.

## Usage

```bash
/discuss [topic]
```

**Examples:**
```bash
/discuss State management for React app
/discuss Caching strategy for high-traffic API
/discuss How to implement file uploads?
```

## Key Principles

1. **Understand Context**: If code/files are mentioned, read them first with `Glob` → `Grep` → `Read`. Otherwise, proceed based on user's input method.
2. **Ask**: Use `AskUserQuestion` tool to gather specific requirements and preferences
3. **Brief Options**: Present 1-2 sentence summaries first
4. **Detail Later**: Provide full recommendations after user selection

## Output Format

### Initial Response

```markdown
## Problem Analysis
[Brief problem description]

## Possible Approaches
1. **[Approach 1]**: Brief description
2. **[Approach 2]**: Brief description
3. **[Approach 3]**: Brief description

## Your Needs?
Which approach interests you? What constraints? Any preferences?
```

### After User Selection

```markdown
## Recommendation: [Selected Approach]

### Implementation
[Code examples or guidance]

### Pros / Cons
- Pros: ...
- Cons: ...

### Next Steps
1. ...
2. ...
```

## Example

**Input**: `/discuss State management for React app`

**Output**:
```markdown
## Problem Analysis
React app needs to manage user data, shopping cart, and UI preferences.

## Possible Approaches
1. **React Context + useReducer**: Built-in, moderate complexity
2. **Zustand**: Lightweight, minimal boilerplate
3. **Redux Toolkit**: Full-featured with devtools

## Your Needs?
Which approach fits your requirements?
```

**If user chooses Zustand**:
```markdown
## Recommendation: Zustand

Best for simplicity. No providers, great TypeScript support.

```typescript
import { create } from 'zustand'

export const useUserStore = create((set) => ({
  user: null,
  login: (user) => set({ user }),
  logout: () => set({ user: null }),
}))
```

**Next Steps**: `bun add zustand` → create stores → integrate
```

## When to Use

- **Architecture decisions**: Patterns, microservices vs monolith
- **Technology selection**: Frameworks, libraries, tools
- **Implementation options**: Approaches to specific features
- **Technical questions**: Any technical topic needing exploration

## Dependencies

May use: `cache` (read architecture), `doc` (reference docs), `git` (understand changes)

---

**Version**: 1.0.0 | **Created**: 2026-03-08
