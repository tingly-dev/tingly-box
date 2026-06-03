# Branch Context Detection

Intelligently detects branch context and base branch for SDLC operations using git state analysis.

**Purpose**: Smart branch and base detection without configuration dependency

## When to Use

Use this skill when you need to determine:
- Current branch and its base branch
- What files changed between branches
- Branch relationship (ahead/behind commits)

Invoke within other skills:
```
"Analyze git state to detect current branch and base branch using merge-base analysis and branch patterns"
```

## Detection Strategy

### 1. Merge-Base Analysis (Primary)
Find the fork point between branches using `git merge-base`:
- Check common ancestors with main/master/develop
- Count commits since fork point to validate relationship
- Prefer branches with recent merge-base

### 2. Branch Name Patterns (Secondary)
Infer base from branch naming conventions:
```
feature/*  → develop (or main if no develop)
hotfix/*   → main (or latest release branch)
bugfix/*   → develop (or main)
release/*  → develop
```

### 3. Commit Message Analysis
Parse recent commits for merge hints:
- Look for "Merge branch X into Y" patterns
- Extract target branch from merge messages

### 4. Graph Analysis
When patterns fail:
- Use `git merge-base --fork-point` to find origin
- Find branches containing the fork-point commit
- Select most likely candidate

## Expected Output

When invoked, provide:

```markdown
## Branch Context

**Current**: feature/user-auth (abc123)
**Base**: main (detected via merge-base)
**Confidence**: HIGH

**Reasoning**:
- Merge-base found at def456 (3 commits ahead, 0 behind)
- Branch name pattern: feature/* typically merges to main
- No recent merge-base with develop

**Files Changed**: 7
- Modified: src/auth/service.ts, src/auth/types.ts, tests/auth.test.ts
- Added: src/auth/middleware.ts
```

## Confidence Levels

| Level | When to Use | Action |
|-------|-------------|--------|
| HIGH | Clear merge-base + pattern match | Use detected base |
| MEDIUM | Merge-base exists but ambiguous | Use detected, explain reasoning |
| LOW | No clear pattern found | Ask user to specify |

## Scenarios

### Feature Branch
```
Current: feature/auth
Merge-base: main (3 commits ahead)
→ Base: main (HIGH confidence)
```

### Hotfix from Release
```
Current: hotfix/security-patch
Merge-base with release/v1.2 more recent than main
→ Base: release/v1.2 (HIGH confidence)
```

### Ambiguous Case
```
Current: random-branch-name
No clear merge-base, pattern doesn't match
→ Base: ??? (LOW confidence, ask user)
```

## Edge Cases

| Case | Strategy |
|------|----------|
| Detached HEAD | Find nearest branch, report commit |
| No merge-base | Use branch name patterns only |
| On main branch | No base needed, working dir only |
| Multiple candidates | Prefer main/master/develop, report ambiguity |

## Integration Example

**In pr.md**:
```
"Use branch context detection to find base branch.
If high confidence, use detected base. If low, ask user to specify."
```

**In compare.md**:
```
"Detect current and base branches, then compare changes.
Report detection confidence and reasoning."
```

## Best Practices

- Always explain detection reasoning
- Report confidence level
- Allow user override when uncertain
- Use git analysis, not config files
