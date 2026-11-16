package shortcut

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs()
	if got := strings.Join(args, " "); got != "open" {
		t.Fatalf("unexpected launch args: %q", got)
	}
}

func TestDesktopEntryContent(t *testing.T) {
	argv := []string{"/opt/tingly box/tingly-box", "restart", "--daemon"}
	content := desktopEntryContent("Tingly Box", argv)

	if !strings.Contains(content, "Name=Tingly Box") {
		t.Errorf("missing Name field:\n%s", content)
	}
	if !strings.Contains(content, "Exec='/opt/tingly box/tingly-box' 'restart' '--daemon'") {
		t.Errorf("Exec line not quoted as expected:\n%s", content)
	}
	if !strings.Contains(content, "Terminal=false") {
		t.Errorf("missing Terminal field:\n%s", content)
	}
}

func TestCommandScriptContent(t *testing.T) {
	argv := []string{"/opt/tingly box/tingly-box", "restart", "--daemon"}
	content := commandScriptContent(argv)

	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("missing shebang:\n%s", content)
	}
	if !strings.Contains(content, "'/opt/tingly box/tingly-box' 'restart' '--daemon'") {
		t.Errorf("command line not quoted as expected:\n%s", content)
	}
	if !strings.Contains(content, `tell application \"Terminal\" to close`) {
		t.Errorf("missing auto-close-on-success logic:\n%s", content)
	}
	if !strings.Contains(content, "Press Enter to close this window") {
		t.Errorf("missing keep-open-on-failure prompt:\n%s", content)
	}
}

func TestLauncherScriptContent(t *testing.T) {
	argv := []string{"/opt/tingly box/tingly-box", "restart", "--daemon"}
	content := launcherScriptContent(argv)

	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("missing shebang:\n%s", content)
	}
	if !strings.Contains(content, "exec '/opt/tingly box/tingly-box' 'restart' '--daemon'") {
		t.Errorf("exec line not quoted as expected:\n%s", content)
	}
}

func TestCreateLinuxShortcutsAlwaysWritesLauncherScript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")

	created, err := createLinuxShortcuts(Options{Name: "Tingly Box"}, spec)
	if err != nil {
		t.Fatalf("createLinuxShortcuts: %v", err)
	}

	// No ~/Desktop here, so: the menu .desktop entry plus the launcher script
	// — the latter written unconditionally, since it is the only artifact a
	// headless user can actually run and environment sniffing (DISPLAY etc.)
	// is too unreliable to condition on.
	wantEntry := filepath.Join(home, ".local", "share", "applications", "tingly-box.desktop")
	wantScript := filepath.Join(home, ".local", "bin", "tingly-box.sh")
	if len(created) != 2 || created[0] != wantEntry || created[1] != wantScript {
		t.Fatalf("expected [%s %s], got %v", wantEntry, wantScript, created)
	}

	info, err := os.Stat(wantScript)
	if err != nil {
		t.Fatalf("launcher script not written: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("launcher script is not executable: %v", info.Mode())
	}
	content, err := os.ReadFile(wantScript)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "exec '/usr/local/bin/tingly-box' 'open'") {
		t.Errorf("unexpected launcher script content:\n%s", content)
	}
}

func TestCreateLinuxShortcutsRespectsAllOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")

	created, err := createLinuxShortcuts(Options{Name: "Tingly Box", NoDesktop: true, NoMenu: true}, spec)
	if err != nil || len(created) != 0 {
		t.Errorf("--no-desktop --no-menu should produce nothing, got %v (err=%v)", created, err)
	}
}

func TestResolveLaunchBinary(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")

	if want := []string{"/usr/local/bin/tingly-box", "open"}; strings.Join(spec.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected argv: %v", spec.Argv)
	}
	if spec.WinTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("unexpected winTarget: %q", spec.WinTarget)
	}
	if spec.WinArgs != "open" {
		t.Errorf("unexpected winArgs: %q", spec.WinArgs)
	}
}

func TestResolveLaunchEmptySourceDefaultsToBinary(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "", "1.4.2")

	if spec.WinTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("expected binary default, got winTarget=%q", spec.WinTarget)
	}
}

func TestResolveLaunchNpmDoesNotPinVersion(t *testing.T) {
	for _, source := range []string{SourceNpx, SourceNpm} {
		for _, version := range []string{"1.4.2", "2.0.0", "", "dev", "unknown"} {
			spec := ResolveLaunch("/local/tingly box", source, version)
			want := `/d /c if exist "/local/tingly box" ("/local/tingly box" --source=` + source + ` open) else (npx --prefer-offline -y tingly-box open)`
			if spec.WinArgs != want {
				t.Errorf("source=%s version=%s: %s", source, version, spec.WinArgs)
			}
			if len(spec.Argv) != 3 || spec.Argv[0] != "sh" || spec.Argv[1] != "-lc" || strings.Contains(spec.Argv[2], "@") {
				t.Errorf("unexpected POSIX launch: %v", spec.Argv)
			}
		}
	}
}

