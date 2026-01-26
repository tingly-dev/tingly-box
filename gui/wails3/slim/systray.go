package main

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed icons.icns
var icon []byte

var (
	SystemTray *application.SystemTray
)

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return fmt.Errorf("unsupported platform")
	}

	return exec.Command(cmd, args...).Start()
}

func useSlimSystray(app *application.App) {
	// Create the SystemTray menu
	menu := app.Menu.New()

	// Dashboard menu item
	_ = menu.
		Add("Dashboard").
		OnClick(func(ctx *application.Context) {
			url := fmt.Sprintf("http://localhost:%d/?token=%s",
				tinglyService.GetPort(),
				tinglyService.GetUserAuthToken())
			if err := openBrowser(url); err != nil {
				fmt.Printf("Failed to open browser: %v\n", err)
			}
		})

	menu.AddSeparator()

	// OpenAI menu item
	_ = menu.
		Add("OpenAI").
		OnClick(func(ctx *application.Context) {
			url := fmt.Sprintf("http://localhost:%d/use-openai?token=%s",
				tinglyService.GetPort(),
				tinglyService.GetUserAuthToken())
			if err := openBrowser(url); err != nil {
				fmt.Printf("Failed to open browser: %v\n", err)
			}
		})

	// Anthropic menu item
	_ = menu.
		Add("Anthropic").
		OnClick(func(ctx *application.Context) {
			url := fmt.Sprintf("http://localhost:%d/use-anthropic?token=%s",
				tinglyService.GetPort(),
				tinglyService.GetUserAuthToken())
			if err := openBrowser(url); err != nil {
				fmt.Printf("Failed to open browser: %v\n", err)
			}
		})

	// Claude Code menu item
	_ = menu.
		Add("Claude Code").
		OnClick(func(ctx *application.Context) {
			url := fmt.Sprintf("http://localhost:%d/use-claude-code?token=%s",
				tinglyService.GetPort(),
				tinglyService.GetUserAuthToken())
			if err := openBrowser(url); err != nil {
				fmt.Printf("Failed to open browser: %v\n", err)
			}
		})

	menu.AddSeparator()

	// Exit menu item
	_ = menu.
		Add("Exit").
		OnClick(func(ctx *application.Context) {
			app.Quit()
		})

	// Create SystemTray
	SystemTray = app.SystemTray.New().
		SetMenu(menu).
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Use custom icon
	SystemTray.SetIcon(icon)
}
