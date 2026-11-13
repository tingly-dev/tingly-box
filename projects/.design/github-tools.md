# Issue and PR tools for threads

> Status: **batch 1 built** (the five tools, the link record, the refresher, the project setting, the rail card). Items under [Later](#later) are proposed only.
> Builds on [mcp-tools.md](./mcp-tools.md) (one table of tools, scopes, golden files), [projects-harness.md](./projects-harness.md) and `.design/projects-thread-rail.md` (the rail and the coordinator's Status tab). The refresh back-off follows the idea of `.design/quota-backoff.md`.

## Problem

A thread opens issues and PRs by running `gh` in its shell. Three things go wrong:

1. **Authorization is repeated and unreliable.** In auto mode the classifier denies `gh pr create` with no way for the user to approve it, so `grantAutoGit` puts it on an `ask` list. The user answers the same card in every thread, and "allow for this thread" does not carry to the next one.
2. **The system knows nothing.** A PR is text in a report. Nothing records which thread opened which issue or PR, so the status bar cannot show it and the coordinator cannot tell a thread whose PR is green from one whose PR conflicts.
3. **`gh` is a poor interface for a model.** `gh pr edit` fails on this repository (Projects classic GraphQL error; `gh api -X PATCH repos/…/pulls/<n>` works), the PR template rules are re-read every time, and a push, a PR and a body are three separate commands, each of which can fail half way.

## What we build

Five tools for the thread's own session, one persisted record per issue or PR a thread touched, and one card that shows the records.

| Tool | Offered to | What it does |
| --- | --- | --- |
| `create_issue` | threads | Open an issue in the thread's repository (`title`, `body`). |
| `comment_issue` | threads | Comment on an issue or PR of the thread's repository (`number`, `body`). |
| `create_pr` | threads | Push the thread's own branch (never forced), open a PR from it (`title`, `body`, optional `base`, `draft`). If the branch already has an open PR it returns that one instead of failing. |
| `update_pr` | threads | Change `title`, `body` or `base` of a PR **this thread opened**. |
| `get_pr_status` | threads | State, draft, CI and mergeability of a PR of this thread (`number` optional when the thread has one PR); also refreshes the record. |

**Who gets them.** Worker threads and folder sessions, never the coordinator (a new `ThreadOnly` on `Tool`, the counterpart of `CoordinatorOnly`). The coordinator has no checkout, no branch and no shell, and opening a PR is the work of the thread that made the commits. What the coordinator needs is to *see* the result, so `get_thread` and the thread list show a thread's links from the record (no request to GitHub, with the age of the reading). A coordinator-side tool such as `refresh_links` is not needed yet.

**What a tool decides for itself.** The model passes content, never targets:

- *Repository*: from the thread's own checkout (`git remote get-url origin`, parsed to host/owner/name; credentials in the URL are dropped, not echoed). A remote that is not GitHub is refused. There is no `repo` input, so a thread cannot write to another repository.
- *Head branch*: `Thread.Branch`; a folder session uses the branch its folder is on. Refused when it is detached, equals the base, or is the repository's default branch.
- *Base*: the thread's `BaseBranch` unless `base` is given; a folder session's default is the repository's default branch.
- *Push*: `git push origin refs/heads/<branch>:refs/heads/<branch>`: one explicit refspec, no `+`, no `--force`, so it can only fast-forward the thread's own branch. A rejected push is an error that says to rebase; there is no retry with force.
- *PR needs commits*: refused when the branch has no commit beyond its base.
- *Transport*: `gh api` with a JSON body for every call (REST, no GraphQL). That avoids the `gh pr edit` failure and the shell quoting of bodies.
- *No footer.* The tool adds nothing to the body (no requester note, thread link or marker): the repository's PR template governs the text and the model writes it.

## Authorization

This is the part that must not turn into a way round the rules.

**Rule: a tool call is the same request as the `gh` command it replaces, and is answered the same way.** The thread's Claude Code session decides whether a call goes ahead, exactly as for `send_file` or a Bash command: through the permission mode, the settings file Projects writes for the thread, and the approval card. The tools add no approval path of their own and no tool is auto-approved by the server.

What changes is only where the user can say "yes, always":

- A new project setting, **`thread_github_access`**: `ask` (default) or `allow`. It is the "allowed items of the project settings" the user asked for.
  - `ask`: the four write tools are written to the thread's settings as `ask` rules. In auto mode the classifier would otherwise deny an unlisted MCP tool silently with no card, which is the problem in (1); an `ask` rule makes the card appear. In the other modes the tool already prompts; the rule makes it explicit.
  - `allow`: the four write tools are written as `allow` rules, so they run without a card. This is a standing decision of the project owner, made in the project settings (web; the coordinator cannot change it: `update_instructions` is the only settings tool and it does not touch this field). It is **off** until someone turns it on.
  - `get_pr_status` only reads GitHub for the thread's own PRs and is always allowed.
- `gh pr create` / `gh issue create` / `gh pr merge` stay on the `ask` list (`grantAutoGit`); the setting does not weaken them. The thread prompt says to use the tools when it has them.

Boundaries the setting cannot move (enforced in code, not in the prompt):

- **No merge, no close, no reopen, no delete branch, no force push, no label or assignee changes, no review actions.** None of these exists as a tool, and `update_pr` has no `state` field, so it cannot close a PR. When one of them is added later it needs the explicit-approval rule from "Later" in `mcp-tools.md` (the call carries the user's own message ids), and it is never covered by `thread_github_access`.
- **Only the thread's own PRs can be edited.** `update_pr` and `get_pr_status` look the number up in the thread's records, role `opened`. A PR by someone else is out of reach even if the number is known. `comment_issue` may comment on any issue or PR of the thread's repository (a thread working on "issue #12" needs to), the comment is visible in the approval card, and the record it leaves has the role `commented`.
- **Nothing the environment refused is retried here.** If the user denies a tool call, Claude Code does not call the tool. If the user denied the matching `gh` command, the thread's settings still carry that rule, and the tool is a different call that the user answers separately: there is no hidden hand-over. A denial by the host (no `gh`, not signed in) comes back as an error that says what is missing.
- **Credentials**: only the host's existing `gh` login and git credentials are used. The tools never read, log, store or return a token: the child process gets the server's environment unchanged, output is passed through a redactor (GitHub token shapes, `Authorization` headers, credentials in URLs) before it becomes an error, a log line or a tool result, and no token is a tool input.
- **One call, one effect.** Each tool does what its name says. `create_pr` pushes one branch and creates one PR; it does not create the issue, set auto-merge or enable anything else.

## The record

Table `thread_links` (one row per thread and issue or PR):

| Column | Meaning |
| --- | --- |
| `thread_id`, `project_id` | owner |
| `kind` | `issue` or `pr` |
| `repo`, `number`, `url`, `title` | identity and what the card shows |
| `role` | `opened` (the thread created it) or `commented` |
| `state` | `open`, `closed`, `merged` |
| `draft`, `ci`, `merge_state` | PR only. `ci`: `none`, `pending`, `passing`, `failing`. `merge_state`: `ready`, `conflicts`, `blocked`, `behind`, `unknown` |
| `refreshed_at`, `failures`, `last_error` | refresh bookkeeping |
| `created_at` | when the thread first touched it |

Unique on (thread, repo, kind, number). A call that succeeds writes the row from GitHub's answer (not from the model's input) before the tool returns, and publishes a `thread` event. `Thread.Links` (not stored on the thread row) carries the rows to the API and the events through `Threads.Annotate`, so the thread list, the thread, the project detail and the coordinator's view all show the same data without a new endpoint.

**If the write to the database fails** after GitHub accepted the call, the tool still reports success with the URL and says the record could not be saved; it never retries the create (that would make a duplicate).

### Threads that existed before

Not imported in batch 1. Options weighed:

- *Parse PR links out of old reports.* Cheap, but it cannot tell "I opened this" from "this is related", and it would put guesses into a table that otherwise only holds facts.
- *`gh pr list --head <branch>` per thread with a branch.* Precise (the PR whose head is the thread's branch), but one request per thread, and the user may not have a login at all.
- Proposed for a later change: the second one, run once per thread the first time its branch is seen with no PR record and only on the refresher's schedule, recorded with role `found`. Until then an old thread shows no links and a new PR it opens with the tool is recorded as usual.

## Refresh: bounded, read-only, from the server

The UI never asks GitHub; it reads the record. A background sweep keeps open items fresh:

- Runs once a minute; looks at records with `state = open` whose project is not archived; **terminal states (`closed`, `merged`) are never refreshed again**.
- An item is due `5 min` after its last refresh. After a failure the wait doubles (10, 20, 40 … capped at 1 hour), and the last good state is kept with `last_error` set; one success clears the count. Same shape as the quota back-off, kept in the row so a restart does not forget a failing item.
- At most 20 items per sweep, one at a time. A PR refresh is three REST calls (pull, check runs, combined status), so 20 open PRs at 5 minutes cost about 720 calls an hour against a 5000 limit, and fewer in practice.
- `get_pr_status` refreshes on demand but returns the stored reading if it is under 30 seconds old, so a model that polls cannot hammer GitHub.
- A change (state, CI, merge state) publishes a `thread` event so open pages update; an unchanged reading only moves `refreshed_at` and publishes nothing.
- When `gh` is missing or not signed in, every refresh fails the same way and backs off; the card says "could not refresh" with the age of the last reading.

Not done: webhooks (the host is usually behind NAT), review decisions (GraphQL only), notifying the coordinator when a PR is merged (an event line in the thread transcript is the obvious next step).

## Display

- **Thread rail** (`ThreadInfoRail`, the expanded status area): a new "Issues & PRs" card after Changes, listing each record: icon, `#number title`, state chip, and for a PR its CI and merge state; a row opens the URL. Shown only when the thread has records. Reading age in the tooltip; "could not refresh" when `last_error` is set.
- **Coordinator's Status tab**: a card with all records of the project's threads, newest first, each with the thread's title, so "which PRs are open and how are they doing" is one place.
- **Narrow layouts** that have no rail keep the header chips; a link count chip is not added in batch 1.
- Project settings dialog: one select, "Issues and PRs: ask each time / allow", under the permission mode.

## Tests

- Fake `GitHub` for the tools and the refresher; a fake `gh` executable (a shell script on `PATH`) for the real `ghCLI`: arguments, JSON body, `gh api -X PATCH` for updates, the redactor on a token in stderr.
- Tools through the in-memory MCP client: scope lists (golden and `TestToolsOfEachScope`), each tool, refusals (no origin, non-GitHub origin, default branch, base equals head, no commits, someone else's PR number), idempotent `create_pr`, a failed push leaves nothing recorded.
- Settings rules: `ask` default, `allow` when set, `get_pr_status` always allowed, `gh pr create` still on `ask`.
- Refresher with an injected clock: due time, back-off doubling and cap, terminal states skipped, last good state kept, change publishes and no change does not.
- Harness journey: a thread opens a PR through the tool, the coordinator sees it in `get_thread`; mutation-checked.
- Frontend: the rail card and the coordinator card.

## Later

Each its own change; any that closes, merges or deletes needs the explicit approval rule first.

- Import of PRs older threads opened (above).
- `merge_pr`, `close_pr`, labels and assignees, `request_review`; PR review comments and CI log reading (a thread fixing a red check).
- Continue work on an existing PR's branch (`start_thread` with a PR number), the intent `mcp-tools.md` kept for the PR-awareness design.
- Other hosts (GitLab) behind the same `GitHub` interface.
- A manual "refresh" button on the card; a transcript line when a linked PR merges or its CI turns red.
