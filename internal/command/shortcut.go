package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ShortcutCmdKong creates a desktop / start-menu shortcut that launches
// Tingly Box (restart in daemon mode and open the web UI) with a double-click,
// so users don't have to remember and type the startup command — especially on
// Windows.
type ShortcutCmdKong struct {
	Name      string `kong:"flag,name='name',default='Tingly Box',help='Shortcut name'"`
	NoDesktop bool   `kong:"flag,name='no-desktop',help='Do not create a desktop shortcut'"`
	NoMenu    bool   `kong:"flag,name='no-menu',help='Do not create a Start Menu / application menu entry'"`
}

// shortcutLaunchArgs are the CLI args the shortcut runs: restart the daemon and
// (since --browser defaults to true) open the web UI.
func shortcutLaunchArgs() []string {
	return []string{"restart", "--daemon"}
}

func (s *ShortcutCmdKong) Run(appManager *AppManager) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}

	created, err := createShortcuts(s, exePath, shortcutLaunchArgs())
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

// createShortcuts dispatches to the platform-specific implementation and
// returns the paths of the shortcuts it created.
func createShortcuts(s *ShortcutCmdKong, exePath string, args []string) ([]string, error) {
	switch runtime.GOOS {
	case "windows":
		return createWindowsShortcuts(s, exePath, args)
	case "darwin":
		return createMacShortcuts(s, exePath, args)
	default:
		return createLinuxShortcuts(s, exePath, args)
	}
}

// ---------------- Windows ----------------

func createWindowsShortcuts(s *ShortcutCmdKong, exePath string, args []string) ([]string, error) {
	workDir := filepath.Dir(exePath)
	script := windowsShortcutScript(s, exePath, args, workDir)

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
func windowsShortcutScript(s *ShortcutCmdKong, exePath string, args []string, workDir string) string {
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Stop'\n")
	b.WriteString("$ws = New-Object -ComObject WScript.Shell\n")
	b.WriteString(fmt.Sprintf("$target = %s\n", psQuote(exePath)))
	b.WriteString(fmt.Sprintf("$arguments = %s\n", psQuote(strings.Join(args, " "))))
	b.WriteString(fmt.Sprintf("$workdir = %s\n", psQuote(workDir)))
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

func createMacShortcuts(s *ShortcutCmdKong, exePath string, args []string) ([]string, error) {
	content := commandScriptContent(exePath, args)

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
func commandScriptContent(exePath string, args []string) string {
	return fmt.Sprintf("#!/bin/sh\nexec %s %s\n", shQuote(exePath), shJoin(args))
}

// ---------------- Linux ----------------

func createLinuxShortcuts(s *ShortcutCmdKong, exePath string, args []string) ([]string, error) {
	content := desktopEntryContent(s.Name, exePath, args)
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
func desktopEntryContent(name, exePath string, args []string) string {
	exec := shQuote(exePath)
	if joined := shJoin(args); joined != "" {
		exec += " " + joined
	}
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=Start Tingly Box and open the web UI
Exec=%s
Terminal=false
Categories=Utility;Network;
`, name, exec)
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
