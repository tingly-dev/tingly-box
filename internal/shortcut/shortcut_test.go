package shortcut

import (
	"strings"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs()
	if got := strings.Join(args, " "); got != "restart --daemon" {
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

func TestResolveLaunchBinary(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "binary", "1.4.2")

	if want := []string{"/usr/local/bin/tingly-box", "restart", "--daemon"}; strings.Join(spec.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected argv: %v", spec.Argv)
	}
	if spec.WinTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("unexpected winTarget: %q", spec.WinTarget)
	}
	if spec.WinArgs != "restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.WinArgs)
	}
}

func TestResolveLaunchEmptySourceDefaultsToBinary(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "", "1.4.2")

	if spec.WinTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("expected binary default, got winTarget=%q", spec.WinTarget)
	}
}

func TestResolveLaunchNpxPinsVersion(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "npx", "1.4.2")

	wantArgv := []string{"sh", "-lc", "npx -y tingly-box@1.4.2 restart --daemon"}
	if strings.Join(spec.Argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("unexpected argv: %#v", spec.Argv)
	}
	if spec.WinArgs != "/c npx -y tingly-box@1.4.2 restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.WinArgs)
	}
}

func TestResolveLaunchNpxBundlePinsVersion(t *testing.T) {
	spec := ResolveLaunch("/usr/local/bin/tingly-box", "npx-bundle", "1.4.2")

	if spec.WinArgs != "/c npx -y tingly-box-bundle@1.4.2 restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.WinArgs)
	}
}

func TestResolveLaunchNpxUnknownVersionFallsBackToLatest(t *testing.T) {
	for _, v := range []string{"", "dev", "unknown"} {
		spec := ResolveLaunch("/usr/local/bin/tingly-box", "npx", v)
		if spec.WinArgs != "/c npx -y tingly-box@latest restart --daemon" {
			t.Errorf("version=%q: unexpected winArgs: %q", v, spec.WinArgs)
		}
	}
}

func TestDesktopFileName(t *testing.T) {
	if got := desktopFileName("Tingly Box"); got != "tingly-box.desktop" {
		t.Fatalf("unexpected desktop file name: %q", got)
	}
}

func TestWindowsShortcutScriptHiddenLaunch(t *testing.T) {
	spec := LaunchSpec{
		WinTarget: `C:\Program Files\tingly-box\tingly-box.exe`,
		WinArgs:   "restart --daemon",
		WorkDir:   `C:\Program Files\tingly-box`,
	}
	script := windowsShortcutScript(Options{Name: "Tingly Box"}, spec)

	// The .lnk must target wscript.exe (hidden launch), not the binary/cmd
	// directly, so no console window ever appears.
	if !strings.Contains(script, `$sc.TargetPath = $wscript`) {
		t.Errorf("shortcut should target wscript.exe for a hidden launch:\n%s", script)
	}
	if !strings.Contains(script, `'sh.Run "' + $cmdEsc + '", 0, False'`) {
		t.Errorf("vbs helper should call WshShell.Run with window style 0 (hidden):\n%s", script)
	}
	if !strings.Contains(script, `//B //Nologo`) {
		t.Errorf("wscript should be invoked in batch/no-logo mode:\n%s", script)
	}
	if !strings.Contains(script, `$sc.IconLocation = $icon + ',0'`) {
		t.Errorf("shortcut should keep the original executable's icon:\n%s", script)
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
