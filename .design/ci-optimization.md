# CI Optimization (PR-time workflows)

> Analysis only. No workflow is changed by this document; it records what the
> data says, the options, and what needs a maintainer decision.
> Related: `.github/workflows/AGENTS.md` (rules for editing workflows),
> [`harness-matrix.md`](./harness-matrix.md), [`harness-npm.md`](./harness-npm.md).

## 1. Data and method

- Source: `gh api` read-only queries on `tingly-dev/tingly-box`, workflow runs
  created 2026-11-05 → 2026-11-19 (14 days, 711 runs), plus per-job timings for
  666 completed runs of the PR-time and release workflows, and the file lists
  of the 150 PRs opened 2026-11-09 → 2026-11-19 (~14 PRs/day).
- "Job-hours" = sum of job wall-clock (completed − started) over success and
  failure jobs. It is not billed time: the repo is **public**, so standard
  Linux runners are free. What it measures is runner load and, more usefully,
  how many checks every PR has to wade through.
- Estimates marked *est.* are simulations or guesses, not measurements.

Facts that frame the whole discussion:

- `main` has **no required status checks** (branch protection 404) and both
  rulesets are `disabled`. All PR CI is advisory today. Cutting a check cannot
  block merges; it can only remove a signal.
- Jobs start within 1–3 s of being created (median 1 s, p90 2 s over 169 jobs):
  **no runner queueing**. The cost is volume/noise/attention, not latency.
- Wall time per run is already short (Verify Compile median 2.9 min, p90 5.8;
  Harness PR 3.7 min; IM 3.3 min).

## 2. Workflow inventory

| Workflow | Trigger | Paths / gating | Jobs | Matrix | Concurrency cancel |
|---|---|---|---|---|---|
| `verify-compile.yml` | `pull_request`, dispatch | broad (`**` minus docs/CI/tests/frontend-md…, plus `frontend/**`); `dorny/paths-filter` per surface | `changes` → `compile-cli`, `compile-gui`, `test-projects`, `test-remote`, `compile-frontend` | none | yes (per ref) |
| `verify-imbot-projects.yml` | `pull_request`, dispatch | `agentboot imbot remote projects`, `internal/{config,db,server,tbclient}`, `libs`, `go.mod/sum`, `go.work*` | 1 (`go test -race ./internal/server/module/projects -run ^TestTingly`) | none | yes |
| `harness-pr.yml` | `pull_request` (opened/sync/reopened/ready), dispatch | protocol data-plane + harness paths | 1 (build harness, `catalog check`, `matrix --mode=all`); skips drafts | none | yes |
| `harness-matrix.yml` | `push ci/**`, `pull_request: closed` into main (merged only), dispatch | same path set as harness-pr + `tests/clients/**` | `gate` → `harness-template.yml` (build + **21 legs** + result) | 21 legs (11 modes, 5 client drivers, replay×2, lb, duo, routing) | yes (see §4.5) |
| `harness-template.yml` | `workflow_call` only | – | shared matrix definition | – | – |
| `verify-ci.yml` | `pull_request`, dispatch | `.github/{workflows,actions,scripts}/**` | actionlint + shellcheck | none | yes |
| `harness-docker.yml` | `pull_request`, `push main`, dispatch | `build/docker/**`, its script | 1 smoke test | none | yes |
| `homepage.yml` | `pull_request`, `push main`, dispatch | `homepage/**`, icons | build (+ deploy on main) | none | no (`cancel-in-progress: false`) |
| `issue-area.yml` | `issues: opened` | – | label by area (≈0.1 min) | none | – |
| `release.yml` | tag `v*`, dispatch | – | frontend + harness gate → `release-cli` (6 targets) + `release-gui` (6 packages) → announce | 12 build jobs + 21-leg harness | – |
| `release-cli/gui.yml`, `verify-release-*.yml`, `build-frontend.yml` | `workflow_call`/dispatch (+ `release: published` for verify-release-cli) | – | – | – | – |
| `npm.yml`, `harness-npm.yml`, `docker-source.yml` | dispatch only | – | – | – | – |

The Actions list also shows four workflows whose files no longer exist
(`commit-packages`, `gui-build-check`, `python-sdk`, `verify-docker`); they have
no runs in the window and cost nothing.

## 3. What actually runs per PR (14-day measurement)

