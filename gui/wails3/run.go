package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	commandgui "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/appconfig"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/lock"
	"github.com/tingly-dev/tingly-box/internal/server"
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
//
// It runs before AppConfig is built and reads only ConfigDir, so losing the
// race never opens the running instance's database.
//
// A held lock is reported as lock.ErrLocked (wrapped), which is how Start tells
// "another instance is running" from any other failure, such as an unusable
// config directory: only the former has an instance to focus or take over.
func acquireSingleInstanceLock(appManager *app.AppManager) (*lock.FileLock, error) {
	fileLock := lock.NewFileLock(appManager.ConfigDir())
	if fileLock.IsLocked() {
		pid, _ := fileLock.GetPID()
		return nil, &alreadyRunningError{pid: pid}
	}
	if err := fileLock.TryLock(); err != nil {
		return nil, fmt.Errorf("failed to acquire single-instance lock: %w", err)
	}
	return fileLock, nil
}

// alreadyRunningError is the user-facing "already running" message; it unwraps
// to lock.ErrLocked so errors.Is identifies a lost race.
type alreadyRunningError struct{ pid int }

func (e *alreadyRunningError) Error() string {
	return fmt.Sprintf("Tingly Box is already running (pid %d).\n\nUse the running instance, or stop it first (e.g. `tingly-box stop`).", e.pid)
}

func (e *alreadyRunningError) Unwrap() error { return lock.ErrLocked }

// notifyRunningGUI asks an already-running GUI instance (same config dir,
// same port, same token) to show its main window, so launching the app a
// second time focuses the running instance instead of erroring out.
//
// It uses the in-process HTTP server's GUI-only /api/v1/gui/open route
// (registered by TinglyService.ServiceStartup) rather than wails'
// SingleInstanceOptions: application.New is a process-wide singleton in
// wails3, so its built-in second-instance check cannot run before our
// error-app paths — and the HTTP nudge also distinguishes a running GUI
// (route exists → 200) from a running CLI server (route absent → 404) for
// free.
//
// Like the CLI's `open` on a running server, it reads only the runtime port
// file and config.json, never the database the running instance owns.
func notifyRunningGUI(appManager *app.AppManager) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/gui/open", appManager.GetRuntimeServerPort())
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+appconfig.UserTokenFromFile(appManager.ConfigDir()))

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from running instance", resp.StatusCode)
	}
	return nil
}

// offerTakeover handles a launch that lost the single-instance race to a
// non-GUI server (CLI / npx / daemon): the window says exactly what is running
// (pid, port, version) and offers what `tingly-box restart` does — stop it and
// start again, here as the app — so GUI and CLI start/stop behave alike
// instead of dead-ending in an error. On confirm it stops the old server and
// relaunches this binary: wails3's application.New is a process-wide
// singleton, so the main app cannot be built after the notice app in-process.
func offerTakeover(appManager *app.AppManager, lockErr error) error {
	fileLock := lock.NewFileLock(appManager.ConfigDir())
	pid, _ := fileLock.GetPID()
	port := appManager.GetRuntimeServerPort()
	version, _ := fileLock.ReadVersion()
	if version == "" {
		version = "unknown version"
	}

	message := fmt.Sprintf("Tingly Box is already running in the background (pid %d, port %d, %s).\n\nIt was started from the command line, so it has no window. You can restart it as the app, or keep using the running instance.", pid, port, version)
	confirmed := runNoticeApp("Tingly Box Is Already Running", message, &noticeAction{
		Prompt: fmt.Sprintf("Stop the running instance (pid %d, port %d) and restart it as the app?\n\nIn-flight AI requests will be interrupted.", pid, port),
		Label:  "Restart as App",
	})
	if !confirmed {
		return lockErr
	}

	if err := command.StopRunningServer(appManager.ConfigDir()); err != nil {
		return fmt.Errorf("failed to stop the running instance (pid %d): %w", pid, err)
	}
	// Stopping returns once the lock is free, but give a force-killed process a
	// moment to drop it so the relaunch never loses the single-instance race
	// to the instance it just stopped.
	for deadline := time.Now().Add(5 * time.Second); fileLock.IsLocked(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			// No second error window: application.New is a singleton and the
			// notice app has already run, so report through the log + error.
			return fmt.Errorf("the previous instance (pid %d) did not release its lock", pid)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// A restart continues on the port the old server was actually using, like
	// `tingly-box restart` (see RestartCmdKong). Args are kept so flags such as
	// --host survive; an explicit --port the user passed still wins.
	args := append([]string(nil), os.Args[1:]...)
	if port > 0 && !hasPortFlag(args) {
		insertAt := len(args)
		for i, a := range args {
			if a == "--" {
				insertAt = i
				break
			}
		}
		args = append(args[:insertAt], append([]string{"--port", fmt.Sprint(port)}, args[insertAt:]...)...)
	}
	log.Printf("Stopped pid %d; relaunching GUI on port %d", pid, port)
	return exec.Command(exe, args...).Start()
}

func hasPortFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--port" || a == "-p" || strings.HasPrefix(a, "--port=") || strings.HasPrefix(a, "-p=") ||
			(len(a) > 2 && strings.HasPrefix(a, "-p") && a[2] >= '0' && a[2] <= '9') {
			return true
		}
	}
	return false
}

