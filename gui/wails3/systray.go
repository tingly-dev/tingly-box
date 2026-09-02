package main

import (
	_ "embed"
	"fmt"

	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed icons.icns
var icon []byte

var SystemTray *application.SystemTray

// hubWindowWidth/Height size the tray's hub panel (see
// frontend/src/pages/HubPage.tsx) — a narrow strip like a menu-bar dropdown
// rather than a small app window. The hub panel and the main app window
// (WindowMain — see window.go) are two distinct windows: the panel only ever
// renders /hub, the main window is the real app, opened on demand from the
// panel's Home/Dashboard actions or the tray's right-click menu.
const (
	hubWindowWidth  = 320
	hubWindowHeight = 560
)

func useSystray(app *application.App, tinglyService *services.TinglyService) {
	// Create the SystemTray menu - kept minimal since the hub panel itself
	// is the primary navigation surface now (see frontend HubPage.tsx).
	menu := app.Menu.New()

	_ = menu.
		Add("Show Hub").
		OnClick(func(ctx *application.Context) {
			SystemTray.ShowWindow()
		})

	_ = menu.
		Add("Open App").
		OnClick(func(ctx *application.Context) {
			showMainWindow(app, tinglyService, "")
		})

	menu.AddSeparator()

	// Exit menu item
	_ = menu.
		Add("Exit").
		OnClick(func(ctx *application.Context) {
			app.Quit()
		})

	// Create the hub panel - a small, dedicated window that only ever shows
	// /hub (never navigated elsewhere; "Home"/"Dashboard" on the hub page
	// open the separate main window instead - see the OpenMainWindow handler
	// below).
	//
	// Frameless + DisableResize are required for AttachWindow below: Wails
	// anchors the panel under the tray icon by reading the window's *current*
	// frame size at click time (see systemtray_darwin.m's positionWindow) -
	// resizing it after creation (our earlier Show+SetSize+Center approach)
	// made that anchor drift on every subsequent show.
	//
	// On macOS the panel is a real non-activating NSPanel
	// (MacWindowClassPanel + NonActivating): showing it never activates our
	// app or deactivates the frontmost one, exactly like 1Password/Bartender
	// dropdowns. That also makes HideOnFocusLost safe to use again - the old
	// spurious focus-loss (clicks on Home/Dashboard silently landing on a
	// window already mid-hide) was activation churn from showing a regular
	// AlwaysOnTop window, which a non-activating panel doesn't cause. So:
	// click outside → panel resigns key → hides, like a native dropdown.
	//
	// No BackgroundColour: the translucent backdrop shows until the webview
	// paints its own theme, which is neutral in both light and dark mode
	// (the old hardcoded dark RGB flashed wrong in light mode).
	WindowSlim = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "hub-panel",
		Title:         AppName,
		Width:         hubWindowWidth,
		Height:        hubWindowHeight,
		Frameless:     true,
		DisableResize: true,
		// AlwaysOnTop keeps the panel over other windows on Windows/Linux,
		// where it stays a plain frameless window; on macOS FloatingPanel
		// already supplies the floating window level.
		AlwaysOnTop:     true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		Mac: application.MacWindow{
			Backdrop:    application.MacBackdropTranslucent,
			WindowClass: application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				FloatingPanel: true,
				NonActivating: true,
			},
			// CanJoinAllSpaces + FullScreenAuxiliary lets the panel float
			// above a fullscreen app too, like Bartender/1Password's
			// menu-bar dropdown - without this, showing it while another
			// app owns the fullscreen Space would silently do nothing.
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
		// The Login page does a hard `window.location.href` reload after
		// auth (see Login.tsx). ?next=/hub tells it where to land so the
		// panel always ends up showing the hub, regardless of load timing.
		URL:    fmt.Sprintf("/login/%s?next=/hub", tinglyService.GetUserAuthToken()),
		Hidden: true,
	})

	// Create SystemTray and attach the hub panel: with a menu set and a
	// window attached but no explicit OnClick/OnRightClick, Wails' smart
	// defaults wire left-click to toggle+anchor the panel under the icon
	// (SystemTray.ToggleWindow) and right-click to open the menu.
	SystemTray = app.SystemTray.New().
		SetMenu(menu).
		AttachWindow(WindowSlim).
		WindowOffset(6)

	// Use custom icon
	SystemTray.SetIcon(icon)

	// Wire the hub panel's Home/Dashboard actions to open the main app
	// window: a direct bound method (TinglyService.OpenMainWindow) rather
	// than a fire-and-forget Events.Emit, so it's a single traceable call
	// instead of a pub/sub round trip. The same handler serves the
	// /api/v1/gui/open HTTP nudge used by the hub panel's action rows and by
	// a second GUI launch (see run.go's notifyRunningGUI).
	tinglyService.SetOpenMainWindowHandler(func(path string) {
		// Hide the panel first: it's AlwaysOnTop (needed to float above the
		// tray icon), so leaving it shown would sit visually on top of the
		// freshly-maximised main window, making the click look like a no-op.
		WindowSlim.Hide()
		showMainWindow(app, tinglyService, path)
	})

	// Prevent window from being destroyed on close - just hide it
	WindowSlim.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		WindowSlim.Hide()
	})
}
