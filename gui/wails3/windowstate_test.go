package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func screen(x, y, w, h int, primary bool) *application.Screen {
	return &application.Screen{
		WorkArea:  application.Rect{X: x, Y: y, Width: w, Height: h},
		IsPrimary: primary,
	}
}

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui-state.json")

	if got := loadWindowState(path); got != nil {
		t.Fatalf("missing file: want nil, got %+v", got)
	}

	want := WindowState{X: 10, Y: 20, Width: 800, Height: 600, Maximised: true}
	if err := saveWindowState(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadWindowState(path)
	if got == nil || *got != want {
		t.Fatalf("round trip: want %+v, got %+v", want, got)
	}
}

func TestLoadWindowStateCorrupt(t *testing.T) {
	dir := t.TempDir()

	cases := map[string]string{
		"not-json":     "{oops",
		"zero-size":    `{"x":0,"y":0,"width":0,"height":0}`,
		"negative":     `{"x":0,"y":0,"width":-1,"height":600}`,
		"empty-object": `{}`,
	}
	for name, content := range cases {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := loadWindowState(path); got != nil {
			t.Errorf("%s: want nil, got %+v", name, got)
		}
	}
}

func TestClampWindowStateKeepsVisible(t *testing.T) {
	screens := []*application.Screen{screen(0, 0, 1920, 1080, true)}
	s := WindowState{X: 100, Y: 100, Width: 800, Height: 600}
	if got := clampWindowState(s, screens); got != s {
		t.Fatalf("visible frame changed: %+v", got)
	}

	// Partially off-screen but still >= minVisible in both axes: keep.
	s = WindowState{X: -700, Y: -500, Width: 800, Height: 600}
	if got := clampWindowState(s, screens); got != s {
		t.Fatalf("partially visible frame changed: %+v", got)
	}
}

func TestClampWindowStateCentersOffscreen(t *testing.T) {
	screens := []*application.Screen{
		screen(2000, 0, 1280, 720, false), // secondary
		screen(0, 0, 1920, 1080, true),    // primary
	}
	// Fully off every screen (e.g. saved on an unplugged monitor position).
	s := WindowState{X: -5000, Y: -5000, Width: 800, Height: 600}
	got := clampWindowState(s, screens)
	want := WindowState{X: (1920 - 800) / 2, Y: (1080 - 600) / 2, Width: 800, Height: 600}
	if got != want {
		t.Fatalf("want centered %+v, got %+v", want, got)
	}
}

func TestClampWindowStateShrinksOversized(t *testing.T) {
	screens := []*application.Screen{screen(0, 0, 1280, 720, true)}
	s := WindowState{X: -5000, Y: -5000, Width: 3000, Height: 2000}
	got := clampWindowState(s, screens)
	if got.Width != 1280 || got.Height != 720 || got.X != 0 || got.Y != 0 {
		t.Fatalf("oversized not clamped to work area: %+v", got)
	}
}
