# Harness Specification Phase Skill

Creates high-value verification harnesses that capture usage flows, functional constraints, and usage patterns with full dependency chains. A **harness** defines what MUST be true - creating a validation harness that any implementation must satisfy.

## Usage

```
/sdlc harness [scope] [title]
```

**Arguments:**
- `scope`: (optional) Code path, module, or reference document to analyze
- `title`: (optional) Descriptive title for the harness

**Examples:**
```bash
# Analyze existing code to extract verification rules
/sdlc harness auth "Authentication Flow Invariants"
/sdlc harness payment "Payment Transaction Constraints"
/sdlc harness src/data "Data Layer Validation Rules"

# Create from reference documentation
/sdlc harness "docs/api-contract.md" "API Contract Validation"

# Analyze current changes
/sdlc harness . "Current Implementation Validation"
```

## Guideline

**ALWAYS follow this sequence:**

1. **Understand the Source Material**
   - If `scope` is code: Read and analyze the implementation
   - If `scope` is a document: Parse and extract requirements
   - If no scope: Analyze current codebase state

2. **Identify High-Value Verification Points**
   - Usage flows (complete user journeys)
   - Functional constraints (invariants, boundaries)
   - Usage patterns (behaviors, interactions)
   - Dependency chains (traceability)

3. **Write the Verification Harness**
   - Save to `.sdlc/harness/category-feature-date.harness.md`
   - Focus on what MUST be true, not how it's implemented
   - Include complete dependency chains
   - Document both positive and negative cases

4. **Present the Verification Harness**
   - Show key invariants and flows
   - Highlight critical constraints
   - Provide actionable validation checklist

## What is a Harness?

A **Harness (verification specification)** is different from a regular spec:

| Aspect | Spec (`/sdlc spec`) | Harness (`/sdlc harness`) |
|--------|---------------------|----------------------|
| **Question** | What are we building? | What must be true? |
| **Focus** | Features, APIs, architecture | Invariants, flows, constraints |
| **Timing** | Before implementation | Anytime (before/after) |
| **Use Case** | Guide implementation | Validate correctness |
| **Output** | Implementation plan | Validation harness |

## Harness Content Structure

### 1. Overview
- Purpose and scope
- What system/feature this validates
- Criticality level (high/medium/low)

### 2. Invariants
Properties that MUST always hold true:

```markdown
## Invariants

### INV-001: User Authentication State
**MUST hold:** A user is either authenticated OR not authenticated, never both

**Validation:**
- [ ] Session token valid → user exists and is active
- [ ] Session token invalid/expired → no authenticated user
- [ ] User logout → all sessions invalidated

**Dependency:** Session Store → User Store → Auth Token Service
```

### 3. Usage Flows
Complete user journeys with validation points:

```markdown
## Usage Flows

### FLOW-001: Login Success Flow
**Entry:** User submits valid credentials
**Exit:** User authenticated with valid session

**Steps:**
1. POST /auth/login → receives credentials
   - Validate: email format, password present
2. Verify credentials against User Store
   - Validate: user exists, password matches
3. Generate session token
   - Validate: token unique, expiration set
4. Return auth response
   - Validate: contains user object, token, refresh token

**Failure Modes:**
- Invalid credentials → 401, no session created
- Inactive user → 403, no session created
- Rate limit exceeded → 429, no session created

**Dependency Chain:**
API → Auth Service → User Store → Session Store → Token Service
```

### 4. Functional Constraints
Boundaries and critical requirements:

```markdown
## Functional Constraints

### CONSTR-001: Rate Limiting
**MUST enforce:**
- Max 10 login attempts per IP per 5 minutes
- Max 5 login attempts per email per 15 minutes
- Max 100 API requests per user per minute

**Validation:**
- [ ] Rate limits configured before auth check
- [ ] Rate limits checked on every request
- [ ] Rate limit errors return proper headers (Retry-After)

**Failure Impact:** High (DoS vulnerability if not enforced)
```

### 5. Dependency Chains
Full traceability of relationships:

```markdown
## Dependency Chains

### DEP-001: User Deletion Cascade
**Trigger:** User deleted

**MUST cascade:**
1. User.deleted_at set
2. All sessions invalidated
3. All refresh tokens revoked
4. All user data anonymized (audit trail retained)
5. All dependent records soft-deleted or re-owned

**Validation Order:**
- [ ] Verify no active sessions exist
- [ ] Verify no valid tokens exist
- [ ] Verify data anonymization complete
- [ ] Verify audit trail retained

**Criticality:** High (GDPR compliance)
```

### 6. Negative Cases
What must NOT happen:

```markdown
## Negative Cases

### NEG-001: Authentication Bypass Prevention
**MUST NOT allow:**
- [ ] Session hijacking (token binding to IP/device)
- [ ] Token reuse after logout (token blacklist)
- [ ] Privilege escalation (role validation on every request)
- [ ] Timing attacks (constant-time comparison)

### NEG-002: Data Leak Prevention
**MUST NOT expose:**
- [ ] Password hashes in any response
- [ ] Internal IDs in public APIs
- [ ] Other users' data in queries
- [ ] Sensitive data in logs
```

### 7. Validation Checklist
Actionable verification steps:

```markdown
## Validation Checklist

### Pre-Implementation (if harness created before coding)
- [ ] All invariants documented
- [ ] All flows have entry/exit criteria
- [ ] All constraints have measurable thresholds
- [ ] All negative cases covered

### Post-Implementation (if harness created from existing code)
- [ ] Code implements all invariants
- [ ] Tests cover all flows
- [ ] Constraints are enforced
- [ ] Dependency chains are traceable
- [ ] Negative cases are prevented

### Continuous Validation
- [ ] Automated tests for each invariant
- [ ] Integration tests for each flow
- [ ] Monitoring for constraint violations
- [ ] Audit logs for dependency chains
```

