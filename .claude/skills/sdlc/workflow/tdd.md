# TDD Workflow

**Purpose**: Test-Driven Development workflow with behavior-first approach for AI-assisted coding.

> **Why TDD with AI?** Traditional TDD: Human writes test → Human writes code → Human refactors. **AI-assisted TDD**: Human defines behavior → AI writes tests → AI writes code → Human reviews → AI refactors. The human defines "what is correct", AI handles "how to implement it".

## When to Use

Use this workflow when:
- Building new features with complex behavior contracts
- Implementing protocol handlers, API clients, or stateful logic
- Working on code where correctness is critical
- You want to define behavior contracts before implementation
- The user asks "how do I test X" or "help me build X"

## Workflow Sequence

```
START
  │
  ▼
understand → clarify → spec → test → coding → review → test → commit → pr → MERGE
```

**Key Difference**: Tests are written **before** implementation, not after.

## Phase Details

### 0. Understand (Required First Step)
```bash
/sdlc understand [scope]
```
- Build context of the codebase architecture
- Create or reuse architecture cache in `.sdlc/docs/arch/`
- Identify relevant components and integration points
- **Do not skip this step!**

### 1. Clarify (Behavior Contract)
```bash
/sdlc clarify "Define behavior boundaries"
```
Before writing any test or code, define:

1. **What is the behavior contract?**
   - "What should this do, and what should it NOT do?"

2. **What are the boundaries?**
   - Happy path inputs/outputs
   - Edge cases (empty, null, overflow, malformed)
   - Error conditions (network failure, invalid state)

3. **What is the interface?**
   - Function signatures, API endpoints, message formats

4. **What is out of scope?**
   - Explicitly name what this unit does NOT handle

**Output**: Behavior Contract Summary
```
## Behavior Contract: <ComponentName>

**Does:**
- [ ] ...
- [ ] ...

**Does NOT:**
- [ ] ...

**Interface:**
<function/API signature>

**Key edge cases to handle:**
- ...
```

Wait for user confirmation before proceeding to Test phase.

### 2. Spec (Required Before Tests)
```bash
/sdlc spec "Design specification with test requirements"
```
- Define requirements with testability in mind
- Design APIs and interfaces
- Document data structures
- List test scenarios that must be covered

### 3. Test (Write Tests BEFORE Code)
```bash
/sdlc test --tdd-write
```
Generate test cases covering:
1. **Core happy path** ← must pass first
2. **Input boundary conditions** ← empty, min, max, malformed
3. **Error/failure conditions** ← network, invalid state, timeout
4. **Behavioral invariants** ← properties that always hold
5. **Regression anchors** ← known past bugs (if any)

**Test naming convention** — use behavior-describing names:
```python
# ✅ Good: describes behavior
def test_incomplete_sse_frame_does_not_emit_event():
def test_tool_input_json_accumulates_across_deltas():
def test_rate_limit_error_is_retryable():

# ❌ Bad: describes implementation
def test_buffer_append():
def test_parse_function():
```

**The Delete Test**: For each test, ask: *"If I deleted the implementation and rewrote it from scratch, would this test still be valid?"*
If no → the test is testing implementation, not behavior. Rewrite it.

Present tests to user. Get confirmation before generating implementation.

### 4. Coding (AI Generates Code)
```
[AI writes code to pass tests]
```
Only after tests are confirmed:
1. Generate minimal implementation that passes all tests
2. Do NOT over-engineer — implement exactly what tests require
3. No logic without a corresponding test
4. If implementation requires something not covered by tests → surface it

**Implementation principles:**
- Smallest possible surface area
- Explicit over implicit
- No logic without a corresponding test

### 5. Review (Human in the Loop)
```bash
/sdlc review
```
Structured review prompt:
```
## Implementation Review Checklist

**Behavior coverage:**
- [ ] Does it pass all the tests we wrote?
- [ ] Are there behaviors in the code NOT covered by tests?

**Design:**
- [ ] Is the interface clean?
- [ ] Are there hidden dependencies we should make explicit?

**Risk areas:**
- [ ] What's the most likely failure mode in production?
- [ ] What would break this under load?

**Missing tests (if any):**
- <list any behaviors in the implementation not yet tested>
```

