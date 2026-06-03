# /commit

/commit creates a git commit with proper file selection and message formatting. Works standalone or within SDLC workflow.

**Purpose**: Commit changes with proper file selection, separation of concerns, and conventional commit messages

## Usage

```
/commit [message]
```

**Arguments:**
- `message`: Commit message (optional - auto-generates if not provided)

**Examples:**
- `/commit` - Auto-generate commit message from changes
- `/commit "feat: add user authentication"` - Use custom message

**Standalone Use:**
```bash
# Use anytime without SDLC workflow
/commit
/commit "fix: resolve login timeout"
```

**SDLC Workflow Use:**
```bash
# Part of SDLC workflow - runs pre-commit checks
/sdlc commit
/sdlc commit "feat: add oauth"
```

## File Selection Principles

### 1. Only Commit Related Files
- Commit files that are logically related to each other
- Each commit should have a single, coherent purpose
- Group files that implement the same feature/fix together

### 2. Never Commit

**Secrets & Credentials:**
- `.env`, `.env.local`, `.env.production`
- `credentials.json`, `.pem`, `.key`, `.cert`
- `secrets/`, `private/`

**IDE/Editor Files:**
- `.idea/`, `.vscode/`, `*.swp`, `*.swo`
- `.DS_Store`, `Thumbs.db`

**Build Artifacts:**
- `node_modules/`, `dist/`, `build/`, `*.lockb`
- `*.pyc`, `__pycache__/`, `.pytest_cache/`

**Test/Temp Files:**
- Files with `test-` prefix (unless explicitly requested)
- `*.log`, `npm-debug.log`, `yarn-error.log`
- `tmp/`, `temp/`, `.tmp/`

### 3. Separate Different Changes
- Different features → separate commits
- Bug fixes vs new features → separate commits
- Refactoring vs functional changes → separate commits
- Docs vs code → separate commits (unless doc change directly relates to code change)
- Different modules/packages → separate commits if logically independent

## Commit Message Format

Follow conventional commits format:

```
<type>: <subject>
```

**Types:**
- `bugfix:` - Bug fixes
- `feat:` - New features
- `command:` - Command-related changes
- `chore:` - Chores/maintenance
- `mv:` - File/directory moves
- `doc:` - Documentation changes
- `perf:` - Performance improvements
- `refactor:` - Code refactoring
- `test:` - Test additions or changes
- `ci:` - CI/CD changes

**Examples:**
```
feat: add user authentication
bugfix: fix token validation edge cases
refactor: extract validation logic to service
doc: update README with setup instructions
```

## Commit Process

### 1. Check Status
```bash
git status        # See all untracked and modified files
git diff          # See actual changes
```

### 2. Group Related Changes
- Identify logical groups among changed files
- Plan multiple commits if changes fall into distinct categories

### 3. Stage Files Selectively
```bash
git add <specific-files>    # NOT git add . or git add -A
```
Stage only the files for the current commit. Double-check: do these files all relate to the same change?

### 4. Write Commit Message
- Follow commit message format above
- Keep it focused on what changed and why
- Lowercase prefix and description

### 5. Verify and Commit
```bash
git status          # Verify staged files
git diff --staged   # Review changes
git commit -m "message"
```
Repeat for remaining changes if applicable.

## Commit Examples

**Scenario 1: Single focused change**
```
Changed files: auth.go, auth_test.go, login.html
All related to login feature
→ Single commit: "feat: add user login functionality"
```

**Scenario 2: Multiple unrelated changes**
```
Changed files:
- user.go, user_test.go (new feature)
- auth.go (bugfix)
- README.md (docs)
- .env.local (should be ignored!)

→ Commit 1: "bugfix: fix token validation in auth"
→ Commit 2: "feat: add user profile management"
→ Commit 3: "doc: update README with new features"
→ Ignore .env.local
```

**Scenario 3: With suspicious files**
```
Changed files:
- api.go (feature)
- .env (NEVER COMMIT)
- test-api.sh (test file, skip unless asked)

→ Commit: "feat: add new API endpoints"
→ Warn about .env, skip test-api.sh
```

## SDLC Integration

When used in SDLC workflow (`/sdlc commit`), additional checks apply:

### Pre-commit Checklist

**Code Quality:**
- [ ] All tests passing (`/sdlc test`)
- [ ] Verification complete (`/sdlc verify`)
- [ ] Security scan passed (`/sdlc secure`)
- [ ] Code review approved (`/sdlc cr`)

