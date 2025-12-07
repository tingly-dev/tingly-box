package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	Systray              *application.SystemTray
	SystrayMenu          *application.Menu
	SystrayMenuDashboard *application.MenuItem
	SystrayMenuExit      *application.MenuItem
)

func useSystray(a *application.App) {
	// Create the systray menu
	SystrayMenu = a.Menu.New()

	// Dashboard menu item
	SystrayMenuDashboard = SystrayMenu.
		Add("Dashboard").
		OnClick(func(ctx *application.Context) {
			// Open dashboard URL - adjust this to your dashboard URL
			a.Browser.OpenURL("http://localhost:8080")
		})

	SystrayMenu.AddSeparator()

	// Exit menu item
	SystrayMenuExit = SystrayMenu.
		Add("Exit").
		OnClick(func(ctx *application.Context) {
			a.Quit()
		})

	// Create systray
	Systray = a.SystemTray.New().
		SetMenu(SystrayMenu).
		OnRightClick(func() {
			Systray.OpenMenu()
		})
}
