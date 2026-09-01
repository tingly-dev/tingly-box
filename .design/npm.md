# npm distribution

How tingly-box ships through npm, and the plan to make `npm install -g` viable
again.

## Architecture today

Three packages under `build/npx/`, published by `.github/workflows/npm.yml` on
each GitHub release:

- **`tingly-box`** — thin shim (`bin.js` + `package.json`, ~16 KB published).
  On first run it downloads the platform zip from GitHub Releases into
  `~/.cache/tingly-box/<tag>/bin/` and execs the Go binary from there. CI bakes
  the release tag into `BINARY_RELEASE_BRANCH` at publish time, so npm package
  version ↔ binary version are 1:1 coupled.
- **`tingly-box-bundle`** — same shim plus the platform zips inside the package
  (~70 MB, offline-ready). npx-oriented; not a global-install target.
- **`tingly-box-gui`** — shim variant for the desktop UI, published on demand.

All three expose `tingly-box` and `tb` bins. The shim never writes into its own
install dir — binaries and caches live under `~/.cache/tingly-box/`.

No-args behavior is split by invocation (see `cli-entry-semantics.md`): under
npx / `npm exec` (`npm_command=exec`) the cli and bundle shims keep the
historical run-now behavior as `restart --daemon -y` (the invocation is the
consent a bare `restart` prompts for); run as an installed bin (global
install) they pass `--help` instead — server lifecycle is explicit (`tingly-box start`,
which daemonizes by default) so a casual `tingly-box` can't kill in-flight AI
requests. `--source` follows the same split (`npx`/`npm`, `npx-bundle`/
`npm-bundle`) with unified handling downstream.

A global install of `tingly-box-bundle` works (same bins, same entry
semantics, binaries extracted from the bundled zips) and gets mitigations
A + B like the cli package. The residual difference on `npm install -g`
updates is payload, not dependency sprawl: the retire-rename still moves
~70 MB of zips — a handful of large files rather than the pre-A hundreds of
small ones, so the window is far narrower than it was but wider than the
cli package's two tiny files. One footnote: both packages expose the same
`tingly-box`/`tb` bin names — installs are one-or-the-other, side-by-side
global installs overwrite each other's bin links.

## Making `npm install -g` viable again

Status: A + B implemented (2026-08), E implemented (2026-09), effective from
the next publish; C is still a proposal. Today the README recommends `npx` only, because
`npm update -g tingly-box` intermittently fails with `ENOTEMPTY` and leaves the
global install broken. The rest of this doc explains the failure and lays out
the path to re-enable global installs.

- A lives in `.github/workflows/npm.yml` ("Bundle shim into a single
  dependency-free file" steps for the cli and bundle legs and the gui job,
  plus per-leg smoke-tests that run the bundled shim with no `node_modules` —
  the cli one against the real release download, the bundle one against the
  packaged zips).
- B is `cleanupRetiredInstallDirs()` in all three `build/npx/*/bin.js`.
- E is `cleanupStaleBinaryCaches()` in all three `build/npx/*/bin.js`.
- `build/npx/test-shim.sh <release-tag>` codifies the verification: it builds
  the published artifact the same way CI does (pin tag, esbuild bundle) and
  runs the matrix — sweep skipped under a fresh `node_modules` mtime, exact
  retire-shape sweep under an old one (sibling/human dirs untouched), an
  end-to-end download + `version` against the real release, and the
  stale-cache sweep guards (rollback kept, fresh/non-tag/file entries
  untouched; Linux only, where `XDG_CACHE_HOME` sandboxes the cache root). Run it before
  touching the shims or the publish workflow. CI runs it too: the
  `verify-npx-shim` job in `verify-build.yml` executes it on every release
  (and on manual runs against any tag).

### The failure, precisely

```
npm error code ENOTEMPTY
npm error syscall rename
npm error path .../lib/node_modules/tingly-box
npm error dest .../lib/node_modules/.tingly-box-BweFGdMf
```

When npm updates a global package it first "retires" the old copy by renaming
`node_modules/tingly-box` → `node_modules/.tingly-box-<hash>`, installs the new
tree, then deletes the retired dir. Two properties of npm's implementation make
this fragile and *sticky*:

