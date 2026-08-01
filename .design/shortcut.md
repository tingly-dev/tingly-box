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

- `tingly-box shortcut` — explicit, user-triggered
- `tingly-box start` / `restart --shortcut` (**default on**) — refreshes it
  every time the server (re)starts, so the shortcut is ready for *next* time
  without the user having to think about it; `--no-shortcut` opts out (e.g.
  headless/CI/Docker runs where writing a desktop file makes no sense)

---

## 2. Module layout

```
internal/shortcut/        # pure domain — no Kong, no CLI imports
    shortcut.go           #   LaunchSpec, Options, ResolveLaunch, Create
    shortcut_test.go

internal/command/
    shortcut.go           # Kong shell: ShortcutCmdKong, LaunchSource type,
                          # refreshShortcut() (called from start/restart)

cli/tingly-box/main.go    # wires the global --source flag, binds LaunchSource
build/npx/*/bin.js        # injects --source=npx / --source=npx-bundle
```

**Rule:** anything platform-specific (PowerShell COM script, `.command`
script, `.desktop` entry) lives in `internal/shortcut/`. Anything Kong-shaped
or stdout-shaped lives in `internal/command/`. A future HTTP handler under
`internal/server/api/` can call `shortcut.ResolveLaunch` + `shortcut.Create`
directly — no CLI dependency.

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

## 4. Refresh on every start/restart

`StartCmdKong` (and `RestartCmdKong`, which embeds it) has an `EnableShortcut
bool` field — `--shortcut`, **default true** — following the same
opt-out-not-opt-in convention as `--ui`/`--browser`/`--adapter` on the same
struct. When set, `internal/command/shortcut.go`'s `refreshShortcut(source
LaunchSource)` runs at the top of `Run()`, before the daemon fork (so it
always sees the original `--source`, not the re-exec'd child's trimmed
args). It silently (re)writes the desktop + menu shortcut with the default
name, matching the current launch method and version. Failures are logged at
debug level and never block startup — a shortcut write is a nice-to-have,
not a precondition for serving traffic.

This means: the very first time a user starts Tingly Box (any of the three
ways), a shortcut is already waiting for them by the time they look for one.
`--no-shortcut` opts out for non-interactive contexts (CI, Docker, headless
servers) where writing a desktop file is meaningless or could hit a
read-only/nonexistent home directory. `tingly-box shortcut` still exists for
re-running explicitly (e.g. after deleting it, or with
`--no-desktop`/`--no-menu`).

---

## 5. Per-platform shortcut formats

| platform | format        | where written                                            | invoked by               |
|----------|---------------|----------------------------------------------------------|--------------------------|
| Windows  | `.lnk`        | Desktop, Start Menu Programs                             | WScript.Shell COM        |
| macOS    | `.command`    | `~/Desktop`, `~/Applications`                            | Terminal.app (double-click) |
| Linux    | `.desktop`    | `~/Desktop` (if present), `~/.local/share/applications`  | freedesktop launcher     |

### Windows: hidden launch via a generated .vbs + wscript.exe

A `.lnk` is a structured shell-link blob, not a symlink. We build it through
the WScript.Shell COM object inside a PowerShell `-Command` script generated
by `windowsShortcutScript`. The script resolves `Desktop` and `Programs` via
`[Environment]::GetFolderPath` (handles **OneDrive redirection**
automatically — the user's "Desktop" often lives under `…\OneDrive\Desktop`)
and emits each created `.lnk` path on its own line so the Go side can echo
them back.

Pointing the `.lnk`'s `TargetPath` straight at the binary (or at
`cmd.exe /c npx …`) — the original approach — pops a visible console window
every time. On some terminal hosts (Windows Terminal set as the default
console host, with a profile's "close on exit" not set to auto) that window
doesn't even close itself afterward; it sits there showing "Process exited"
until the user dismisses it by hand — for a background daemon relaunch, that
window shouldn't exist at all. So instead of TargetPath-ing the real command
directly, we generate a small VBScript helper next to the `.lnk`
(`<name>.vbs`) that does:

```vbscript
Set sh = CreateObject("WScript.Shell")
sh.CurrentDirectory = "<workdir>"
sh.Run "<the real command>", 0, False
```

`Run(cmd, 0, False)` — window style **0** — is the standard WSH idiom for
"launch a process with no window at all". The `.lnk` itself then targets
`wscript.exe //B //Nologo "<name>.vbs"`, with `IconLocation` pointed back at
the original executable so it still looks like a Tingly Box shortcut rather
than a generic script icon. Double quotes inside the real command (e.g.
around a `C:\Program Files\...` path) are doubled (`""`) before being
embedded in the `.vbs`'s string literal, per VBScript's escaping rule.

