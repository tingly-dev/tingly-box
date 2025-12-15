package main

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed assets/icon.icns
var icon []byte

var (
	SystemTray           *application.SystemTray
	SystrayMenu          *application.Menu
	SystrayMenuDashboard *application.MenuItem
	SystrayMenuExit      *application.MenuItem
)

func useSystray(app *application.App) {
	// Create the SystemTray menu
	SystrayMenu = app.Menu.New()

	// Home menu item
	SystrayMenuDashboard = SystrayMenu.
		Add("Home").
		OnClick(func(ctx *application.Context) {
			// Show main window
			WindowMain.Show()
		})

	SystrayMenu.AddSeparator()

	// Exit menu item
	SystrayMenuExit = SystrayMenu.
		Add("Exit").
		OnClick(func(ctx *application.Context) {
			app.Quit()
		})

	// Create SystemTray
	SystemTray = app.SystemTray.New().
		SetMenu(SystrayMenu).
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Use custom icon
	SystemTray.SetIcon(icon)

	//SystemTray.AttachWindow(WindowMain).WindowOffset(5)
}