1. **The retire name is deterministic** — `<hash>` is derived from the path,
   not random. So if one update is interrupted (Ctrl-C, crash, disk full) and
   leaves `.tingly-box-<hash>` behind, **every subsequent update fails** with
   ENOTEMPTY (rename dest is a non-empty dir) until the leftover is removed by
   hand. This matches the observed "fails once, then tb is broken until manual
   cleanup".
2. **The rename moves the whole package dir including its nested
   `node_modules`** — our published shim is 2 files, but at install time npm
   materializes `undici` (hundreds of files) + `unzipper` under
   `node_modules/tingly-box/node_modules/`. A big tree widens the window for
   partial failures (AV scanners, NFS/OneDrive-backed homes, concurrent npx).

Note what is *not* the cause: the shim never writes into its own install dir
(binaries go to `~/.cache/tingly-box/<tag>/bin`), so we aren't corrupting our
own package. This is an npm-side weakness we have to engineer around, same
class of pain that pushed Claude Code off npm-global installs to a native
installer + self-update.

Manual recovery, for installs predating the self-heal (B):

```bash
NPM_GLOBAL_DIR=$(npm root -g)
rm -rf "$NPM_GLOBAL_DIR/tingly-box" "$NPM_GLOBAL_DIR"/.tingly-box-*
npm install -g tingly-box@latest
```

### Plan

Ordered by cost; A+B remove the sharp edges, C removes the *need* to ever run
`npm update -g`, which is the real fix.

#### A. Ship the shim as a zero-dependency single file

Bundle `bin.js` with esbuild at publish time (CI step in `npm.yml`, before
`npm publish`) so `undici` + `unzipper` are inlined and published
`dependencies` become `{}`:

```bash
npx esbuild bin.js --bundle --platform=node --target=node18 \
  --format=esm --banner:js="import{createRequire}from'module';const require=createRequire(import.meta.url);" \
  --outfile=bin.js --allow-overwrite
```

(The `createRequire` banner is needed because `unzipper` is CJS and esbuild's
ESM output otherwise emits bare `require` calls.)

Effect: the global package dir holds 2 files (~1–2 MB), no nested
`node_modules` is ever created, the retire-rename is near-atomic, and
`npm i -g` / `npx` get faster (no dep resolution). This shrinks the ENOTEMPTY
window to almost nothing but does not fix stickiness (B does).

Applies to all three packages (bundle since 2026-08 — its shim only pulled
`unzipper`, and bundling it means an install materializes zero nested
`node_modules`; the 70 MB of zips are package assets, not dependencies, so
they're unaffected either way).

#### B. Self-heal retired leftovers in the shim

On startup, `bin.js` knows its own install location
(`dirname(fileURLToPath(import.meta.url))`). If its parent directory contains
stale `.tingly-box-*` retire dirs, remove them. This un-bricks the *next*
`npm update -g` automatically — the deterministic-name stickiness disappears.

Safety guards (the retired dir is npm's *rollback source* while a transaction
is in flight — `_rollbackMoveBackRetiredUnchanged` in arborist's reify — so
deleting it at the wrong moment would turn a recoverable failed update into a
broken install):

- Only inside a directory literally named `node_modules`, and only names
  matching npm's exact retire shape `.<own-name>-<8 alphanumeric>` (see
  `@npmcli/arborist/lib/retire-path.js`). Cross-package leftovers
  (`.tingly-box-gui-*` seen from `tingly-box`) and human-made dirs
  (`.tingly-box-backup`) never match. If npm changes the shape, the sweep
  degrades to a no-op — the safe direction.
- Skip the sweep entirely when the parent `node_modules` mtime is fresh
  (< 5 min): retiring/extracting/removing entries all touch the parent's
  mtime, so an in-flight npm transaction always looks fresh. True leftovers
  get swept on a later launch — eventual cleanup is the design intent.
- Everything wrapped in try/catch; cleanup never blocks launch.

#### C. Decouple binary version from npm version: `tb update`

