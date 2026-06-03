# /debug

/debug is the entry point for bug fix workflow in the SDLC. This phase focuses on identifying, understanding, and preparing fixes for bugs.

**Purpose**: Debug and fix bugs in the codebase

## Usage

```
/sdlc debug [bug_description]
```

**Arguments:**
- `bug_description`: Natural language description of the bug to debug

**Examples:**
- `/sdlc debug "User login fails with valid credentials"`
- `/sdlc debug "Payment processing timeout"`
- `/sdlc debug "Data not persisting to database"`

## Debug Workflow

The debug phase follows a simplified SDLC flow:

```
/sdlc start bugfix
    ↓
debug → coding → test → verify → commit → pr
    ↓
  MERGE
```

### Phase Steps

1. **debug** - Investigate and understand the bug
2. **coding** - Implement the fix
3. **test** - Verify the fix works
4. **verify** - Ensure no regressions
5. **commit** - Commit the fix
6. **pr** - Create PR for review

## Debug Process

### 1. Understand the Bug

#### Gather Information
- Reproduction steps
- Expected vs actual behavior
- Error messages or stack traces
- Browser/console logs
- User reports

#### Ask Questions
- When does this occur?
- What triggers the bug?
- Is it consistent or intermittent?
- What changed recently?
- Who is affected?

### 2. Investigate

#### Code Analysis
```bash
# Search for related code
grep -r "error message" src/
grep -r "function name" src/

# Check recent changes
git log --oneline -10
git diff HEAD~5
```

#### Debugging Tools
- **Backend**: Debugger, logging, stack traces
- **Frontend**: DevTools, React DevTools, console
- **Database**: Query logs, explain plans
- **API**: Postman, curl, network inspector

#### Reproduce
1. Create minimal reproduction case
2. Test in different environments
3. Vary input parameters
4. Check edge cases

### 3. Root Cause Analysis

#### Common Bug Types
- **Logic errors**: Incorrect algorithms or conditions
- **Null/undefined**: Missing null checks
- **Race conditions**: Timing issues
- **Memory leaks**: Unreleased resources
- **Type mismatches**: Incorrect type assumptions
- **Configuration**: Wrong settings
- **Integration**: API contract violations

#### Document Findings
Create a debug report with:
- Bug description
- Reproduction steps
- Root cause
- Proposed fix
- Related code locations

## Debug Report Template

```markdown
# Debug: [Bug Title]

**Date**: [YYYY-MM-DD]
**Status**: [Investigating | Root Cause Found | Fix Implemented]
**Priority**: [Critical | High | Medium | Low]

---

## Bug Description
[Description of the bug from user reports or observations]

---

## Reproduction Steps
1. [Step 1]
2. [Step 2]
3. [Step 3]

**Expected**: [What should happen]
**Actual**: [What actually happens]

---

## Environment
- **Browser/OS**: [Chrome 120, macOS 14, etc.]
- **Version**: [App version]
- **User**: [Specific user type or all users]

---

## Investigation

### Logs & Errors
```
[Error messages, stack traces]
```

### Code Analysis
**File**: [path/to/file.ts]
**Function**: [functionName]
**Line**: [line number]

```typescript
// Relevant code snippet
```

**Issue**: [Description of what's wrong]

### Recent Changes
- [Recent commit that might be related]

---

## Root Cause
[Clear explanation of the bug's root cause]

---

## Proposed Fix
[Description of the fix]

### Implementation Plan
1. [Step 1]
2. [Step 2]

### Testing Plan
- [Test case 1]
- [Test case 2]

---

## Status
- [x] Bug reproduced
- [x] Root cause identified
- [ ] Fix implemented
- [ ] Tests passing
- [ ] Ready for commit

---

## References
- Issue: #[number]
- Related PR: #[number]
```

## Debug Output Example

```
Debug Investigation Report:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Bug: User login fails with valid credentials
Priority: High
Status: Root Cause Found

━━━ Reproduction Steps ━━━
1. Navigate to /login
2. Enter valid email and password
3. Click login button
4. Observe error: "Invalid credentials"

━━━ Investigation ━━━

✓ Bug Reproduced
  - Issue occurs consistently with valid credentials
  - Works with some accounts, not others
  - No errors in server logs

✓ Code Analysis
  File: src/auth/login.ts
  Function: authenticateUser()
  Line: 42-47

  ```typescript
  const user = await db.users.findOne({ email });
  if (!user || !bcrypt.compare(password, user.passwordHash)) {
    throw new UnauthorizedError('Invalid credentials');
  }
  ```

  Issue: Boolean logic short-circuits - if user is not found,
  the password comparison never runs, but error is generic.

✓ Root Cause Identified
  The password comparison happens only when user exists.
  When user is not found, the same error is thrown.
  This allows username enumeration via timing attacks.

  Additionally, new users have passwordHash stored as
  plain text due to missing pre-save hook.

✓ Recent Changes
  Commit: abc123 - "feat: add user registration"
  Modified User model, removed pre-save hook

━━━ Root Cause ━━━
The User model's pre-save hook for password hashing was
accidentally removed during registration feature implementation.
New users have plain text passwords, which always fail
bcrypt.compare().

━━━ Proposed Fix ━━━
1. Restore pre-save hook in User model
2. Hash existing plain text passwords via migration
3. Add unit test for password hashing

━━━ Next Steps ━━━
1. Implement fix: /sdlc coding
2. Test: /sdlc test
3. Verify: /sdlc verify

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Best Practices

### Debugging Mindset
- **Be systematic**: Follow a logical investigation process
- **Document everything**: Track findings, even dead ends
- **Reproduce first**: Don't fix what you can't reproduce
- **Isolate variables**: Test one thing at a time

### Root Cause Analysis
- **Ask "why" five times**: Dig deep to find true cause
- **Look upstream**: Bugs often have earlier causes
- **Check assumptions**: Verify what you "know" is true
- **Consider data**: Bad data causes many bugs

### Fix Strategy
- **Minimal changes**: Fix only what's broken
- **Add tests**: Prevent regression
- **Document**: Explain the fix
- **Verify**: Test the fix thoroughly

## Debug Output

**Always save debug reports** to `.sdlc/docs/category-feature-date.debug.md` where:
- `category` - Module/category
- `feature` - Feature description
- `date` - Date in YYYYMMDD format
- `debug` - Document type for debug reports

## Completion Conditions

- [ ] Bug reproduced and understood
- [ ] Root cause identified
- [ ] Debug report saved to `.sdlc/docs/debug/`
- [ ] Proposed fix documented
- [ ] Ready to proceed to coding phase

## State Integration

- **Updates**: `sdlc.phase` = `debug`
- **Creates**: Debug report in `.sdlc/docs/category-feature-date.debug.md`
- **Workflow**: Bug fix workflow entry point
- **Next**: Proceed to `/sdlc coding` to implement fix

## Related Skills

- `/sdlc coding` - Next phase to implement the fix
- `/sdlc test` - Verify the fix works
- `/sdlc verify` - Ensure no regressions
- `/sdlc commit` - Commit the bug fix