**Documentation:**
- [ ] Spec document exists (if applicable)
- [ ] Test reports saved
- [ ] Verification report saved
- [ ] Security report saved
- [ ] Code review saved

**Files:**
- [ ] No unintended changes
- [ ] No sensitive data committed
- [ ] No debug console.logs
- [ ] No TODO comments without issues

### Enhanced Commit Output (SDLC Mode)

```
━━━ Pre-commit Checklist ━━━

Code Quality:
✓ All tests passing
✓ Validation complete (100% requirements met)
✓ Security scan passed (no critical issues)
✓ Code review approved

Documentation:
✓ Spec document: .sdlc/docs/auth-user-login-20240319.spec.md
✓ Test report: .sdlc/docs/auth-user-login-20240319.test.md
✓ Validation: .sdlc/docs/auth-user-login-20240319.validate.md
✓ Security: .sdlc/docs/auth-user-login-20240319.secure.md
✓ Code review: .sdlc/docs/auth-user-login-20240319.cr.md

Files:
✓ No unintended changes
✓ No sensitive data committed
✓ No debug code remaining

━━━ Changes to Commit ━━━

Files: 6 changed, 0 deleted
  src/auth/register.ts       | +45 new lines
  src/auth/login.ts          | +38 new lines
  src/auth/service.ts        | +89 new lines
  src/auth/middleware.ts     | +23 new lines
  src/types/auth.ts          | +15 new lines
  tests/unit/auth.test.ts    | +67 new lines

━━━ Commit Message ━━━

feat: add JWT-based authentication

Implement JWT-based authentication system with:
- User registration endpoint (POST /api/auth/register)
- User login endpoint (POST /api/auth/login)
- Token refresh mechanism (POST /api/auth/refresh)

Features:
- bcrypt password hashing (10 rounds)
- JWT tokens (15min access, 7d refresh)
- Email verification
- Rate limiting

Spec: .sdlc/docs/auth-user-login-20240319.spec.md
Tests: 45/45 passing
Coverage: 87%

Closes #123

━━━ Commit Action ━━━
Ready to commit. Use 'git commit' to finalize.
```

## Best Practices

### Commit Granularity
- **One feature per commit**: Keep commits focused
- **Atomic changes**: Each commit should be self-contained
- **Logical grouping**: Group related files together
- **Small commits**: Easier to review and revert if needed
- **Multiple commits**: Better than one large mixed commit

### Commit Message Tips
- **Use imperative mood**: "Add feature" not "Added feature"
- **Limit subject line**: 50 characters or less
- **Reference issues**: Link to related issue/PR numbers
- **Explain why**: Describe the reason for the change
- **Lowercase**: Use lowercase for type and description

### What to Include
- Changed source files
- Test files
- Documentation updates
- Configuration changes

### What to Exclude
- Generated files (lock files, build artifacts)
- IDE settings (.vscode, .idea)
- Environment files (.env.local)
- Debug code
- Sensitive credentials

## Safety Protocols

### Git Safety
- **ALWAYS stage specific files by name**, not `git add .` or `git add -A`
- **NEVER commit secrets**: .env, credentials.json, .pem, .key files
- **ALWAYS review** what you're staging before committing
- **When in doubt**, ask: "Do these changes belong together?"

### Hook Failures
- After pre-commit hook failure, create NEW commit (don't amend)
- Fix the issue, re-stage, and commit again

## Completion Conditions

- [ ] Only related files staged
- [ ] No secrets or sensitive data committed
- [ ] Commit message follows format
- [ ] Commit created successfully
- [ ] (SDLC only) All pre-commit checks passed

## State Integration

- **Updates**: `sdlc.phase` = `commit`
- **Creates**: Git commit
- **Creates**: Commit log in `.sdlc/docs/category-feature-date.commit.md` (SDLC only)
- **Requires**: (SDLC only) `cr` phase completed with approval
- **Next**: (SDLC only) Proceed to `/sdlc pr` phase

## Related Skills

- `/git` - Low-level git operations (status, diff, branch)
- `/sdlc cr` - (SDLC only) Prerequisite: code review must be approved
- `/sdlc pr` - (SDLC only) Next phase: create pull request
- `/sdlc test` - (SDLC only) Tests that must pass before committing
