// Package shortcut creates desktop / start-menu shortcuts that launch
// Tingly Box with a double-click. It is callable from the CLI today and from
// a future HTTP handler, so it has no Kong / cobra / command-layer dependency.
package shortcut

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Launch sources. They describe how Tingly Box is installed/started and which
// command a shortcut should run.
const (
	SourceBinary    = "binary"
	SourceNpx       = "npx"
	SourceNpxBundle = "npx-bundle"
)

// npxPackageForSource returns the npm package + version spec an npx-based
// launch should run. It pins to the currently-running version so the
// shortcut relaunches the exact build the user is already on — not whatever
// happens to be newest on the registry at double-click time — and so it
// works offline once npm has that version cached. version is empty (or the
// "dev"/"unknown" placeholders used by unversioned builds) falls back to
// "@latest".
func npxPackageForSource(source, version string) string {
	pkg := "tingly-box"
	if source == SourceNpxBundle {
		pkg = "tingly-box-bundle"
	}
	if version == "" || version == "dev" || version == "unknown" {
		return pkg + "@latest"
	}
	return pkg + "@" + version
}

// LaunchArgs are the CLI args the shortcut runs: restart the daemon and
// (since --browser defaults to true) open the web UI.
func LaunchArgs() []string {
	return []string{"restart", "--daemon"}
}

// LaunchSpec describes how the shortcut should invoke Tingly Box on each
// platform. Argv is the POSIX-style command vector used for macOS .command and
// Linux .desktop entries; WinTarget/WinArgs are the .lnk TargetPath/Arguments.
type LaunchSpec struct {
	Argv      []string
	WinTarget string
	WinArgs   string
	WorkDir   string
}

// Options controls which shortcuts get written.
type Options struct {
	Name      string
	NoDesktop bool
	NoMenu    bool
}

// ResolveLaunch decides whether the shortcut runs the binary directly or goes
// through npx, then builds the platform-specific launch vectors. source is how
// the *current* process was invoked (SourceNpx / SourceNpxBundle / anything
// else meaning a plain binary) — the caller always knows this first-hand, so
// there is no detection or persistence to do here. version pins an npx-based
// shortcut to the currently-running release (see npxPackageForSource).
func ResolveLaunch(exePath, source, version string) LaunchSpec {
	args := LaunchArgs()

	if source == SourceNpx || source == SourceNpxBundle {
		// e.g. "npx -y tingly-box@1.4.2 restart --daemon"
		npxArgv := append([]string{"npx", "-y", npxPackageForSource(source, version)}, args...)
		cmdStr := strings.Join(npxArgv, " ")
		home, _ := os.UserHomeDir()

		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = "cmd.exe"
		}

		return LaunchSpec{
			// Wrap in a login shell so GUI launches pick up node/npx on PATH.
			Argv:      []string{"sh", "-lc", cmdStr},
			WinTarget: comspec,
			WinArgs:   "/c " + cmdStr,
			WorkDir:   home,
		}
	}

	return LaunchSpec{
		Argv:      append([]string{exePath}, args...),
		WinTarget: exePath,
		WinArgs:   strings.Join(args, " "),
		WorkDir:   filepath.Dir(exePath),
	}
}

// Create dispatches to the platform-specific implementation and returns the
// paths of the shortcuts it created.
func Create(opts Options, spec LaunchSpec) ([]string, error) {
	switch runtime.GOOS {
	case "windows":
		return createWindowsShortcuts(opts, spec)
	case "darwin":
		return createMacShortcuts(opts, spec)
	default:
		return createLinuxShortcuts(opts, spec)
	}
}

// ---------------- Windows ----------------

func createWindowsShortcuts(opts Options, spec LaunchSpec) ([]string, error) {
	script := windowsShortcutScript(opts, spec)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create Windows shortcut: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	var created []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			created = append(created, line)
		}
	}
	return created, nil
}

