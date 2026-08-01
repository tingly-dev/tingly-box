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
- `tingly-box start` / `restart` — silently refreshes it every time the
  server (re)starts, so the shortcut is always ready for *next* time without
  the user having to think about it

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
| `npx tingly-box@latest`       | `sh -lc 'npx -y tingly-box@latest restart --daemon'`     |
| `npx tingly-box-bundle@latest`| `sh -lc 'npx -y tingly-box-bundle@latest restart --daemon'` |

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
func ResolveLaunch(exePath, source string) LaunchSpec
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

`internal/command/shortcut.go` exposes `refreshShortcut(source
LaunchSource)`, called at the top of `StartCmdKong.Run` and
`RestartCmdKong.Run` (before the daemon fork, so it always sees the original
`--source`, not the re-exec'd child's trimmed args). It silently
(re)writes the desktop + menu shortcut with the default name, matching the
current launch method. Failures are logged at debug level and never block
startup — a shortcut write is a nice-to-have, not a precondition for serving
traffic.

This means: the very first time a user starts Tingly Box (any of the three
ways), a shortcut is already waiting for them by the time they look for one.
`tingly-box shortcut` still exists for re-running explicitly (e.g. after
deleting it, or with `--no-desktop`/`--no-menu`).

---

## 5. Per-platform shortcut formats

| platform | format        | where written                                            | invoked by               |
|----------|---------------|----------------------------------------------------------|--------------------------|
| Windows  | `.lnk`        | Desktop, Start Menu Programs                             | WScript.Shell COM        |
| macOS    | `.command`    | `~/Desktop`, `~/Applications`                            | Terminal.app (double-click) |
| Linux    | `.desktop`    | `~/Desktop` (if present), `~/.local/share/applications`  | freedesktop launcher     |

### Windows: PowerShell instead of CreateSymbolicLink

A `.lnk` is a structured shell-link blob, not a symlink. We build it through
the WScript.Shell COM object inside a PowerShell `-Command` script generated
by `windowsShortcutScript`. The script:

- resolves `Desktop` and `Programs` via `[Environment]::GetFolderPath`
  (handles **OneDrive redirection** automatically — the user's "Desktop"
  often lives under `…\OneDrive\Desktop`)
- emits each created path on its own line so the Go side can echo them back

For npx sources the `.lnk` runs `cmd.exe /c npx -y …` rather than the
extracted binary directly, so updates picked up by `npx -y …@latest` apply
immediately.

### macOS: `.command` over `.app`

A `.command` is just a shell script with `chmod +x` and the right extension.
Double-clicking opens Terminal and runs it. We could ship a `.app` bundle
instead, but:

- `.app` requires an `Info.plist`, code-signing for Gatekeeper, and a custom
  icon to look passable
- `.command` works without any of that, and the user already accepts a
  terminal window briefly when installing dev tools

So `.command` wins on UX-vs-cost.

### Linux: `.desktop` with quoted `Exec`

The `Exec` line is built via `shJoin`, which single-quote-wraps every
component. That keeps paths with spaces (e.g. `/opt/tingly box/tingly-box`)
intact across desktop environments. `Terminal=false` because the daemon
detaches itself; no need to flash a terminal window.

### npx sources: wrap in `sh -lc`

Both macOS and Linux non-binary shortcuts run

```sh
sh -lc 'npx -y tingly-box@latest restart --daemon'
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

func ResolveLaunch(exePath, source string) LaunchSpec
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
    spec := shortcut.ResolveLaunch(exePath, h.launchSource) // set once at boot
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
| smart defaults over toggles          | shortcut refreshes automatically on start/restart; `--no-desktop`/`--no-menu` are opt-out on the explicit command |
| show concrete values not aliases     | success output prints the **real paths** written, not "Created 2 shortcuts"           |
| surface the artifact for next action | last line tells the user "Double-click it to start Tingly Box and open the web UI."   |
| scope side effects to current surface| writes only under user-owned dirs (`~/Desktop`, `~/.local/share`, `%APPDATA%`); never sudo |
| diagnostics traverse the real path   | source comes from the actual invocation, not a guess                                  |

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