| Workflow | Runs | ok / fail / cancel / skip | Median / p90 min | Job-hours | Notes |
|---|---|---|---|---|---|
| Verify Compile | 359 | 300 / 34 / 18 / 0 | 2.9 / 5.8 | **40.2** | gui 14.7, cli 10.6, frontend 8.0, projects 6.2, remote 0.3, changes 0.4 |
| Verify IM Bot and Projects | 172 | 162 / 3 / 6 / 0 | 3.3 / 4.0 | **9.1** | one job, `-race`, no build cache |
| Harness PR | 101 | 77 / 3 / 1 / 13 draft-skipped | 3.7 / 5.0 | **4.3** | |
| Harness Matrix (post-merge) | 31 | 23 / 0 / 8 / 0 | 6.1 / 7.3 | **4.0** | 556 leg-jobs, 1 failed |
| Release (8 tags) | 8 | 8 / 0 / 0 / 0 | 18 / 23 | ≈12 | unaffected by anything here |
| Everything else (issue-area, verify-ci, homepage, docker, npm dispatches) | ~40 | – | <1 (npm publish 17–26) | <1 | negligible; one `npm` run sat ~66 h (awaiting `production` approval, not compute) |

PR-triggered job-hours ≈ **53.6 h / 14 days (~3.8 h/day)**, i.e. ~615 runs of
the three big workflows from 179 branches.

Per-job averages (min): compile-gui 4.0, compile-cli 2.8, compile-frontend 2.2,
test-projects 3.9, IM harness 3.3, harness-pr 3.2, harness leg 0.4.

Reruns are not a problem (`run_attempt > 1`: 2 runs).

### Duplication / noise

- **Pushes per branch**: 349 distinct head SHAs over 179 branches (avg 1.95,
  max 13; one branch had 39 runs across the three workflows). Roughly half of
  all PR runs are re-runs of a later push. `cancel-in-progress` is on and works
  (18 compile + 6 IM + 1 harness cancelled), so superseded runs die early, but
  the final run of each push still executes everything.
- **Failure rate**: Verify Compile 9.5% (34/359). Of those: frontend 25
  (`Projects web UI` step 14, `Lint` 10), test-projects 7. Plus **13 jobs failed
  at `Checkout code`** (cli 5, gui 5, projects 3): infra/submodule flakes that
  look like red PRs but say nothing about the code.
- **Draft PRs hardly matter**: only 2 of the 150 PRs are drafts. A "skip drafts"
  gate (already in harness-pr) would save almost nothing for the other
  workflows, because PRs are normally opened ready.
- **Docs-only runs**: `verify-compile` and the others already path-filter
  docs/CI/tests. Only 6 of 150 PRs were "other-only", so there is little left
  to gain here.
- **push + pull_request double triggers**: none on the heavy workflows
  (`harness-docker` and `homepage` double-trigger but only on their own paths,
  and cost <0.5 min). Not a problem.
- **Overlap of work**:
  - `compile-gui` imports `internal/app`, `internal/server`, `internal/command`
    and a few more root packages,
    i.e. it recompiles essentially the same Go tree as `compile-cli`, plus a
    ~33 s `apt-get install` of GTK/WebKit. It ran on **73 of 150 PRs (49%)**
    because its filter is "any Go-side file except `cli/**`". Its unique
    coverage is `gui/wails3/**` and the cgo link surface.
  - `harness-pr` (in-process `matrix --mode=all`) re-runs, in-process, what the
    post-merge matrix and the release gate run again per mode. This is a
    deliberate three-layer design (PR → post-merge → release); see §5.
- **Cache situation (suspected root cause of slow cold runs)**:
  - `actions/cache` entries made on a PR ref (`refs/pull/N/merge`) are only
    readable by that PR; a PR only sees caches from its base branch.
    **There are zero caches on `main`** (the Go workflows never run on `main`),
    so the first run of every PR is cold, and only its own later pushes warm up.
  - Active cache usage is **11.2 GB, above the 10 GB repo limit** (11 caches,
    ~1 GB each; 6 belong to one release tag ref), so GitHub is evicting
    continuously.
  - `compile-cli` warm: build 35 s of a 1.3 min job; the 2.8 min average
    includes cold runs. Same for `test-projects` (`go test -race` 61 s warm vs
    3.9 min average job).
- **Small waste**: the non-blocking `pnpm typecheck` step (21 s,
  `continue-on-error`, only emits a warning) runs in all 224 frontend jobs
  (≈1.3 h). `Projects web UI` (typecheck+test+build, 75 s) is the most valuable
  frontend gate (14 of the 25 frontend failures).

## 4. Options, graded

Estimates are against the 53.6 h/14 d PR-triggered baseline unless noted.

### 4.1 Keep as is (core)

