# Validate Phase Skill

Performs active verification against harness specifications or user goals. Validates that the system behaves correctly through testing.

## Usage

```
/sdlc validate [target] [criteria]
```

**Arguments:**
- `target`: (optional) What to validate - code path, module, or "harness" name
- `criteria`: (optional) Validation criteria - harness file, goal description, or "all"

**Examples:**
```bash
# Validate against a harness
/sdlc validate auth "auth-flow-invariants"
/sdlc validate harness "auth-flow-invariants"

# Validate against user goals
/sdlc validate login "User can login with valid credentials"
/sdlc validate payment "Payment succeeds with valid card"

# Validate everything
/sdlc validate . all

# Auto-detect and validate
/sdlc validate
```

## Guideline

**ALWAYS follow this sequence:**

1. **Determine Validation Mode**
   - If `criteria` is a harness file → Harness-based validation
   - If `criteria` is a goal description → Goal-based validation
   - If neither specified → Auto-detect from context

2. **Load Validation Criteria**
   - **Harness mode**: Read harness from `.sdlc/harness/*.md`
   - **Goal mode**: Parse user goal into testable criteria
   - **Auto mode**: Find relevant harness or ask user

3. **Perform Active Validation**
   - Run tests related to the validation criteria
   - Execute code paths to verify behavior
   - Check invariants and constraints
   - Document findings

4. **Generate Validation Report**
   - Save to `.sdlc/docs/category-feature-date.validate.md`
   - Include test results, findings, and recommendations
   - Mark as PASSED/FAILED/PARTIAL

## Validation Modes

### 1. Harness-Based Validation

Validates implementation against a harness specification.

**What it checks:**
- All invariants hold true
- All flows work correctly
- All constraints are enforced
- All dependency chains are intact
- Negative cases are prevented

**Process:**
```bash
/sdlc validate auth "auth-flow-invariants"
```

1. Load harness: `.sdlc/harness/auth-flow-invariants-20240319.harness.md`
2. For each invariant:
   - Run related tests
   - Execute code paths
   - Verify the invariant holds
3. For each flow:
   - Execute the flow
   - Check entry/exit criteria
   - Verify validation points
4. Document results

**Output example:**
```markdown
# Validation Report: auth-flow-invariants

## Summary
**Status:** PASSED ✅
**Date:** 2026-03-19
**Harness:** auth-flow-invariants
**Target:** src/auth/

## Invariants Validation

### INV-001: Mutual Exclusivity of Auth State
**Status:** ✅ PASSED

**Tests:**
- ✅ Valid token authenticates user
- ✅ Invalid token returns 401
- ✅ Logout invalidates token immediately

**Evidence:**
- Test: `test/auth/auth.test.ts:45-67` → PASSED
- Code: `src/auth/middleware.ts:23-31` → Verified

### INV-002: Session Uniqueness
**Status:** ⚠️ PARTIAL

**Tests:**
- ✅ New login creates unique session
- ✅ Session creation is atomic
- ⚠️ Concurrent logups: Race condition detected

**Issues:**
- Race condition when multiple logups from same user
- Location: `src/auth/session.ts:89`

## Flows Validation

### FLOW-001: Successful Login
**Status:** ✅ PASSED

**Validation Points:**
- ✅ Invalid credentials → 401
- ✅ Valid credentials → 200, session created
- ✅ Session token properly signed

**Test Results:**
- `POST /auth/login` with valid creds → 200 OK
- `POST /auth/login` with invalid creds → 401 Unauthorized
- Response contains user, token, refreshToken

## Constraints Validation

### CONSTR-001: Token Expiration
**Status:** ✅ PASSED

**Verification:**
- ✅ Access tokens expire in 15 minutes
- ✅ Refresh tokens expire in 7 days
- ✅ Logout invalidates immediately

## Dependency Chains

### DEP-001: Session Creation
**Status:** ✅ VERIFIED

**Chain:** Auth Service → User Store → Session Store → Token Service

**Verification:**
- User validated BEFORE session created ✅
- Session created BEFORE token signed ✅
- Transaction atomicity verified ✅

## Negative Cases

### NEG-001: Authentication Bypass Prevention
**Status:** ✅ PASSED

**Verification:**
- ✅ Cannot reuse expired tokens
- ✅ Cannot elevate privileges
- ✅ Cannot bypass rate limits

## Issues Found

### MEDIUM: Session Race Condition
**Location:** `src/auth/session.ts:89`
**Description:** Concurrent logups can create duplicate sessions
**Impact:** MEDIUM (affects session uniqueness guarantee)
**Recommendation:** Add unique constraint on (user_id, session_id)

## Conclusion

**Overall Status:** PASSED with notes

**Strengths:**
- All invariants hold under normal operation
- All flows work correctly
- All constraints enforced

**Areas for Improvement:**
- Fix race condition in concurrent logups
- Add stress tests for concurrent operations

**Next Steps:**
1. Fix session race condition
2. Add concurrent operation tests
3. Re-validate after fixes
```

### 2. Goal-Based Validation

Validates that a user goal is achievable.

**What it checks:**
- The goal can be accomplished
- All required steps work
- Error handling is appropriate
- User experience is acceptable

**Process:**
```bash
/sdlc validate login "User can login with valid credentials"
```

1. Parse goal into testable criteria
2. Identify relevant code paths
3. Execute the user journey
4. Verify the goal is met
5. Document results

