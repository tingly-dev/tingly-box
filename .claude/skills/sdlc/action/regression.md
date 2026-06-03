# /regression

Performs detailed regression testing to ensure branch changes don't break existing functionality.

**Purpose**: Detect functional regressions and breaking changes

## Usage

```
/sdlc regression [source-branch] [target-branch]
```

**Arguments:**
- `source-branch`: (optional) Branch to check - defaults to current branch
- `target-branch`: (optional) Base branch to compare against - defaults to `main`

**Examples:**
```bash
# Check current branch against main
/sdlc regression

# Check specific branches
/sdlc regression feature/auth main
/sdlc regression develop staging
```

## Guideline

**ALWAYS follow this sequence:**

1. **Detect Branch Context** (use `/branch detect`)
   - Get current branch name and commit
   - Detect base branch (from args, state.json, or auto-detect)
   - Verify both branches exist

2. **Identify Comparison Scope**
   - Get list of changed files
   - Categorize changes by type and impact

2. **Analyze Change Impact**
   - Identify modified functions/classes
   - Trace call chain dependencies
   - Find potential breaking changes

3. **Verify Logical Consistency**
   - Check function signature changes
   - Verify data flow integrity
   - Validate interface contracts
   - Check API compatibility

4. **Detect Functional Regression**
   - Identify removed/replaced functionality
   - Check for behavior changes
   - Verify test coverage
   - Validate edge cases

5. **Generate Comparison Report**
   - Save to `.sdlc/docs/category-feature-date.compare.md`
   - Include findings, risks, and recommendations
   - Mark status as SAFE/CAUTION/UNSAFE

## Comparison Dimensions

### 1. Logical Consistency

**Function Signature Changes:**
- Parameter additions/removals
- Return type changes
- Method visibility changes
- Interface implementation changes

**Data Flow Integrity:**
- Variable type changes
- Data transformation changes
- State mutation patterns
- Error handling changes

**Interface Contracts:**
- API endpoint changes
- Protocol/message format changes
- Dependency injection changes
- Configuration changes

### 2. Functional Regression

**Removed Functionality:**
- Deleted functions/methods
- Removed features
- Disabled capabilities
- Deprecated APIs

**Behavioral Changes:**
- Algorithm changes
- Business logic changes
- Validation rule changes
- Error handling changes

**Test Coverage:**
- Removed tests
- Broken tests
- Missing test cases
- Outdated assertions

### 3. Integration Safety

**Dependency Changes:**
- Added/removed dependencies
- Version updates
- Breaking changes in deps
- Circular dependency risks

**API Compatibility:**
- Breaking API changes
- Response format changes
- Error code changes
- Rate limit changes

**Data Schema Changes:**
- Database migrations
- Schema evolution
- Data validation changes
- Backward compatibility

## Analysis Process

### Phase 1: Change Discovery

```bash
# Get diff between branches
git diff main...current-branch --name-status

# Categorize changes
# M = Modified
# A = Added
# D = Deleted
# R = Renamed
```

**Output:**
```
Changed Files: 47
- Modified: 32
- Added: 12
- Deleted: 3
- Renamed: 0
```

### Phase 2: Impact Analysis

For each changed file:
1. **Identify affected functions/classes**
2. **Map call chain dependencies**
3. **Find cross-file dependencies**
4. **Assess change severity**

**Severity Levels:**
- 🔴 **HIGH**: Public API, core business logic, data models
- 🟡 **MEDIUM**: Internal APIs, utilities, helpers
- 🟢 **LOW**: Tests, docs, config

### Phase 3: Consistency Verification

**Function Signature Check:**
```python
# Before
def process_order(user_id: int, items: List[Item]) -> Order:
    ...

# After
def process_order(user_id: str, items: List[Item], options: dict) -> Order:
    ...

# Issues:
# - user_id type changed: int → str (BREAKING)
# - Added parameter: options (May break callers)
```

**Interface Contract Check:**
```typescript
// Before
interface UserRepository {
  findById(id: number): Promise<User | null>;
}

// After
interface UserRepository {
  findById(id: number): Promise<User>;  // null removed (BREAKING)
  findByName(name: string): Promise<User>;  // New method
}
```

### Phase 4: Regression Detection

**Check for removed functionality:**
- Functions deleted but still referenced
- Features disabled
- Config options removed
- Endpoints deprecated

**Verify behavior preservation:**
- Test results comparison
- Performance metrics
- Error handling patterns
- Edge case coverage

## Output Format

