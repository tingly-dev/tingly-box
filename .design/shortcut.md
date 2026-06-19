# Desktop Shortcut: Design and Decisions

> Audience: contributors touching the `tingly-box shortcut` command, the npx
> wrappers, or the upcoming HTTP "set up shortcut" handler.

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

The `shortcut` subcommand exists so that a single `tingly-box shortcut`
invocation drops a double-clickable launcher on the desktop / start menu.

---

## 2. Module layout

```
internal/shortcut/        # pure domain — no Kong, no CLI imports
    shortcut.go           #   LaunchSpec, Options, ResolveLaunch, Create,
                          #   IsKnownSource, IsNpxCachedBinary, LaunchArgs
    shortcut_test.go

internal/command/
    shortcut.go           # Kong shell:
                          #   ShortcutCmdKong (flags), Run(), PersistLaunchSource

cli/tingly-box/main.go    # wires the global --source flag + subcommand
build/npx/*/bin.js        # injects --source=npx / --source=npx-bundle
```

**Rule:** anything platform-specific (PowerShell COM script, `.command`
script, `.desktop` entry, npx cache detection) lives in `internal/shortcut/`.
Anything Kong-shaped or stdout-shaped lives in `internal/command/`. A
future HTTP handler under `internal/server/api/` can call
`shortcut.ResolveLaunch` + `shortcut.Create` directly — no CLI dependency.

Why split: keeping the platform writers behind a Kong struct (the previous
shape) would have forced the API handler to instantiate Kong types and
re-parse `--target=auto`. The domain types (`LaunchSpec`, `Options`) are the
real contract; flags and JSON are two surfaces on top of them.

---

## 3. Three install shapes, one shortcut command

Tingly Box ships through three install paths, and the shortcut has to launch
the **same** path the user is already using — otherwise the shortcut updates
through a different channel than the rest of the user's invocations:

| install path                  | how shortcut launches it                          |
|-------------------------------|---------------------------------------------------|
| native binary (Homebrew, etc.)| `<exePath> restart --daemon`                       |
| `npx tingly-box@latest`       | `sh -lc 'npx -y tingly-box@latest restart --daemon'` |
| `npx tingly-box-bundle@latest`| `sh -lc 'npx -y tingly-box-bundle@latest restart --daemon'` |

This is exposed via `--target`:

```
tingly-box shortcut                 # auto-detect (recommended)
tingly-box shortcut --target=binary
tingly-box shortcut --target=npx
tingly-box shortcut --target=npx-bundle
```

### Why a single `--target` flag (not a mode picker)

UX principle: **eliminate mode pickers when one default is right for 95% of
users.** `auto` is the default and resolves correctly without user input in
the common case. Users who have a strong opinion (e.g. installed both
ways and want the npx variant pinned) can still set the value explicitly.

### Auto resolution

`shortcut.ResolveLaunch(exePath, target, persistedSource)` decides the launch
shape with this precedence:

1. If `target` is an explicit known source → use it.
2. Else if the config has a `launch_source` recorded → use that.
3. Else if `exePath` lives under the npx cache root → assume `npx`.
4. Else → `binary`.

(2) is the interesting one. See §4.

---

## 4. Recording the launch source

The `auto` resolver needs to know how the **current** process was started,
because by the time the user runs `tingly-box shortcut` we cannot
distinguish:

- "Homebrew binary on PATH" — should drop a binary shortcut
- "`npx tingly-box@latest shortcut`" — should drop an npx shortcut
- "`npx tingly-box-bundle@latest shortcut`" — should drop an npx-bundle shortcut

`os.Executable()` returns the resolved path on disk in all three cases, and
under npx that path can look indistinguishable from a regular install once
the binary is extracted. Path-sniffing alone is unreliable (cache locations
move; symlinks lie).

**Fix: the npx wrappers tell us.**

```js
// build/npx/tingly-box/bin.js
const SOURCE_ARGS = ["--source=npx"];
spawn(binary, [...SOURCE_ARGS, ...process.argv.slice(2)], …);
```

