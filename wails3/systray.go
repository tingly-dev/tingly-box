package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

var (
	SystemTray           *application.SystemTray
	SystrayMenu          *application.Menu
	SystrayMenuDashboard *application.MenuItem
	SystrayMenuExit      *application.MenuItem
)

func useSystray(a *application.App) {
	// Create the SystemTray menu
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

	// Create SystemTray
	SystemTray = a.SystemTray.New().
		SetMenu(SystrayMenu).
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Support for template icons on macOS
	if runtime.GOOS == "darwin" {
		SystemTray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		// Support for light/dark mode icons
		SystemTray.SetDarkModeIcon(icons.SystrayDark)
		SystemTray.SetIcon(icons.SystrayLight)
	}

	SystemTray.AttachWindow(WindowMain).WindowOffset(5)
}
