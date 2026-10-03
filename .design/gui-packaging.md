# Desktop GUI packaging

What the Wails GUI (`gui/wails3`) ships as, per platform, and why. Built by
`.github/workflows/release-gui.yml`, its own pipeline: `release.yml` (`build_gui`) can call
it, or dispatch it alone with an existing release tag to attach the packages
later. Publishing `tingly-box-gui` to npm (`npm.yml`, `publish_gui`) is a
separate step; the three can run in any order.

| Platform | Asset | How users get it |
|---|---|---|
| macOS Apple Silicon | `tingly-box-gui-macos-arm64.zip` (`TinglyBox.app`, ad-hoc signed) | `npx tingly-box-gui`, or the zip |
| Windows x64 | `tingly-box-gui-windows-amd64.zip` (`tingly-box-gui.exe`) | the zip |
| Linux x64 | `tingly-box-gui-linux-amd64.deb` / `.rpm` | `apt install ./…deb` / `dnf install ./…rpm` |

Not built: Intel macOS, Linux arm64, Windows arm64. The CLI still ships
all of those, and it is the full product (the GUI is the same gateway with a
window and a tray). A GUI build is only worth its CI time and support
surface where the desktop audience is.

## npm: per-platform packages

`tingly-box-gui` follows the CLI's scheme (`npm.md` F): the shim pins
`@tingly-dev/tingly-box-gui-darwin-arm64` and `@tingly-dev/tingly-box-gui-win32-x64`
as exact-version `optionalDependencies`; each carries the release zip
(`shared/platform.js` `GUI_PLATFORM_PACKAGES`, built by
`build-platform-packages.sh … gui`). Linux has none (deb/rpm, see below). The
shim extracts the zip into its versioned cache and launches from there; the
GitHub download is the fallback (`--no-optional`, mirror lag, version
mismatch, `--transport-version`).

- **macOS**: the app is ad-hoc signed in CI. Extraction can break the bundle
  seal, so the shim runs `codesign --verify` and re-signs ad hoc only if that
  fails. npm and the shim never set the quarantine flag, so Gatekeeper does
  not prompt on this path (a browser download still needs "Open Anyway").
- **Windows**: the shim extracts `tingly-box-gui.exe` and starts it detached.
  Unsigned: SmartScreen may still warn on first run.
- The packages are published by `npm.yml`'s `publish-gui` job, which needs
  the GUI zips already on the release. A platform package that does not exist on npm yet is skipped (warning, not
  published, not pinned; that platform falls back to the release download), so a
  missing package never fails the run. Each new package name needs a Trusted
  Publisher (repo `tingly-dev/tingly-box`, workflow `npm.yml`, environment
  `production`) on npmjs.com before its first publish.

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

**Not in the npm shim.** `npx tingly-box-gui` on Linux prints the two
install commands instead of downloading anything: a package manager install
is what pulls in the libraries.
