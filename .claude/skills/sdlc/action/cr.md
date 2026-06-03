# /cr

/cr performs code review with a specific purpose or focus area.

**Purpose**: Targeted code quality assessment

## Usage

```
/sdlc cr [what to review]
```

Describe what you want to review in natural language:

- `/sdlc cr` - Review staged changes
- `/sdlc cr unstaged changes`
- `/sdlc cr src/auth for security issues`
- `/sdlc cr changes vs main focusing on performance`
- `/sdlc cr src/api architecture`
- `/sdlc cr src/auth/login.ts type safety`

## Review Types

### Pre-PR Review (Gatekeeper)
Review before creating PR to catch issues early.
- Run on: Staged or uncommitted changes
- Focus: All categories, blocking on 🚨 Critical issues
- Output: Must pass before proceeding to `/sdlc commit`

### Post-Commit Review (Retro)
Review after merge for learning and documentation.
- Run on: Committed changes, branch diffs
- Focus: Patterns, learnings, archive-worthy insights
- Output: Saved to `.sdlc/docs/cr/` for team reference

### Targeted Review
Deep dive into a specific concern.
- Run on: Any scope (file, dir, module)
- Focus: Single area (security, performance, etc.)
- Output: Specific findings and recommendations

## Review Scope

### Code Quality
- Clean, maintainable, and well-structured code
- Proper naming conventions
- Reasonable function/class complexity
- Minimal code duplication

### Best Practices
- Error handling and edge cases
- Input validation
- Resource cleanup (connections, file handles, defer)
- Proper async/await usage

### Performance
- Algorithm efficiency
- Database query optimization (N+1 problems)
- Caching opportunities
- Bundle size considerations

### Security
- Common vulnerabilities (XSS, SQL injection, etc.)
- Authentication/authorization issues
- Data sanitization
- Secrets management

### Type Safety
- For TypeScript/Go: type correctness
- Null/undefined handling
- Proper interface usage

### Language-Specific Checks

#### TypeScript/JavaScript
- Proper type annotations
- Null/undefined handling
- Async/await error handling
- Memory leaks (event listeners, subscriptions)
- Bundle size considerations

#### Go
- Error handling patterns (don't ignore errors)
- Goroutine safety and concurrency issues
- Resource cleanup (defer, close)
- Effective use of interfaces
- Package structure and exports

## Output Format

```
## Code Review Summary

**Target**: [files/changes reviewed]
**Files Changed**: [number]
**Total Issues**: [number]

### Issues Found (N issues)

[1] 🚨 Issue description - file:line
     - Impact: [description]
     - Suggestion: [specific fix]

[2] ⚠️ Issue description - file:line
     - Impact: [description]
     - Suggestion: [specific fix]

[3] 💡 Suggestion - file:line
     - Suggestion: [specific improvement]

### ✅ Strengths
- [Good practice 1]
- [Good practice 2]

### Overall Assessment
[Brief summary and recommendations]

---
**💡 Tip**: Use issue numbers `[1]`, `[2]`, `[3]`... to request specific fixes
**💡 Legend**: 🚨 Critical | ⚠️ Major | 💡 Minor
```

## Severity Levels

- 🚨 **Critical**: Must fix (security, crashes, data loss)
- ⚠️ **Major**: Should fix (performance, maintainability, bugs)
- 💡 **Minor**: Nice to fix (style, minor improvements)

## Best Practices

### Review Process
1. **Read & Understand**: Thoroughly read the code to understand its purpose
2. **Identify Issues**: Categorize findings by severity
3. **Provide Solutions**: For each issue, suggest specific improvements
4. **Positive Feedback**: Acknowledge good patterns and practices used

### What to Look For
- **Correctness**: Does the code do what it's supposed to?
- **Readability**: Is the code easy to understand?
- **Maintainability**: Will this be easy to change later?
- **Performance**: Are there obvious performance issues?
- **Security**: Are there security vulnerabilities?

### Output Requirements
- Use severity levels (🚨/⚠️/💡)
- Number issues for easy reference [1], [2], [3]...
- Include file links in plain text format (e.g., `internal/server/openai_chat.go:270`)
- Be specific and actionable

## Completion Conditions

### Pre-PR Review
- [ ] All review categories assessed
- [ ] Issues numbered with severity levels
- [ ] No 🚨 Critical issues (blocking)
- [ ] Report saved to `.sdlc/docs/category-feature-date.cr.md`
- [ ] Code approved (PASS)

### Post-Commit Review
- [ ] Specified focus area assessed
- [ ] Key findings documented
- [ ] Report saved to `.sdlc/docs/category-feature-date.cr.md`

### Targeted Review
- [ ] Focus area deeply assessed
- [ ] Specific findings with recommendations
- [ ] Report saved to `.sdlc/docs/category-feature-date.cr.md`

## State Integration

**Pre-PR Review** (workflow phase):
- **Updates**: `sdlc.phase` = `cr`
- **Creates**: Code review report in `.sdlc/docs/category-feature-date.cr.md`
- **Requires**: `secure` phase completed
- **Next**: Proceed to `/sdlc commit` phase

**Post-Commit / Targeted Review** (standalone):
- **Creates**: Code review report in `.sdlc/docs/category-feature-date.cr.md`
- **No state updates**: Can be run anytime

## Related Skills

**Workflow Phase**:
- `/sdlc secure` - Prerequisite: security checks completed
- `/sdlc coding` - The code being reviewed
- `/sdlc commit` - Next phase after approval
- `/sdlc test` - Tests that should pass

**Standalone Use**:
- `/sdlc cr src/auth for security` - Security-focused review
- `/sdlc cr changes vs main for performance` - Performance retro review
