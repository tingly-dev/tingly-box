package feature

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

// TestDirectoryButtonsCarryPaths pins the change that let the snapshot go:
// each directory button names the path it navigates to, rather than an index
// into a listing the server had to remember.
func TestDirectoryButtonsCarryPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	b := NewDirectoryBrowser()
	if _, err := b.StartAt("chat-1", root); err != nil {
		t.Fatal(err)
	}

	_, kb, _, err := b.BuildKeyboard("chat-1")
	if err != nil {
		t.Fatal(err)
	}

	var dirPayloads []imbot.Payload
	for _, row := range kb.Build().InlineKeyboard {
		for _, btn := range row {
			if btn.Payload.Name() == "bind" && btn.Payload.Arg(1) == "dir" {
				dirPayloads = append(dirPayloads, btn.Payload)
			}
		}
	}

	if len(dirPayloads) != 2 {
		t.Fatalf("expected 2 directory buttons, got %d", len(dirPayloads))
	}
	for _, p := range dirPayloads {
		target := p.Arg(2)
		if !filepath.IsAbs(target) {
			t.Errorf("button carries %q, want an absolute path", target)
		}
		if info, err := os.Stat(target); err != nil || !info.IsDir() {
			t.Errorf("button target %q is not a directory", target)
		}
	}
}

// TestNavigateAcceptsPathWithSeparator is what the NUL escape existed to fake.
// A directory whose name contains ":" is legal on Linux, and it now travels
// through a button unchanged.
func TestNavigateAcceptsPathWithSeparator(t *testing.T) {
	root := t.TempDir()
	odd := filepath.Join(root, "release:2026")
	if err := os.Mkdir(odd, 0o755); err != nil {
		t.Skipf("filesystem rejects ':' in names: %v", err)
	}

	b := NewDirectoryBrowser()
	if _, err := b.StartAt("chat-1", root); err != nil {
		t.Fatal(err)
	}
	_, kb, _, err := b.BuildKeyboard("chat-1")
	if err != nil {
		t.Fatal(err)
	}

	var target string
	for _, row := range kb.Build().InlineKeyboard {
		for _, btn := range row {
			if btn.Payload.Name() == "bind" && btn.Payload.Arg(1) == "dir" {
				target = btn.Payload.Arg(2)
			}
		}
	}
	if !strings.Contains(target, ":") {
		t.Fatalf("expected the colon to survive into the button, got %q", target)
	}
	if err := b.Navigate("chat-1", target); err != nil {
		t.Fatalf("navigating to %q failed: %v", target, err)
	}
	if got := b.GetCurrentPath("chat-1"); got != odd {
		t.Errorf("current path = %q, want %q", got, odd)
	}
}

// TestBuildCreateConfirmCarriesRawPath guards the flow that used to break
// outright: a long path made the confirmation button exceed Telegram's
// callback_data limit, which rejected the entire message.
func TestBuildCreateConfirmCarriesRawPath(t *testing.T) {
	path := "/home/somebody/projects/an-organisation/a-fairly-long-repository-name"
	kb, _ := BuildCreateConfirmKeyboard(path)

	for _, row := range kb.Build().InlineKeyboard {
		for _, btn := range row {
			if btn.Payload.Arg(1) == "create" {
				if got := btn.Payload.Arg(2); got != path {
					t.Errorf("payload path = %q, want %q", got, path)
				}
				return
			}
		}
	}
	t.Fatal("no create button was rendered")
}
