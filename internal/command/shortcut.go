package command

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
	sourceBinary    = "binary"
	sourceNpx       = "npx"
	sourceNpxBundle = "npx-bundle"
)

// npxPackageForSource returns the npm package an npx-based launch should run.
func npxPackageForSource(source string) string {
	if source == sourceNpxBundle {
		return "tingly-box-bundle@latest"
	}
	return "tingly-box@latest"
}

// isKnownSource reports whether source is one we recognize for persistence.
func isKnownSource(source string) bool {
	switch source {
	case sourceBinary, sourceNpx, sourceNpxBundle:
		return true
	default:
		return false
	}
}

// PersistLaunchSource records how tingly-box was launched (best-effort) so that
// `shortcut --source=auto` can generate a matching shortcut later. It is a no-op
// for empty/unknown/"auto" values and only writes when the value changes.
func PersistLaunchSource(appManager *AppManager, source string) {
	source = strings.TrimSpace(source)
	if source == "" || source == "auto" || !isKnownSource(source) {
		return
	}
	cfg := appManager.GetGlobalConfig()
	if cfg == nil || cfg.GetLaunchSource() == source {
		return
	}
	_ = cfg.SetLaunchSource(source)
}

// ShortcutCmdKong creates a desktop / start-menu shortcut that launches
// Tingly Box (restart in daemon mode and open the web UI) with a double-click,
// so users don't have to remember and type the startup command — especially on
// Windows.
type ShortcutCmdKong struct {
	Name      string `kong:"flag,name='name',default='Tingly Box',help='Shortcut name'"`
	Target    string `kong:"flag,name='target',enum='auto,binary,npx,npx-bundle',default='auto',help='What the shortcut runs: binary (this executable), npx, npx-bundle, or auto-detect from the recorded launch source'"`
	NoDesktop bool   `kong:"flag,name='no-desktop',help='Do not create a desktop shortcut'"`
	NoMenu    bool   `kong:"flag,name='no-menu',help='Do not create a Start Menu / application menu entry'"`
}

// shortcutLaunchArgs are the CLI args the shortcut runs: restart the daemon and
// (since --browser defaults to true) open the web UI.
func shortcutLaunchArgs() []string {
	return []string{"restart", "--daemon"}
}

// launchSpec describes how the shortcut should invoke Tingly Box on each
// platform. argv is the POSIX-style command vector used for macOS .command and
// Linux .desktop entries; winTarget/winArgs are the .lnk TargetPath/Arguments.
type launchSpec struct {
	argv      []string
	winTarget string
	winArgs   string
	workDir   string
}

func (s *ShortcutCmdKong) Run(appManager *AppManager) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}

	var persisted string
	if cfg := appManager.GetGlobalConfig(); cfg != nil {
		persisted = cfg.GetLaunchSource()
	}

	spec := s.resolveLaunch(exePath, persisted)

	created, err := createShortcuts(s, spec)
	if err != nil {
		return err
	}

	if len(created) == 0 {
		fmt.Println("Nothing to do (both --no-desktop and --no-menu were set).")
		return nil
	}

	fmt.Println("Created shortcut(s):")
	for _, p := range created {
		fmt.Printf("  - %s\n", p)
	}
	fmt.Println("\nDouble-click it to start Tingly Box and open the web UI.")
	return nil
}

// resolveLaunch decides whether the shortcut runs this binary directly or goes
// through npx, then builds the platform-specific launch vectors. When the source
// is "auto" it prefers the recorded launch source, then falls back to detecting
// the npx cache, and finally to the binary.
func (s *ShortcutCmdKong) resolveLaunch(exePath, persistedSource string) launchSpec {
	source := s.Target
	if source == "" || source == "auto" {
		switch {
		case isKnownSource(persistedSource):
			source = persistedSource
		case isNpxCachedBinary(exePath):
			source = sourceNpx
		default:
			source = sourceBinary
		}
	}

	args := shortcutLaunchArgs()

	if source == sourceNpx || source == sourceNpxBundle {
		// e.g. "npx -y tingly-box@latest restart --daemon"
		npxArgv := append([]string{"npx", "-y", npxPackageForSource(source)}, args...)
		cmdStr := strings.Join(npxArgv, " ")
		home, _ := os.UserHomeDir()

		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = "cmd.exe"
		}

		return launchSpec{
			// Wrap in a login shell so GUI launches pick up node/npx on PATH.
			argv:      []string{"sh", "-lc", cmdStr},
			winTarget: comspec,
			winArgs:   "/c " + cmdStr,
			workDir:   home,
		}
	}

	return launchSpec{
		argv:      append([]string{exePath}, args...),
		winTarget: exePath,
		winArgs:   strings.Join(args, " "),
		workDir:   filepath.Dir(exePath),
	}
}

