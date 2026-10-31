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
	linuxLauncherTemplate   = template.Must(template.ParseFS(templateFS, "templates/linux_launcher.sh.tmpl"))
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
	SourceNpm       = "npm" // installed bin from `npm install -g tingly-box` (shim run outside npx)
	SourceNpxBundle = "npx-bundle"
	SourceNpmBundle = "npm-bundle" // installed bin from `npm install -g tingly-box-bundle`
)

// npmShimSource reports whether source reaches the Go binary through an npm
// shim, whatever the channel (npx or a global install) and package (cli or
// bundle). They all get the same treatment; the split values only keep the
// record truthful and preserve which npm package a shortcut should relaunch.
func npmShimSource(source string) bool {
	switch source {
	case SourceNpx, SourceNpm, SourceNpxBundle, SourceNpmBundle:
		return true
	}
	return false
}

// npxPackageForSource preserves the install's package without pinning a version.
func npxPackageForSource(source string) string {
	if strings.HasSuffix(source, "-bundle") {
		return "tingly-box-bundle"
	}
	return "tingly-box"
}

// LaunchArgs are the CLI args the shortcut runs. `open` matches what a
// double-click means: server running -> open the web UI; not running ->
// start it (daemonized by default) and open the UI. Rationale for moving off
// `restart --daemon`: .design/cli-entry-semantics.md.
func LaunchArgs() []string {
	return []string{"open"}
}

// ResolveExePath returns the running process's own executable path, with
// symlinks resolved (e.g. a Homebrew Cellar symlink) so shortcuts target the
// real binary. Shared by every caller that builds a LaunchSpec for the
// process's own executable — the CLI's `shortcut` command and the HTTP
// handler backing it both need exactly this.
func ResolveExePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	return exePath, nil
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

// ResolveLaunch prefers the current local executable. For npm installs, if
// that executable is removed (for example by cache cleanup), an unversioned
// npx invocation prefers cached packages before fetching from the registry.
// version is retained in the API for compatibility, but is never pinned.
func ResolveLaunch(exePath, source, _ string) LaunchSpec {
	args := LaunchArgs()

	if npmShimSource(source) {
		npxArgv := append([]string{"npx", "--prefer-offline", "-y", npxPackageForSource(source)}, args...)
		localArgs := append([]string{"--source=" + source}, args...)
		localCommand := shJoin(append([]string{exePath}, localArgs...))
		cmdStr := "if [ -x " + shJoin([]string{exePath}) + " ]; then " + localCommand +
			"; else " + shJoin(npxArgv) + "; fi"
		// Quote local paths for cmd.exe, including installations under Program Files.
		winCommand := `if exist "` + exePath + `" ("` + exePath + `" ` + strings.Join(localArgs, " ") +
			`) else (` + strings.Join(npxArgv, " ") + `)`
		home, _ := os.UserHomeDir()

		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = "cmd.exe"
		}

		return LaunchSpec{
			// Wrap in a login shell so GUI launches pick up node/npx on PATH.
			Argv:      []string{"sh", "-lc", cmdStr},
			WinTarget: comspec,
			WinArgs:   "/d /c " + winCommand,
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

// ExpectedPaths returns the paths Create would write to for opts on this
// platform, without touching disk — used to answer "does a shortcut already
// exist" (a plain os.Stat per path) so the UI can show existing state instead
// of always presenting a bare "Create" action with no memory.
//
// Windows is best-effort here: it uses the plain (non-OneDrive-redirected)
// Desktop/Start Menu locations, unlike the real PowerShell/COM-driven Create,
// which resolves OneDrive redirection correctly. A false "not created" on a
// redirected Desktop is a harmless status miss — Create itself is idempotent,
// so clicking it again just re-writes to the right place.
func ExpectedPaths(opts Options) []string {
	name := slugName(opts.Name)
	var paths []string

	switch runtime.GOOS {
	case "windows":
		if !opts.NoDesktop {
			if home, err := os.UserHomeDir(); err == nil {
				paths = append(paths, filepath.Join(home, "Desktop", name+".lnk"))
			}
		}
		if !opts.NoMenu {
			if appData := os.Getenv("APPDATA"); appData != "" {
				paths = append(paths, filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", name+".lnk"))
			}
		}
	case "darwin":
		if !opts.NoDesktop {
			if dir, err := userSubdir("Desktop"); err == nil {
				paths = append(paths, filepath.Join(dir, name+".command"))
			}
		}
	default:
		if !opts.NoMenu {
			if dir, err := userDataSubdir("applications"); err == nil {
				paths = append(paths, filepath.Join(dir, name+".desktop"))
			}
		}
		if !opts.NoDesktop {
			// Matches createLinuxShortcuts: the Desktop copy is only written
			// when ~/Desktop already exists (a headless box may not have
			// one), so it must only be "expected" under the same condition.
			if dir, err := userSubdir("Desktop"); err == nil {
				if _, statErr := os.Stat(dir); statErr == nil {
					paths = append(paths, filepath.Join(dir, name+".desktop"))
				}
			}
		}
		if script := ExpectedLinuxScriptPath(opts); script != "" {
			paths = append(paths, script)
		}
	}
	return paths
}

// ExpectedLinuxScriptPath returns the path of the headless launcher script
// Create would write on Linux, or "" when nothing would be written (both
// --no-desktop and --no-menu) or off-Linux.
func ExpectedLinuxScriptPath(opts Options) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return ""
	}
	if opts.NoDesktop && opts.NoMenu {
		return ""
	}
	dir, err := userSubdir(filepath.Join(".local", "bin"))
	if err != nil {
		return ""
	}
	return filepath.Join(dir, slugName(opts.Name)+".sh")
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
	for line := range strings.SplitSeq(string(out), "\n") {
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

	// A .desktop entry is only launchable from a graphical session, so on
	// Linux always also write a plain executable launcher script — on
	// headless boxes (servers, containers, SSH) it is the only artifact the
	// user can actually run, and detecting "headless" from the environment
	// (DISPLAY etc.) is unreliable enough that conditioning on it would just
	// make the command's output unpredictable.
	if !(opts.NoDesktop && opts.NoMenu) {
		path, err := writeLinuxLauncherScript(opts, spec)
		if err != nil {
			return created, err
		}
		if path != "" {
			created = append(created, path)
		}
	}
	return created, nil
}

// writeLinuxLauncherScript writes the executable launcher script to
// ~/.local/bin (commonly on PATH per the systemd file-hierarchy convention,
// and user-owned either way). An unresolvable home or an uncreatable
// directory skips the script rather than failing shortcut creation.
func writeLinuxLauncherScript(opts Options, spec LaunchSpec) (string, error) {
	dir, err := userSubdir(filepath.Join(".local", "bin"))
	if err != nil {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil
	}
	path := filepath.Join(dir, slugName(opts.Name)+".sh")
	if err := os.WriteFile(path, []byte(launcherScriptContent(spec.Argv)), 0o755); err != nil {
		return "", fmt.Errorf("failed to write shortcut %s: %w", path, err)
	}
	return path, nil
}

// desktopEntryContent renders internal/shortcut/templates/linux_desktop.desktop.tmpl,
// a freedesktop .desktop entry.
func desktopEntryContent(name string, argv []string) string {
	return render(linuxDesktopTemplate, struct{ Name, Exec string }{Name: name, Exec: shJoin(argv)})
}

// launcherScriptContent renders internal/shortcut/templates/linux_launcher.sh.tmpl,
// a plain shell script that exec's the launch command.
func launcherScriptContent(argv []string) string {
	return render(linuxLauncherTemplate, struct{ Command string }{Command: shJoin(argv)})
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
