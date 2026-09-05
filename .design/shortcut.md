# Desktop Shortcut: Design and Decisions

> Audience: contributors touching the `tingly-box shortcut` command, `start`/
> `restart`, the npx wrappers, or a future HTTP "set up shortcut" handler.

---

## 1. Background

Tingly Box is a locally-hosted gateway. Once installed the user needs to
re-launch it after every reboot. The original answer was "run `tingly-box
restart --daemon` in a terminal" — which fails the UX bar set by
`.design/ux-principles.md`:

- new users on Windows have no muscle memory for terminals
- the npx path (`npx tingly-box@latest …`) is even longer
- "remembering and typing the right command" is exactly the kind of cognitive
  load the product is supposed to remove

So: a shortcut on the desktop / start menu that launches Tingly Box with a
double-click. Two things create/refresh it:

- `tingly-box shortcut` — explicit, user-triggered; the primary, documented
  way to get one
- `tingly-box start --shortcut` / `restart --shortcut` (**default off**) —
  an opt-in convenience for doing both in one command. It's off by default
  on purpose: writing files to the user's Desktop/Start Menu on every
  ordinary `start` — something a user runs constantly, including headless
  in CI/Docker/scripts — is a surprising side effect for a command whose job
  is "start the server." Making it explicit means it only happens when
  someone actually asks for it.

---

## 2. Module layout

```
internal/shortcut/        # pure domain — no Kong, no CLI, no HTTP imports
    shortcut.go           #   LaunchSpec, Options, ResolveLaunch, Create,
                          #   ExpectedPaths, ExpectedLinuxScriptPath, ResolveExePath
    shortcut_test.go
    templates/            #   the actual script/entry text, as text/template
        windows_shortcut.ps1.tmpl
        macos_command.sh.tmpl
        linux_desktop.desktop.tmpl

internal/command/
    shortcut.go           # Kong shell: ShortcutCmdKong, LaunchSource type,
                          # refreshShortcut() (called from start/restart)

internal/server/module/shortcut/  # HTTP handler: GET/POST /api/v1/shortcut
    handler.go, routes.go, types.go, handler_test.go

