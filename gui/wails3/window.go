package main

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	WindowMainName = "window-main"
)

var (
	WindowMain *application.WebviewWindow
	WindowSlim *application.WebviewWindow
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
func showMainWindow(app *application.App, tinglyService *services.TinglyService, path string) {
	if path == "" {
		path = "/agent"
	}

	if WindowMain == nil {
		WindowMain = app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:  WindowMainName,
			Title: AppName,
			Mac: application.MacWindow{
				Backdrop: application.MacBackdropTranslucent,
				TitleBar: application.MacTitleBarDefault,
			},
			BackgroundColour: application.NewRGB(27, 38, 54),
			URL:              fmt.Sprintf("/login/%s?next=%s", tinglyService.GetUserAuthToken(), path),
			Hidden:           true,
		})
		WindowMain.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			WindowMain.Hide()
		})
	} else {
		WindowMain.EmitEvent("systray-navigate", path)
	}

	WindowMain.Show()
	WindowMain.Maximise()
	WindowMain.Focus()
}