Today CI pins `BINARY_RELEASE_BRANCH` to the release tag inside the published
`bin.js`, so getting a new Go binary requires a new npm package — that's why
users run `npm update -g` at all. Invert this:

1. **Go CLI grows `tingly-box update`** (and the webui "new version" banner
   points at it): query latest release, download the platform zip into
   `~/.cache/tingly-box/<new-tag>/bin/` (each version in its own dir — no
   in-place overwrite, so no Windows locked-exe problem, and a running daemon
   keeps its old inode), verify it runs (`--version`), then atomically write a
   `~/.cache/tingly-box/current` pointer file (write temp + rename).
2. **Shim resolves `current` first**: if the pointer exists and the binary it
   names exists, exec that; otherwise fall back to the baked-in
   `BINARY_RELEASE_BRANCH` tag (first-run bootstrap, and the floor version).
   The stale `getLatestVersion()` in `bin.js` (it fetches the download URL and
   would fail to parse) gets deleted — "what is latest" is the Go side's job.
3. `update` also GCs old `~/.cache/tingly-box/<tag>` dirs, keeping the
   previous one as rollback — same keep policy as E, which the shim already
   applies on every launch; `update` just makes the GC immediate instead of
   waiting for the next shim run.

After C, the npm package is a thin installer/launcher that changes rarely
(only when shim logic changes); users update via `tb update` and never touch
`npm update -g`. This is the same endgame as Claude Code's native installer,
reached without leaving npm as the distribution channel.

#### E. GC stale binary caches

Orthogonal to the install-dir problems above: every release downloads (cli,
gui) or extracts (bundle) its binaries into a fresh
`~/.cache/<package>/<tag>/` dir — by design, so updates never overwrite a
binary in place (no Windows locked-exe problem, a running daemon keeps its
old inode) — but nothing ever removed the old tag dirs, so the cache grew by
one full binary per release forever.

`cleanupStaleBinaryCaches()` in all three `build/npx/*/bin.js` sweeps them on
every launch, once the current tag's binary is confirmed in place. Policy and
guards:

- Keep the tag in use, plus the most recently touched *other* tag dir as
  rollback; sweep the rest.
- Only directories whose name is exactly a release-tag shape (`latest` or
  `vX.Y.Z[-pre]`, the `validateTransportVersion` grammar) are candidates —
  files (e.g. plan C's future `current` pointer) and human-made dirs never
  match.
- Skip any candidate touched within the last hour: a concurrent
  version-pinned `npx tingly-box@old` may be downloading into it right now.
  True stale dirs get swept on a later launch — eventual cleanup, same intent
  as B.
- Deleting a binary still running from a swept dir is safe on POSIX (the
  inode outlives the unlink); on Windows the locked exe makes `rmSync` throw,
  the error is swallowed, and the sweep retries on a later launch.
- Everything in try/catch; cleanup never blocks launch. Each package sweeps
  only its own cache root (`tingly-box`, `tingly-box-gui`,
  `tingly-box-bundle`).

Note the sweep covers *our* cache only. npm's own `_npx` / `_cacache` stores
also accumulate one entry per `npx tingly-box@<version>` invocation, but
those are npm's caches with npm's own eviction (`npm cache` handles them);
the shim has no business reaching into them.

#### D. README posture

Done (2026-08, alongside the entry-semantics split): the README and user
manual advertise both paths — npx one-shot, and
`npm install -g tingly-box@latest` + `tb start`, with updates via
`npm install -g tingly-box@latest && tb restart` (prefer install over
`npm update -g` — the install path re-resolves cleanly and also crosses
major versions). Once C lands, the update instruction becomes `tb update`.

### Rollout

1. A + B in the next shim release (no Go changes; CI + bin.js only). ✅
   (A extended to the bundle package 2026-08.)
2. Entry-semantics split + README/user-manual flip to "npm install -g
   supported" (see `cli-entry-semantics.md`). ✅ 2026-08
3. E in the next shim release (bin.js only). ✅ 2026-09
4. C behind a normal feature PR (Go `update` command + shim `current`
   resolution); ship shim change in the same release train as the Go command.
