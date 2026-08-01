// Package shortcut creates desktop / start-menu shortcuts that launch
// Tingly Box with a double-click. It is callable from the CLI today and from
// a future HTTP handler, so it has no Kong / cobra / command-layer dependency.
package shortcut

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var (
	windowsShortcutTemplate = template.Must(template.ParseFS(templateFS, "templates/windows_shortcut.ps1.tmpl"))
	macCommandTemplate      = template.Must(template.ParseFS(templateFS, "templates/macos_command.sh.tmpl"))
	linuxDesktopTemplate    = template.Must(template.ParseFS(templateFS, "templates/linux_desktop.desktop.tmpl"))
)

// render executes a parsed template against data and returns the result.
// Errors here would only come from a template/data mismatch, i.e. a coding
// mistake caught by the package's own tests — not something callers need to
// handle at runtime, so we panic like template.Must does for parsing.
func render(t *template.Template, data any) string {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		panic(fmt.Sprintf("shortcut: template %s: %v", t.Name(), err))
	}
	return b.String()
}

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

// windowsShortcutScript renders internal/shortcut/templates/windows_shortcut.ps1.tmpl,
// a PowerShell script that resolves the Desktop and Start Menu Programs
// folders at runtime (handling OneDrive redirection) and writes a .lnk via
// the WScript.Shell COM object. It points TargetPath directly at the real
// command (the binary, or cmd.exe for the npx case) — deliberately not
// through a generated script that launches something with a hidden window.
// That's the standard shape of a malware dropper (write a .vbs, run it via
// wscript with window style 0) and real antivirus/SmartScreen heuristics
// flag it; VBScript is also being phased out on Windows. WindowStyle=7
// (start minimized) is the one mitigation that stays inside IShellLink's own
// documented, unsuspicious surface — it won't stop `cmd /c` from lingering
// on a terminal host with "close on exit" set to never, but it keeps the
// window out of the way while it's up. It prints each created path on its
// own line.
func windowsShortcutScript(opts Options, spec LaunchSpec) string {
	return render(windowsShortcutTemplate, struct {
		Target, Arguments, WorkDir, Name string
		IncludeDesktop, IncludeMenu      bool
	}{
		Target:         psQuote(spec.WinTarget),
		Arguments:      psQuote(spec.WinArgs),
		WorkDir:        psQuote(spec.WorkDir),
		Name:           psQuote(slugName(opts.Name)),
		IncludeDesktop: !opts.NoDesktop,
		IncludeMenu:    !opts.NoMenu,
	})
}

// psQuote wraps a string as a PowerShell single-quoted literal, escaping single
// quotes by doubling them.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// ---------------- macOS ----------------

// createMacShortcuts only ever writes a Desktop shortcut. There's no macOS
// equivalent of a Start Menu for a plain .command script: Launchpad and
// Spotlight's "Applications" category only index real .app bundles (they
// read the bundle's Info.plist), so dropping a .command file into
// ~/Applications doesn't make it launchable from either — it would just be
// an inert, harder-to-find copy of the Desktop one. opts.NoMenu is a no-op
// here as a result (see .design/shortcut.md for the full writeup).
func createMacShortcuts(opts Options, spec LaunchSpec) ([]string, error) {
	if opts.NoDesktop {
		return nil, nil
	}

	content := commandScriptContent(spec.Argv)
	dir, err := userSubdir("Desktop")
	if err != nil {
		return nil, nil
	}
	path := filepath.Join(dir, slugName(opts.Name)+".command")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return nil, fmt.Errorf("failed to write shortcut %s: %w", path, err)
	}
	return []string{path}, nil
}

// commandScriptContent renders internal/shortcut/templates/macos_command.sh.tmpl,
// a macOS .command shell script that launches the binary. Double-clicking a
// .command file runs it in Terminal.app, which by default leaves the window
// open showing "[Process completed]" after the script exits — the user has
// to close it by hand every time. We close it for them on success
// (identifying "this" window by its tty so we don't touch any other open
// Terminal window); on failure the window stays open so the error is
// visible instead of vanishing.
func commandScriptContent(argv []string) string {
	return render(macCommandTemplate, struct{ Command string }{Command: shJoin(argv)})
}

// ---------------- Linux ----------------

func createLinuxShortcuts(opts Options, spec LaunchSpec) ([]string, error) {
	content := desktopEntryContent(opts.Name, spec.Argv)
	fileName := slugName(opts.Name) + ".desktop"

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

// desktopEntryContent renders internal/shortcut/templates/linux_desktop.desktop.tmpl,
// a freedesktop .desktop entry.
func desktopEntryContent(name string, argv []string) string {
	return render(linuxDesktopTemplate, struct{ Name, Exec string }{Name: name, Exec: shJoin(argv)})
}

// ---------------- shared helpers ----------------

// slugName converts a display name like "Tingly Box" into the space-free
// base filename every generated shortcut uses ("tingly-box"), regardless of
// platform. Filenames with spaces need extra quoting wherever they're
// referenced (shell commands, other generated scripts, path arguments) and
// are an easy source of subtle bugs; the display "Name" (used inside a
// .desktop entry's Name= field, or just passed via --name) is free to keep
// spaces since it's never used as a path component.
func slugName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

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
