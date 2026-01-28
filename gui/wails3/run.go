package main

import (
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	command2 "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/pkg/network"
	"github.com/wailsapp/wails/v3/pkg/application"
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
	SystemTray := app.SystemTray.New().
		SetMenu(menu).
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Use custom icon
	SystemTray.SetIcon(slimIcon)
}

// appLauncher implements the AppLauncher interface
type appLauncher struct{}

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

	// Convert RecordMode string to obs.RecordMode
	var recordMode obs.RecordMode
	if opts.RecordMode != "" {
		recordMode = obs.RecordMode(opts.RecordMode)
	}

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		command.WithUI(opts.EnableUI),
		command.WithAdaptor(opts.EnableAdaptor),
		command.WithDebug(opts.EnableDebug),
		command.WithOpenBrowser(opts.EnableOpenBrowser),
		command.WithHost(opts.Host),
		command.WithHTTPSEnabled(opts.HTTPS.Enabled),
		command.WithHTTPSCertDir(opts.HTTPS.CertDir),
		command.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
		command.WithRecordMode(recordMode),
		command.WithRecordDir(opts.RecordDir),
		command.WithExperimentalFeatures(opts.ExperimentalFeatures),
	)

	// Create Wails app with ServerManager embedded
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// Note: Server is started by TinglyService.ServiceStartup() when the Wails app runs
	// No need to call serverManager.Start() here

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

	// Convert RecordMode string to obs.RecordMode
	var recordMode obs.RecordMode
	if opts.RecordMode != "" {
		recordMode = obs.RecordMode(opts.RecordMode)
	}

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		command.WithUI(opts.EnableUI),
		command.WithAdaptor(opts.EnableAdaptor),
		command.WithDebug(opts.EnableDebug),
		command.WithOpenBrowser(opts.EnableOpenBrowser),
		command.WithHost(opts.Host),
		command.WithHTTPSEnabled(opts.HTTPS.Enabled),
		command.WithHTTPSCertDir(opts.HTTPS.CertDir),
		command.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
		command.WithRecordMode(recordMode),
		command.WithRecordDir(opts.RecordDir),
		command.WithExperimentalFeatures(opts.ExperimentalFeatures),
	)

	// Create slim Wails app with ServerManager embedded
	app := newSlimAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// Note: Server is started by TinglyService.ServiceStartup() when the Wails app runs
	// No need to call serverManager.Start() here

	// Run the Wails app
	return app.Run()
}

// NewAppLauncher creates a new AppLauncher instance
func NewAppLauncher() command2.AppLauncher {
	return &appLauncher{}
}

// newSlimAppWithServerManager creates a new slim GUI app with a pre-configured ServerManager
// This is in the main package because the slim package has its own main() and cannot be imported
func newSlimAppWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager, debug bool) *application.App {
	// Create UI service with existing serverManager
	tinglyService := services.NewTinglyServiceWithServerManager(appManager, serverManager)

	// Create a new Wails application for slim version (no embedded UI)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(&services.GreetService{}),
			application.NewService(tinglyService),
		},
		// No Assets handler - slim version opens browser instead
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			ActivationPolicy: application.ActivationPolicyAccessory, // Tray-only: no dock icon, no default window
		},
		Windows: application.WindowsOptions{},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "tingly-box.slim.single-instance",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// Just focus/notify - slim version has no window to restore
				log.Printf("Second instance launch detected: %v", data)
			},
			AdditionalData: map[string]string{
				"launchtime": time.Now().Local().String(),
			},
			ExitCode:      0,
			EncryptionKey: [32]byte([]byte("Ml!Zjj@Lfw#Wqq$Wxb%Mjy^&*()_+1234567890-=")[:32]),
		},
	})

	// IMPORTANT: Set up systray after creating the app
	useSlimSystray(app, tinglyService)

	return app
}
