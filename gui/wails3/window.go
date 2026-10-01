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

	// mainWindowMu serialises showMainWindow: it runs from application-event
	// and tray goroutines (startup, dock reopen, hub panel), and two callers
	// must not both see WindowMain == nil and create two windows.
	mainWindowMu sync.Mutex

	// windowStatePath is <configDir>/gui-state.json; set once in Start.
	windowStatePath string
)

// showMainWindow shows the main app window (the real app, as opposed to the
// hub panel), creating it on first use. path is where it should land; an
// empty path means "just show it" — the window is brought forward without
// navigating (dock reopen / "Open Tingly Box" shouldn't yank the user off the page
// they were on).
//
// Navigation deliberately avoids the EmitEvent-based SPA hop: events ride
// the wails IPC bridge, which has proven unreliable in these webviews — the
// same reason the hub panel's actions call the HTTP nudge instead of the
// bound method. A new window loads through /login/<token>?next=<path>
// (Login.tsx hard-reloads after auth); a warm window navigates straight to
// the path via SetURL, since it's already authenticated.
//
// Call it only once the app is running (see run.go): before app.Run(), Wails
// defers Show() and turns Maximise() into a start state that macOS applies
// before positioning the window.
func showMainWindow(app *application.App, tinglyService *services.TinglyService, path string) {
	mainWindowMu.Lock()
	defer mainWindowMu.Unlock()
	if WindowMain == nil {
		target := path
		if target == "" {
			target = RouteAgent
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
		// Always open maximised: the app's pages (rail + sidebar + rule
		// graphs) need the room, and a smaller saved frame cut content off.
		// The saved frame above is still applied first, so "restore down"
		// returns to the size the user last chose.
		WindowMain.Maximise()
		WindowMain.Focus()
		return
	}

	// Warm window: it already authenticated on its first load (token in
	// localStorage), so navigate straight to the SPA route — the asset
	// middleware's SPA fallback serves index.html and BrowserRouter picks up
	// the path. Routing through /login/<token> again also worked, but
	// flashed the login screen on every hub action; if auth was somehow
	// lost, ProtectedRoute redirects to login by itself. An empty path just
	// brings the window forward.
	if path != "" {
		WindowMain.SetURL(path)
	}
	WindowMain.Show()
	WindowMain.Focus()
}

// persistWindowStateOnChange saves the window's normal (un-maximised) frame to
// windowStatePath, debounced (move/resize fire in bursts while dragging).
// Nothing is saved while maximised, so the file keeps the last normal frame —
// the one "restore down" returns to on the next launch.
func persistWindowStateOnChange(w *application.WebviewWindow) {
	var mu sync.Mutex
	var timer *time.Timer

	save := func() {
		mu.Lock()
		defer mu.Unlock()
		if w.IsMaximised() {
			return
		}
		var state WindowState
		state.X, state.Y = w.Position()
		state.Width, state.Height = w.Size()
		if state.Width > 0 && state.Height > 0 {
			_ = saveWindowState(windowStatePath, state)
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
