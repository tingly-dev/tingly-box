# Desktop GUI packaging

What the Wails GUI (`gui/wails3`) ships as, per platform, and why. Built by
`.github/workflows/release-gui.yml`, a pipeline independent of the CLI's
(`release-cli.yml`). `release.yml` is the entry point: a tag push builds the web
UI once (`build-frontend.yml`, artifact `frontend-dist`), runs the harness once,
and then starts both pipelines in parallel, gated on that harness. The GUI
runners take the shared frontend as is (`FRONTEND_PREBUILT=true`, see
`build/Taskfile.yml`), so none of them needs Node. To redo just the GUI,
dispatch `release-gui.yml` with a tag (before or after the CLI release); run
alone it builds its own frontend and skips the harness. Whichever pipeline finishes first creates the GitHub release
(`.github/actions/ensure-release`); the other attaches to it. The GUI pipeline
also dispatches its own npm publish (`npm.yml`, `publish_gui`, pending approval).

| Platform | Asset | How users get it |
|---|---|---|
| macOS Apple Silicon | `tingly-box-gui-macos-arm64.zip` (`TinglyBox.app`, ad-hoc signed) | `npx tingly-box-gui`, or the zip |
| Windows x64 | `tingly-box-gui-windows-amd64.zip` (`tingly-box-gui.exe`) | the zip |
| Linux x64 | `tingly-box-gui-linux-amd64.deb` / `.rpm` (GTK4) | `apt install ./…deb` / `dnf install ./…rpm` |
| Linux x64 / arm64 | `tingly-box-gui-linux-{amd64,arm64}.zip` (bare binary, GTK3) | `npx tingly-box-gui`, or the zip |

Not built: Intel macOS, Windows arm64, and a Linux arm64 deb/rpm (arm64 Linux gets the zip only). The CLI still ships
all of those, and it is the full product (the GUI is the same gateway with a
window and a tray). A GUI build is only worth its CI time and support
surface where the desktop audience is.

## npm: per-platform packages

`tingly-box-gui` follows the CLI's scheme (`npm.md` F): the shim pins
`@tingly-dev/tingly-box-gui-darwin-arm64`, `-win32-x64`, `-linux-x64` and `-linux-arm64`
as exact-version `optionalDependencies`; each carries the release zip
(`shared/platform.js` `GUI_PLATFORM_PACKAGES`, built by
`build-platform-packages.sh … gui`). The Linux one holds the GTK3 zip, not the
deb/rpm payload (see "Linux" below). The
shim extracts the zip into its versioned cache and launches from there; the
GitHub download is the fallback (`--no-optional`, mirror lag, version
mismatch, `--transport-version`).

- **macOS**: the app is ad-hoc signed in CI. Extraction can break the bundle
  seal, so the shim runs `codesign --verify` and re-signs ad hoc only if that
  fails. npm and the shim never set the quarantine flag, so Gatekeeper does
  not prompt on this path (a browser download still needs "Open Anyway").
- **Windows**: the shim extracts `tingly-box-gui.exe` and starts it detached.
  Unsigned: SmartScreen may still warn on first run.
- **Linux**: the shim first checks for a desktop session (`DISPLAY` /
  `WAYLAND_DISPLAY`) and for `libgtk-3.so.0` / `libwebkit2gtk-4.1.so.0` in
  `ldconfig -p` (`shared/linuxgui.js`), before downloading anything. A miss
  prints the missing library, the install command for the distribution (from
  `/etc/os-release`) and `npx tingly-box` as the way to use the product without
  any system library. It then extracts the binary and starts it detached,
  but watches the first 2 s: a nonzero early exit is reported with the app's
  stderr (exit 0 is a running instance taking over; the app is
  single-instance). Verified by `test-shim.sh` T7 (stub app, fake `ldconfig`).
- The packages are published by `npm.yml`'s `publish-gui` job, which needs
  the GUI zips already on the release. A platform package that does not exist on npm yet is skipped (warning, not
  published, not pinned; that platform falls back to the release download), so a
  missing package never fails the run. Each new package name needs a Trusted
  Publisher (repo `tingly-dev/tingly-box`, workflow `npm.yml`, environment
  `production`) on npmjs.com before its first publish.

## Verification

The GUI has its own post-release check, `verify-release-gui.yml`, separate from
the CLI's `verify-release-cli.yml`. It inspects the packages already on the release
(checksum coverage, macOS arm64 app, Windows x64 exe, deb metadata/contents and
version, rpm format) without rebuilding. `release-gui.yml` runs it after
attaching. The build itself
(including installing the deb and the `ldd` check) stays in `release-gui.yml`.