```markdown
# Comparison Report: feature/auth vs main

## Summary
**Status:** CAUTION ⚠️
**Date:** 2026-04-08
**Source:** feature/auth (commit: abc123)
**Target:** main (commit: def456)
**Files Changed:** 47 (32 modified, 12 added, 3 deleted)

## Change Categories

### 🔴 HIGH Impact Changes (5)

**[1] src/auth/service.ts**
- **Change:** `authenticate()` signature modified
- **Impact:** BREAKING - Return type changed
- **Details:**
  ```typescript
  // Before: Promise<User | null>
  // After:  Promise<AuthResult>
  ```
- **Callers affected:** 3 files
  - `src/api/login.ts:45`
  - `src/middleware/auth.ts:23`
  - `src/tests/auth.test.ts:112`

**[2] src/user/model.ts**
- **Change:** `User.email` type changed
- **Impact:** BREAKING - Type constraint relaxed
- **Details:**
  ```typescript
  // Before: email: string & EmailPattern
  // After:  email: string
  ```
- **Risk:** Invalid emails may pass validation

### 🟡 MEDIUM Impact Changes (12)

**[1] src/utils/crypto.ts**
- **Change:** Added salt parameter to `hashPassword()`
- **Impact:** May break existing password hashes
- **Mitigation:** Migration required

**[2] src/api/routes.ts**
- **Change:** Endpoint `/api/auth/refresh` removed
- **Impact:** Feature removed
- **Alternatives:** Use `/api/auth/token` instead

### 🟢 LOW Impact Changes (30)

- Test files updated
- Documentation changes
- Configuration updates

## Logical Consistency Analysis

### Function Signature Changes ❌

**Breaking Changes Found:**
1. `authenticate()` - Return type changed
2. `User.email` - Type constraint removed
3. `refreshToken()` - Removed (3 call sites)

**Recommendations:**
- Add adapter layer for backward compatibility
- Update all callers before merging
- Run full integration test suite

### Data Flow Integrity ✅

- State mutations are properly contained
- No circular dependencies detected
- Error handling is consistent

### Interface Contracts ⚠️

**API Compatibility Issues:**
1. `/api/auth/refresh` endpoint removed
2. Response format changed for `/api/auth/login`

**Action Required:**
- API version bump recommended
- Update API documentation

## Functional Regression Analysis

### Removed Functionality ⚠️

**[1] Token Refresh Endpoint**
- **File:** `src/api/routes.ts:78`
- **Impact:** Clients using refresh flow will break
- **Recommendation:** Keep endpoint, mark as deprecated

**[2] Password Strength Validator**
- **File:** `src/utils/validation.ts:45`
- **Impact:** Weaker password requirements
- **Recommendation:** Restore validator

### Behavioral Changes

**[1] Authentication Failure Handling**
- **Change:** Throws specific exception instead of returning null
- **Impact:** Callers expecting null may crash
- **Status:** ⚠️ Needs caller updates

**[2] Session Timeout**
- **Change:** Extended from 15min to 30min
- **Impact:** Security consideration
- **Status:** ℹ️ Informational

### Test Coverage

**Broken Tests:** 3
- `src/tests/auth.test.ts:145` - Uses removed endpoint
- `src/tests/integration/login.test.ts:67` - Expects null return
- `src/tests/e2e/session.test.ts:23` - Old timeout value

**Missing Tests:** 5
- New `AuthResult` type not covered
- Error handling paths not tested
- Edge cases for new options parameter

## Integration Safety

### Dependency Changes ✅

- No new dependencies added
- No version updates
- No breaking changes from deps

### API Compatibility ⚠️

**Breaking Changes:**
1. `/api/auth/refresh` removed
2. `/api/auth/login` response format changed

**Recommendations:**
- API version bump: v1 → v2
- Maintain v1 endpoint with deprecation warning

### Data Schema Changes ✅

- No database migrations required
- No schema changes
- Backward compatible

## Risk Assessment

### Overall Risk Level: MEDIUM-HIGH ⚠️

**Critical Issues:**
- 3 breaking function signature changes
- 1 public API endpoint removed
- 3 broken tests

**Recommendations Before Merge:**

1. **Must Fix (Blocking):**
   - [ ] Update all callers of `authenticate()`
   - [ ] Restore `/api/auth/refresh` or add migration guide
   - [ ] Fix broken tests
   - [ ] Restore email validation

2. **Should Fix (Important):**
   - [ ] Add tests for new `AuthResult` type
   - [ ] Document API changes
   - [ ] Consider API versioning

3. **Nice to Have:**
   - [ ] Add integration tests for new behavior
   - [ ] Performance testing for new session timeout
   - [ ] Update documentation

## Verdict

**Status:** CAUTION ⚠️

**Can Merge:** NO - Blocking issues present

**After Fixes:** YES - With API version bump recommended

**Confidence:** HIGH (comprehensive analysis completed)

---

## Check Commands

### Verify Issues

```bash
# Check function signature changes
grep -r "authenticate(" src/ --include="*.ts" | grep -v "def authenticate"

