# Feature Development Workflow

**Purpose**: Develop new features from research to production deployment.

> **Critical**: Always start with `/sdlc understand` and `/sdlc spec` before any coding. This ensures you understand the existing codebase architecture and have a clear specification before implementation.

## When to Use

Use this workflow when:
- Adding a new feature to the codebase
- Implementing new functionality
- Creating new capabilities or services
- Building new user-facing features

## Workflow Sequence

```
START
  │
  ▼
understand → research → spec → coding → test → validate → secure → cr → commit → pr → MERGE
```

**First Two Steps are Non-Negotiable:**
1. `understand` - Build context, create architecture cache
2. `spec` - Define requirements and design

## Phase Details

### 0. Understand (Required First Step)
```bash
/sdlc understand [scope]
```
- Build context of the codebase architecture
- Create or reuse architecture cache in `.sdlc/docs/arch/`
- Identify relevant components and integration points
- Understand existing patterns and conventions
- **Do not skip this step!**

### 1. Research
```bash
/sdlc research "Investigate technology options"
```
- Research technical approaches
- Evaluate libraries and frameworks
- Document findings
- Identify potential risks

### 2. Spec (Required Before Coding)
```bash
/sdlc spec "Design feature specification"
```
- Define requirements
- Design APIs and interfaces
- Document data structures
- Create implementation plan
- **Must be completed before coding starts**

### 3. Coding
```
[Manual coding phase]
```
- Implement the feature based on spec
- Write code following project conventions
- Add inline documentation
- **Only start after understand + spec are complete**

### 4. Test
```bash
/sdlc test
```
**What it checks:**
- lint (code style)
- typecheck (type validation)
- format (code formatting)
- unit (unit tests)
- integ (integration tests)
- e2e (end-to-end tests)
- coverage (test coverage)

**Question answered**: "Can the code run?"

### 5. Validate
```bash
/sdlc validate [target] [criteria]
```
**What it checks:**
- Harness-based validation (invariants, flows, constraints)
- Goal-based validation (user goals achievable)
- Active testing of behavior
- Dependency chain verification

**Question answered**: "Does it work correctly?"

### 6. Secure
```bash
/sdlc secure
```
**What it checks:**
- Vulnerability scanning
- Dependency security
- Secret detection

### 7. Code Review
```bash
/sdlc cr
```
**What it checks:**
- Best practices
- Architecture design
- Code maintainability
- Style consistency

### 8. Commit
```bash
/sdlc commit
```
- Create structured commit message
- Reference spec document
- Link to issue/ticket

### 9. Pull Request
```bash
/sdlc pr
```
- Create PR with description
- Include test results
- Request review

### 10. Merge
```
[Merge PR after approval]
```
- Merge to main branch
- Update changelog
- Deploy to production

## Usage Example

```bash
# Start feature workflow
/sdlc start feature "User authentication"

# Step 1: Understand the codebase (MANDATORY)
/sdlc understand auth
# → Creates/reuses .sdlc/docs/arch/main/auth-arch.md

# Step 2: Research authentication approaches
/sdlc research "Evaluate auth libraries: NextAuth vs Clerk vs custom"

# Step 3: Create specification (MANDATORY BEFORE CODING)
/sdlc spec "Define auth endpoints, session management, and security"
# → Creates .sdlc/docs/spec/auth-spec.md

# Step 4: [Manual coding - implement the feature based on spec]

# Step 5: Run tests after coding
/sdlc test

# Step 6: Validate implementation
/sdlc validate auth "authentication-invariants"

# Step 7: Security check
/sdlc secure

# Step 8: Code review
/sdlc cr

# Step 9: Commit changes
/sdlc commit

# Step 10: Create pull request
/sdlc pr

# [Merge after review approval]
```

## Anti-Pattern: What NOT to Do

```bash
# ❌ BAD: Jumping straight to coding
/sdlc start feature "User authentication"
# [Start coding immediately without understanding or spec]

# ✅ GOOD: Following the proper workflow
/sdlc understand    # Always first
/sdlc spec          # Always second
# [Then code based on understanding and spec]
```

## Completion Checklist

- [ ] **Understand phase completed** (architecture cache created/reused)
- [ ] Research documented
- [ ] Spec approved
- [ ] Code implemented (only after spec is complete)
- [ ] All tests passing
- [ ] Harness/goals validated
- [ ] Security scan clean
- [ ] Code review approved
- [ ] Committed with proper message
- [ ] PR created and merged

## Notes

- **The first two phases (understand + spec) are non-negotiable** - they prevent costly mistakes
- The **coding** phase is manual - all other phases are automated
- Use `/doc`, `/pencil`, `/cache` anytime during workflow
- Each phase validates specific quality aspects
- Can skip later phases with documented reason (but not understand + spec)
- Test phase includes multiple check types
