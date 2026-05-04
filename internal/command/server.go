package command

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"github.com/sirupsen/logrus"
	obs2 "github.com/tingly-dev/tingly-box/pkg/obs"

	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

const (
	// URL templates for displaying to users
	webUITpl             = "http://localhost:%d/"
	webUILoginTpl        = "http://localhost:%d/login/%s"
	openAIEndpointTpl    = "http://localhost:%d/tingly/openai/v1/chat/completions"
	anthropicEndpointTpl = "http://localhost:%d/tingly/anthropic/v1/messages"
)

// BannerConfig holds configuration for banner display
type BannerConfig struct {
	Port         int
	Host         string
	EnableUI     bool
	GlobalConfig *serverconfig.Config
	IsDaemon     bool
}

// printBanner prints the server access banner
func printBanner(cfg BannerConfig) {
	if !cfg.EnableUI {
		// Resolve host for display
		resolvedHost := network.ResolveHost(cfg.Host)
		fmt.Printf("API endpoint: http://%s:%d/v1/chat/completions\n", resolvedHost, cfg.Port)
		return
	}

	// Show all access URLs when UI is enabled
	fmt.Println("\n┌────────────────────────────────────────────────────────────────────┐")
	fmt.Println("                         Access Information                            ")
	fmt.Println("├────────────────────────────────────────────────────────────────────┤")
	if cfg.GlobalConfig.HasUserToken() {
		fmt.Printf("  Web UI:       http://localhost:%d/login/%s\n", cfg.Port, cfg.GlobalConfig.GetUserToken())
	} else {
		fmt.Printf("  Web UI:       http://localhost:%d/\n", cfg.Port)
	}
	fmt.Printf("  OpenAI API:   http://localhost:%d/tingly/openai/v1/chat/completions\n", cfg.Port)
	fmt.Printf("  Anthropic API: http://localhost:%d/tingly/anthropic/v1/messages\n", cfg.Port)

	// Show login token for easy copy
	if cfg.GlobalConfig.HasUserToken() {
		fmt.Printf("\n  Login Token:  %s\n", cfg.GlobalConfig.GetUserToken())
	}
	fmt.Println("└────────────────────────────────────────────────────────────────────┘")

	if cfg.IsDaemon {
		fmt.Println("\nServer is running in background. Use 'tingly-box stop' to stop.")
	}
}

// openBrowserURL opens the given URL in the default browser
func openBrowserURL(url string) error {
	return browser.OpenURL(url)
}

// resolveStartOptions is implemented in platform-specific files:
// - server_windows.go for Windows (uses process.Kill())
// - server_unix.go for Unix-like systems (uses SIGTERM/SIGKILL)

func doStopServer(appManager *AppManager) error {
	appConfig := appManager.AppConfig()
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	if !fileLock.IsLocked() {
		fmt.Println("Server is not running")
		return nil
	}

	fmt.Println("Stopping server...")
	if err := stopServerWithFileLock(fileLock); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	fmt.Println("Server stopped successfully")
	return nil
}

// startServer handles the server starting logic
func startServer(appManager *AppManager, opts options.StartServerOptions) error {
	return startServerWithHook(appManager, opts)
}

