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
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed icons.icns
var slimIcon []byte

// acquireSingleInstanceLock ensures at most one tingly-box server instance
// (GUI, tray, slim, or CLI) touches this config dir's server at a time.
//
// This exists alongside the port probe-and-release check in Start*: that
// check only proves *some* process is reachable on the port, not that it's
// *this* config dir's server — a stale CLI/npx instance holding the port
// still lets a dial-based probe succeed, so a GUI launch can slip past the
// port check, render its window (which never touches the network — see
// app.go's in-process middleware), and only fail silently later when its own
// ListenAndServe loses the race. FileLock is the same PID/flock primitive
// the CLI already uses in server.go to detect a running instance, and unlike
// a TCP probe it can't be fooled by an unrelated listener answering on the
// same port.
func acquireSingleInstanceLock(appManager *command.AppManager) (*lock.FileLock, error) {
	fileLock := lock.NewFileLock(appManager.AppConfig().ConfigDir())
	if fileLock.IsLocked() {
		pid, _ := fileLock.GetPID()
		return nil, fmt.Errorf("Tingly Box is already running (pid %d).\n\nUse the running instance, or stop it first (e.g. `tingly-box stop`).", pid)
	}
	if err := fileLock.TryLock(); err != nil {
		return nil, fmt.Errorf("failed to acquire single-instance lock: %w", err)
	}
	return fileLock, nil
}

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
			url := fmt.Sprintf("http://localhost:%d/login/%s",
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
			url := fmt.Sprintf("http://localhost:%d/login/%s",
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
			url := fmt.Sprintf("http://localhost:%d/login/%s",
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
			url := fmt.Sprintf("http://localhost:%d/login/%s",
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
}

// hubWindowWidth/Height size the tray's compact "hub" landing page (see
// frontend/src/pages/HubPage.tsx) — small enough to read as a tray popover
// rather than the full app.
const (
	hubWindowWidth  = 480
	hubWindowHeight = 640
)

// showHubWindow shows the tray window at the compact hub size and navigates
// it to /hub. Any other navigation (Home, Dashboard, ...) maximises the
// window instead - see the "hub-left"/"hub-entered" event handlers below,
// which the frontend's useHubWindowMode hook drives on route change.
func showHubWindow() {
	WindowSlim.Show()
	WindowSlim.SetSize(hubWindowWidth, hubWindowHeight)
	WindowSlim.Center()
	WindowSlim.Focus()
	WindowSlim.EmitEvent("systray-navigate", "/hub")
}

func useWebSystray(app *application.App, tinglyService *services.TinglyService) {
	// Create the SystemTray menu - kept minimal since the hub page itself
	// is the primary navigation surface now (see frontend HubPage.tsx).
	menu := app.Menu.New()

	_ = menu.
		Add("Show Hub").
		OnClick(func(ctx *application.Context) {
			showHubWindow()
		})

	menu.AddSeparator()

	// Exit menu item
	_ = menu.
		Add("Exit").
		OnClick(func(ctx *application.Context) {
			app.Quit()
		})

	// Create SystemTray
	// Left-click opens the hub directly; right-click shows the (small) menu.
	SystemTray = app.SystemTray.New().
		SetMenu(menu).
		OnClick(func() {
			showHubWindow()
		}).
		OnRightClick(func() {
			SystemTray.OpenMenu()
		})

	// Use custom icon
	SystemTray.SetIcon(slimIcon)

	// Create a regular window (not attached to tray) - hidden by default,
	// starts at the compact hub size; shown via tray click.
	WindowSlim = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "menu-window",
		Title:  AppName,
		Width:  hubWindowWidth,
		Height: hubWindowHeight,
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarDefault,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		// The Login page does a hard `window.location.href` reload after
		// auth (see Login.tsx) — a plain "/login/:token" URL would lose the
		// EmitEvent("systray-navigate", "/hub") call below on first launch,
		// since that reload remounts the whole app past the point where the
		// event was received. ?next=/hub tells Login where to land instead.
		URL:    fmt.Sprintf("/login/%s?next=/hub", tinglyService.GetUserAuthToken()),
		Hidden: true,
	})

	// The frontend emits these when route navigation crosses the /hub
	// boundary (useHubWindowMode), so the window follows: compact for the
	// hub, full-size for every other page.
	app.Event.On("hub-left", func(event *application.CustomEvent) {
		WindowSlim.Maximise()
	})
	app.Event.On("hub-entered", func(event *application.CustomEvent) {
		WindowSlim.SetSize(hubWindowWidth, hubWindowHeight)
		WindowSlim.Center()
	})

	// Prevent window from being destroyed on close - just hide it
	WindowSlim.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		WindowSlim.Hide()
	})
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

	// Single-instance check FIRST: catches a running tingly-box (CLI/npx/GUI)
	// that the port probe below can't reliably tell apart from an unrelated
	// process on the same port. See acquireSingleInstanceLock's doc comment.
	if _, err := acquireSingleInstanceLock(appManager); err != nil {
		runErrorApp(err.Error())
		return err
	}

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

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithRecordDir(opts.RecordDir),
	)

	// Create Wails app with ServerManager embedded (full GUI: show dock icon)
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug, application.ActivationPolicyRegular)

	// IMPORTANT: Set up windows and systray after creating the app
	useWindows(app)
	useSystray(app)

	// Run the Wails app
	return app.Run()
}

// StartTray launches a systray only application with webui in menu
func (l *appLauncher) StartTray(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting tray GUI mode with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Single-instance check FIRST: see acquireSingleInstanceLock's doc comment.
	if _, err := acquireSingleInstanceLock(appManager); err != nil {
		runErrorApp(err.Error())
		return err
	}

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

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithRecordDir(opts.RecordDir),
	)

	// Create Wails app with ServerManager embedded (tray mode: keep dock icon like most apps)
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug, application.ActivationPolicyRegular)

	// IMPORTANT: Set up systray after creating the app
	useWebSystray(app, tinglyService)

	// Run the Wails app
	return app.Run()
}

// StartSlim launches the slim GUI application (systray only)
func (l *appLauncher) StartSlim(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting slim GUI mode with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Single-instance check FIRST: see acquireSingleInstanceLock's doc comment.
	if _, err := acquireSingleInstanceLock(appManager); err != nil {
		runErrorApp(err.Error())
		return err
	}

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

	// Create ServerManager with options
	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithUI(opts.EnableUI),
		server.WithDebug(opts.EnableDebug),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithRecordDir(opts.RecordDir),
	)

	// Create slim Wails app with ServerManager embedded
	app := newSlimAppWithServerManager(appManager, serverManager, opts.EnableDebug)

	// Note: Server is started by TinglyService.ServiceStartup() when the Wails app runs
	// No need to call serverManager.Start() here

	// Run the Wails app
	return app.Run()
}