This eliminates the lingering-console problem entirely rather than papering
over one terminal host's default setting — no window ever appears, on any
terminal host.

### macOS: `.command`, closing itself on success

A `.command` is just a shell script with `chmod +x` and the right extension.
Double-clicking opens Terminal.app and runs it. We could ship a `.app`
bundle instead (sidestepping Terminal entirely, the way the Windows fix
above sidesteps its console), but:

- `.app` requires an `Info.plist`, code-signing for Gatekeeper, and a custom
  icon to look passable
- `.command` works without any of that

So `.command` still wins on UX-vs-cost — but Terminal.app's default behavior
is to leave the window open after the script exits, showing
`[Process completed]`, requiring a manual close every single time. The
generated script now closes that window itself **only on success**:

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

### Future HTTP handler sketch

A running server process was itself started via `start`/`restart` with some
`--source`, and could stash that value in memory (not disk) at boot to
answer an API request later:

```go
// POST /api/v1/shortcut
func (h *ShortcutAPI) Create(c *gin.Context) {
    var req struct {
        Name      string `json:"name"`
        NoDesktop bool   `json:"no_desktop"`
        NoMenu    bool   `json:"no_menu"`
    }
    _ = c.BindJSON(&req)

    exePath, _ := os.Executable()
    // both set once at boot from how this process itself was invoked
    spec := shortcut.ResolveLaunch(exePath, h.launchSource, h.version)
    created, err := shortcut.Create(shortcut.Options{
        Name: req.Name, NoDesktop: req.NoDesktop, NoMenu: req.NoMenu,
    }, spec)
    // ... response
}
```

No new domain logic required, and still no disk persistence — `launchSource`
lives only as long as the process that was actually launched that way.

---

## 7. UX checklist (against `.design/ux-principles.md`)

| principle                            | how this feature satisfies it                                                       |
|--------------------------------------|--------------------------------------------------------------------------------------|
| smart defaults over toggles          | `--shortcut` defaults on for start/restart (opt-out, `--no-shortcut`); `--no-desktop`/`--no-menu` are opt-out on the explicit command |
| show concrete values not aliases     | success output prints the **real paths** written, not "Created 2 shortcuts"; npx shortcuts pin a real version number, not the `latest` alias |
| surface the artifact for next action | last line tells the user "Double-click it to start Tingly Box and open the web UI."   |
| scope side effects to current surface| writes only under user-owned dirs (`~/Desktop`, `~/.local/share`, `%APPDATA%`); never sudo |
| diagnostics traverse the real path   | source comes from the actual invocation, not a guess                                  |
| reduce visual noise                  | Windows never shows a console at all (hidden wscript launch); macOS closes its Terminal window on success, only staying open when there's an error to show |

> **Testing note:** the Windows generator (§5) was verified by hand-tracing
> the PowerShell/VBScript quote-escaping and by unit-testing the generated
> script's text (`TestWindowsShortcutScriptHiddenLaunch`), not by executing
> it on real Windows — this dev environment has no Windows host. Worth a
> smoke test on an actual machine before treating it as fully proven.

---

## 8. Related files

| ref                                          | content                                  |
|----------------------------------------------|------------------------------------------|
| `internal/shortcut/shortcut.go`              | domain (this package, reusable)          |
| `internal/shortcut/shortcut_test.go`         | tests against public API                 |
| `internal/command/shortcut.go`               | Kong shell, `LaunchSource`, `refreshShortcut` |
| `internal/command/server.go`                 | `start`/`restart` call `refreshShortcut` |
| `cli/tingly-box/main.go`                     | global `--source` flag, binds `LaunchSource` into `ctx.Run` |
| `build/npx/tingly-box/bin.js`                | npx wrapper, injects `--source=npx`      |
| `build/npx/tingly-box-bundle/bin.js`         | bundle wrapper, injects `--source=npx-bundle` |
