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
// hub panel), creating it on first use. path is where it should land; an
// empty path means "just show it" — the window is brought forward without
// navigating (dock reopen / "Open App" shouldn't yank the user off the page
// they were on).
//
// Navigation always rides the /login/<token>?next=<path> reload — for a new
// window because Login.tsx hard-reloads after auth anyway, and for a warm
// window because the alternative (an EmitEvent the SPA listens for) goes
// through the wails IPC bridge, which has proven unreliable in these
// webviews — the same reason the hub panel's actions call the HTTP nudge
// instead of the bound method.
func showMainWindow(app *application.App, tinglyService *services.TinglyService, path string) {
	if WindowMain == nil {
		target := path
		if target == "" {
			target = "/agent"
		}

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
			// No BackgroundColour: the translucent backdrop shows until the
			// webview paints its own theme - neutral in light and dark mode.
			URL:    fmt.Sprintf("/login/%s?next=%s", tinglyService.GetUserAuthToken(), target),
			Hidden: true,
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

	// Warm window: navigate by reloading through the login shell (see the
	// doc comment above); an empty path just brings the window forward.
	if path != "" {
		WindowMain.SetURL(fmt.Sprintf("/login/%s?next=%s", tinglyService.GetUserAuthToken(), path))
	}
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
