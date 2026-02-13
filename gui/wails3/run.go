package main

import (
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	commandgui "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/pkg/network"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed icons.icns
var slimIcon []byte

// openBrowser opens the default browser to the given URL
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

// useSlimSystray sets up the system tray for slim mode
func useSlimSystray(app *application.App, tinglyService *services.TinglyService) {
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
	SystemTray.SetIcon(slimIcon)

	//// Create a window similar to GUI mode but hidden by default
	//WindowSlim = app.Window.NewWithOptions(application.WebviewWindowOptions{
	//	Name:  "window-slim",
	//	Title: AppName,
	//	Mac: application.MacWindow{
	//		Backdrop: application.MacBackdropTranslucent,
	//		TitleBar: application.MacTitleBarDefault,
	//	},
	//	BackgroundColour: application.NewRGB(27, 38, 54),
	//	URL:              fmt.Sprintf("/?token=%s", tinglyService.GetUserAuthToken()),
	//	Hidden:           true, // Start hidden
	//})
	//
	//SystemTray.AttachWindow(WindowSlim)
}

func useWebSystray(app *application.App, tinglyService *services.TinglyService) {
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
				log.Printf("Failed to open browser: %v\n", err)
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
	SystemTray.SetIcon(slimIcon)

	// Create a window similar to GUI mode but hidden by default
	WindowSlim = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:  "menu-window",
		Title: AppName,
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarDefault,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              fmt.Sprintf("/?token=%s", tinglyService.GetUserAuthToken()),
		Hidden:           true, // Start hidden
	})

	SystemTray.AttachWindow(WindowSlim)
}

// appLauncher implements the AppLauncher interface
type appLauncher struct{}

// NewAppLauncher creates a new AppLauncher instance
func NewAppLauncher() commandgui.AppLauncher {
	return &appLauncher{}
}

// StartGUI launches the full GUI application
func (l *appLauncher) StartGUI(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting full GUI mode with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Check if port is available before starting the app
	available, info := network.IsPortAvailableWithInfo(opts.Host, opts.Port)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", opts.Port, available, info)

	if !available {
		// Create a minimal error-only app and run it (this will block until the user closes it)
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", opts.Port, info))
		return fmt.Errorf("port %d is already in use", opts.Port)
	}

	log.Printf("[Port Check] Port %d is available, starting application...", opts.Port)

	// IMPORTANT: GUI mode should NOT auto-open browser (user uses the GUI window instead)
	// Only CLI mode defaults to opening the browser
	opts.EnableOpenBrowser = false

	// Convert RecordMode string to obs.RecordMode
	var recordMode obs.RecordMode
	if opts.RecordMode != "" {
		recordMode = obs.RecordMode(opts.RecordMode)
	}

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithHTTPSEnabled(opts.HTTPS.Enabled),
		server.WithHTTPSCertDir(opts.HTTPS.CertDir),
		server.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
		server.WithRecordMode(recordMode),
		server.WithRecordDir(opts.RecordDir),
		server.WithExperimentalFeatures(opts.ExperimentalFeatures),
	)

	// Create Wails app with ServerManager embedded
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// IMPORTANT: Set up windows and systray after creating the app
	useWindows(app)
	useSystray(app)

	// Run the Wails app
	return app.Run()
}

// StartTray launches a systray only application with webui in menu
func (l *appLauncher) StartTray(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting tray GUI mode with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Check if port is available before starting the app
	available, info := network.IsPortAvailableWithInfo(opts.Host, opts.Port)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", opts.Port, available, info)

	if !available {
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", opts.Port, info))
		return fmt.Errorf("port %d is already in use", opts.Port)
	}

	log.Printf("[Port Check] Port %d is available, starting tray application...", opts.Port)

	// IMPORTANT: Tray mode should NOT auto-open browser (user opens via systray menu)
	// Only CLI mode defaults to opening the browser
	opts.EnableOpenBrowser = false

	// Convert RecordMode string to obs.RecordMode
	var recordMode obs.RecordMode
	if opts.RecordMode != "" {
		recordMode = obs.RecordMode(opts.RecordMode)
	}

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithHTTPSEnabled(opts.HTTPS.Enabled),
		server.WithHTTPSCertDir(opts.HTTPS.CertDir),
		server.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
		server.WithRecordMode(recordMode),
		server.WithRecordDir(opts.RecordDir),
		server.WithExperimentalFeatures(opts.ExperimentalFeatures),
	)

	// Create slim Wails app with ServerManager embedded
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// IMPORTANT: Set up systray after creating the app
	useWebSystray(app, tinglyService)

	// Run the Wails app
	return app.Run()
}

// StartSlim launches the slim GUI application (systray only)
func (l *appLauncher) StartSlim(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting slim GUI mode with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Check if port is available before starting the app
	available, info := network.IsPortAvailableWithInfo(opts.Host, opts.Port)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", opts.Port, available, info)

	if !available {
		// Create a minimal error-only app and run it (this will block until the user closes it)
		// For slim mode, we just use the same error app as full mode
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", opts.Port, info))
		return fmt.Errorf("port %d is already in use", opts.Port)
	}

	log.Printf("[Port Check] Port %d is available, starting slim application...", opts.Port)

	// IMPORTANT: Slim mode should NOT auto-open browser (user opens via systray menu)
	// Only CLI mode defaults to opening the browser
	opts.EnableOpenBrowser = false

	// Convert RecordMode string to obs.RecordMode
	var recordMode obs.RecordMode
	if opts.RecordMode != "" {
		recordMode = obs.RecordMode(opts.RecordMode)
	}

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithHTTPSEnabled(opts.HTTPS.Enabled),
		server.WithHTTPSCertDir(opts.HTTPS.CertDir),
		server.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
		server.WithRecordMode(recordMode),
		server.WithRecordDir(opts.RecordDir),
		server.WithExperimentalFeatures(opts.ExperimentalFeatures),
	)

	// Create slim Wails app with ServerManager embedded
	app := newSlimAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// Note: Server is started by TinglyService.ServiceStartup() when the Wails app runs
	// No need to call serverManager.Start() here

	// Run the Wails app
	return app.Run()
}