# Check for removed endpoint usage
grep -r "/api/auth/refresh" src/ tests/

# Run broken tests
npm test -- src/tests/auth.test.ts:145
```

### Fix Steps

```bash
# 1. Update function signatures
# 2. Update API version
# 3. Fix tests
# 4. Run full test suite
npm test
npm run integration
npm run e2e
```

## Re-compare After Fixes

```bash
# After making fixes, re-run compare
/sdlc compare

# Should show SAFE status
```
```

## Status Levels

### SAFE ✅
- No breaking changes
- No functional regression
- All tests passing
- Ready to merge

### CAUTION ⚠️
- Some breaking changes (documented)
- Minor behavioral changes
- Non-blocking issues
- Can merge with awareness

### UNSAFE ❌
- Critical breaking changes
- Functional regression detected
- Blocking issues present
- Should NOT merge

## Completion Conditions

- [ ] Branch comparison completed
- [ ] All changed files analyzed
- [ ] Logical consistency verified
- [ ] Functional regression checked
- [ ] Risk assessment completed
- [ ] Report saved to `.sdlc/docs/category-feature-date.compare.md`
- [ ] Status determined (SAFE/CAUTION/UNSAFE)
- [ ] Recommendations provided

## State Integration

**Standalone Use:**
- **Creates**: Comparison report in `.sdlc/docs/category-feature-date.compare.md`
- **No state updates**: Can be run anytime on any branches

**Pre-Merge Check:**
- **Recommended**: Run before `/sdlc pr`
- **Integration**: Use after `/sdlc test` passes
- **Output**: Merge decision guidance

## Related Skills

- **`action:branch`** - Branch and base detection (use this for consistent branch detection)
- **`/sdlc cr`** - Code review focuses on code quality
- **`/sdlc test`** - Test verification
- **`/sdlc validate`** - Active validation against harness
- **`/sdlc pr`** - Create PR after comparison passes

## Key Differences

| Skill | Focus | When to Use |
|-------|-------|-------------|
| `/sdlc compare` | Branch change consistency, regression | Before merging branches |
| `/sdlc cr` | Code quality, best practices | During development |
| `/sdlc validate` | Behavior verification against spec | After implementation |
| `/sdlc test` | Automated test execution | Continuous |

## Example Usage Scenarios

### Scenario 1: Pre-Merge Verification
```bash
# Develop feature branch
git checkout -b feature/auth
# ... make changes ...

# Before merging, compare with main
/sdlc compare

# Review report
cat .sdlc/docs/feature-auth-compare-20260408.compare.md

# Fix issues if needed
# ... fix breaking changes ...

# Re-compare
/sdlc compare

# If SAFE, proceed to PR
/sdlc pr
```

### Scenario 2: Continuous Comparison
```bash
# In CI/CD pipeline
/sdlc compare $CI_BRANCH origin/main

# Check exit code
if [ $? -eq 0 ]; then
  echo "SAFE to merge"
else
  echo "UNSAFE - review comparison report"
  exit 1
fi
```

### Scenario 3: Targeted Comparison
```bash
# Compare specific directories
/sdlc compare feature/payment main --scope=src/payment

# Focus on API changes
/sdlc compare develop main --focus=api

# Check for regressions only
/sdlc compare . origin/main --check=regression
```

### Scenario 4: Multi-Branch Comparison
```bash
# Compare feature branches
/sdlc compare feature/auth feature/profile

# Compare staging environments
/sdlc compare staging production

# Verify release branch
/sdlc compare release/v1.2 main
```

## Exit Codes

For CI/CD integration:
- `0` - SAFE (no issues)
- `1` - UNSAFE (blocking issues)
- `2` - CAUTION (non-blocking issues)

## Best Practices

### 1. Compare Early and Often
- Run comparison before committing major changes
- Compare with target branch daily
- Use in PR reviews

### 2. Fix Issues Promptly
- Address breaking changes immediately
- Update all affected call sites
- Run full test suite after fixes

### 3. Document Breaking Changes
- Use API versioning
- Update CHANGELOG
- Provide migration guides

### 4. Maintain Backward Compatibility
- Prefer additive changes
- Use deprecation periods
- Provide adapter layers