// appLauncher implements the AppLauncher interface
type appLauncher struct{}

// NewAppLauncher creates a new AppLauncher instance
func NewAppLauncher() commandgui.AppLauncher {
	return &appLauncher{}
}

// Start launches the unified GUI application: in-process server + tray icon
// with hub panel + main app window.
func (l *appLauncher) Start(appManager *app.AppManager, flags command.ServerFlagsKong) error {
	// Single-instance check FIRST: catches a running tingly-box (CLI/npx/GUI)
	// that the port probe below can't reliably tell apart from an unrelated
	// process on the same port. See acquireSingleInstanceLock's doc comment.
	// If the holder is another GUI instance, focus it and exit quietly
	// instead of showing an error.
	fileLock, err := acquireSingleInstanceLock(appManager)
	if err != nil {
		if !errors.Is(err, lock.ErrLocked) {
			// Not a lost race (e.g. the config directory cannot be created):
			// there is no other instance to focus or take over, so say what
			// actually went wrong instead of offering to stop something.
			log.Printf("Cannot start: %v", err)
			runNoticeApp("Tingly Box Could Not Start", err.Error(), nil)
			return err
		}
		if notifyErr := notifyRunningGUI(appManager); notifyErr == nil {
			log.Printf("Another GUI instance is running; asked it to show its window")
			return nil
		}
		return offerTakeover(appManager, err)
	}
	// Unlock also removes the runtime port/version files written below.
	defer fileLock.Unlock()

	// Only now, holding the lock, build AppConfig (via Resolve). A desktop
	// app serves localhost unless --host says otherwise (the CLI's empty
	// default binds every interface), and never opens a browser — the main
	// window is the UI — so the browser/daemon/log-file options stay zero.
	if flags.Host == "" {
		flags.Host = "localhost"
	}
	opts := flags.Resolve(appManager.AppConfig(), options.StartFlags{})
	log.Printf("Starting GUI with options: port=%d, host=%s, debug=%v", opts.Port, opts.Host, opts.EnableDebug)

	// Same bounded wait as the CLI's startServer: right after a stopped server
	// exits (restart, or the "Restart as App" takeover) the OS may not have
	// released its socket yet, and a single instant probe would fail spuriously.
	available, info := false, ""
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if available, info = network.IsPortAvailableWithInfo(opts.Host, opts.Port); available || time.Now().After(deadline) {
			break
		}
	}
	if !available {
		log.Printf("[Port Check] Port %d unavailable: %s", opts.Port, info)
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", opts.Port, info))
		return fmt.Errorf("port %d is already in use", opts.Port)
	}

	// The same runtime files the CLI server writes next to its lock
	// (.design/runtime-port-file.md): they are how a second GUI launch
	// (notifyRunningGUI) and CLI readers (`tb cc`, `tb log`, `tb open`, `tb
	// start`'s version hint) find this instance, since --port is never
	// persisted to config.json.
	if err := fileLock.WritePort(opts.Port); err != nil {
		log.Printf("Failed to record server port: %v", err)
	}
	if err := fileLock.WriteVersion(command.BuildVersion); err != nil {
		log.Printf("Failed to record server version: %v", err)
	}

	// Create ServerManager with options
	serverManager := app.NewServerManager(
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

	// Set up the tray icon + hub panel and the macOS menu bar (must run
	// after creating the app)
	openMain := useSystray(app, tinglyService)
	useAppMenu(app, openMain, opts.EnableDebug)

	// Launching a desktop app should show its window: open the main window
	// at startup, maximised (see showMainWindow) — once the app is running,
	// not before app.Run(). Before Run, Wails only records intent: Show() is
	// a no-op and Maximise() becomes StartState, which macOS applies by
	// zooming the window and THEN moving it to the saved X/Y, so a window
	// with a saved frame never opened maximised. After Run, the window is
	// created and positioned first, then shown and maximised — the same
	// path the tray's "Open App" has always taken.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		showMainWindow(app, tinglyService, "")
	})

	// Clicking the dock icon while the window is hidden should bring it
	// back, like any regular macOS app. (The window is hidden, not closed,
	// on close - see showMainWindow's WindowClosing hook.)
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		showMainWindow(app, tinglyService, "")
	})

	// Run the Wails app
	return app.Run()
}
