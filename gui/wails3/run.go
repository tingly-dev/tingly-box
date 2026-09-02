package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	commandgui "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// acquireSingleInstanceLock ensures at most one tingly-box server instance
// (GUI or CLI) touches this config dir's server at a time.
//
// This exists alongside the port probe-and-release check in Start: that
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

// notifyRunningGUI asks an already-running GUI instance (same config dir,
// same port, same token) to show its main window, so launching the app a
// second time focuses the running instance instead of erroring out.
//
// It uses the in-process HTTP server's GUI-only /gui/open route (registered
// by TinglyService.ServiceStartup) rather than wails' SingleInstanceOptions:
// application.New is a process-wide singleton in wails3, so its built-in
// second-instance check cannot run before our error-app paths — and the HTTP
// nudge also distinguishes a running GUI (route exists → 204) from a running
// CLI server (route absent → 404) for free.
func notifyRunningGUI(appConfig *config.AppConfig) error {
	url := fmt.Sprintf("http://localhost:%d/gui/open", appConfig.GetServerPort())
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+appConfig.GetGlobalConfig().GetUserToken())

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status %d from running instance", resp.StatusCode)
	}
	return nil
}

// appLauncher implements the AppLauncher interface
type appLauncher struct{}

// NewAppLauncher creates a new AppLauncher instance
func NewAppLauncher() commandgui.AppLauncher {
	return &appLauncher{}
}

// Start launches the unified GUI application: in-process server + tray icon
// with hub panel + main app window.
func (l *appLauncher) Start(appManager *command.AppManager, opts options.StartServerOptions) error {
	log.Printf("Starting GUI with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Single-instance check FIRST: catches a running tingly-box (CLI/npx/GUI)
	// that the port probe below can't reliably tell apart from an unrelated
	// process on the same port. See acquireSingleInstanceLock's doc comment.
	// If the holder is another GUI instance, focus it and exit quietly
	// instead of showing an error.
	if _, err := acquireSingleInstanceLock(appManager); err != nil {
		if notifyErr := notifyRunningGUI(appManager.AppConfig()); notifyErr == nil {
			log.Printf("Another GUI instance is running; asked it to show its window")
			return nil
		}
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

	// GUI mode should NOT auto-open browser (user uses the GUI window instead)
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

	// Create Wails app with ServerManager embedded
	app := newAppWithServerManager(appManager, serverManager, opts.EnableDebug, application.ActivationPolicyRegular)

	// Main-window geometry persistence target (see windowstate.go).
	windowStatePath = filepath.Join(appManager.AppConfig().ConfigDir(), windowStateFile)

	// Set up the tray icon + hub panel (must run after creating the app)
	useSystray(app, tinglyService)

	// Launching a desktop app should show its window: open the main window
	// at startup (first run maximised, later runs at the saved geometry).
	showMainWindow(app, tinglyService, "")

	// Clicking the dock icon while the window is hidden should bring it
	// back, like any regular macOS app. (The window is hidden, not closed,
	// on close - see showMainWindow's WindowClosing hook.)
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		showMainWindow(app, tinglyService, "")
	})

	// Run the Wails app
	return app.Run()
}
