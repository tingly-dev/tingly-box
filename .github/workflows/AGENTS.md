# CI workflows: agent notes

**After any change under `.github/workflows/` or `.github/actions/`, lint with actionlint before committing, and keep it clean.** YAML parsing alone misses bad `needs`, wrong `with:` inputs, expression errors and shell mistakes.

```bash
go install github.com/rhysd/actionlint/cmd/actionlint@latest   # once, if `actionlint` is not on PATH
SHELLCHECK_OPTS=--severity=warning actionlint -color=false .github/workflows/*.yml
shellcheck .github/scripts/*.sh
```

With `shellcheck` on PATH, actionlint also checks the shell in every `run:`; without it that part is silently skipped. `verify-ci.yml` runs exactly these two commands on PRs that touch `.github/`.

Lint only `workflows/*.yml` (actionlint reads `actions/*/action.yml` as workflows and rejects them). For non-trivial shell in a step, run it locally against a stub `gh` or real release assets first; e.g. `file` output wording differs from what you expect.

## Rules that bite

- Never `! cmd` as an assertion: `set -e` ignores a negated command, so it never fails the step. Use `if cmd; then exit 1; fi`.
- Never put quotes inside a variable that is expanded unquoted (`TAGS="-tags 'x'"`): the quotes reach the program literally.
- Shell shared by several steps or jobs goes in `.github/scripts/` (shellcheck-able, testable against a stub `npm`/`gh`), not copy-pasted `run:` blocks.
- A job `needs:` a job that may be skipped (e.g. `frontend` when `orchestrated`) needs an explicit status check (`if: ${{ !failure() && !cancelled() }}`, or `!cancelled() && needs.x.result == 'success'`), and so does every job downstream of it: a skipped ancestor fails the default `success()`.
- A called workflow's job permissions can only narrow its caller's: grant write scopes on the `uses:` job (see `release.yml`).
- `npm.yml` is bound to npm Trusted Publishing by its file name and the `production` environment. Do not rename it.
