package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	WindowMainName = "window-main"

	// windowStateFile is the persisted main-window geometry, relative to the
	// config dir (see windowstate.go). Set as an absolute path in
	// windowStatePath by run.go's Start before any window exists.
	windowStateFile = "gui-state.json"
)

var (
	WindowMain *application.WebviewWindow
	WindowSlim *application.WebviewWindow

	// windowStatePath is <configDir>/gui-state.json; set once in Start.
	windowStatePath string
)

// showMainWindow shows the main app window (the real app, as opposed to the
// hub panel), creating it on first use. path is where it should land.
//
// Login.tsx does a hard reload after auth, so a *new* window can only be
// relied on to land where its initial URL's ?next= points - that's why
// this window is created lazily with path baked in, rather than eagerly
// with a fixed default and relying on EmitEvent to redirect it (the same
// race the hub panel avoids via its own ?next=/hub). Once created, the
// window is reused warm and EmitEvent-based navigation works normally.
//
// Geometry: the first-ever launch maximises; afterwards the window restores
// the user's last size/position (persisted debounced on move/resize - see
// persistWindowStateOnChange) instead of the old always-Maximise behavior.
func showMainWindow(app *application.App, tinglyService *services.TinglyService, path string) {
	if path == "" {
		path = "/agent"
	}

	if WindowMain == nil {
		saved := loadWindowState(windowStatePath)
		if saved != nil {
			if screens := app.Screen.GetAll(); len(screens) > 0 {
				clamped := clampWindowState(*saved, screens)
				saved = &clamped
			}
		}

		opts := application.WebviewWindowOptions{
			Name:  WindowMainName,
			Title: AppName,
			Mac: application.MacWindow{
				Backdrop: application.MacBackdropTranslucent,
				TitleBar: application.MacTitleBarDefault,
			},
			BackgroundColour: application.NewRGB(27, 38, 54),
			URL:              fmt.Sprintf("/login/%s?next=%s", tinglyService.GetUserAuthToken(), path),
			Hidden:           true,
		}
		if saved != nil {
			opts.InitialPosition = application.WindowXY
			opts.X, opts.Y = saved.X, saved.Y
			opts.Width, opts.Height = saved.Width, saved.Height
		}

		WindowMain = app.Window.NewWithOptions(opts)
		WindowMain.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			WindowMain.Hide()
		})
		persistWindowStateOnChange(WindowMain)

		WindowMain.Show()
		if saved == nil {
			// First run ever: no saved geometry to honor.
			WindowMain.Maximise()
		} else if saved.Maximised {
			WindowMain.Maximise()
		}
		WindowMain.Focus()
		return
	}

	WindowMain.EmitEvent("systray-navigate", path)
	WindowMain.Show()
	WindowMain.Focus()
}

// persistWindowStateOnChange saves the window's geometry to windowStatePath,
// debounced (move/resize fire in bursts while dragging). While maximised only
// the flag is updated, so un-maximising returns to the last normal frame.
func persistWindowStateOnChange(w *application.WebviewWindow) {
	var mu sync.Mutex
	var timer *time.Timer

	save := func() {
		mu.Lock()
		defer mu.Unlock()
		state := loadWindowState(windowStatePath)
		if state == nil {
			state = &WindowState{}
		}
		if w.IsMaximised() {
			state.Maximised = true
		} else {
			state.Maximised = false
			state.X, state.Y = w.Position()
			state.Width, state.Height = w.Size()
		}
		if state.Width > 0 && state.Height > 0 {
			_ = saveWindowState(windowStatePath, *state)
		}
	}

	debounced := func(event *application.WindowEvent) {
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(500*time.Millisecond, save)
		mu.Unlock()
	}

	w.RegisterHook(events.Common.WindowDidMove, debounced)
	w.RegisterHook(events.Common.WindowDidResize, debounced)
}