// startServerWithHook handles the server starting logic with optional setup hooks.
func startServerWithHook(appManager *AppManager, opts options.StartServerOptions, hooks ...func(*ServerManager) error) error {
	appConfig := appManager.AppConfig()

	// Set logrus level based on debug flag
	if opts.EnableDebug {
		appConfig.SetDebug(true)
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Info("Debug mode enabled - detailed logging will be shown")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	// Determine log file path - always use log file with rotation
	logFile := opts.LogFile
	if logFile == "" {
		// Default to config directory
		logFile = appConfig.ConfigDir() + "/log/tingly-box.log"
	}

	// Create multi-mode logger (text + JSON)
	multiLoggerCfg := obs2.DefaultMultiLoggerConfig(appConfig.ConfigDir())
	multiLoggerCfg.TextLogPath = logFile
	multiLogger, err := obs2.NewMultiLogger(multiLoggerCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize multi-mode logger: %w", err)
	}

	// Sync multiLogger level with logrus level based on debug flag
	if opts.EnableDebug {
		multiLogger.SetLevel(logrus.DebugLevel)
	}

	// Set up logrus to write to both stdout and file with rotation
	if opts.Daemon {
		// In daemon mode, only write to file
		logrus.SetOutput(multiLogger)
	} else {
		// In non-daemon mode, write to both stdout and file
		multiWriter := io.MultiWriter(os.Stdout, multiLogger)
		logrus.SetOutput(multiWriter)
	}

	// Add hook for JSON logging
	logrus.AddHook(obs2.NewMultiLoggerHook(multiLogger, nil))

	logrus.Infof("Logging to file: %s (with rotation)", logFile)
	logrus.Infof("JSON logging to: %s (for frontend/API)", multiLoggerCfg.JSONLogPath)

	var port int = opts.Port
	if port == 0 {
		port = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(port)
	}

	// Check if port is available before proceeding
	if !network.IsPortAvailable(port) {
		return fmt.Errorf("port %d is already in use by another process", port)
	}

	// Create file lock
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	// Check if server is already running using file lock
	if fileLock.IsLocked() {
		logrus.Printf("Server is already running on port %d\n", port)
		printBanner(BannerConfig{
			Port:         port,
			Host:         opts.Host,
			EnableUI:     opts.EnableUI,
			GlobalConfig: appConfig.GetGlobalConfig(),
			IsDaemon:     false,
		})

		// If prompt-restart is enabled, ask user if they want to restart
		if opts.PromptRestart {
			fmt.Print("\nDo you want to restart the server? [y/N]: ")
			var response string
			fmt.Scanln(&response)

			// Check if user wants to restart
			if strings.ToLower(strings.TrimSpace(response)) == "y" || strings.ToLower(strings.TrimSpace(response)) == "yes" {
				fmt.Println("\nRestarting server...")
				// Stop the existing server first
				if err := stopServerWithFileLock(fileLock); err != nil {
					return fmt.Errorf("failed to stop existing server: %w", err)
				}
				// Give a moment for cleanup
				time.Sleep(1 * time.Second)
				// Continue to start the server (fall through to the rest of the function)
			} else {
				fmt.Println("\nRestart cancelled.")
				return nil
			}
		} else {
			fmt.Println("Tip: Use 'tingly-box restart' or 'npx tingly-box restart' to restart the server")
			fmt.Println("     Use 'tingly-box stop' or 'npx tingly-box stop' to stop it")
			return nil
		}
	}

	// Acquire lock before starting server
	if err := fileLock.TryLock(); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	fmt.Printf("Lock acquired: %s\n", fileLock.GetLockFilePath())

	serverManager := NewServerManager(
		appConfig,
		server.WithDebug(opts.EnableDebug),
		server.WithUI(opts.EnableUI),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithRecordMode(obs.RecordMode(opts.RecordMode)),
		server.WithRecordDir(opts.RecordDir),
		server.WithExperimentalFeatures(opts.ExperimentalFeatures),
		server.WithMultiLogger(multiLogger),
	)

	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(serverManager); err != nil {
			fileLock.Unlock()
			return err
		}
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine to keep it non-blocking
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serverManager.Start()
	}()

	fmt.Printf("Server starting on port %d...\n", port)

	printBanner(BannerConfig{
		Port:         port,
		Host:         opts.Host,
		EnableUI:     opts.EnableUI,
		GlobalConfig: appConfig.GetGlobalConfig(),
		IsDaemon:     false,
	})

	// Wait for either server error, shutdown signal, or web UI stop request
	select {
	case err := <-serverErr:
		// Release lock on error
		fileLock.Unlock()
		return fmt.Errorf("server stopped unexpectedly: %w", err)
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal, stopping server...")
		// Release lock on shutdown
		fileLock.Unlock()
		return serverManager.Stop()
	case <-server.GetShutdownChannel():
		fmt.Println("\nReceived stop request from web UI, stopping server...")
		// Release lock on shutdown
		fileLock.Unlock()
		return serverManager.Stop()
	}
}

// CreateAppManagerForDir creates a new AppManager for the specified config directory.
// This is used when a command specifies a different config directory than the global one.
func CreateAppManagerForDir(configDir string) (*AppManager, error) {
	// Create app config for the specified directory
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create app config for directory %s: %w", configDir, err)
	}
	return NewAppManagerWithConfig(appConfig), nil
}

// doStopServerWithFileLock stops the server using the provided file lock
func doStopServerWithFileLock(fileLock *lock.FileLock) error {
	if !fileLock.IsLocked() {
		fmt.Println("Server is not running")
		return nil
	}

	fmt.Println("Stopping server...")
	if err := stopServerWithFileLock(fileLock); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	fmt.Println("Server stopped successfully")
	return nil
}