// createShortcuts dispatches to the platform-specific implementation and
// returns the paths of the shortcuts it created.
func createShortcuts(s *ShortcutCmdKong, spec launchSpec) ([]string, error) {
	switch runtime.GOOS {
	case "windows":
		return createWindowsShortcuts(s, spec)
	case "darwin":
		return createMacShortcuts(s, spec)
	default:
		return createLinuxShortcuts(s, spec)
	}
}

// ---------------- Windows ----------------

func createWindowsShortcuts(s *ShortcutCmdKong, spec launchSpec) ([]string, error) {
	script := windowsShortcutScript(s, spec)

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

// windowsShortcutScript builds a PowerShell script that resolves the Desktop and
// Start Menu Programs folders at runtime (handling OneDrive redirection) and
// writes a .lnk via the WScript.Shell COM object. It prints each created path on
// its own line.
func windowsShortcutScript(s *ShortcutCmdKong, spec launchSpec) string {
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Stop'\n")
	b.WriteString("$ws = New-Object -ComObject WScript.Shell\n")
	b.WriteString(fmt.Sprintf("$target = %s\n", psQuote(spec.winTarget)))
	b.WriteString(fmt.Sprintf("$arguments = %s\n", psQuote(spec.winArgs)))
	b.WriteString(fmt.Sprintf("$workdir = %s\n", psQuote(spec.workDir)))
	b.WriteString(fmt.Sprintf("$name = %s\n", psQuote(s.Name)))
	b.WriteString("$dests = @()\n")
	if !s.NoDesktop {
		b.WriteString("$dests += [Environment]::GetFolderPath('Desktop')\n")
	}
	if !s.NoMenu {
		b.WriteString("$dests += [Environment]::GetFolderPath('Programs')\n")
	}
	b.WriteString("foreach ($dir in $dests) {\n")
	b.WriteString("  if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }\n")
	b.WriteString("  $lnk = Join-Path $dir ($name + '.lnk')\n")
	b.WriteString("  $sc = $ws.CreateShortcut($lnk)\n")
	b.WriteString("  $sc.TargetPath = $target\n")
	b.WriteString("  $sc.Arguments = $arguments\n")
	b.WriteString("  $sc.WorkingDirectory = $workdir\n")
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

func createMacShortcuts(s *ShortcutCmdKong, spec launchSpec) ([]string, error) {
	content := commandScriptContent(spec.argv)

	var targets []string
	if !s.NoDesktop {
		if dir, err := userSubdir("Desktop"); err == nil {
			targets = append(targets, filepath.Join(dir, s.Name+".command"))
		}
	}
	if !s.NoMenu {
		if dir, err := userSubdir("Applications"); err == nil {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
				targets = append(targets, filepath.Join(dir, s.Name+".command"))
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
// binary. Double-clicking a .command file runs it in Terminal.
func commandScriptContent(argv []string) string {
	return fmt.Sprintf("#!/bin/sh\nexec %s\n", shJoin(argv))
}

// ---------------- Linux ----------------

func createLinuxShortcuts(s *ShortcutCmdKong, spec launchSpec) ([]string, error) {
	content := desktopEntryContent(s.Name, spec.argv)
	fileName := desktopFileName(s.Name)

	var targets []string
	if !s.NoMenu {
		if dir, err := userDataSubdir("applications"); err == nil {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
				targets = append(targets, filepath.Join(dir, fileName))
			}
		}
	}
	if !s.NoDesktop {
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

// ---------------- npx detection ----------------

// npxCacheRoot mirrors the directory the npx wrapper (build/npx/tingly-box/bin.js)
// extracts the binary into: <os-cache-dir>/tingly-box. Returns "" if it cannot
// be determined.
func npxCacheRoot() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			if up := os.Getenv("USERPROFILE"); up != "" {
				base = filepath.Join(up, "AppData", "Local")
			}
		}
		if base == "" {
			return ""
		}
		return filepath.Join(base, "tingly-box")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, "Library", "Caches", "tingly-box")
	default:
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			return filepath.Join(xdg, "tingly-box")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".cache", "tingly-box")
	}
}

// isNpxCachedBinary reports whether exePath lives inside the npx cache directory,
// i.e. the binary was launched via `npx tingly-box`.
func isNpxCachedBinary(exePath string) bool {
	root := npxCacheRoot()
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, exePath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
