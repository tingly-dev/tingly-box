# /pr

Generate pull request content that explains **why** the change exists and **what** it achieves.

**Core Principle**: A good PR describes the purpose and impact, not a laundry list of changes.

## Usage

```
/pr [base-branch] [--fetch]
```

**Flags**:
- `--fetch` — Fetch base branch from remote before diffing (default: local-only)

## Process

### 1. Resolve Base & Diff

**MANDATORY: Resolve base branch before running any git commands.**

Follow this priority order and STOP at the first match:

#### Priority 1 — Command arg
If user provided a base (e.g. `/pr develop`), use it directly. Skip detection.

#### Priority 2 — Interactive branch selection (REQUIRED when no arg)

Run this command to get recent branches with upstream tracking info:
```bash
git branch -vv --sort=-committerdate | head -10
```

This shows each branch with its upstream (e.g. `[origin/main: ahead 2]`), helping identify the right base.

**When building the candidate list:**

1. **Show local and remote tracking branches separately** — always include both local and remote as distinct options:
   - `main` (local) — diff against local HEAD
   - `origin/main` (remote tracking) — diff against remote HEAD (must have fetched locally first)

2. **Include common bases** — always add `main` and `develop` (and their `origin/` tracking refs) if they exist.

3. **Show upstream tracking status** in every option's description:
   - Local: `main (local) · behind 1`
   - Remote: `origin/main · tracks origin/main`
   - No upstream: `feature-branch · no upstream`

4. **Deduplicate by commit hash** — if multiple branches point to the same commit, group them as one option with a note listing the aliases.

5. **IMPORTANT** — When user selects a remote tracking ref like `origin/main`, keep it as-is and diff against that ref directly. DO NOT strip the `origin/` prefix — the user explicitly chose the remote ref.

Then use `AskUserQuestion` to present the candidates. Once user selects, proceed immediately — no confidence analysis needed.

---

**After base is confirmed**, validate then diff:

```bash
# local ref: git rev-parse refs/heads/<base>
# remote ref: git rev-parse <base>  (must be fetched first)
git log <base>..HEAD --oneline
git diff <base>..HEAD --stat
git diff <base>..HEAD
```

### 2. Find the Purpose (Why > What > How)

**CRITICAL**: Before writing anything, answer these three questions:

1. **Why?** What problem motivated this change?
   - What was broken, missing, or painful?
   - What triggered the work?

2. **So what?** Who benefits and how?
   - What is the user-facing impact?
   - What behavior actually changes?

3. **Now what?** What's the outcome?
   - Don't list functions/files
   - Describe the result, not the activity

**Example transformation:**
| Too Technical                                                                                                                             | Purpose-Driven                                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| "Added ClaudeClient type, moved transport logic to createSessionBoundTransport(), added AnthropicClientInterface, removed claude_util.go" | "OAuth provider support was duplicated across clients. Unified transport setup and extracted ClaudeClient for consistent behavior." |

Only after answering these questions, proceed to understand the technical details.

### 3. Write Title

- Format: `[prefix](scope): [description]`
- Prefixes: `bugfix`, `feat`, `refactor`, `doc`, `build`, `test`, `chore`
- Scope: module/area affected (e.g., `server`, `protocol`, `bot`, `frontend`). Use comma for multiple: `bot,smart_guide`
- Lowercase, under 72 chars
- Describe the **outcome**, not the action

| Bad                        | Good                                                   |
| -------------------------- | ------------------------------------------------------ |
| `feat: add login function` | `feat(auth): implement user authentication`            |
| `refactor: rename files`   | `refactor(command): unify provider command interface`  |
| `fix: bug in auth`         | `bugfix(server): resolve authentication timeout issue` |

### 4. Write Description

**Structure:**
```markdown
## Summary
[1-2 sentences: Why did we do this? What problem did it solve?]

### Major
[Core changes that define this PR - the main purpose and impact]

### Minor
[Supporting changes - refactoring, cleanup, internal improvements]
```

**Major** = The "main thing" this PR accomplishes — what problem, what impact, what changed for users.

**Minor** = Supporting work — cleanup, refactoring, non-user-facing changes.

**Before outputting, verify:**
- Does the Summary answer "Why did we do this?"
- Does Major describe outcomes, not functions/files?
- Would a non-technical stakeholder understand the impact?

### 5. Output Behavior

Output the PR content as **separate plain blocks** — do NOT inline the body into a shell command. This avoids line-wrap issues and makes the output easy to copy.

**Format:**

```
## Pull Request Ready

**Base**: main → **HEAD**: feat/foo  |  3 commits

**Title**
feat(auth): implement user authentication

**Description**
## Summary
...

### Major
- ...

### Minor
- ...

---

**Create PR:**
- GitHub web: https://github.com/[owner]/[repo]/compare/[base]...[head]
- CLI: `gh pr create --title "[full title here]" --base [base]`  (use Description block above as body)
```

**Key rules:**
- Title on its own line (no inline shell quoting)
- Description as a clean unescaped block — user copies it directly
- **GitHub compare URL must be complete and clickable** — resolve `[owner]`, `[repo]`, `[base]`, `[head]` from `git remote get-url origin` and actual branch names
- **CLI command must be complete** — include the full `--title` value; body is separate (user pastes from Description block)
- Never embed the full body inline in the shell command — keep body as its own copy block

## Example

**Title:** `refactor(command): unified provider command with interactive mode`

**Description:**
```markdown
## Summary
Provider management was scattered across 4 separate commands with inconsistent UX. Consolidated into a single interactive command.

### Major
- Single command handles all provider operations (add, list, get, update, delete)
- Interactive mode guides users through available actions
- UUID-based lookups prevent errors from duplicate provider names

### Minor
- Renamed add.go to provider_add.go for consistency
- Removed unused shell command code
```

## Related Skills

- `/commit` - Commits must exist before creating PR
- `/sdlc cr` - Code review before PR review
- `/sdlc test` - Tests that must pass

---

**Version**: 1.12.0 | **Updated**: 2026-05-10