## Naming

Every GUI artifact carries `tingly-box-gui`, never the CLI's `tingly-box`:
the Wails `BIN_NAME` (`Taskfile.wails.yml`) is `tingly-box-gui`, so the macOS
`Contents/MacOS/tingly-box-gui` (and `CFBundleExecutable`), the Windows
`tingly-box-gui.exe`, the Linux `bin/tingly-box-gui` and the npm package's bin
all match. The npm package `tingly-box-gui` exposes only a `tingly-box-gui`
bin: when it also exposed `tingly-box`/`tb` it collided with the CLI package's
bins (the EEXIST described in `npm.md` F). The shim only ever downloads
`tingly-box-gui-<os>-<arch>.zip`; the platform packages carry those same zips.

## Linux

**deb + rpm, not AppImage or a bare binary.** Wails v3 links GTK4 and
WebKitGTK 6.0 dynamically (cgo; no static build is possible). A distribution
package declares them (`build/linux/nfpm/nfpm.yaml`), so `apt`/`dnf` install
them. An AppImage would have to bundle WebKitGTK, including its helper
processes, which is fragile. A bare binary would leave the user to find the
libraries from a linker error. WebKitGTK 6.0 sets the floor: Ubuntu 24.04+,
Debian 13+, Fedora 40+. CI builds on `ubuntu-24.04` and installs the
resulting deb before uploading it.

**The binary is `/usr/bin/tingly-box-gui`, never `tingly-box`.** That name
belongs to the CLI (`npm install -g`, the release binary), where a bare run
prints help (`cli-entry-semantics.md`). A GUI binary shadowing it on PATH
would open a window instead. The package is named `tingly-box-gui` for the
same reason.

**One id everywhere: `dev.tingly.box`.** It is the macOS `CFBundleIdentifier`,
the GTK application id (`Options.Linux.ApplicationID`), the `.desktop` file
name and its `StartupWMClass`, and the icon name. On Wayland the shell pairs a
window with its launcher entry by app id. A mismatch shows a generic icon in
the dock and a second, ungrouped entry.

**Version.** deb/rpm versions must start with a digit, so the Taskfile
(`build/linux/Taskfile.yml`, `PKG_VERSION`) strips the tag's `v` and turns
`-` into `~`. Anything non-numeric (`dev-<sha>`) becomes `0.0.0~<it>`. `~`
sorts before the empty string, so a prerelease or dev build never outranks
the release it precedes.

**Two Linux builds from the same code.** Wails v3 picks its toolkit with a
build tag: the default links GTK4 + WebKitGTK 6.0 (`pkg-config: gtk4
webkitgtk-6.0`, `linux_cgo.go`), `-tags gtk3` links GTK 3 + WebKitGTK 4.1
(`gtk+-3.0 webkit2gtk-4.1`, `linux_cgo_gtk3.go`, a full implementation, not a
stub). The deb/rpm are the GTK4 build, for the distributions that have it. The
zip behind npm is the GTK3 build (`task linux:build:gtk3`), built on
`ubuntu-22.04`, so it runs on Ubuntu 22.04+, Debian 12+, Fedora 36+: a binary
links forward-compatibly only, so it is built on the oldest glibc it should
run on, and `release-gui.yml` fails the build if it needs glibc newer than
2.35 or links the GTK4 stack.

**A bare binary, not a package, for npm.** It does not install anything into
the system, so `npx` needs no root; the system supplies GTK 3 / WebKitGTK 4.1
and the shim reports what is missing instead of leaving a linker error.
Trade-offs: no `.desktop` entry or icon (use the deb/rpm for that), and the
libraries are the user's to install. amd64 and arm64: cgo links GTK, so
each is built natively on a runner of its architecture (`ubuntu-22.04` and
`ubuntu-22.04-arm`), no cross-compile.

**Tray.** Wails' Linux tray is pure D-Bus (StatusNotifierItem + dbusmenu,
`pkg/application/systemtray_linux.go`, no cgo, no extra library) and registers
with `org.kde.StatusNotifierWatcher`. Where no host answers (stock GNOME
without the AppIndicator extension) the registration only logs an error and
no icon is shown; the hub panel hangs off that icon, so the main window is
the way in there. This is a desktop-environment dependency, not a package one.
