# Desktop GUI packaging

What the Wails GUI (`gui/wails3`) ships as, per platform, and why. Built by
`.github/workflows/gui.yml`, its own pipeline: `release.yml` (`build_gui`) can call
it, or dispatch it alone with an existing release tag to attach the packages
later. Publishing `tingly-box-gui` to npm (`npm.yml`, `publish_gui`) is a
separate step; the three can run in any order.

| Platform | Asset | How users get it |
|---|---|---|
| macOS Apple Silicon | `tingly-box-gui-macos-arm64.zip` (`TinglyBox.app`, ad-hoc signed) | `npx tingly-box-gui`, or the zip |
| Windows x64 | `tingly-box-gui-windows-amd64.zip` (`tingly-box.exe`) | the zip |
| Linux x64 | `tingly-box-gui-linux-amd64.deb` / `.rpm` | `apt install ./…deb` / `dnf install ./…rpm` |

Not built: Intel macOS, Linux arm64, Windows arm64. The CLI still ships
all of those, and it is the full product (the GUI is the same gateway with a
window and a tray). A GUI build is only worth its CI time and support
surface where the desktop audience is.

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
