package command

import (
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
	content := desktopEntryContent("Tingly Box", "/opt/tingly box/tingly-box", shortcutLaunchArgs())

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
	content := commandScriptContent("/opt/tingly box/tingly-box", shortcutLaunchArgs())

	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("missing shebang:\n%s", content)
	}
	if !strings.Contains(content, "exec '/opt/tingly box/tingly-box' 'restart' '--daemon'") {
		t.Errorf("exec line not quoted as expected:\n%s", content)
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