The Go binary treats `--source` as a **global flag** (declared on the root
Kong struct, not on a subcommand) so every invocation through npx is
tagged. On startup, `command.PersistLaunchSource` writes the value into
`config.json` if it's known and different from the recorded value.

Then `shortcut --target=auto` reads it back.

### Why `--source` is global, not per-subcommand

Earlier the flag lived on `start` only. That broke `npx tingly-box-bundle
restart` and any other subcommand the npx wrapper might forward in the
future. Promoting it to the root means **every** invocation through the npx
wrapper, regardless of subcommand, gets recorded. The Kong-level cost is
zero (the flag is a single string on the root struct).

### Why the persisted value can be wrong, and why that's OK

The user can launch via Homebrew on Monday and via npx on Tuesday. The
config records the **most recent** source. `--target` lets the user
override. The cost of a wrong default is "the shortcut runs npm instead of
the binary" — annoying, not destructive. The benefit of auto-detection is
that 95% of users never see this flag.

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

For npx targets the `.lnk` runs `cmd.exe /c npx -y …` rather than the
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

### npx targets: wrap in `sh -lc`

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

func IsKnownSource(source string) bool
func LaunchArgs() []string                 // ["restart", "--daemon"]
func IsNpxCachedBinary(exePath string) bool

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

func ResolveLaunch(exePath, target, persistedSource string) LaunchSpec
func Create(opts Options, spec LaunchSpec) ([]string, error)
```

`Create` returns the list of paths written so the caller (CLI today, HTTP
handler tomorrow) can display them. Nothing in this package writes to
`stdout` or imports a CLI framework.

### Future HTTP handler sketch

```go
// POST /api/v1/shortcut
func (h *ShortcutAPI) Create(c *gin.Context) {
    var req struct {
        Name      string `json:"name"`
        Target    string `json:"target"`     // auto|binary|npx|npx-bundle
        NoDesktop bool   `json:"no_desktop"`
        NoMenu    bool   `json:"no_menu"`
    }
    _ = c.BindJSON(&req)

    exePath, _ := os.Executable()
    persisted := h.appCfg.GetLaunchSource()
    spec := shortcut.ResolveLaunch(exePath, req.Target, persisted)
    created, err := shortcut.Create(shortcut.Options{
        Name: req.Name, NoDesktop: req.NoDesktop, NoMenu: req.NoMenu,
    }, spec)
    // ... response
}
```

No new domain logic required.

---

## 7. UX checklist (against `.design/ux-principles.md`)

| principle                            | how this feature satisfies it                                                       |
|--------------------------------------|--------------------------------------------------------------------------------------|
| eliminate mode pickers               | `--target=auto` is the default and resolves the right thing without input             |
| smart defaults over toggles          | `--no-desktop` / `--no-menu` are opt-out, not opt-in                                  |
| show concrete values not aliases     | success output prints the **real paths** written, not "Created 2 shortcuts"           |
| surface the artifact for next action | last line tells the user "Double-click it to start Tingly Box and open the web UI."   |
| scope side effects to current surface| writes only under user-owned dirs (`~/Desktop`, `~/.local/share`, `%APPDATA%`); never sudo |
| diagnostics traverse the real path   | npx detection uses the **actual** cache path, not a hard-coded directory              |

---

## 8. Related files

| ref                                          | content                                  |
|----------------------------------------------|------------------------------------------|
| `internal/shortcut/shortcut.go`              | domain (this package, reusable)          |
| `internal/shortcut/shortcut_test.go`         | tests against public API                 |
| `internal/command/shortcut.go`               | Kong shell + `PersistLaunchSource`       |
| `cli/tingly-box/main.go`                     | global `--source` + subcommand wiring    |
| `build/npx/tingly-box/bin.js`                | npx wrapper, injects `--source=npx`      |
| `build/npx/tingly-box-bundle/bin.js`         | bundle wrapper, injects `--source=npx-bundle` |
| `internal/server/config/config.go`           | `launch_source` field, getter/setter     |
| `internal/config/app_config.go`              | AppConfig delegators for launch source   |