| Item | Why |
|---|---|
| `compile-frontend` (build, `@tingly/projects` typecheck/test/build, lint) | User priority; it catches the most real failures (25 red runs, all genuine step failures). ~65% of PRs touch `frontend/`. |
| `harness-pr` | User priority; already path-filtered, draft-skipping, cancelling, 3.7 min. |
| `harness-matrix` post-merge + release gate | User priority; 4 h per 14 d, 0 real failures in the window but it is the only full-client-driver coverage. Bursts of merges already collapse into the last run (§4.5). |
| `test-projects`, `test-remote` | Already `changes`-gated; run only when their module changes (95 / 13 runs). |
| `compile-cli` | The single cheapest "does the Go tree build and vet" signal. Keep. |
| `verify-ci` (actionlint) | 0.2 min, only on `.github/` PRs. |
| `issue-area`, `homepage`, `harness-docker`, `release*`, `npm*`, `verify-release-*`, `harness-npm`, `docker-source` | Negligible cost or release-time/manual-only already. |

### 4.2 Narrow by path (recommended)

**a. `compile-gui` only for the surfaces it can uniquely break.**
Filter: `gui/**` plus the root-module packages `gui/` imports directly
(`internal/app`, `internal/command`, `internal/lock`, `internal/appconfig`,
`internal/server`, top-level `internal/*.go`, `pkg/network`, `pkg/fs`), plus
`go.mod`, `go.sum`, `go.work*`, `libs/**` and the workflow file. The direct-import
list comes from `grep` over `gui/**/*.go`; derive the final list from
`go list -deps ./gui/wails3/...` before landing.

- Saving *est.*: simulating that filter on the 150 PRs gives 14 matches vs 73
  today (−81%) → **≈ −12 job-hours / 14 d (−22% of PR CI)**, and it removes the
  apt install from most PRs.
- Risk: a change in a package *outside* that list that only the GUI build
  breaks (cgo/webview-specific) is not caught at PR time. Everything those
  packages pull in transitively is still compiled by `compile-cli`, so the
  residual risk is the cgo link/GTK surface, which `release-gui` builds for
  all 6 packages on every tag (behind the harness gate).
- Backstop: add `compile-gui` to the nightly seed run (§4.4) so a break shows up
  within a day instead of at release.

**b. Drop the non-blocking typecheck step in `compile-frontend`** (or run it
only on dispatch). It cannot fail the job. *est.* −1.3 h/14 d. Risk: lose a
log-only warning; keep `pnpm typecheck` available as a manual/pre-release
check. Optional; only do it if nobody reads those warnings.

**Not worth doing** (simulated, no gain): narrowing `verify-imbot-projects`
paths to the packages the test really needs (48 → 46 of 150 PRs, because its
test depends on `internal/server`, `libs`, `go.mod` which change together);
path-gating `Projects web UI` (69 of 98 frontend PRs touch
`frontend/packages/**`); a draft-PR gate (2 of 150 PRs are drafts).

### 4.3 Merge

- Fold `verify-imbot-projects.yml` into `verify-compile.yml` as one more
  `changes`-gated job (`test-imbot`). It shares checkout/Go setup/cache with
  `test-projects` and gives PRs one workflow, one concurrency group and one
  status block. Saving: small in job-hours (*est.* −0.3 h, one fewer
  `changes`-less workflow start); the benefit is fewer, more readable checks.
  Risk: `verify-compile` becomes a larger file (already 409 lines); the
  `paths-filter` list must carry the IM paths, and `AGENTS.md` rule about
  skipped `needs` applies. Low priority.
- Do **not** merge `harness-pr` into `verify-compile`: different path set and
  the draft rule differ.

### 4.4 Fix caching (largest wall-time lever, not in job-hours)

1. Seed `main`: add a `schedule` (daily, off the :00 mark) + `workflow_dispatch`
   job that runs `compile-cli`, `compile-gui`, `test-projects` (and the harness
   build) on `main` with the same cache keys, so every PR's first run restores
   a warm Go build/mod cache. Cost: ~15 job-minutes/day. Do **not** seed on
   every push to main: ~14 merges/day × ~12 job-minutes would cost more than it
   saves.
2. Get under the 10 GB cache limit: ~1 GB caches × many refs are being evicted.
   Candidates: cache only `GOMODCACHE` (not `GOCACHE`) on PR refs, one shared
   key per job instead of one per `go.sum` hash; drop the per-tag release
   caches (6 of 11 entries).
3. `verify-imbot-projects` and `harness-pr` have **no** cache restore on the Go
   build cache beyond `setup-go` defaults (IM uses `cache: false` and no
   `actions/cache`), so every IM run builds `-race` from scratch (158 s test
   step). Reuse the same cache pattern as `test-projects`.

Saving *est.*: 30–50% of per-job wall time on first-push runs; no change in
job count. Risk: stale cache poisoning is low with Go (content-addressed), but
the key/restore-keys design must keep `-race` and non-race builds separate.

### 4.5 Manual / label / schedule / merge queue

