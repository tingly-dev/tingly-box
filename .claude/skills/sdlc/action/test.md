# /test

/test runs comprehensive test verification to ensure code works correctly. This phase validates that the implementation functions as expected through automated checks.

**Purpose**: Verify code quality through testing (lint + typecheck + format + unit + integ + e2e)

## Usage

```
/sdlc test [check] [check...]
```

**Checks:**
- `lint` - Code style and linting checks
- `typecheck` - Type checking (TypeScript, etc.)
- `format` - Code formatting verification
- `unit` - Unit tests
- `integ` - Integration tests
- `e2e` - End-to-end tests
- `coverage` - Test coverage analysis
- `all` - Run all checks (default)

**Examples:**
- `/sdlc test` - Run all test checks
- `/sdlc test lint typecheck` - Run only lint and type check
- `/sdlc test unit` - Run only unit tests

## Execution Order

Checks run in this order (fast-fail strategy):

1. **lint** - Quick style checks (fail fast)
2. **format** - Auto-fix formatting issues where possible
3. **typecheck** - Type system validation
4. **unit** - Unit test execution
5. **integ** - Integration test execution
6. **e2e** - End-to-end test execution
7. **coverage** - Coverage analysis and reporting

## Test Categories

### 1. Lint
**Purpose**: Catch code style and potential issues early

**Tools**: ESLint, TSLint, Pylint, etc.

**Output example**:
```markdown
### Lint
✅ ESLint: 0 errors, 2 warnings
- ⚠ unused variable 'temp' (line 42)
- ⚠ consider using const instead of let (line 57)
```

### 2. Format
**Purpose**: Ensure consistent code formatting

**Tools**: Prettier, Black, gofmt, etc.

**Output example**:
```markdown
### Format
✅ Prettier: All files formatted
- Formatted 3 files automatically
```

### 3. Typecheck
**Purpose**: Validate type correctness

**Tools**: TypeScript, mypy, etc.

**Output example**:
```markdown
### Type Check
✅ TypeScript: No errors
- Checked 156 files in 2.3s
```

### 4. Unit Tests
**Purpose**: Test individual functions/components in isolation

**Coverage**: Business logic, utilities, pure functions

**Output example**:
```markdown
### Unit Tests
✅ 45/45 passed (234ms)

- auth/login - 12 tests
- auth/register - 8 tests
- utils/format - 15 tests
- components/Button - 10 tests
```

### 5. Integration Tests
**Purpose**: Test module interactions and integrations

**Coverage**: API endpoints, database operations, service interactions

**Output example**:
```markdown
### Integration Tests
✅ 12/12 passed (1.2s)

- API /auth/* endpoints - 5 tests
- Database migrations - 3 tests
- External service integrations - 4 tests
```

### 6. E2E Tests
**Purpose**: Test complete user workflow

**Coverage**: Critical user paths, cross-system flows

**Output example**:
```markdown
### E2E Tests
✅ 8/8 passed (5.6s)

- User registration and login flow
- Password reset flow
- Checkout process
- Admin dashboard navigation
```

### 7. Coverage
**Purpose**: Measure test coverage

**Thresholds**: Typically 80% minimum

**Output example**:
```markdown
### Coverage
Lines: 87% | Functions: 82% | Branches: 79%

- ⚠ src/auth/legacy.ts - Lines: 45%
- ⚠ src/utils/debug.ts - Lines: 0%

✅ Coverage above 80% threshold
```

## Full Test Report Example

```markdown
## Test Suite Results

### Lint
✅ ESLint: 0 errors, 2 warnings
- ⚠ unused variable 'temp' (line 42)
- ⚠ consider using const instead of let (line 57)

### Format
✅ Prettier: All files formatted
- Formatted 3 files automatically

### Type Check
✅ TypeScript: No errors
- Checked 156 files in 2.3s

### Unit Tests
✅ 45/45 passed (234ms)
- auth/login - 12 tests
- auth/register - 8 tests
- utils/format - 15 tests
- components/Button - 10 tests

### Integration Tests
✅ 12/12 passed (1.2s)
- API /auth/* endpoints - 5 tests
- Database operations - 3 tests
- Service integrations - 4 tests

### E2E Tests
✅ 8/8 passed (5.6s)
- User registration flow
- Password reset flow
- Checkout process

### Coverage
Lines: 87% | Functions: 82% | Branches: 79%
✅ Coverage above 80% threshold

---

## Summary

**Duration**: 8.4s
**Tests**: 65 passed, 0 failed
**Warnings**: 2 (lint)
```

## Test Output

**Always save test results** to `.sdlc/docs/category-feature-date.test.md` where:
- `category` - Module/category (e.g., `auth`, `payment`, `user`)
- `feature` - Feature description in kebab-case
- `date` - Date in YYYYMMDD format
- `test` - Document type for test reports

## Best Practices

### Writing Tests
- **Arrange-Act-Assert**: Structure tests clearly
- **One assertion per test**: Keep tests focused
- **Descriptive names**: Test names should describe what they test
- **Test boundaries**: Test edge cases and error conditions

### Test Organization
```
tests/
├── unit/              # Fast, isolated tests
│   ├── services/
│   ├── utils/
│   └── components/
├── integration/       # Module interaction tests
│   ├── api/
│   ├── database/
│   └── services/
└── e2e/              # Full workflow tests
    └── scenarios/
```

### Coverage Goals
- **Lines**: 80% minimum
- **Functions**: 80% minimum
- **Branches**: 75% minimum
- **Critical paths**: 100% coverage

## Completion Conditions

- [ ] Lint passes (or warnings acknowledged)
- [ ] Typecheck passes with no errors
- [ ] Code is properly formatted
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] All E2E tests pass
- [ ] Coverage meets threshold
- [ ] Test report saved to `.sdlc/docs/test/`

## State Integration

- **Updates**: `sdlc.phase` = `test`
- **Creates**: Test report in `.sdlc/docs/category-feature-date.test.md`
- **Requires**: `coding` phase completed
- **Next**: Proceed to `/sdlc validate` phase

## Related Skills

- `/sdlc coding` - Implementation phase that creates code to test
- `/sdlc verify` - Next phase to verify against spec
- `/sdlc debug` - Used if tests fail and debugging is needed
