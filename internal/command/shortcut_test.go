package command

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShortcutLaunchArgs(t *testing.T) {
	args := shortcutLaunchArgs()
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
	if !strings.Contains(content, "exec '/opt/tingly box/tingly-box' 'restart' '--daemon'") {
		t.Errorf("exec line not quoted as expected:\n%s", content)
	}
}

func TestResolveLaunchBinary(t *testing.T) {
	s := &ShortcutCmdKong{Target: "binary"}
	spec := s.resolveLaunch("/usr/local/bin/tingly-box", "")

	if want := []string{"/usr/local/bin/tingly-box", "restart", "--daemon"}; strings.Join(spec.argv, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected argv: %v", spec.argv)
	}
	if spec.winTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("unexpected winTarget: %q", spec.winTarget)
	}
	if spec.winArgs != "restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.winArgs)
	}
}

func TestResolveLaunchNpx(t *testing.T) {
	s := &ShortcutCmdKong{Target: "npx"}
	spec := s.resolveLaunch("/usr/local/bin/tingly-box", "")

	wantArgv := []string{"sh", "-lc", "npx -y tingly-box@latest restart --daemon"}
	if strings.Join(spec.argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("unexpected argv: %#v", spec.argv)
	}
	if spec.winArgs != "/c npx -y tingly-box@latest restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.winArgs)
	}
}

func TestResolveLaunchNpxBundle(t *testing.T) {
	s := &ShortcutCmdKong{Target: "npx-bundle"}
	spec := s.resolveLaunch("/usr/local/bin/tingly-box", "")

	if spec.winArgs != "/c npx -y tingly-box-bundle@latest restart --daemon" {
		t.Errorf("unexpected winArgs: %q", spec.winArgs)
	}
}

func TestResolveLaunchAutoUsesPersistedSource(t *testing.T) {
	s := &ShortcutCmdKong{Target: "auto"}
	// A binary not in the npx cache, but the recorded launch source was npx-bundle.
	spec := s.resolveLaunch("/usr/local/bin/tingly-box", "npx-bundle")

	if spec.winArgs != "/c npx -y tingly-box-bundle@latest restart --daemon" {
		t.Errorf("auto did not honor persisted source: %q", spec.winArgs)
	}
}

func TestResolveLaunchAutoFallsBackToBinary(t *testing.T) {
	s := &ShortcutCmdKong{Target: "auto"}
	spec := s.resolveLaunch("/usr/local/bin/tingly-box", "")

	if spec.winTarget != "/usr/local/bin/tingly-box" {
		t.Errorf("expected binary fallback, got winTarget=%q", spec.winTarget)
	}
}

func TestIsNpxCachedBinary(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/home/u/.cache")

	cached := filepath.Join("/home/u/.cache", "tingly-box", "latest", "bin", "tingly-box")
	if !isNpxCachedBinary(cached) {
		t.Errorf("expected %q to be detected as npx-cached", cached)
	}
	if isNpxCachedBinary("/usr/local/bin/tingly-box") {
		t.Errorf("did not expect /usr/local/bin/tingly-box to be npx-cached")
	}
}

func TestDesktopFileName(t *testing.T) {
	if got := desktopFileName("Tingly Box"); got != "tingly-box.desktop" {
		t.Fatalf("unexpected desktop file name: %q", got)
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