- **Heavy checks on a label (`ci-full`) or at `ready_for_review` only**:
  candidates are `verify-imbot-projects` (9.1 h, `-race`) and `harness-pr`
  (4.3 h). Saves up to ~13 h/14 d *est.* but each PR loses its per-push signal
  for the integration harness. Since `main` has no required checks, "before
  merge" is not enforceable, so this only works if the merger (the
  `tingly-projects` merge flow) runs/awaits the label. Needs a decision (§6).
- **Merge queue**: not recommended. Needs a ruleset with required checks (both
  rulesets are disabled), would add a run per merge on top of the post-merge
  matrix, and the failure rate of PRs against main is low.
- **Post-merge matrix → nightly**: would save 4 h/14 d but loses
  "which merge broke the harness". Bursts already coalesce: for a merged PR,
  `github.ref` is `refs/heads/main`, so the concurrency group
  `harness-matrix-refs/heads/main` is shared and 8 of 31 runs were cancelled by
  a later merge (e.g. stacked `tp/limits-1..4` merged within 5 s), which is
  fine because the last run includes the earlier changes. Recommend keeping
  it, and keeping the release gate as the hard backstop.
- **Collapse the 18 matrix legs into ~5 jobs by tier** (matrix / client
  drivers / replay+lb / duo / routing): legs average 0.4 min of which most is
  checkout + setup + artifact download. *est.* −2 to −3 h/14 d on the
  post-merge run and on every release. Cost: coarser per-leg visibility in the
  UI. Optional, low priority.

### 4.6 Close

Nothing is recommended for outright removal. The two candidates considered:
`compile-gui` (instead narrowed, §4.2a) and the non-blocking typecheck step
(§4.2b). The dead workflow entries in the Actions list need no action.

## 5. Why the harness layers stay

`harness-pr` (in-process `matrix --mode=all`, 3 min) → `harness-matrix`
(post-merge, per-mode legs and the five client drivers, 6 min) →
`release.yml` gate (same template, per tag) is a deliberate funnel: the cheap
layer runs on every relevant PR push, the expensive one once per merge, the
full one once per release. The matrix legs themselves total a few seconds each;
the cost is build/setup, not coverage. None of the three can replace another
(client-driver legs and `replay/lb/duo/routing` are not in `harness-pr`).

## 6. Recommendation and decisions needed

**Recommended package** (low risk, ≈ −25% PR job-hours, plus faster cold runs):

1. Narrow `compile-gui` to its real surface (§4.2a). *Biggest single win.*
2. Seed `main` caches nightly and fix cache budget/keys; add the IM job to the
   same cache pattern (§4.4).
3. Optionally drop the non-blocking typecheck (§4.2b) and fold IM into
   `verify-compile` (§4.3).
4. Leave frontend, harness-pr, harness-matrix, release gates untouched.

**Decisions for the maintainer:**

1. **What is the pain?** Data shows no runner queueing, free minutes (public
   repo), and median runs under 4 min. If the pain is "too many red/green
   checks on each PR" or "agents/threads waiting on CI per push", the levers
   are §4.5 (label / ready-only), not path filters. Which one is it?
2. **Heavy Go checks per push or on demand?** Move `verify-imbot-projects`
   and/or `harness-pr` to "on `ready_for_review` + label `ci-full` + dispatch"?
   Gains ~13 h/14 d but PRs get no per-push integration signal, and nothing
   enforces running them before merge (no required checks). Do you want
   required checks / a ruleset turned on at all?
3. **Should threads open PRs as drafts** and promote them when done? Only then
   does a draft gate pay off (today 2 of 150 PRs are drafts) — and
   `verify-compile`/IM would also need `ready_for_review` in their `types`.
4. **`compile-gui` backstop**: nightly seed run only, or also a manual dispatch
   before release? (Release already builds the GUI.)
5. **Cache seeding**: OK to add a scheduled workflow run on `main`
   (~15 job-min/day)?
6. **Post-merge harness**: keep per-merge (recommended) or nightly?

## 7. Reproducing the numbers

```bash
# all runs in a window (read-only)
gh api -X GET repos/tingly-dev/tingly-box/actions/runs -f created=">=2026-11-05" \
  -f per_page=100 --paginate \
  --jq '.workflow_runs[]|[.id,.name,.event,.conclusion,.created_at,.run_started_at,.updated_at,.head_branch,.head_sha[0:8]]|@tsv'
# per-job timing for one run
gh api repos/tingly-dev/tingly-box/actions/runs/<id>/jobs
# cache state (note: usage > 10 GB, nothing on refs/heads/main)
gh api repos/tingly-dev/tingly-box/actions/cache/usage
gh api "repos/tingly-dev/tingly-box/actions/caches?per_page=100"
```

Window-specific: the measured period includes the release of 8 tags and an
unusually busy stretch (30 PRs opened on 2026-11-13 and 29 on 11-14), so
absolute numbers will vary; the ratios between workflows are the stable part.