// windowsShortcutScript builds a PowerShell script that resolves the Desktop
// and Start Menu Programs folders at runtime (handling OneDrive redirection)
// and, for each, writes a small hidden-launch .vbs helper plus a .lnk that
// runs it via wscript.exe.
//
// A .lnk pointing straight at cmd.exe/the binary always pops a console
// window, and on some terminal hosts (Windows Terminal set as default, with
// "close on exit" not set to auto) it lingers after the command finishes
// showing "Process exited". WScript.Shell's Run(cmd, 0, False) — window
// style 0 — is the standard way to launch a process with no window at all,
// so we route through it instead of launching TargetPath/Arguments directly.
// It prints each created .lnk path on its own line.
func windowsShortcutScript(opts Options, spec LaunchSpec) string {
	vbsCommand := fmt.Sprintf("\"%s\" %s", spec.WinTarget, spec.WinArgs)

	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Stop'\n")
	b.WriteString("$ws = New-Object -ComObject WScript.Shell\n")
	b.WriteString(fmt.Sprintf("$icon = %s\n", psQuote(spec.WinTarget)))
	b.WriteString(fmt.Sprintf("$workdir = %s\n", psQuote(spec.WorkDir)))
	b.WriteString(fmt.Sprintf("$name = %s\n", psQuote(opts.Name)))
	b.WriteString(fmt.Sprintf("$vbsCommand = %s\n", psQuote(vbsCommand)))
	b.WriteString("$wscript = Join-Path $env:SystemRoot 'System32\\wscript.exe'\n")
	b.WriteString("$dests = @()\n")
	if !opts.NoDesktop {
		b.WriteString("$dests += [Environment]::GetFolderPath('Desktop')\n")
	}
	if !opts.NoMenu {
		b.WriteString("$dests += [Environment]::GetFolderPath('Programs')\n")
	}
	b.WriteString("foreach ($dir in $dests) {\n")
	b.WriteString("  if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }\n")
	b.WriteString("  $vbs = Join-Path $dir ($name + '.vbs')\n")
	b.WriteString("  $cmdEsc = $vbsCommand.Replace('\"', '\"\"')\n")
	b.WriteString("  $dirEsc = $workdir.Replace('\"', '\"\"')\n")
	b.WriteString("  $lines = @(\n")
	b.WriteString("    'Set sh = CreateObject(\"WScript.Shell\")',\n")
	b.WriteString("    'sh.CurrentDirectory = \"' + $dirEsc + '\"',\n")
	b.WriteString("    'sh.Run \"' + $cmdEsc + '\", 0, False'\n")
	b.WriteString("  )\n")
	b.WriteString("  Set-Content -Path $vbs -Value $lines -Encoding UTF8\n")
	b.WriteString("  $lnk = Join-Path $dir ($name + '.lnk')\n")
	b.WriteString("  $sc = $ws.CreateShortcut($lnk)\n")
	b.WriteString("  $sc.TargetPath = $wscript\n")
	b.WriteString("  $sc.Arguments = '//B //Nologo \"' + $vbs + '\"'\n")
	b.WriteString("  $sc.WorkingDirectory = $workdir\n")
	b.WriteString("  $sc.IconLocation = $icon + ',0'\n")
	b.WriteString("  $sc.Description = 'Start Tingly Box and open the web UI'\n")
	b.WriteString("  $sc.Save()\n")
	b.WriteString("  Write-Output $lnk\n")
	b.WriteString("}\n")
	return b.String()
}

// psQuote wraps a string as a PowerShell single-quoted literal, escaping single
// quotes by doubling them.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// ---------------- macOS ----------------

func createMacShortcuts(opts Options, spec LaunchSpec) ([]string, error) {
	content := commandScriptContent(spec.Argv)

	var targets []string
	if !opts.NoDesktop {
		if dir, err := userSubdir("Desktop"); err == nil {
			targets = append(targets, filepath.Join(dir, opts.Name+".command"))
		}
	}
	if !opts.NoMenu {
		if dir, err := userSubdir("Applications"); err == nil {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
				targets = append(targets, filepath.Join(dir, opts.Name+".command"))
			}
		}
	}

	var created []string
	for _, path := range targets {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return created, fmt.Errorf("failed to write shortcut %s: %w", path, err)
		}
		created = append(created, path)
	}
	return created, nil
}

// commandScriptContent builds a macOS .command shell script that launches the
// binary. Double-clicking a .command file runs it in Terminal.app, which by
// default leaves the window open showing "[Process completed]" after the
// script exits — the user has to close it by hand every time. We close it
// for them on success (identifying "this" window by its tty so we don't
// touch any other open Terminal window); on failure the window stays open
// so the error is visible instead of vanishing.
func commandScriptContent(argv []string) string {
	return fmt.Sprintf(`#!/bin/sh
%s
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
`, shJoin(argv))
}

// ---------------- Linux ----------------

func createLinuxShortcuts(opts Options, spec LaunchSpec) ([]string, error) {
	content := desktopEntryContent(opts.Name, spec.Argv)
	fileName := desktopFileName(opts.Name)

	var targets []string
	if !opts.NoMenu {
		if dir, err := userDataSubdir("applications"); err == nil {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
				targets = append(targets, filepath.Join(dir, fileName))
			}
		}
	}
	if !opts.NoDesktop {
		if dir, err := userSubdir("Desktop"); err == nil {
			if _, statErr := os.Stat(dir); statErr == nil {
				targets = append(targets, filepath.Join(dir, fileName))
			}
		}
	}

	var created []string
	for _, path := range targets {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return created, fmt.Errorf("failed to write shortcut %s: %w", path, err)
		}
		created = append(created, path)
	}
	return created, nil
}

// desktopEntryContent builds a freedesktop .desktop entry.
func desktopEntryContent(name string, argv []string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=Start Tingly Box and open the web UI
Exec=%s
Terminal=false
Categories=Utility;Network;
`, name, shJoin(argv))
}

func desktopFileName(name string) string {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return slug + ".desktop"
}

// ---------------- shared helpers ----------------

func userSubdir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, name), nil
}

func userDataSubdir(name string) (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, name), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", name), nil
}

// shQuote wraps a string as a POSIX single-quoted literal.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shQuote(a)
	}
	return strings.Join(quoted, " ")
}
