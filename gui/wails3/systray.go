package main

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed icons.icns
var icon []byte

var (
	SystemTray            *application.SystemTray
	SystrayMenu           *application.Menu
	SystrayMenuDashboard  *application.MenuItem
	SystrayMenuOpenAI     *application.MenuItem
	SystrayMenuAnthropic  *application.MenuItem
	SystrayMenuClaudeCode *application.MenuItem
	SystrayMenuExit       *application.MenuItem
)

// navigateToPath emits an event to navigate the main window to the given path
func navigateToPath(path string) {
	WindowMain.Show()
	WindowMain.Focus()
	// Emit event for frontend React Router to handle navigation
	WindowMain.EmitEvent("systray-navigate", path)
}

func useSystray(app *application.App) {
	// Create the SystemTray menu
	SystrayMenu = app.Menu.New()

	// Dashboard menu item - navigate to dashboard
	SystrayMenuDashboard = SystrayMenu.
		Add("Dashboard").
		OnClick(func(ctx *application.Context) {
			navigateToPath("/dashboard")
		})

	SystrayMenu.AddSeparator()

	// OpenAI menu item - navigate to OpenAI page
	SystrayMenuOpenAI = SystrayMenu.
		Add("OpenAI").
		OnClick(func(ctx *application.Context) {
			navigateToPath("/use-openai")
		})

	// Anthropic menu item - navigate to Anthropic page
	SystrayMenuAnthropic = SystrayMenu.
		Add("Anthropic").
		OnClick(func(ctx *application.Context) {
			navigateToPath("/use-anthropic")
		})

	// Claude Code menu item - navigate to Claude Code page
	SystrayMenuClaudeCode = SystrayMenu.
		Add("Claude Code").
		OnClick(func(ctx *application.Context) {
			navigateToPath("/use-claude-code")
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
		// Left-click: navigate to dashboard
		OnClick(func() {
			navigateToPath("/")
		}).
		// Right-click: show menu
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Use custom icon
	SystemTray.SetIcon(icon)

	//SystemTray.AttachWindow(WindowMain).WindowOffset(5)
}