## Output Format

Save harness to `.sdlc/harness/category-feature-date.harness.md`

**Example filenames**:
- `auth-flow-invariants-20240319.harness.md`
- `payment-constraints-20240319.harness.md`
- `sdlc-documentation-system-20240319.harness.md`

## Best Practices

### 1. Focus on High-Value Verification
Prioritize:
- **Security-critical** flows (auth, payments, data access)
- **Data integrity** constraints (consistency, validity)
- **User experience** guarantees (performance, reliability)
- **Compliance** requirements (GDPR, SOC2, etc.)

Skip:
- Trivial validation (basic input formats handled by frameworks)
- Implementation details (unless they affect correctness)
- Nice-to-have features (focus on MUST, not SHOULD)

### 2. Maintain Dependency Chains
Every harness should answer:
- **What** must be true
- **Why** it matters
- **Where** it's enforced
- **How** to verify it

### 3. Make It Actionable
Each section should enable:
- Automated testing
- Code review checklists
- Monitoring and alerting
- Compliance verification

### 4. Keep Harnesses Living Documents
- Update when implementation changes
- Reference from regular specs
- Use in code reviews
- Link to tests

## Integration with Other Phases

### With `/sdlc spec`
A `spec` describes what to build. A `harness` defines what must be true.

```bash
/sdlc spec "Add OAuth"           # Define OAuth implementation
/sdlc harness auth "Auth Invariants"  # Define auth validation rules
```

### With `/sdlc verify`
A `harness` provides the validation harness. The `verify` phase checks implementation against both `spec` and `harness`.

```bash
/sdlc verify  # Checks against spec (what we built)
              # AND harness (what must be true)
```

### With `/sdlc understand`
Use `understand` to analyze code before creating harness:

```bash
/sdlc understand auth       # Build architecture cache
/sdlc harness auth "Auth Flow Validation"  # Create harness from cache
```

## State Integration

- **Creates**: Verification harness in `.sdlc/harness/category-feature-date.harness.md`
- **Reads**: Code files, reference docs, architecture cache
- **Updates**: `sdlc.phase` = `harness`
- **References**: Specs, research docs, architecture cache

## Completion Conditions

- [ ] All invariants identified and documented
- [ ] All critical flows have validation points
- [ ] All constraints have measurable thresholds
- [ ] All dependency chains are traceable
- [ ] Negative cases are documented
- [ ] Validation checklist is actionable
- [ ] Harness saved to `.sdlc/harness/` with `.harness.md` suffix

## Related Skills

- **spec.md** - Implementation specification (complements harness)
- **verify.md** - Verification phase (uses harness as validation harness)
- **understand.md** - Architecture understanding (feeds into harness)
- **cr.md** - Code review (can reference harness)

## Example Output

See `.sdlc/docs/harness/examples/` for full examples:
- `auth-flow-invariants-harness.md`
- `payment-constraints-harness.md`
- `data-layer-validation-harness.md`

Quick example:

```markdown
# Authentication Flow Invariants - Verification Harness

## Overview
Validates critical authentication properties across login, session management, and logout flows.

**Criticality:** HIGH (Security-critical)

## Invariants

### INV-001: Mutual Exclusivity of Auth State
A user session is EITHER valid OR invalid, never ambiguous.

**Validation:**
- [ ] Token valid → session exists, user active, not expired
- [ ] Token invalid → no authenticated context
- [ ] Logout → token immediately invalid

**Dependency:** Token Validator → Session Store → User Store

### INV-002: Session Uniqueness
A user cannot have conflicting valid sessions.

**Validation:**
- [ ] New login invalidates old sessions (if single-session mode)
- [ ] Session creation atomic with user lookup
- [ ] Concurrent logups handled correctly

## Usage Flows

### FLOW-001: Successful Login
1. POST /auth/login(email, password)
   - → Validate credentials
   - → Create session
   - → Return token

**Entry:** User has valid credentials
**Exit:** User authenticated with valid session

**Validation Points:**
- [ ] Invalid credentials → 401, no session
- [ ] Valid credentials → 200, session created
- [ ] Session token properly signed

## Functional Constraints

### CONSTR-001: Token Expiration
- Access tokens: MUST expire within 15 minutes
- Refresh tokens: MUST expire within 7 days
- Logout: MUST immediately invalidate tokens

## Dependency Chains

### DEP-001: Session Creation
Auth Service → User Store (validate) → Session Store (create) → Token Service (sign)

**Order Matters:**
1. User validated BEFORE session created
2. Session created BEFORE token signed
3. Token signed AFTER session persisted

## Negative Cases

### NEG-001: Prevent Authentication Bypass
- [ ] Cannot reuse expired tokens
- [ ] Cannot elevate privileges
- [ ] Cannot bypass rate limits

## Validation Checklist

### Automated Tests
- [ ] Test INV-001: Valid token authenticates, invalid rejects
- [ ] Test INV-002: Concurrent logins handled correctly
- [ ] Test FLOW-001: Full login journey
- [ ] Test CONSTR-001: Token expiration enforced
- [ ] Test NEG-001: Bypass attempts blocked

### Code Review
- [ ] All invariants enforced in code
- [ ] All flows have error handling
- [ ] All constraints are tested
- [ ] Dependency chains respected
```