### 6. Test (Run All Tests)
```bash
/sdlc test
```
Verify:
- All TDD tests pass
- No regressions in existing tests
- Test coverage is adequate

### 7. Commit
```bash
/sdlc commit
```
- Reference spec document
- Note that TDD workflow was used
- Link to issue/ticket

### 8. Pull Request
```bash
/sdlc pr
```
- Include test suite documentation
- Highlight behavior contract
- Request review

## Special Cases

### Bug Fix Workflow (TDD Style)
```
1. Write a test that REPRODUCES the bug (it should FAIL)
2. Confirm the test fails for the right reason
3. Fix the bug
4. Confirm the test now passes
5. Check no other tests broke
```
**Never fix a bug without a test. The test IS the bug report.**

### Existing Code — Adding Tests Retroactively
When the user has existing code and wants to add tests:
1. Ask: "What behavior are you most afraid of breaking?"
2. Start there — not at 100% coverage
3. Focus on behavior contracts, not line coverage
4. Use tests as safety net before refactoring

### Protocol / Streaming Code
These have uniquely high TDD value. Prioritize tests for:
- Incomplete/partial frames (buffering behavior)
- Multi-chunk reassembly (state machine correctness)
- Concurrent streams (isolation between streams)
- Error mid-stream (partial output handling)

## Usage Example

```bash
# Start TDD workflow
/sdlc start tdd "SSE message parser"

# Step 1: Understand the codebase
/sdlc understand sse/parsing
# → Creates/reuses .sdlc/docs/arch/main/sse-parsing-arch.md

# Step 2: Clarify behavior contract
/sdlc clarify "Define SSE message parsing behavior"
# → Output: Behavior Contract Summary

# Step 3: Create specification
/sdlc spec "SSE parser interface and test requirements"

# Step 4: Write tests FIRST
/sdlc test --tdd-write
# → Generates test cases for happy path, edge cases, errors
# → User confirms tests look correct

# Step 5: AI writes implementation to pass tests
# → Minimal implementation that satisfies all tests

# Step 6: Review implementation
/sdlc review
# → Structured review of behavior coverage

# Step 7: Run all tests
/sdlc test
# → All tests pass, no regressions

# Step 8: Commit
/sdlc commit

# Step 9: Create PR
/sdlc pr
```

## Anti-Patterns to Avoid

| Anti-pattern | Why it's harmful | Better approach |
|-------------|------------------|-----------------|
| "Write me code for X" | Skips behavior definition | "Here's the behavior contract, now write tests, then code" |
| Tests written after code | Tests describe implementation, not behavior | Write tests from the interface spec |
| Mocking internals | Tests break on refactor | Mock only external boundaries |
| Testing exact AI output | Brittle, non-deterministic | Test structural properties instead |
| 100% coverage as a goal | Incentivizes meaningless tests | Test behavior, coverage follows |

## Adapting to Project Phase

| Phase | TDD approach |
|-------|--------------|
| Greenfield new feature | Full TDD: clarify → test → implement |
| Bug fix | Test-first fix: reproduce with test → fix → verify |
| Refactor | Tests first as safety net, then refactor |
| Existing code, no tests | Prioritize by risk: add tests to critical paths first |
| Spike / prototype | Skip TDD, but add tests before merging |

## Completion Checklist

- [ ] **Understand phase completed**
- [ ] **Behavior contract clarified and approved**
- [ ] **Spec completed with test requirements**
- [ ] **Tests written BEFORE implementation**
- [ ] All tests pass
- [ ] No untested behavior in implementation
- [ ] Code review approved
- [ ] Committed with proper message
- [ ] PR created and merged

## Key Principles

1. **Behavior First**: Define what the code should do before writing any code
2. **Test First**: Write tests before implementation (red → green → refactor)
3. **Minimal Implementation**: Implement exactly what tests require, no more
4. **Human in Control**: Human defines behavior, AI generates code to satisfy it

---

**Version**: 1.0.0 | **Updated**: 2026-04-09
