# Lint Action Skill

Run language-appropriate linters on changed or target code, apply reasonable auto-fixes, and report remaining issues — without requiring every issue to be resolved.

## Usage

```
/sdlc lint [scope] [--report-only]
```

**Arguments:**
- `scope`: (optional) Files or directories to lint. Defaults to changed files (`git diff`).
- `--report-only`: Report issues without applying any fixes.

**Examples:**
```bash
# Lint all changed files (default)
/sdlc lint

# Lint a specific directory
/sdlc lint src/auth/

# Report only, no fixes
/sdlc lint --report-only

# Lint specific language only
/sdlc lint --lang go
/sdlc lint --lang ts
```

## Guideline

**ALWAYS follow this sequence:**

1. **Detect Scope**
   - If no scope given: use `git diff` (staged + unstaged) to identify changed files
   - If scope given: use specified files/directories
   - Skip: generated files, vendor dirs, `node_modules`, lock files, `.sdlc/` docs

2. **Detect Languages**
   - Infer from file extensions in scope:
     - `.go` → Go
     - `.ts`, `.tsx`, `.js`, `.jsx` → TypeScript/JavaScript
     - `.py` → Python
     - `.rs` → Rust
     - `.sh`, `.bash` → Shell
     - `.css`, `.scss` → CSS
     - `.md` → Markdown
   - Multiple languages in one run is expected and normal

3. **Resolve Linter — user config wins**
   - Check for explicit linter config in priority order:
     1. Project config file declares a linter (e.g. `package.json` scripts, `Makefile` lint target, `.golangci.yml`)
     2. Linter binary already installed in the project/environment (whichever is found first)
     3. **Nothing found → skip this language** and output a recommendation to the user (see Linter Resolution section)
   - Never auto-select or run a linter the user hasn't set up
   - Never auto-install linters

4. **Load Project Config**
   - Always use the project's existing lint config (`.eslintrc`, `.eslintrc.js`, `golangci.yml`, `pyproject.toml`, `.clippy.toml`, `stylelint.config.js`, etc.)
   - If the project defines a lint script (e.g. `npm run lint`, `make lint`), prefer running that over invoking the linter directly — it captures the team's exact flags
   - Never override or ignore project config

5. **Run Linters in Parallel**
   - Group changed files by top-level directory (e.g. `src/auth/`, `src/api/`, `internal/`)
   - Run lint + fix for each directory group concurrently — directories are independent
   - Also run different languages in parallel (e.g. Go and TypeScript simultaneously)
   - Collect all findings; merge into a single report at the end

6. **Classify Findings**
   - 🚨 **Critical**: Security issues, crashes, data loss risk
   - ⚠️ **Major**: Logic errors, deprecated APIs, correctness issues
   - 💡 **Minor**: Style, formatting, naming, import order

7. **Apply Reasonable Fixes in Parallel** (unless `--report-only`)
   - Apply fixes per directory concurrently — same grouping as step 5
   - Follow the Reasonableness Policy below
   - After all parallel fixes complete, re-run linter to confirm no regressions introduced

8. **Output Report**
   - Show: fixed, skipped (with reason), remaining issues
   - No `.sdlc/` document created (lightweight action)

## Linter Resolution (Priority Order)

For each language, resolve the linter to use in this order:

1. **Project script** — `package.json` `lint` script, `Makefile` `lint` target, `justfile`, etc.
2. **Installed binary** — whichever linter binary is present in the environment (first found wins)
3. **Not found → skip + recommend** — if nothing above resolves, skip that language and tell the user what to set up

**When no linter is found for a language**, output a recommendation like:

```
⚠️  Go: no linter found — consider installing golangci-lint or adding a `lint` target to your Makefile
⚠️  TypeScript: no linter found — consider installing eslint or biome and adding a `lint` script to package.json
```

Do NOT fall back to running an arbitrary linter. The user's toolchain is their choice.

**Common linters by language** (for recommendation hints only, not auto-selected):

