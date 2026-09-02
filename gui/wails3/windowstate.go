package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowState is the persisted main-window geometry, stored as JSON at
// <configDir>/gui-state.json. A missing file means "first run": the window
// maximises once and starts persisting from there; afterwards the user's
// size/position always wins (no forced Maximise on show).
type WindowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

// minVisible is how many pixels of the saved frame must land on some screen
// (in both axes) for the saved position to be trusted. Below that the window
// could be effectively unreachable (e.g. saved on a monitor that was
// unplugged), so clampWindowState falls back to centering.
const minVisible = 50

// loadWindowState reads the saved state. (nil, nil) means no saved state
// (first run); a corrupt file is treated the same way rather than erroring —
// worst case is one extra maximised launch.
func loadWindowState(path string) *WindowState {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s WindowState
	if err := json.Unmarshal(data, &s); err != nil || s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	return &s
}

// saveWindowState writes the state via a temp-file rename so a crash mid-write
// can't leave a truncated JSON behind.
func saveWindowState(path string, s WindowState) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// clampWindowState validates a saved frame against the current screen layout:
// if at least minVisible px of the frame is visible on any screen's work
// area, the saved position is kept; otherwise the window is centered on the
// primary screen (size preserved, clamped to fit). screens must be non-empty;
// the first entry with IsPrimary (or index 0) is the centering target.
func clampWindowState(s WindowState, screens []*application.Screen) WindowState {
	for _, sc := range screens {
		wa := sc.WorkArea
		overlapX := min(s.X+s.Width, wa.X+wa.Width) - max(s.X, wa.X)
		overlapY := min(s.Y+s.Height, wa.Y+wa.Height) - max(s.Y, wa.Y)
		if overlapX >= minVisible && overlapY >= minVisible {
			return s
		}
	}

	primary := screens[0]
	for _, sc := range screens {
		if sc.IsPrimary {
			primary = sc
			break
		}
	}
	wa := primary.WorkArea
	out := s
	out.Width = min(s.Width, wa.Width)
	out.Height = min(s.Height, wa.Height)
	out.X = wa.X + (wa.Width-out.Width)/2
	out.Y = wa.Y + (wa.Height-out.Height)/2
	return out
}