func TestNpmLaunchLocalAndFallback(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires a POSIX shell")
	}
	dir := t.TempDir()
	local := filepath.Join(dir, "local ' executable")
	if err := os.WriteFile(local, []byte("#!/bin/sh\nprintf 'local:%s' \"$*\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	npx := filepath.Join(dir, "npx")
	if err := os.WriteFile(npx, []byte("#!/bin/sh\nprintf 'fallback:%s' \"$*\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	spec := ResolveLaunch(local, SourceNpx, "1.4.2")
	run := func(want string, wantError bool) {
		t.Helper()
		// Avoid the login shell resetting this test's isolated PATH.
		out, err := exec.Command("sh", "-c", spec.Argv[2]).CombinedOutput()
		if string(out) != want || (err != nil) != wantError {
			t.Fatalf("got %q, %v; want %q, error=%v", out, err, want, wantError)
		}
	}
	run("local:--source=npx open", false)
	// A local launch error must remain visible, rather than fetching another version.
	if err := os.WriteFile(local, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	run("", true)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	run("fallback:--prefer-offline -y tingly-box open", false)
}

func TestSlugName(t *testing.T) {
	if got := slugName("Tingly Box"); got != "tingly-box" {
		t.Fatalf("unexpected slug: %q", got)
	}
}

func TestWindowsShortcutScript(t *testing.T) {
	spec := LaunchSpec{
		WinTarget: `C:\Program Files\tingly-box\tingly-box.exe`,
		WinArgs:   "open",
		WorkDir:   `C:\Program Files\tingly-box`,
	}
	script := windowsShortcutScript(Options{Name: "Tingly Box"}, spec)

	// The .lnk must target the real command directly — no generated helper
	// script launched with a hidden window. That shape (write a script, run
	// it via a host with window style 0) is a classic dropper pattern and
	// trips antivirus/SmartScreen heuristics; it's also written in VBScript,
	// which Windows is phasing out. Keep it simple and inspectable.
	if !strings.Contains(script, `$sc.TargetPath = $target`) {
		t.Errorf("shortcut should target the real command directly:\n%s", script)
	}
	if strings.Contains(script, "wscript") || strings.Contains(script, ".vbs") {
		t.Errorf("shortcut should not route through a generated hidden-launch script:\n%s", script)
	}
	if !strings.Contains(script, "$target = 'C:\\Program Files\\tingly-box\\tingly-box.exe'") {
		t.Errorf("missing quoted target:\n%s", script)
	}
	if !strings.Contains(script, "$sc.WindowStyle = 7") {
		t.Errorf("shortcut should start minimized (documented IShellLink property, not a hidden launch):\n%s", script)
	}
	if !strings.Contains(script, "[Environment]::GetFolderPath('Desktop')") {
		t.Errorf("missing Desktop destination:\n%s", script)
	}
	if !strings.Contains(script, "[Environment]::GetFolderPath('Programs')") {
		t.Errorf("missing Programs destination:\n%s", script)
	}
}

func TestWindowsShortcutScriptRespectsNoDesktopNoMenu(t *testing.T) {
	spec := LaunchSpec{WinTarget: `C:\tingly-box.exe`, WinArgs: "open", WorkDir: `C:\`}

	noDesktop := windowsShortcutScript(Options{Name: "Tingly Box", NoDesktop: true}, spec)
	if strings.Contains(noDesktop, "'Desktop'") {
		t.Errorf("--no-desktop should drop the Desktop destination:\n%s", noDesktop)
	}
	if !strings.Contains(noDesktop, "'Programs'") {
		t.Errorf("--no-desktop should still include Programs:\n%s", noDesktop)
	}

	noMenu := windowsShortcutScript(Options{Name: "Tingly Box", NoMenu: true}, spec)
	if strings.Contains(noMenu, "'Programs'") {
		t.Errorf("--no-menu should drop the Programs destination:\n%s", noMenu)
	}
}

func TestCreateMacShortcuts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")

	// NoMenu is a no-op on macOS: there's no Launchpad/Spotlight-indexed
	// equivalent for a plain .command script, so ~/Applications is never
	// written to regardless of the flag — only the Desktop copy exists.
	created, err := createMacShortcuts(Options{Name: "Tingly Box", NoMenu: true}, spec)
	if err != nil {
		t.Fatalf("createMacShortcuts: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected exactly one shortcut (Desktop only), got %v", created)
	}
	want := filepath.Join(home, "Desktop", "tingly-box.command")
	if created[0] != want {
		t.Errorf("expected space-free filename %q, got %q", want, created[0])
	}
	if _, err := os.Stat(filepath.Join(home, "Applications")); err == nil {
		t.Errorf("~/Applications should never be created")
	}

	if created, err := createMacShortcuts(Options{Name: "Tingly Box", NoDesktop: true}, spec); err != nil || len(created) != 0 {
		t.Errorf("--no-desktop should produce nothing, got %v (err=%v)", created, err)
	}
}

func TestExpectedPathsMatchesCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	if err := os.MkdirAll(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")
	opts := Options{Name: "Tingly Box"}

	expected := ExpectedPaths(opts)
	created, err := Create(opts, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if strings.Join(expected, "\x00") != strings.Join(created, "\x00") {
		t.Fatalf("ExpectedPaths %v does not match what Create wrote %v", expected, created)
	}
}

func TestExpectedPathsRespectsAllOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	if got := ExpectedPaths(Options{Name: "Tingly Box", NoDesktop: true, NoMenu: true}); len(got) != 0 {
		t.Errorf("--no-desktop --no-menu should expect nothing, got %v", got)
	}
}

func TestExpectedLinuxScriptPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".local", "bin", "tingly-box.sh")
	if got := ExpectedLinuxScriptPath(Options{Name: "Tingly Box"}); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if got := ExpectedLinuxScriptPath(Options{Name: "Tingly Box", NoDesktop: true, NoMenu: true}); got != "" {
		t.Errorf("--no-desktop --no-menu should expect no script, got %q", got)
	}
}

func TestResolveExePath(t *testing.T) {
	path, err := ResolveExePath()
	if err != nil {
		t.Fatalf("ResolveExePath: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

func TestPSQuote(t *testing.T) {
	if got := psQuote(`C:\it's\path`); got != `'C:\it''s\path'` {
		t.Fatalf("unexpected ps quote: %q", got)
	}
}

func TestSHQuote(t *testing.T) {
	if got := shQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("unexpected sh quote: %q", got)
	}
}
