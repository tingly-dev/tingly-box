package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	WindowMainName = "window-main"
)

var (
	WindowMain *application.WebviewWindow
)

func useWindows(a *application.App) {

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	WindowMain = a.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                     WindowMainName,
		Title:                    AppName,
		MinWidth:                 1024,
		MinHeight:                640,
		Width:                    1024,
		Height:                   640,
		BackgroundColour:         application.NewRGB(27, 38, 54),
		ContentProtectionEnabled: true,
		URL:                      "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})
	WindowMain.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		WindowMain.Hide()
	})
}
