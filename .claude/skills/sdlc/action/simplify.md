# Simplify Phase Skill

Simplify changed code on the current branch by improving readability, reducing complexity, and enhancing reuse — while preserving functional design and behavior.

## Usage

```
/sdlc simplify [scope]
```

**Arguments:**
- `scope`: (optional) Specific files or directories to focus on. Defaults to all changed files.

**Examples:**
```bash
# Simplify all uncommitted/branch changes
/sdlc simplify

# Simplify specific files
/sdlc simplify src/services/auth.ts
/sdlc simplify src/components/

# Simplify with focus area
/sdlc simplify --focus reuse
/sdlc simplify --focus complexity
```

## Guideline

**ALWAYS follow this sequence:**

1. **Detect Changes**
   - Run `git diff` and `git diff --cached` to identify changed files
   - Compare against base branch (main) to see full branch diff if needed
   - Skip files that are purely auto-generated, config, or documentation
   - Group changed files by domain/module

2. **Analyze Each Changed File**
   For each changed file, evaluate for simplification opportunities:
   - **Reusability**: Is logic duplicated within the change set? Can shared logic be extracted?
   - **Quality**: Are there code smells, unnecessary abstractions, or over-engineering?
   - **Efficiency**: Can loops, data structures, or algorithms be simplified?
   - **Readability**: Can variable names, control flow, or structure be clearer?
   - **Idiomaticity**: Does the code follow language/project conventions?

3. **Plan Simplifications**
   - List all identified simplifications with rationale
   - Classify by risk: safe (no behavior change), low-risk (minor refactor), needs-test
   - Present the plan to the user for confirmation
   - Do NOT suggest changes that alter functional design or API contracts

4. **Execute Simplifications**
   - Apply approved changes one at a time
   - Preserve all existing functionality and behavior
   - Keep changes minimal — only simplify, don't re-architect
   - Follow existing code patterns in the project

5. **Verify**
   - Run related tests to confirm no regressions
   - Run lint/typecheck if available
   - Confirm the functional design is unchanged

## Simplification Categories

### 1. Reduce Duplication (Reuse)
- Extract repeated logic into shared functions
- Consolidate similar patterns across changed files
- Remove copy-paste code within the change set

### 2. Reduce Complexity
- Flatten nested conditionals
- Replace complex boolean expressions with named variables
- Break down long functions into focused, well-named helpers
- Remove unnecessary intermediate variables

### 3. Improve Efficiency
- Replace O(n²) patterns with O(n) where possible
- Use appropriate data structures (Set vs Array for lookups)
- Eliminate redundant computations
- Simplify iteration patterns

### 4. Improve Readability
- Use descriptive variable/function names
- Simplify control flow (early returns, guard clauses)
- Remove dead code or unreachable paths
- Align with project naming conventions

## What Simplify Does NOT Do

- Change functional design or business logic
- Alter API contracts or interfaces
- Introduce new dependencies or abstractions
- Restructure module boundaries or architecture
- Add new features or change behavior
- Modify test expectations (tests should pass unchanged)

## Key Difference from Refactor

| Aspect | Refactor | Simplify |
|--------|----------|----------|
| **Scope** | Targeted file/directory | Current branch changes |
| **Goal** | Restructure, reorganize, re-architect | Reduce complexity, improve quality |
| **Input** | User-specified target | `git diff` from base branch |
| **Spec Phase** | Required | Not required (changes are small) |
| **Understand Phase** | Required | Light-weight (analyze changes only) |
| **Design Changes** | Allowed (with spec) | Not allowed |

## Output

If simplifications are found and applied, generate a summary:

```
📋 Simplification Report

**Files analyzed:** 5
**Simplifications applied:** 3

1. src/services/auth.ts — Extracted duplicate validation logic
2. src/components/UserList.tsx — Flattened nested ternary, used early return
3. src/utils/helpers.ts — Replaced Array.includes loop with Set

**Tests:** ✅ All passing
**Functional design:** ✅ Preserved
```

If no simplifications are needed:

```
✅ No simplification opportunities found.

Analyzed 3 changed files — code is clean and follows project conventions.
```

## State Integration

- **Reads**: Git diff (staged + unstaged), git diff against base branch
- **Updates**: Working tree files
- **References**: Existing tests, project conventions
- **Creates**: No `.sdlc` documents (lightweight action)

## Completion Conditions

- [ ] Git diff analyzed for all changed files
- [ ] Each file evaluated for simplification opportunities
- [ ] Plan presented to user
- [ ] Approved simplifications applied
- [ ] Tests pass (no regressions)
- [ ] Functional design preserved

## Integration with Other Phases

```bash
# After coding, before commit — clean up
/sdlc coding "implement auth"
/sdlc simplify    # clean up the changes
/sdlc test
/sdlc commit

# As part of PR preparation
/sdlc simplify
/sdlc cr
/sdlc pr
```

## Related Skills

- **refactor workflow** - For structural/architectural changes (not simplification)
- **cr** - Code review finds issues; simplify fixes them
- **coding** - Where code is first written
- **test** - Verification after simplification
- **minor workflow** - For small direct changes

## Examples

### Example 1: Simplify After Feature Work

```bash
/sdlc simplify
```

Git diff shows changes in 4 files. Analysis finds:
- `src/api/users.ts` — duplicated error handling in 3 endpoints
- `src/components/Dashboard.tsx` — deeply nested conditional rendering
- `src/hooks/useAuth.ts` — unnecessary state variable

Result: Extract error handler, use early returns, remove unused state.

### Example 2: Simplify Specific Files

```bash
/sdlc simplify src/services/
```

Only analyzes changed files within `src/services/`.

### Example 3: No Changes Needed

```bash
/sdlc simplify
```

```
✅ No simplification opportunities found.
Analyzed 2 changed files — code is clean.
```

---

**Version**: 1.0.0 | **Updated**: 2026-03-29
