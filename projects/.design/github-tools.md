# Issue and PR tools for threads

> Status: **batch 1 built** (the five tools and, later, `merge`, the link record, the refresher, the project setting, the rail and Status-tab cards, the coordinator's text view) and **PR amending built** ([Putting a PR right](#putting-a-pr-right)) and **stacks built** (`create_pr` takes a `head`; see [Stacked work](#stacked-work-more-than-one-branch)) and **branch scope, force-with-lease and approval asks built** ([Which branches a thread may push, and what asks the user](#which-branches-a-thread-may-push-and-what-asks-the-user)). Items under [Later](#later) are proposed only. `file:line` references are not used; search by name.
> Builds on [mcp-tools.md](./mcp-tools.md) (one table of tools, scopes, golden files), [projects-harness.md](./projects-harness.md) and `.design/projects-thread-rail.md` (the rail and the coordinator's Status tab). The refresh back-off follows the idea of `.design/quota-backoff.md`.

## Problem

A thread opens issues and PRs by running `gh` in its shell. Three things go wrong:

1. **Authorization is repeated and unreliable.** In auto mode the classifier denies `gh pr create` with no way for the user to approve it, so `grantAutoGit` puts it on an `ask` list. The user answers the same card in every thread, and "allow for this thread" does not carry to the next one.
2. **The system knows nothing.** A PR is text in a report. Nothing records which thread opened which issue or PR, so the status bar cannot show it and the coordinator cannot tell a thread whose PR is green from one whose PR conflicts.
3. **`gh` is a poor interface for a model.** `gh pr edit` fails on this repository (Projects classic GraphQL error; `gh api -X PATCH repos/…/pulls/<n>` works), the PR template rules are re-read every time, and a push, a PR and a body are three separate commands, each of which can fail half way.

## What we build

Six tools for the thread's own session (in the table below), one persisted record per issue or PR a thread touched, and one card that shows the records.

| Tool | What it does |
| --- | --- |
| `create_issue` | Open an issue in the thread's repository (`title`, `body`). |
| `create_pr` | Push a local branch of the thread's checkout (a fast-forward, or `--force-with-lease` with `force`), open a PR from it (`title`, `body`, optional `head`, `base`, `draft`, `force`). `head` is the thread's own branch unless another local branch is named. If the branch already has an open PR it pushes the branch again (so the PR carries the latest commits) and returns that PR instead of opening a second one; for a PR the thread opened it also puts the PR right to what the call says (see [Putting a PR right](#putting-a-pr-right)). |
| `update` | Change `title` and `body` of an issue, or `title`, `body` and `base` of a PR, **that this thread opened**; addressed by `number` (optional only when the thread has one PR). |
| `comment` | Comment on an issue or PR of the thread's repository (`number`, `body`). |
| `view` | Read an issue or PR of the thread's repository by `number`: state and description, and for an open PR its checks and mergeability. The one read tool. |
| `merge` | Merge the thread's own branch on GitHub, by `method`: `squash` (push, open the PR if none is open, then squash-merge it) or `merge_request` (push and open the PR only). See [Merge](#merge). |

### Why five, and why these (`merge` came later, see below)

One tool per action would be seven (add `update_issue`, `get_issue`) and keeps growing with every action. The opposite, one tool per object with an `action` argument, would put reads and writes under one name, and the permission rules are written per tool name (below): "reads always, writes ask" could no longer be said. So the split follows what differs in practice:

- **Issues and PRs share a number and most actions**, so `update`, `comment` and `view` take a `number` and work for either. `comment` already needed this (the issues API serves PRs); it looks the kind up and the record gets the right one.
- **Creation stays two tools**: a PR needs a head, a base and a push, an issue needs none of them; one `create(kind=…)` would make the required fields depend on a value.
- **Reads are one tool, writes four**, so the read rule is one line.

GitHub's own MCP server went the same way after starting with one tool per action: it merged its issue and PR tools into `issue_read`/`issue_write` and `pull_request_read` with a `method` argument (to cut the tokens its tool list cost), and kept `create_pull_request` and `update_pull_request` separate. We have far fewer tools (the five cost about 550 tokens, thread-side only), so we keep a name per verb and no `method` argument; `view` can take an optional `include` later (comments, diff) the way theirs does, without a new tool. Sources: github/github-mcp-server [PR #1211](https://github.com/github/github-mcp-server/pull/1211) and its [Oct 2025 changelog](https://github.blog/changelog/2025-10-14-github-mcp-server-now-supports-github-projects-and-more/).

### Merge

`merge` is the one tool that cannot be taken back, so it follows different rules from the other five:

- **Two methods, both on GitHub.** `squash` pushes the branch, finds or opens its PR (the `create_pr` code), then `PUT pulls/N/merge` with `merge_method: squash` (an empty `message` is worded as `gh pr merge --squash` words it: the PR's title and `(#N)`, and its description) and the pushed head `sha` (GitHub refuses if the head moved). `merge_request` stops after the PR is open. Nothing is merged in the local checkout and the branch is not deleted. If GitHub refuses the merge (conflicts, required checks, protection rules, a draft) the error says so and the PR stays open and linked.
- **The same code serves the user's Merge button.** `Links.Merge` is the implementation; the tool calls it from a thread's turn, and `POST /threads/:id/merge` (the Merge dialog) calls it as an independent entry point, where the click is the user's approval. Neither closes the thread (`thread-lifecycle.md`: only the user closes a thread): a squash from the dialog records `MergedAt` and the thread rests again. After a tool merge the record says `merged`; a later Merge click only records `MergedAt` (GitHub is not asked again) while that merge still holds the branch as it stands (no open PR for it, nothing unpushed); commits made after it are merged with a new PR.
- **Every call asks the user.** `alwaysAskTools` lists `merge`: `grantBuiltinTools` writes an `ask` rule for it in every project and permission mode (it wins over the server-wide allow), and the approval prompt never answers for it (`builtinToolAllowed`), so a card reaches the user whatever `thread_github_access` says. The prompt tells the thread to merge only when the user asked, with this tool, and never by hand. Not probed against the real CLI in `bypassPermissions` mode, where Claude Code may skip `ask` rules; the "carries the user's own message ids" form in `mcp-tools.md` (Later) is the stronger check if that matters.
- Still true: no close, no reopen, no delete branch, and no force push except `create_pr`'s `force` (leased; see [Which branches a thread may push, and what asks the user](#which-branches-a-thread-may-push-and-what-asks-the-user)).

**Who gets them.** Worker threads and folder sessions, never the coordinator (a new `ThreadOnly` on `Tool`, the counterpart of `CoordinatorOnly`). The coordinator has no checkout, no branch and no shell, and opening a PR is the work of the thread that made the commits. What the coordinator needs is to *see* the result, so `get_thread` and the thread list show a thread's links from the record (no request to GitHub, with the age of the reading). A coordinator-side tool such as `refresh_links` is not needed yet.

**What a tool decides for itself.** The model passes content, never targets:

- *Repository*: from the thread's own checkout's origin, in any form, parsed to owner/name (see [Which remote](#which-remote-identity-and-transport)); credentials in the URL are dropped, not echoed. A remote that is not `github.com` is refused with the host it is on (GitHub Enterprise: later). There is no `repo` input, so a thread cannot write to another repository.
- *Head branch*: `head` if given, else `Thread.Branch`; a folder session uses the branch its folder is on. A named head has to be a valid name and an existing **local** branch of the checkout (`refs/heads/<head>`; remote-only names, refspecs and options are refused). Refused when it is detached, equals the base, is the repository's default branch, or GitHub says it is protected.
- *Base*: `base` if given (any branch on GitHub, including another branch of a stack), else the thread's `BaseBranch` for the thread's own branch, and the repository's default branch for any other head. The base must already be on GitHub (`git ls-remote`, the push's way, before anything is pushed): it is never pushed by this call, so a PR for the lower branch comes first.
- *Push*: `git push https://github.com/<owner>/<name>.git <sha>:refs/heads/<head>` the tools' way (not over origin: [Which remote](#which-remote-identity-and-transport)), then `refs/remotes/origin/<head>` is moved to `<sha>` as a push to origin would: one explicit refspec, no `+`, no `--force`, so it can only fast-forward that one branch (`force` adds a lease, below, on the same transport). A push that lost to newer commits on origin (`non-fast-forward`, `fetch first`) is an error that says to rebase; any other rejection (a hook, a protected branch) is reported as it is, without the rebase advice. There is never an automatic retry with force: the model asks for it.
- *PR needs commits*: refused when the branch has no commit beyond its base.
- *Read before write*: `comment` reads the item in full (the issues API, then the PR's own read for a PR) before it posts, so a failed read posts nothing and never leaves a record built from half an answer (a merged PR must not be recorded as closed).
- *An edit keeps what it cannot know*: the answer to a PR edit carries no checks and often a mergeability GitHub has not worked out yet (`unknown`); `update` keeps the last known value of both until the next reading.
- *Transport*: `gh api` with a JSON body for every call. Writes are REST; every read is one GraphQL query we write ourselves (see [Cost of a read](#cost-of-a-read)). Neither touches `gh pr edit`'s failing query, and the bodies never pass a shell.
- *No footer.* The tool adds nothing to the body (no requester note, thread link or marker): the repository's PR template governs the text and the model writes it.

### Which remote: identity and transport

Origin answers two questions, kept apart, and origin itself is never rewritten (`remote.go`):

- **Which repository** (owner/name, for the API and the record): read from `remote.origin.url` as configured, in any of git's forms: `https://github.com/o/r(.git)(/)` (with or without a user or token in it), `http://`, `git://github.com/o/r.git`, `ssh://git@github.com(:22)/o/r.git`, `git+ssh://`, the scp-like `git@github.com:o/r(.git)` and `github.com:o/r` (no user), any case of the host, and `ssh.github.com` (GitHub's SSH over 443). Owner and name must have GitHub's shape (`..`, `%20`, a leading `-` are refused), as they become an API path and a URL. An SSH host that is not github.com may be an alias (`Host github.com-work` in `~/.ssh/config`): `ssh -G <host>` (reads the config, connects nowhere; the host must look like a host name, so it cannot be an option) says what it stands for. A value that is none of these (a shorthand such as `gh:o/r`) is read again as git expands it (`git remote get-url origin`, which applies `url.<base>.insteadOf`). Errors name the host and nothing else of the URL: "origin is on gitlab.com, not github.com …", "origin is on gl, an SSH alias for gitlab.com …", "… whether it is an SSH alias for github.com could not be read (`ssh -G …`: …)".
- **How git reaches it** (push, and `ls-remote` for the base check): never over origin. A thread has no terminal, so an SSH key with a passphrase, an agent that asks for confirmation or a hardware key that wants a touch fails or hangs there, and the user's git setup (a global `url."git@github.com:".insteadOf https://github.com/`, a credential helper that would store a token, the checkout's own config and hooks, which the thread's shell can write) is not the tools'. So the tools' git:
  - runs from a **scratch bare repository** (`git init --bare --template=` in the server's temp dir, removed after) whose `objects/info/alternates` points at the checkout's `<git-common-dir>/objects` (a shallow clone's `shallow` file is copied too), so none of the checkout's config or hooks are read, and with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, so none of the user's either; every `GIT_*` variable of the process is dropped (one could point git elsewhere);
  - pushes to `https://github.com/<owner>/<name>.git`, built from the parsed identity, never from origin's URL;
  - signs in with the credential from `GitHubAuth.Credential` (today `gh auth token --hostname github.com`, user `x-access-token`). Its config comes in `GIT_CONFIG_COUNT/KEY/VALUE` variables: `credential.helper=` (clears the list) and a helper for `https://github.com` only that answers `get` from two variables and ignores `store`/`erase`. The token is in that one git's environment and nowhere else: not in an argument (`ps`), not in a config value, not in a file, and git's output is scrubbed of it (and of any token shape) before it becomes an error;
  - keeps three settings of the user's global (else system) config for `https://github.com/`, the ones about reaching the internet: `http.proxy`, `http.sslCAInfo`, `http.sslCAPath`. Never `http.sslVerify`, never the checkout's;
  - never prompts: `GIT_TERMINAL_PROMPT=0`, no askpass.

  A push therefore runs no hook of the user's (`pre-push` included; GitHub's own checks still run). After a push the checkout's `refs/remotes/origin/<head>` is set to the pushed commit with `update-ref`, which is what the diff and the unpushed count read. No credential (gh missing or signed out) is the usual "gh is missing or not signed in" error, before anything is asked of GitHub or pushed. A 401/403 says the sign-in was not accepted.
- **The seam.** `GitHubAuth` (`Credential(ctx, repo) (GitHubCredential, error)`, with `Token`, `Username`, `Source`) is the one place the push's token comes from; `ghCLI` implements it. The long-term source chain (a connected GitHub App, PATs, then `gh`) replaces only its body: `.design/github-auth.md` (proposal). A backend that is not github.com itself says where git reaches a repository (`GitURL`: the tests' local bare repositories; later an Enterprise host).
- **What still goes over origin**: the diff's best-effort fetch of a base (`fetchBase`, 5 s), as the user set origin up, but never waiting for a person: `GIT_TERMINAL_PROMPT=0`, `SSH_ASKPASS_REQUIRE=never`, and SSH in `BatchMode=yes` with a connect timeout unless the user set their own SSH command (`GIT_SSH_COMMAND`, `GIT_SSH`, `core.sshCommand`), which is left alone. When it fails the diff says `base_stale`. Moving it to the scratch form (fetching into the checkout's objects) is a later step.

## Putting a PR right

Asked: a thread that opens a PR wrong (wrong base, wrong branch, wrong title or text) had only one way out, which was to have the user close it and open another. What was missing, found by tests (`links_amend_test.go`) before anything changed:

- `create_pr` on a branch that already had an open PR pushed again and returned it, but ignored `title`, `body` and `base`: the model was told "already open" and the PR kept the wrong text.
- `update` could change a PR's title, body and base (base since stacks) but a model that did not know that opened a second PR, and the thread's records then held two open PRs for one branch.
- When the user closed the wrong PR on GitHub, the thread's record kept saying `open` until the next refresh. Opening the right PR from the same branch worked (GitHub allows a new PR once the old one is closed) but a bare `update` then failed with "several PRs", and `update` of the closed one said only "is closed".

What it does now, and what it still does not:

- **`create_pr` on a branch with an open PR amends it.** The branch is pushed again as before, then the fields that differ are edited: `title` always (it is required), `body` only when given (empty means "leave it"; clearing a description is `update`), `base` only when named (the default is never applied to an existing PR, or a bare `create_pr` would retarget a stack's PR to `main`). Same words send nothing (descriptions are compared with CRLF and outer whitespace ignored). `draft` is not changed: REST cannot convert a PR, and the GraphQL mutation is not worth a second client. A new base is checked as for a new PR (not the head, a valid name, already on GitHub) before the push and the edit, so a refused call changes nothing. The answer says which fields were updated.
- **Only a PR this thread opened is edited.** The edit needs the thread's record of the PR with role `opened`, which is the same rule as `update`. A PR from the thread's branch that the thread has no record of (opened by hand, or its record lost) is adopted as it always was, recorded as the thread's, and *not* edited on that call; the answer says so, and the next call edits it. One call that quietly rewrote a hand-written description would be worse than one more call.
- **`merge` never edits.** Its title and body are defaults taken from the thread (`merge_request` is also what the Review dialog calls), so `merge` finds the PR and leaves its text alone.
- **`update` changes base** (it already did) and now refuses a closed or merged PR with a way on: commit and call `create_pr`, which opens a new PR for the branch.
- **The head cannot be changed.** GitHub does not allow it. The options, by what was wrong: the *content* is wrong, commit to the PR's own branch and call `create_pr` (it pushes, so the PR carries the commits); the *branch* is wrong, open a new PR with `create_pr(head=…)`. Closing the old PR stays the user's: this change adds no `close_pr` (still under [Later](#later)). To keep the wrong one from lingering unseen, opening a PR leaves a note when the thread has another open PR it opened against the **same base** from a different branch (a stack's PRs have different bases, so no note): "#N (x → main) is still open … the user has to close it".
- **The record follows GitHub.** When `create_pr` finds no open PR for the branch, any of the thread's records for that branch that still say `open` are read again and stored as `closed` or `merged` (if GitHub cannot be read they are stored as `closed`: GitHub just said no open PR has that head). The answer names them ("#11 was closed … this is a new PR"). The old record stays in the list as history, told apart by its state, and a bare `update` means the one open PR again. A merged branch with no new commit still cannot get a new PR ("no commit beyond"), but its record is now `merged`.
- **Boundary.** No tool closes, merges (apart from `merge`) or deletes a branch. A PR the thread did not open is edited only after the user approves (see [Which branches a thread may push, and what asks the user](#which-branches-a-thread-may-push-and-what-asks-the-user)). The only force is `create_pr`'s leased `force`.

## Authorization

This is the part that must not turn into a way round the rules.

**Rule: the Projects tools are Tingly Box's own, so they run without a card by default. This is a decision of the product, not a per-tool exception.** A person who hands a thread work expects it to open the issue and the PR; asking each time turned every thread into a wait for a tap, and the thread's launch profile (which the user owns and Projects cannot edit) is where such rules would otherwise have to live. So Projects writes the rule into the settings file it passes with `--settings`, next to the rules it already writes, and answers the prompt itself for what still reaches it:

- **The settings file** (`grantBuiltinTools`, for project threads and folder sessions alike) allows `mcp__tingly-projects`: the whole server, so tools added later are covered without touching this code. It is written in every permission mode.
- **The approval prompt** (`threadPrompter.OnApproval`) approves any `mcp__tingly-projects__*` call that still arrives, the way the coordinator's prompter does. This is what makes it hold against a rule the settings file cannot outrank (see Probe below).
- **Scope is this server only.** Claude's own tools (Bash, Edit, Write, ...) keep following the permission mode and the profile. `gh pr create` / `gh issue create` / `gh pr merge` stay on the `ask` list of `grantAutoGit`; the tools are the way to publish, the Bash commands are not loosened.
- **Existing projects are included**: the setting below is empty on them, and empty means allow. No migration.

The one remaining control is the project setting **`thread_github_access`**: `allow` (the default, also when empty) or `ask`. `ask` is an opt-out for the four tools that publish on GitHub (`create_issue`, `create_pr`, `update`, `comment`): they are written as `ask` rules, which win over the allow rule (deny, ask, then allow), and the prompt fallback leaves those four to the card. The other tools of the server (`view`, `send_file`, ...) stay allowed. There is no global settings table or page for this; if a user needs per-tool control across projects, that is a later design.

### Probe

Checked with the real Claude Code CLI (2.1.259) against a scripted model and a real `tingly-project` on an isolated data directory (the thread calls `view`, then `create_issue`; an approval card means the call asked): 

| Setup | auto | acceptEdits | default | bypassPermissions |
|---|---|---|---|---|
| Before: `view` allowed, writes unlisted/`ask` | `view` runs, `create_issue` asks | same | same | not run |
| Allow rule `mcp__tingly-projects`, prompt fallback off | no card | no card | no card | not run |
| Allow rule + prompt fallback | no card | no card | no card | no card |
| Rule only, `ask` rule for `create_issue` in the user's Claude settings (what a profile can add) | card | not run | card | not run |
| Rule + fallback, same profile `ask` rule | no card | not run | no card | not run |
| Project opt-out (`ask`), rule + fallback | card for `create_issue` | card | card | not run |

What it shows: the server-wide allow rule alone is enough in all three modes, including auto, whose classifier would otherwise deny an unlisted MCP tool; a profile's `ask` rule beats an allow rule (ask is checked first), so the rule alone is not proof against a profile; the prompt fallback closes that, and it is also what applies the opt-out (the same prompt carries the request, the project decides). The probe is `showcase/e2e-demo/mcp_approval_probe.py`.

### The Bash commands

- `gh pr create` / `gh issue create` / `gh pr merge` stay on the `ask` list (`grantAutoGit`); nothing here weakens them. The thread prompt says to use the tools when it has them.

Boundaries the setting cannot move (enforced in code, not in the prompt):

- **No close, no reopen, no delete branch, no unleased force push, no label or assignee changes, no review actions.** None of these exists as a tool, and `update` has no `state` field, so it cannot close an issue or PR. Merging exists only as `merge` (see [Merge](#merge)), which asks on every call. When another irreversible one is added later it needs the same treatment (an always-ask rule), and it is never covered by `thread_github_access`.
- **What the thread did not open is edited only with the user's approval.** `update` looks the number up in the thread's records; role `opened` runs, anything else (not recorded, or only commented on) reads the item and asks, with the change on the card. See [Not opened by this thread](#not-opened-by-this-thread). `comment` and `view` may touch any issue or PR of the thread's repository (a thread working on "issue #12" needs to): the comment is visible in the approval card and leaves a record with role `commented`, while `view` leaves no record unless the thread already has one.
- **What `view` returns is other people's text.** Titles and descriptions come back inside an `<event source="github" kind="view" trust="relay">` envelope with tag characters escaped (the same one the coordinator's relayed text uses), and the tool's description says to read it as information. The body is cut at 2000 characters.
- **Nothing the environment refused is retried here.** If the user denies a tool call, Claude Code does not call the tool. If the user denied the matching `gh` command, the thread's settings still carry that rule, and the tool is a different call that the user answers separately: there is no hidden hand-over. A denial by the host (no `gh`, not signed in) comes back as an error that says what is missing.
- **Credentials**: only the host's existing `gh` login and git credentials are used. The tools never read, log, store or return a token: the child process gets the server's environment unchanged, output is passed through a redactor (GitHub token shapes, `Authorization` headers, credentials in URLs) before it becomes an error, a log line or a tool result, and no token is a tool input.
- **Tools end with the thread.** A thread's tool URL is revoked when it is resolved, when its session is archived, and when its project is paused or archived, together with its process (`mcp-tools.md`), so a finished or stopped thread cannot publish anything on GitHub; a resumed one gets fresh tools on its next turn.
- **One call, one effect.** Each tool does what its name says. `create_pr` pushes one branch and creates one PR; it does not create the issue, set auto-merge or enable anything else.

## The record

**Where it lives: a JSON column of the thread's own row** (`threads.links`, `Thread.Links`), not a table of its own. It is a short list that belongs to one thread, is read together with the thread everywhere (list, detail, events, the coordinator's view) and goes when the thread goes, so a table would add a migration, a cascade, a batch query for every thread list and a second place that must stay in step with the thread, to buy a constraint the code can keep. (`Project.Repos` is stored the same way.) This is the default for anything of this shape; a table is for records that are queried across threads on their own terms.

The one hazard of a column is that `SaveThread` writes a whole row from whatever copy its caller holds, which could undo a refresh made meanwhile. So `SaveThread` leaves the column out, and only `UpdateThreadLinks` writes it: a transaction that reads the links, changes them and writes just that column (as `SaveThreadUsage` does for its columns). A test saves a stale copy of a thread after a refresh and checks the links survive; leaving `Omit("Links")` out of `SaveThread` fails it.

Each entry of the list:

| Field | Meaning |
| --- | --- |
| `kind` | `issue` or `pr` |
| `repo`, `number`, `url`, `title` | identity and what the card shows |
| `role` | `opened` (the thread created it) or `commented` |
| `state` | `open`, `closed`, `merged` |
| `head`, `base` | PR only: the branches as GitHub last said, so the PRs of a stack are told apart and a retargeted base is read back. Empty on records from before stacks until their next refresh. |
| `draft`, `ci`, `merge_state` | PR only. `ci`: `none`, `pending`, `passing`, `failing`. `merge_state`: `ready`, `conflicts`, `blocked`, `behind`, `unknown` |
| `refreshed_at`, `attempted_at`, `failures`, `last_error` | refresh bookkeeping |
| `created_at` | when the thread first touched it |

An item is (repo, kind, number) within its thread; a number always means the item of the thread's current origin, so `update` and `view` look it up by repo and number, and the same number in another repository is another entry. A call that succeeds writes the entry from GitHub's answer (not from the model's input) before the tool returns, and publishes a `thread` event, which already carries the links because they are part of the thread. A tool reads an item in full before it writes about it: `comment` posts only after the issue or PR was read, so a failed read never leaves a record that says a merged PR is closed. An edit's own answer carries no checks and often no mergeability yet, so `update` keeps the last reading of both. The refresh sweep reads the threads that have links (`LinkedThreads`: projects that are not archived) and walks their open entries, least recently read first.

**If the write to the database fails** after GitHub accepted the call, the tool still reports success with the URL and says the record could not be saved; it never retries the create (that would make a duplicate).

### Threads that existed before

Not imported in batch 1. Options weighed:

- *Parse PR links out of old reports.* Cheap, but it cannot tell "I opened this" from "this is related", and it would put guesses into a list that otherwise only holds facts.
- *`gh pr list --head <branch>` per thread with a branch.* Precise (the PR whose head is the thread's branch), but one request per thread, and the user may not have a login at all.
- Proposed for a later change: the second one, run once per thread the first time its branch is seen with no PR record and only on the refresher's schedule, recorded with role `found`. Until then an old thread shows no links and a new PR it opens with the tool is recorded as usual.

## Stacked work (more than one branch)

A thread may work on several branches, typically a stack: `a` off `main`, `b` off `a`, `c` off `b`. The first version fixed `create_pr` to the thread's own branch and the repository's default, which made a stack impossible. What changed, and what did not:

- **`create_pr` takes `head` and `base`.** A stack is `create_pr(head=a)` (base defaults to the thread's base), `create_pr(head=b, base=a)`, … in that order, since a base must already be on GitHub. The model still never names a repository, a remote or a refspec.
- **One branch per push.** Each call pushes exactly one branch with `refs/heads/<head>:refs/heads/<head>`: no `+`, no `--all`, no deleting. A branch that diverged on origin is the "rebase" error, which now also says that `force=true` exists.
- **Branches that ask before they are a head**: the repository's default branch (asked of GitHub) and any branch GitHub reports `protected` (`GET /branches/<b>`; a branch GitHub does not have counts as unprotected, any other failure is an error, never "unprotected"). A name that is not a local branch is refused, so a head cannot reach a branch that is not in the thread's repository. The protection is GitHub's own setting: where the user has not protected `release/*`, a thread can push a branch of that name, as it always could with `git push` (the permission rules for `git push` are separate and unchanged).
- **Any local branch of the checkout.** Worktrees share the repository's refs, so a head can be any branch the thread made in its worktree (a side fix, a stack) but also one another thread created; the latter asks the user, see the next section.
- **`update` retargets.** `base` was already editable on a PR the thread opened; this is the "lower PR merged, point the upper one back at `main`" step. It does not create or push anything, and GitHub's own checks apply (a base equal to the head, a base that does not exist, are 422s that come back as the error).
- **Several PRs in one record.** The links list was always one entry per (repo, kind, number), so a stack needed no new storage: each PR is an entry, with `head`/`base` stored to tell them apart. `update`/`view` without a number still means "the thread's only PR" and, with several, name them as `#12 (b → a)` and ask for a number; a merged or closed one is skipped when exactly one is open.
- **Tool stays one call, one effect.** No tool creates the stack, rebases it or retargets it on its own; the model does each step, each is a separate approval (or a standing `thread_github_access = allow`).

## Which branches a thread may push, and what asks the user

The first rules kept a thread to one branch: the prompt said "Do not merge, force-push, or touch other branches", `create_pr` never forced, and a rebased branch could only be pushed by hand (`git push --force-with-lease` is on `grantAutoGit`'s ask list). Threads read the prompt literally and stopped.

**Principle: inside the boundary a call runs; outside it the user is asked with a normal approval card (web and IM). Nothing is refused that the user could allow, and nothing is left to a question in text.** Only what nobody could allow stays an error: a head that is its own base, a non-local branch, a base not on GitHub, a branch with no commit beyond its base.

| Call | |
| --- | --- |
| Push the thread's branches (its own, others it made in the worktree), open or update their PRs | runs |
| `force=true` on its own branch or one it opened a PR from | runs: `--force-with-lease` on the commit this checkout last saw |
| Another thread's branch; the default or a protected branch as head; `force` on a branch it neither created nor opened a PR from | **asks** (one card lists every reason) |
| Force when origin has commits this checkout never saw (or never saw the branch) | error, no card: "fetch, rebase onto origin/<b> and call again" |
| `update` of an issue or PR it did not open | **asks**, the card shows the change; recorded as `commented`, so the next edit asks again |
| `merge` | always asks (unchanged) |

- **How a tool asks.** A permission rule names a tool and cannot see its arguments, and a second `ask`-ruled tool would split the five-tool shape. So the tool asks itself (`approval.go`): `Links.authorize` → `Threads.askApproval` puts an `approval` event through the thread's prompter, so cards, IM relay and waiting state are the existing ones, and the MCP call returns on the answer. "Allow for this thread" is kept per exact request (repo, branch, force). A no returns "the user did not approve this … nothing was changed" with their words. With no approver (tests) the call errors; it never goes ahead. The ask comes after every check that can fail outright and before the first push. Not covered by `thread_github_access`.
- **Whose branch.** Another thread's: one started on it (`threads.branch`, same `repo_path`, archived included) or holding a PR record with that `head`, unless this thread holds one too. By name over the records; an unrecorded branch cannot be told from scratch work and does not ask.
- **The lease.** `--force-with-lease=refs/heads/<b>:<sha>` with `<sha>` the commit in `refs/remotes/origin/<b>` (empty: "must not exist"). No `--force`, no `+`. A fetch the model ran counts as having seen the commits, as in git. A lease that does not hold is an error, not a second card (decided with the user): someone else's commits are never overwritten, and the error carries what the agent needs to go on (origin's commit, the one this checkout saw, git's output, the steps).
- **Bash is unchanged.** `git push --force…`, `-f`, `--delete` stay on the ask list. A plain `git push origin <other branch>` in auto mode is left to the classifier, which may deny without a card (ask beats allow, so the own-branch allow cannot be kept and widened); the prompt sends such pushes through `create_pr`.
- **Trade-off.** Stricter than the first stacks version (another thread's branch pushed freely), looser than the old prompt (own branch only). A branch recorded nowhere does not ask; only GitHub's protection covers it.

### Not opened by this thread

`update` on a PR the thread did not open first answered no (it rewrites other people's text and base). It now asks: the card shows the change, the setting `thread_github_access = allow` does not cover it, "allow for this thread" is per repository and number, and the record is `commented`, so only what the thread opened runs without asking. `comment` is still the better tool for someone else's PR.

## Refresh: bounded, read-only, from the server

The UI never asks GitHub; it reads the record. A background sweep keeps open items fresh:

- Runs once a minute; looks at records with `state = open` whose project is not archived; **terminal states (`closed`, `merged`) are never refreshed again**.
- An item is due `5 min` after its last refresh. After a failure the wait doubles (10, 20, 40 … capped at 1 hour), and the last good state is kept with `last_error` set; one success clears the count. Same shape as the quota back-off, kept in the row so a restart does not forget a failing item.
- At most 20 items per sweep, one at a time. A read is one request ([Cost of a read](#cost-of-a-read)), so 20 open PRs at 5 minutes cost 240 points an hour against GitHub's 5000.
- `view` asks GitHub on demand (an issue read, and for a PR the PR, its check runs and its status) and brings a linked item's record up to date, but answers from its last answer when that is under 30 seconds old, so a model that polls cannot hammer GitHub; an `update` or `comment` clears it.
- A change (state, CI, merge state) publishes a `thread` event so open pages update; an unchanged reading only moves `refreshed_at` and publishes nothing.
- When `gh` is missing or not signed in, every refresh fails the same way and backs off; the card says "could not refresh" with the age of the last reading.

Not done: webhooks (the host is usually behind NAT), review decisions (one more field in the query; left out because nothing here acts on it), notifying the coordinator when a PR is merged (an event line in the thread transcript is the obvious next step).

### Cost of a read

GitHub's rate limit is the one shared resource a refresh spends, so a read is built to cost as little as GitHub allows:

- **One request per item, whatever it is.** A read is a single GraphQL query on `repository.issueOrPullRequest(number)` (one point). It returns the item, for a PR its draft flag, branches, merge status and the head commit's check rollup (names of the failing checks included), and `rateLimit{remaining resetAt}`. Before this a PR cost three REST requests (pull, check runs, combined status) and an item of unknown kind a fourth (the issues API first, to find out whether it was a PR). Writes stay REST.
- **The budget rides on every answer.** `Links` keeps the last `rateLimit`. Under `rateFloor` (300 of 5000) no read is made, neither the sweep's nor a person's, until the window resets; the rest is for the tools that write. A manual refresh then answers `limited` and leaves the record alone; an error that names the rate limit (GitHub gave no reset time) pauses reads for ten minutes.
- **Manual reads are throttled in the server.** An item read or attempted under `linkManualGap` (30 s) ago answers `recent` without a request; refreshing a whole project is allowed once per `linkProjectGap` (a minute), at most 20 items, and a second press inside it answers `recent` for every item. Two refreshes of one item in flight are one read.
- **Not read at all**: author, reviews, labels, the check runs that pass (they are counted, not named), the head commit's other statuses beyond the first 100 (a failure past the cut still shows, through the rollup's own state).

### Manual refresh and detail (UI: `.design/workspace-layout.md` §3.6)

The sweep keeps open items fresh; a person looking at a row can also ask. This adds no write to GitHub and no new trust: it is the sweep's own read, on demand.

- **Endpoints.** `POST /threads/{id}/links/refresh` with an optional `{repo, kind, number}` (one item; none = all of the thread's links) and `POST /projects/{id}/links/refresh` (open links of the project's threads, at most `linkSweepMax` per call, least recently read first). Both answer `{results: [{repo, kind, number, status, error?}]}`, `status` one of `refreshed`, `recent`, `failed`, `limited`; an item the thread does not have is a 404, not a silent skip. The answer is only a status: the data travels on the thread event.
- **Any state.** Unlike the sweep, a manual refresh also reads `closed` and `merged` items (a PR can be reopened).
- **Failure does not feed the back-off.** A failed manual read stores `last_error` and `attempted_at` but leaves `failures` as it was, so a person pressing refresh while `gh` is signed out cannot push the sweep's wait to an hour. A success clears both, as in the sweep.
- **Always publish.** The sweep stays silent for an unchanged reading; a manual refresh publishes the `thread` event even when only `refreshed_at` moved, because "read just now" is what the person asked to see. Clients never merge: the event replaces the thread, and the server is the only writer of the records.
- **What the record gains** (optional, filled by the next read; old records simply lack it): for an open PR `checks`: counts `passing`/`failing`/`pending` and the failing ones by name and URL, at most 5 (`more` counts the rest). Nothing else was added: a person deciding whether to act needs what failed, whether it can merge and how old the reading is.

## A PR's diff

The thread's own "branch against base" diff is gone; a diff is always one PR's, head against base (`GET /threads/:id/prs/:number/diff`, and `/diff/file?path=` for one patch). The server finds the branches from the thread's link record (`head`/`base` as GitHub last said); the client never names a ref. Only a PR of the thread that is **open** has one: a merged or closed PR has nothing left to review, and a record that GitHub has not been read about yet has no branches to diff.

**Local only for now; GitHub's list is built but switched off.**

| | local `git diff base...head` | GitHub `pulls/N/files` |
| --- | --- | --- |
| cost | no network, no token, one `git diff` | one request per 100 files (cap 3000), gh signed in, counts against the request budget |
| exactness | `refs/remotes/origin/<head>` is what GitHub has (a push keeps it current); the local branch may be ahead | exactly the PR |
| big PRs | per-file loading, 512 KiB patch cap, long lines shortened, lock and generated files folded | no `patch` on big files; capped at 3000 files |
| gone | a deleted or never-fetched branch has no ref | still there while the PR is open |

The rule: read `refs/remotes/origin/<head>` (else the local branch) against `refs/remotes/origin/<base>` (else the local base). When either side is in neither place (another machine pushed it, the branch was deleted) the answer is an error that says so, and the dialog's "Open on GitHub" is the way on. The GitHub read (`GitHub.PRFiles`, cached for two minutes so that opening one file after another asks once; the answer would say `source: github`) is implemented and tested but off: `Threads.remoteDiff` is false, because it costs requests, a token and rate budget. Turning it on is that one field (a project or server setting later, if it is wanted). On the local path:

- origin's base is fetched first, briefly (5 s, no prompt, best effort), because a stale `origin/main` makes files other PRs changed on `main` show up; if the fetch fails the answer carries `base_stale`.
- commits the local head holds beyond `origin/<head>` are not in the PR: `unpushed` says how many and the dialog says they are not shown, instead of quietly showing more than GitHub does.

The dialog (`PrDiffDialog`) shows these notes above the existing file list and patch viewer, "Open on GitHub", and, for the PR of the thread's own branch, **Merge…**. Merging a PR other than the thread's own branch is not offered (the merge works on the thread's branch).

### The words of a merge

`gh pr create --fill` and `gh pr merge` set the model for the Merge dialog (`GET /threads/:id/merge/draft`): the dialog never composes text of its own.

- **A PR is open**: the PR's own title and description, read from GitHub when the dialog opens. A squash starts from `title (#N)` and the description, the way `gh pr merge --squash` words its commit; the fields are editable, and a squash with them untouched sends exactly that.
- **No PR yet**: one commit gives its subject and body; several give the branch name spelled out (`fix-the-bug` → "Fix the bug") and a list of their subjects, as `gh pr create --fill` does. The fields word the PR that is opened, and a squash then takes the PR's title and `(#N)` and its description. The thread's last report is no longer used for a PR body.
- The `merge` tool's `title`/`body` default the same way, and an empty `message` is the PR's own title with its number, and its description.
- If the draft cannot be read the dialog still works, starting from the thread's title.

Not done: `gh pr create` also reads the repository's PR template when it is not asked to fill; the draft does not.

## Display

- **Thread rail** (`ThreadInfoRail`, the expanded status area): two cards after the host's cards, **Pull requests** and **Issues**, one record per row: icon, `#number title`, state chip, and for a PR its CI and merge state; a row opens the URL. A card is shown only when the thread has records of its kind. Reading age in the tooltip; "could not refresh" when `last_error` is set. The old Changes card (commits, files, Review) is gone: a PR row carries a **Files changed** button instead (see [A PR's diff](#a-prs-diff)). A branch with commits and no open PR of its own has one row in the Pull requests card in place of the PR it lacks: "No PR yet · ahead of main · N commits · M files" and **Merge…**.
- **The composer strip** (`ComposerStrip`) lists the same rows, PRs first, and gives an open PR the same Files changed button, so a narrow layout without the rail reaches the diff too.
- **Coordinator's Status tab**: a card with all records of the project's threads, newest first, each with the thread's title, so "which PRs are open and how are they doing" is one place.
- **Narrow layouts** that have no rail keep the header chips; a link count chip is not added in batch 1.
- Project settings dialog: one select, "Issues and PRs: allowed (default) / ask each time"; its help text says the default and that the other Projects tools are not affected. `PATCH /projects/{id}` takes `thread_github_access` (`ask` or `allow`, anything else is a 400).
- The coordinator sees the same records as text: a short `prs=#12(open checks=failing merge=conflicts)` on each thread line (its prompt table and `list_threads`) and the full list, with the age of each reading, in `get_thread`. A field not read yet shows as `unread`.

## Tests

- Branch scope, force and asks (`links_force_test.go`, `approval_test.go`): own-branch force runs with no card; a lease that does not hold (commits not seen, or a branch never seen) is an error that asks no one and succeeds after fetch and rebase; another thread's branch, a default/protected head, an unearned force and `update` of a foreign PR each ask (no: nothing changes; yes: it goes through); no approver is an error; the card is a pending request the call waits on, with "allow for this thread" per key (real prompter). `TestThreadPromptDoesNotDependOnOtherThreadsOrLinks` pins that the prompt (cached prefix) is static.
- Stacks (`links_stack_test.go`, harness G3): a head that is another local branch is pushed alone (origin gets nothing else); two PRs recorded with `head`/`base`; a bare `update` names them; a base retarget is read back; refusals (default branch as head, protected, not a local branch, a refspec or option as head, head = base, a base that is not on origin) push and record nothing; a diverged head is not forced; an existing PR for the head is returned; a fake `gh` for `Protected` (a 404 is unprotected, any other error is an error). Harness G3 runs the whole stack through the MCP tools with an actor that switches branches.
- Mutation-checked: a forced refspec, the missing default-branch guard, the missing protected, local-branch and base-on-origin checks, the missing "opened by this thread" check and a back-off that never doubles each fail a test.
- Putting a PR right (`links_amend_test.go`): the open PR is amended not duplicated; only differing fields are sent and a base is never guessed; a refused base changes nothing; a PR the thread does not hold is adopted, not edited; `merge` never edits; a closed PR's record becomes `closed` (also when GitHub cannot be read) and the branch gets a new PR that a bare `update` finds; a merged branch with nothing new is refused with the record `merged`; the same-base note, and no note for a stack. The fake GitHub's `OpenPR` finds an open PR by head like GitHub does.
- Fake `GitHub` for the tools and the refresher; a fake `gh` executable (a shell script on `PATH`) for the real `ghCLI`: arguments, JSON body, `gh api -X PATCH` for updates, the redactor on a token in stderr.
- Remotes and transport (`remote_test.go`): every form above parses to owner/name or is refused with the host named and no credentials; an SSH alias is resolved with `ssh -G -F <test config>`, an alias for another host and an unreadable config each say so, and a host that would be an ssh option is never passed; a `gh:` shorthand is read through `insteadOf`. The tools' git: `git credential fill` gets the token and `approve` reaches no helper of the user's (global or the checkout's), another host is never offered it, and the token is in no `GIT_CONFIG_*` value; with a test home whose `~/.gitconfig` rewrites https to SSH (insteadOf and pushInsteadOf), sets `core.hooksPath` and `core.sshCommand` and a proxy, and a checkout with its own hook, SSH command and a more specific rewrite (both shown to be live for plain git), a push goes to `https://github.com` through the carried proxy and runs no hook and no SSH; `create_pr` on an SSH origin runs neither origin's SSH command nor the checkout's `pre-push` hook, the branch arrives, `refs/remotes/origin/<head>` moves, origin's URL is unchanged and no file under `.git` or the remote holds the token; a worktree of a shallow clone pushes; no credential pushes nothing; a receiving hook that prints the token comes back `[redacted]`. Mutation-checked: pushing over origin, no `GIT_CONFIG_GLOBAL=/dev/null`, no helper reset, no carried proxy each fail a test. Probed once against github.com: `ls-remote` from an SSH origin went over https with gh's token (read only; no real push in tests).
- Tools through the in-memory MCP client: scope lists (golden and `TestToolsOfEachScope`), each tool, refusals (no origin, non-GitHub origin, default branch, base equals head, no commits, someone else's PR number), idempotent `create_pr`, a failed push leaves nothing recorded.
- Settings rules: `ask` default, `allow` when set, `view` always allowed, `gh pr create` still on `ask`.
- Refresher with an injected clock: due time, back-off doubling and cap, terminal states skipped, last good state kept, change publishes and no change does not.
- Harness journey: a thread opens a PR through the tool, the coordinator sees it in `get_thread`; mutation-checked.
- Frontend: the rail card and the coordinator card.

## Later

Each its own change; any that closes, merges or deletes needs the explicit approval rule first.

- Import of PRs older threads opened (above).
- `merge_pr`, `close_pr`, labels and assignees, `request_review`; PR review comments and CI log reading (a thread fixing a red check).
- Continue work on an existing PR's branch (`start_thread` with a PR number), the intent `mcp-tools.md` kept for the PR-awareness design.
- Other hosts (GitLab) behind the same `GitHub` interface.
- A transcript line when a linked PR merges or its CI turns red.