**Output example:**
```markdown
# Validation Report: User Login Goal

## Goal
"User can login with valid credentials"

## Status
**PASSED** ✅

## Criteria Breakdown

### 1. User can submit credentials
**Status:** ✅ PASSED

**Verification:**
- Login endpoint exists: `POST /auth/login`
- Accepts email and password
- Validates input format

### 2. Valid credentials authenticate user
**Status:** ✅ PASSED

**Verification:**
- Credentials checked against user store
- Correct password returns auth token
- Response includes user data

### 3. Invalid credentials are rejected
**Status:** ✅ PASSED

**Verification:**
- Wrong password returns 401
- Non-existent user returns 401
- Error message is generic (security)

### 4. Session is established
**Status:** ✅ PASSED

**Verification:**
- Auth token is valid
- Token contains user identity
- Subsequent requests authenticated

## User Journey Test

**Steps:**
1. User navigates to login page ✅
2. User enters email and password ✅
3. User clicks login button ✅
4. System validates credentials ✅
5. User is redirected to dashboard ✅
6. User can access protected resources ✅

**Total Time:** ~2.3 seconds
**User Experience:** GOOD

## Issues

None found.

## Recommendations

- Consider adding "remember me" option
- Add password strength indicator
- Consider social login options
```

### 3. Auto-Detection Mode

When no criteria specified, automatically detects what to validate.

**Process:**
1. Check for recent harness files
2. Check for recent code changes
3. Ask user what to validate
4. Perform validation

```bash
/sdlc validate
# → Detected recent harness: auth-flow-invariants
# → Detected recent changes: src/auth/login.ts
# → Validate auth-flow-invariants against current implementation? [Y/n]
```

## Validation vs Verification

| Aspect | Verify (`/sdlc verify`) | Validate (`/sdlc validate`) |
|--------|------------------------|---------------------------|
| **Question** | Did we build what we planned? | Does it work correctly? |
| **Method** | Static comparison | Active testing |
| **Input** | Spec + Implementation | Harness/Goal + Implementation |
| **Output** | Coverage report | Test results + findings |
| **Timing** | After implementation | Anytime (during/after) |

## Best Practices

### 1. Validate Early and Often
- Run validation after each implementation change
- Validate before committing
- Use in CI/CD pipeline

### 2. Use Harnesses for Critical Systems
- Security-critical flows (auth, payments)
- Data integrity constraints
- Compliance requirements

### 3. Use Goals for User Features
- New feature development
- User experience validation
- Integration testing

### 4. Document Findings
- Always save validation reports
- Track issues and recommendations
- Follow up on failures

## Integration with Other Phases

### With `/sdlc test`
```bash
/sdlc test unit     # Run unit tests
/sdlc validate ... # Run validation (includes integration/e2e testing)
```

### With `/sdlc harness`
```bash
/sdlc harness auth "Auth Invariants"  # Create validation harness
/sdlc validate auth "auth-invariants" # Validate against harness
```

### With `/sdlc coding`
```bash
/sdlc coding "Add login"
/sdlc validate login "User can login"  # Verify goal achieved
```

## State Integration

- **Creates**: Validation report in `.sdlc/docs/category-feature-date.validate.md`
- **Reads**: Harness files, code, tests
- **Updates**: `sdlc.phase` = `validate`
- **References**: Harness, specs, test reports

## Completion Conditions

- [ ] Validation mode determined
- [ ] Criteria loaded (harness or goal)
- [ ] Active validation performed
- [ ] All invariants/goals tested
- [ ] Findings documented
- [ ] Report saved to `.sdlc/docs/validate/`
- [ ] Status determined (PASSED/FAILED/PARTIAL)

## Validation Status Levels

### PASSED ✅
- All criteria met
- All tests passing
- No issues found

### PARTIAL ⚠️
- Most criteria met
- Some issues found (non-blocking)
- Recommendations provided

### FAILED ❌
- Critical criteria not met
- Blocking issues found
- Fixes required before proceeding

## Output Format

Save validation reports to `.sdlc/docs/category-feature-date.validate.md`

**Example filenames**:
- `auth-invariants-20240319.validate.md`
- `login-goal-20240319.validate.md`
- `payment-flow-20240319.validate.md`

## Related Skills

- **harness.md** - Create validation harnesses
- **test.md** - Run automated tests
- **spec.md** - Implementation specifications

## Example Usage Scenarios

### Scenario 1: Validate Critical Auth Flow
```bash
# First, create a harness for auth
/sdlc harness auth "Authentication Invariants"

# Implement the auth system
/sdlc coding "Implement authentication"

# Validate against the harness
/sdlc validate auth "authentication-invariants"

# Review the validation report
cat .sdlc/docs/validate/20260319-authentication-invariants-validation.md
```

### Scenario 2: Validate User Goal
```bash
# Implement a feature
/sdlc coding "Add password reset"

# Validate the user goal
/sdlc validate reset "User can reset password via email"

# Check if goal is met
cat .sdlc/docs/validate/20260319-password-reset-validation.md
```

### Scenario 3: Continuous Validation
```bash
# Run in CI/CD pipeline
/sdlc validate . all

# Or validate specific module
/sdlc validate payment all

# Check status
echo $?  # 0 = PASSED, 1 = FAILED, 2 = PARTIAL
```

### Scenario 4: Validate After Bug Fix
```bash
# Fix a bug
/sdlc debug "Session not invalidating"

# After fix, validate the harness
/sdlc validate auth "session-invariants"

# Ensure fix doesn't break other invariants
```

## Exit Codes

For CI/CD integration:
- `0` - All validations PASSED
- `1` - One or more validations FAILED
- `2` - One or more validations PARTIAL (warnings)