| Language | Common options |
|----------|---------------|
| Go | `golangci-lint`, `staticcheck`, `go vet` |
| TypeScript / JavaScript | `eslint`, `biome`, `oxlint` |
| Python | `ruff`, `flake8` + `black`, `pylint` |
| Rust | `cargo clippy` |
| Shell | `shellcheck` |
| CSS / SCSS | `stylelint` |
| Markdown | `markdownlint` |

## Reasonableness Policy

Not every lint issue warrants a fix. Apply judgment:

### Auto-fix silently
- Formatting: indentation, trailing whitespace, blank lines
- Import order and unused imports (clearly dead code)
- Simple naming convention violations enforced by config
- Trivially inferable type annotations

### Fix with notice (apply but mention in report)
- Deprecated API usage with a clear replacement
- Simple missing error handling (e.g., Go `err != nil` pattern)
- Obvious type widening or narrowing

### Skip and report (do not auto-fix)
- 🚨 Critical issues — flag for manual review, never auto-fix
- Fixes that require understanding logic or intent
- Highly subjective style preferences not in project config
- Issues in test files where changing assertions would mask bugs
- Any fix that would change function signatures or public APIs

## Output Format

```
🔍 Lint Report

Scope: src/auth/ (3 files) | Languages: TypeScript, Shell
Linters: eslint ✅  shellcheck ✅

Fixed (3)
  ✅ src/auth/login.ts:12      — unused import removed (no-unused-vars)
  ✅ src/auth/session.ts:45    — trailing whitespace (no-trailing-spaces)
  ✅ scripts/deploy.sh:8       — unquoted variable (SC2086)

Skipped (2)
  ⚠️  src/auth/login.ts:89     — complex return type annotation (@typescript-eslint/explicit-return-type)
                                  → Requires logic understanding, not auto-fixable
  💡 src/auth/session.ts:102   — prefer const over let (prefer-const)
                                  → Already in-flight refactor, skipping

Remaining Issues (1)
  🚨 src/auth/login.ts:67      — possible injection risk (detect-possible-timing-attacks)
                                  → Manual review required

---
Fixed: 3 | Skipped: 2 | Requires attention: 1
```

If no issues found:

```
✅ No lint issues found.

Analyzed 3 changed files across 2 languages — clean.
```

## What `/sdlc lint` Does NOT Do

- Change business logic or functional behavior
- Auto-fix 🚨 Critical security issues (flags only)
- Override project lint configuration
- Lint generated files, vendor dirs, or `node_modules`
- Install missing linters
- Replace `/sdlc simplify` (code quality/complexity) or `/sdlc cr` (full code review)

## Difference from Related Skills

| Skill | Focus | Auto-fixes | Creates Docs |
|-------|-------|-----------|-------------|
| `lint` | Linter rule violations | Yes (safe only) | No |
| `simplify` | Complexity, reuse, readability | Yes | No |
| `cr` | Full quality/security review | No | Yes |

## State Integration

- **Reads**: Git diff, project lint configs
- **Updates**: Working tree files (auto-fixes only)
- **Creates**: No `.sdlc` documents (lightweight action)

## Completion Conditions

- [ ] Scope determined (git diff or explicit target)
- [ ] Languages detected from file extensions
- [ ] Linters checked for availability
- [ ] Project config loaded
- [ ] Linters executed
- [ ] Findings classified by severity
- [ ] Reasonable fixes applied (or skipped with reason)
- [ ] Report shown to user
- [ ] No new issues introduced by fixes

## Integration with Other Skills

```bash
# Standard workflow: after coding, before commit
/sdlc coding "implement auth"
/sdlc lint          # fix lint issues
/sdlc simplify      # optional: code quality pass
/sdlc test
/sdlc commit

# PR preparation
/sdlc lint
/sdlc cr
/sdlc pr
```

## Natural Language Triggers

- `lint`, `linting`, `linter`
- `fix lint issues`, `fix format issues`
- `run eslint`, `run golangci-lint`, `run ruff`
- `check code style`

## Related Skills

- **simplify** — Reduces complexity and improves readability (not rule-based)
- **cr** — Full code review including security, performance, architecture
- **test** — Verification after lint fixes
- **coding** — Where code is first written

---

**Version**: 1.0.0 | **Updated**: 2026-04-11