cli/tingly-box/main.go    # wires the global --source flag, binds LaunchSource
build/npx/*/bin.js        # injects --source=npx / --source=npx-bundle

frontend/src/components/ShortcutCard.tsx  # the card (web/npx/binary only, not Wails GUI)
frontend/src/components/ProvidersCard.tsx # unrelated to shortcuts, but shares HelpPage —
                                           # the old Onboarding page's provider-browse content
frontend/src/pages/HelpPage.tsx           # hosts ShortcutCard + ProvidersCard
frontend/src/layout/ActivityBar.tsx       # lightbulb entry point, route /help (replaces the
                                           # old "Quick Add Provider" wand's exact nav slot)
```

**Rule:** anything platform-specific (PowerShell COM script, `.command`
script, `.desktop` entry) lives in `internal/shortcut/`. Anything Kong-shaped
or stdout-shaped lives in `internal/command/`. A future HTTP handler under
`internal/server/api/` can call `shortcut.ResolveLaunch` + `shortcut.Create`
directly — no CLI dependency.

The generated file *content* (the actual PowerShell/shell/desktop-entry
text) lives in `templates/*.tmpl`, `//go:embed`-ed and rendered with
`text/template`, rather than assembled via `strings.Builder`/`fmt.Sprintf`
calls in Go. A reviewer (or anyone auditing what this binary writes to a
user's Desktop/Start Menu) can open one `.tmpl` file and read the exact
script top to bottom, instead of reconstructing it mentally from a chain of
`WriteString` calls. Values are still escaped/quoted in Go before being
handed to the template (`psQuote`, `shJoin`) — `text/template` does no
shell/PowerShell-aware escaping on its own, it's just the structural
substitution.

---

## 3. Three install shapes, one launch source

Tingly Box ships through three install paths, and the shortcut has to launch
the **same** path the user is already using:

| install path                  | how the shortcut launches it                          |
|-------------------------------|---------------------------------------------------------|
| native binary (Homebrew, etc.)| `<exePath> restart --daemon`                             |
| `npx tingly-box@latest`       | `sh -lc 'npx -y tingly-box@1.4.2 restart --daemon'`      |
| `npx tingly-box-bundle@latest`| `sh -lc 'npx -y tingly-box-bundle@1.4.2 restart --daemon'` |

The npx package spec is **pinned to the currently-running version**
(`internal/command.BuildVersion`, threaded through as `ResolveLaunch`'s
`version` param), not `@latest`. Two reasons:

- a shortcut that silently pulls whatever is newest on the registry every
  time it's double-clicked is a surprise auto-upgrade the user didn't ask
  for — the whole point of the shortcut is "start the thing I already have
  working again"
- it works **offline** once npm has that exact version cached; `@latest`
  requires hitting the registry to resolve the tag every single launch

The shortcut still tracks upgrades: since `start`/`restart` refresh it every
launch (§4), the moment the user runs a newer version once (any way), the
next refresh repins the shortcut to that version. `version == ""` (or the
`"dev"`/`"unknown"` placeholders unversioned/dev builds report) falls back to
`@latest`, since there's nothing meaningful to pin to.

### How the source is known — no detection, no persistence

Earlier revisions of this feature tried to have `tingly-box shortcut` guess
its install method after the fact: a `--target=auto` flag that fell back to
a **recorded** `launch_source` in `config.json`, which itself fell back to
sniffing whether the executable path lived under the npx cache directory.

That machinery only exists to answer "how was *this* process launched"
*after* the fact, in a *different* invocation than the one that actually
knows. But the npx wrapper already knows for certain, on the **very same
invocation** that runs `shortcut`/`start`/`restart`:

```js
// build/npx/tingly-box/bin.js
const SOURCE_ARGS = ["--source=npx"];
spawn(binary, [...SOURCE_ARGS, ...process.argv.slice(2)], …);
```

So the CLI just uses the `--source` flag **for the invocation it's already
handling** — no config field, no fallback chain, no path sniffing:

```go
func ResolveLaunch(exePath, source, version string) LaunchSpec
```

`source` is `""` (plain binary), `"npx"`, or `"npx-bundle"`. There is nothing
to persist and nothing to detect: whoever is generating or refreshing the
shortcut right now is, by construction, the process that was launched the
way the shortcut should launch again.

`--source` stays a **global** Kong flag (not per-subcommand) purely so the
npx wrapper can prepend it once to every invocation without special-casing
which subcommand is being run. It is bound into `ctx.Run()` as a typed
`command.LaunchSource` value and read directly by `ShortcutCmdKong.Run`,
`StartCmdKong.Run`, and `RestartCmdKong.Run` — never written to disk.

---

## 4. Opt-in refresh on start/restart

`StartCmdKong` (and `RestartCmdKong`, which embeds it) has an `EnableShortcut
bool` field — `--shortcut`, **default false**. When passed,
`internal/command/shortcut.go`'s `refreshShortcut(source LaunchSource)` runs
at the top of `Run()`, before the daemon fork (so it always sees the
original `--source`, not the re-exec'd child's trimmed args). It silently
(re)writes the Desktop/menu shortcut, matching the current launch method and
version. Failures are logged at debug level and never block startup — a
shortcut write is a nice-to-have, not a precondition for serving traffic.

This is deliberately opt-in rather than the `--ui`/`--browser`/`--adapter`
default-on convention used elsewhere on the same struct: those flags affect
things the user is actively doing *right now* (serving the UI, opening a
browser tab) and are easy to notice and undo. Writing files to the user's
Desktop on every routine `start` — a command run constantly, including
headless in CI/Docker/scripts — is a different class of side effect; it
should only happen when someone asks for it. `tingly-box shortcut` remains
the primary, explicit way to create/refresh one (also useful after deleting
it, or with `--no-desktop`/`--no-menu`); `--shortcut` on start/restart is
just a one-command shortcut *to* the shortcut command.

---

## 5. Per-platform shortcut formats

| platform | format        | where written                                            | invoked by               |
|----------|---------------|----------------------------------------------------------|--------------------------|
| Windows  | `.lnk`        | Desktop, Start Menu Programs                             | WScript.Shell COM        |
| macOS    | `.command`    | `~/Desktop` only                                         | Terminal.app (double-click) |
| Linux    | `.desktop`    | `~/Desktop` (if present), `~/.local/share/applications`  | freedesktop launcher     |
| Linux    | + `.sh` launcher script | `~/.local/bin`                                  | user's shell             |

Every platform's file uses the same space-free base name — `tingly-box.lnk`
/ `tingly-box.command` / `tingly-box.desktop` — regardless of the
human-readable `--name` (default "Tingly Box") passed in. `slugName()`
lower-cases and hyphenates it once, shared across all three generators.
Filenames with spaces need extra quoting everywhere they're referenced
(other generated scripts, shell commands, path arguments) and are a
recurring, easy-to-miss source of bugs; the display name is free to keep
its spaces since it's never used as a path component (it only shows up
inside the Linux `.desktop`'s `Name=` field — Windows/macOS have no
separate display-name field distinct from the filename, so their shortcut
is literally captioned "tingly-box" rather than "Tingly Box").

### Windows: PowerShell/COM, TargetPath set directly — no hidden-launch trick

A `.lnk` is a structured shell-link blob, not a symlink. We build it through
the WScript.Shell COM object inside a PowerShell `-Command` script rendered
from `templates/windows_shortcut.ps1.tmpl`. The script resolves `Desktop`
and `Programs` via `[Environment]::GetFolderPath` (handles **OneDrive
redirection** automatically — the user's "Desktop" often lives under
`…\OneDrive\Desktop`) and emits each created `.lnk` path on its own line so
the Go side can echo them back. `TargetPath`/`Arguments` are set directly to
the real command (the binary, or `cmd.exe /c npx …`) and `WindowStyle = 7`
(start minimized) — a documented `IShellLink` property — is the only
cosmetic mitigation applied.

**We deliberately do *not* route the launch through a generated helper
script run with a hidden window** (e.g. writing a `.vbs` next to the `.lnk`
and having it call `WScript.Shell.Run(cmd, 0, False)`, or the PowerShell
equivalent `Start-Process -WindowStyle Hidden`). That shape — a program that
writes a script to disk and launches it invisibly — is close to a textbook
dropper pattern, and real antivirus/SmartScreen heuristics flag exactly
that; VBScript is also being phased out of Windows entirely. Getting
Tingly Box's installer flagged as malware to avoid a console flash is a bad
trade. So a `cmd /c`-based npx launch may still pop a visible (now
minimized) console, and on a terminal host configured to never auto-close,
it can still linger until the user dismisses it — that's an accepted,
lesser cost, not something we've fully solved on Windows yet.

### macOS: `.command`, Desktop only, closing itself on success

A `.command` is just a shell script with `chmod +x` and the right extension.
Double-clicking opens Terminal.app and runs it. We could ship a `.app`
bundle instead (sidestepping Terminal entirely, the way the Windows fix
above sidesteps its console), but:

- `.app` requires an `Info.plist`, code-signing for Gatekeeper, and a custom
  icon to look passable
- `.command` works without any of that

So `.command` still wins on UX-vs-cost. But that tradeoff also means macOS
has no real menu-equivalent location: Launchpad and Spotlight's
"Applications" category only recognize actual `.app` bundles (they read the
bundle's `Info.plist`) — dropping a `.command` file into `~/Applications`
doesn't make it launchable from either, it's just a second, harder-to-find
copy sitting in a folder Finder already treats as "real apps only".
`createMacShortcuts` writes **Desktop only**; `--no-menu` is accepted but is
a no-op there (nothing to turn off). Building a proper `.app` bundle to get
genuine Launchpad integration is the real fix for that gap, and remains the
same not-worth-it-yet tradeoff as above.

Separately, Terminal.app's default behavior is to leave the window open
after the script exits, showing `[Process completed]`, requiring a manual
close every single time. The generated script now closes that window itself
**only on success**:

```sh
#!/bin/sh
'/path/to/tingly-box' 'restart' '--daemon'
status=$?
if [ "$status" -eq 0 ]; then
  tty_path=$(tty)
  osascript -e "tell application \"Terminal\" to close (every window whose tty is \"$tty_path\")" >/dev/null 2>&1 &
  exit 0
fi
echo
echo "tingly-box exited with status $status."
printf 'Press Enter to close this window...'
read -r _
exit "$status"
```

It targets the window by **its own tty** (`$(tty)`), so it never touches any
other Terminal window the user has open. On failure the window stays open
with the error visible and a "press Enter" prompt — auto-closing
unconditionally would hide exactly the information the user needs when
something goes wrong (port in use, permissions, etc.).

### Linux: `.desktop` with quoted `Exec`

The `Exec` line is built via `shJoin`, which single-quote-wraps every
component. That keeps paths with spaces (e.g. `/opt/tingly box/tingly-box`)
intact across desktop environments. `Terminal=false` because the daemon
detaches itself; no need to flash a terminal window.

### Linux: executable launcher script, always

A `.desktop` entry is only launchable from a graphical session. Run
`tingly-box shortcut` on a headless box (server, container, plain SSH) and
the original implementation still "succeeded" — it wrote
`~/.local/share/applications/tingly-box.desktop` — while producing nothing
the user could actually run. That fails "surface the artifact for the next
action": the whole point of the command is a runnable thing.

So on Linux, `createLinuxShortcuts` **additionally** writes an executable
launcher script, rendered from `templates/linux_launcher.sh.tmpl`:

```sh
#!/bin/sh
# Starts Tingly Box: restarts the daemon and opens the web UI.
# Generated by `tingly-box shortcut`; rerun that command to regenerate it.
exec 'npx' '-y' 'tingly-box@1.4.2' 'restart' '--daemon'   # or the binary path
```

Decisions and trade-offs:

- **Written to `~/.local/bin/tingly-box.sh`** — user-owned, and commonly on
  `PATH` (systemd file-hierarchy convention; Debian's default `.profile`
  adds it when present), so after a re-login it's often runnable as just
  `tingly-box.sh`. The `.sh` suffix keeps it from colliding with the actual
  `tingly-box` binary that may also live in `~/.local/bin`.
- **Additive, not a replacement.** The `.desktop` entries are still written:
  `DISPLAY` being empty can also mean an SSH session into a machine that
  *does* have a desktop, where the menu entry stays useful. The reverse
  (script written in a graphical session) is suppressed — desktop users
  already have the double-clickable artifact and don't need a second copy.
- **Gated by both flags together**: the script is the headless stand-in for
  the desktop/menu artifacts as a whole, so only `--no-desktop --no-menu`
  combined suppresses it (same condition as "nothing to do").
- The CLI output swaps "Double-click it…" for "Run `<path>`…" when a script
  was written (detected by the `.sh` suffix in the returned paths).

### npx sources: wrap in `sh -lc`

Both macOS and Linux non-binary shortcuts run

```sh
sh -lc 'npx -y tingly-box@1.4.2 restart --daemon'
```

A login-shell wrapper is required because GUI-launched processes inherit a
**minimal PATH** that often excludes the user's Node install (nvm, asdf,
Homebrew node). `sh -lc` re-sources the user's profile so `npx` resolves.

---

## 6. Public API surface (`internal/shortcut`)

```go
const (
    SourceBinary    = "binary"
    SourceNpx       = "npx"
    SourceNpxBundle = "npx-bundle"
)

func LaunchArgs() []string   // ["restart", "--daemon"]

type LaunchSpec struct {
    Argv      []string   // POSIX command vector — macOS / Linux
    WinTarget string     // .lnk TargetPath
    WinArgs   string     // .lnk Arguments
    WorkDir   string
}

type Options struct {
    Name      string
    NoDesktop bool
    NoMenu    bool
}

func ResolveLaunch(exePath, source, version string) LaunchSpec
func Create(opts Options, spec LaunchSpec) ([]string, error)
```

`Create` returns the list of paths written so the caller (CLI today, HTTP
handler tomorrow) can display them. Nothing in this package writes to
`stdout` or imports a CLI framework.

### HTTP handler and frontend (implemented)

`internal/server/module/shortcut` is the HTTP handler this section used to
only sketch. It follows the plan above exactly: `launchSource` is set once at
boot via `server.WithLaunchSource(string(source))` (threaded from the CLI's
`--source` flag through `startServerWithHook` → `NewServerManager`), stored on
`*Server`, never persisted to disk.

```
GET  /api/v1/shortcut   → ShortcutStatusResponse  (read-only; os.Stat only)
POST /api/v1/shortcut   → ShortcutCreateResponse  (Name optional, defaults "Tingly Box")
```

Both zero-config beyond the optional display name — no `--no-desktop`/
`--no-menu` equivalent on the HTTP surface, since the UI has exactly one
action ("Create Shortcut" / "Recreate") rather than a form. `GET` answers
"does every artifact `Create` would write already exist" via
`shortcut.ExpectedPaths` (a pure path-computation twin of `Create` — same
Windows OneDrive-redirection caveat as `Create` itself doesn't apply to the
best-effort `GET`, see its doc comment) — it never writes to disk, only
`os.Stat`s.

The frontend surfaces this as `ShortcutCard` (`frontend/src/components/ShortcutCard.tsx`),
shown only outside Wails GUI mode (`!isGuiMode()`) — a GUI build already has a
native window/icon and doesn't need a desktop shortcut. The card is
deliberately re-entrant, not a one-shot "done" action
(`.design/ux-principles.md` §10): `POST` is idempotent, so the button stays
clickable after success to recover a deleted shortcut or re-point it after an
upgrade or a different launch method. On success it shows the real paths
written and, on Linux, the headless launcher script path with a copy button —
matching `UpdatePanelDialog`'s "hand over the concrete artifact" pattern (§11)
rather than a bare "Created!" toast.

It first shipped on the System settings page, but that buried it too deep for
a first-run user (the exact person who most needs it — see §1). It now lives
on a dedicated `HelpPage` (`frontend/src/pages/HelpPage.tsx`, route `/help`),
reached via a lightbulb entry in the activity bar
(`frontend/src/layout/ActivityBar.tsx`) that sits in the *same nav slot* the
old standalone "Quick Add Provider" wand button used to occupy — the
lightbulb is the product's onboarding front door now, not an addition next
to it. HelpPage carries a small, always-visible, low-noise collection of
easy-to-miss useful actions (not a linear onboarding tour): `ShortcutCard`
first, then `ProvidersCard` — the old Onboarding page's provider-browsing
content, demoted from a full page to one card among others.
Deliberately *not* a notification badge on the lightbulb itself: `GET
/api/v1/shortcut` does give a real per-machine "exists" signal (that's what
drives the card's own "Recreate"/"Already created" state once you're on the
page), but reusing that to light up a badge on the nav icon would be a second,
separate reactive UI surface for one card — not worth it yet for a page that
currently holds exactly one card. A quiet, always-present link is enough;
revisit if/when this page grows enough cards that "is there anything new
here" becomes a real question.

Explicitly out of scope, on purpose (security/UX call, not a gap to fill
later): the frontend never wires this into `start`/`restart`, and never
auto-creates or auto-refreshes a shortcut on its own — creation is only
ever a direct, visible user click on the System page, same posture as
`tingly-box shortcut` on the CLI side.

---

## 7. UX checklist (against `.design/ux-principles.md`)

| principle                            | how this feature satisfies it                                                       |
|--------------------------------------|--------------------------------------------------------------------------------------|
| smart defaults over toggles          | `--no-desktop`/`--no-menu` are opt-out on the explicit `shortcut` command; `--shortcut` on start/restart is opt-in since it's a side effect on an otherwise-unrelated, frequently-run command |
| show concrete values not aliases     | success output prints the **real paths** written, not "Created 2 shortcuts"; npx shortcuts pin a real version number, not the `latest` alias |
| surface the artifact for next action | last line tells the user "Double-click it to start Tingly Box and open the web UI."   |
| scope side effects to current surface| writes only under user-owned dirs (`~/Desktop`, `~/.local/share`, `%APPDATA%`); never sudo; nothing (not even `~/Applications`) is written when it wouldn't actually be useful |
| diagnostics traverse the real path   | source comes from the actual invocation, not a guess                                  |
| reduce visual noise                  | Windows starts the shortcut minimized; macOS closes its Terminal window on success, only staying open when there's an error to show |

> **Testing note:** none of the platform-specific generators have been
> executed on their real OS in this dev environment (Linux-only sandbox) —
> only verified via unit tests against the rendered script/entry text
> (`internal/shortcut/shortcut_test.go`) plus manual end-to-end runs of the
> Linux path (the one this sandbox can actually execute). Worth a smoke test
> on real Windows/macOS machines before treating those two as fully proven.

---

## 8. Related files

| ref                                          | content                                  |
|----------------------------------------------|------------------------------------------|
| `internal/shortcut/shortcut.go`              | domain (this package, reusable)          |
| `internal/shortcut/templates/*.tmpl`         | the actual generated script/entry text   |
| `internal/shortcut/shortcut_test.go`         | tests against public API                 |
| `internal/command/shortcut.go`               | Kong shell, `LaunchSource`, `refreshShortcut` |
| `internal/command/server.go`                 | `start`/`restart` call `refreshShortcut` |
| `cli/tingly-box/main.go`                     | global `--source` flag, binds `LaunchSource` into `ctx.Run` |
| `build/npx/tingly-box/bin.js`                | npx wrapper, injects `--source=npx`      |
| `internal/server/module/shortcut/`           | HTTP handler for `GET`/`POST /api/v1/shortcut` |
| `internal/server/server_options.go`          | `WithLaunchSource`                       |
| `frontend/src/components/ShortcutCard.tsx`   | the card (web/npx/binary only, not Wails GUI) |
| `frontend/src/components/ProvidersCard.tsx`  | unrelated content, shares HelpPage — old Onboarding page's provider browse/connect |
| `frontend/src/pages/HelpPage.tsx`             | hosts `ShortcutCard` + `ProvidersCard`   |
| `frontend/src/layout/ActivityBar.tsx`         | lightbulb entry point, `/help` route, same nav slot the old wand used |
| `frontend/src/services/api.ts`               | `getShortcutStatus`, `createShortcut`    |
