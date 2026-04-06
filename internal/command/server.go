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
	"github.com/spf13/cobra"
	obs2 "github.com/tingly-dev/tingly-box/pkg/obs"

	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/daemon"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

const (
	// URL templates for displaying to users
	webUITpl             = "%s://localhost:%d/"
	webUILoginTpl        = "%s://localhost:%d/login/%s"
	openAIEndpointTpl    = "%s://localhost:%d/tingly/openai/v1/chat/completions"
	anthropicEndpointTpl = "%s://localhost:%d/tingly/anthropic/v1/messages"
)

// BannerConfig holds configuration for banner display
type BannerConfig struct {
	Port         int
	Host         string
	EnableUI     bool
	GlobalConfig *serverconfig.Config
	IsDaemon     bool
	HTTPEnabled  bool
}

// printBanner prints the server access banner
func printBanner(cfg BannerConfig) {
	scheme := "http"
	if cfg.HTTPEnabled {
		scheme = "https"
	}

	if !cfg.EnableUI {
		// Resolve host for display
		resolvedHost := network.ResolveHost(cfg.Host)
		fmt.Printf("API endpoint: %s://%s:%d/v1/chat/completions\n", scheme, resolvedHost, cfg.Port)
		return
	}

	// Show all access URLs when UI is enabled
	fmt.Println("\n┌────────────────────────────────────────────────────────────────────┐")
	fmt.Println("                         Access Information                            ")
	fmt.Println("├────────────────────────────────────────────────────────────────────┤")
	if cfg.GlobalConfig.HasUserToken() {
		fmt.Printf("  Web UI:       %s\n", fmt.Sprintf(webUILoginTpl, scheme, cfg.Port, cfg.GlobalConfig.GetUserToken()))
	} else {
		fmt.Printf("  Web UI:       %s\n", fmt.Sprintf(webUITpl, scheme, cfg.Port))
	}
	fmt.Printf("  OpenAI API:   %s\n", fmt.Sprintf(openAIEndpointTpl, scheme, cfg.Port))
	fmt.Printf("  Anthropic API: %s\n", fmt.Sprintf(anthropicEndpointTpl, scheme, cfg.Port))

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

	// Handle daemon mode
	if opts.Daemon {
		// If not yet daemonized, fork and exit
		if !daemon.IsDaemonProcess() {
			// Resolve port for display
			port := opts.Port
			if port == 0 {
				port = appConfig.GetServerPort()
			}

			fmt.Printf("Starting daemon process...\n")
			fmt.Printf("Logging to: %s\n", logFile)
			fmt.Printf("Server starting on port %d...\n", port)

			// Show banner in parent process before forking
			printBanner(BannerConfig{
				Port:         port,
				Host:         opts.Host,
				EnableUI:     opts.EnableUI,
				GlobalConfig: appConfig.GetGlobalConfig(),
				IsDaemon:     true,
				HTTPEnabled:  opts.HTTPS.Enabled,
			})

			// Fork and detach
			if err := daemon.Daemonize(); err != nil {
				return fmt.Errorf("failed to daemonize: %w", err)
			}
			// Daemonize() calls os.Exit(0), so we never reach here
		}
	}

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
			HTTPEnabled:  opts.HTTPS.Enabled,
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
		server.WithHTTPSEnabled(opts.HTTPS.Enabled),
		server.WithHTTPSCertDir(opts.HTTPS.CertDir),
		server.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
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
		HTTPEnabled:  opts.HTTPS.Enabled,
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

// StartCommand represents the start server command
func StartCommand(appManager *AppManager) *cobra.Command {
	return StartCommandWithHook(appManager)
}

// StartCommandWithHook represents the start server command with setup hooks
// that run after the server manager is created and before the server starts.
func StartCommandWithHook(appManager *AppManager, hooks ...func(*ServerManager) error) *cobra.Command {
	var flags options.StartFlags
	var localConfigDir string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tingly Box server",
		Long: `Start the Tingly Box HTTP server that provides the unified API endpoint.
The server will handle request routing to configured AI providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve config directory from global and local flags
			resolvedConfigDir, err := resolveConfigDirFromCmd(cmd, appManager.AppConfig().ConfigDir())
			if err != nil {
				return err
			}

			// Use default app manager if config dir matches, otherwise create new one
			var targetManager *AppManager
			if resolvedConfigDir == appManager.AppConfig().ConfigDir() {
				targetManager = appManager
			} else {
				targetManager, err = CreateAppManagerForDir(resolvedConfigDir)
				if err != nil {
					return err
				}
			}

			opts := options.ResolveStartOptions(cmd, flags, targetManager.AppConfig())
			return startServerWithHook(targetManager, opts, hooks...)
		},
	}

	options.AddStartFlags(cmd, &flags)
	cmd.Flags().StringVar(&localConfigDir, "config-dir", "",
		"configuration directory (overrides global --config-dir)")
	return cmd
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

// StopCommand represents the stop server command
func StopCommand(appManager *AppManager) *cobra.Command {
	var localConfigDir string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Tingly Box server",
		Long: `Stop the running Tingly Box HTTP server gracefully.
All ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve config directory from global and local flags
			resolvedConfigDir, err := resolveConfigDirFromCmd(cmd, appManager.AppConfig().ConfigDir())
			if err != nil {
				return err
			}

			// Create file lock with resolved config dir
			fileLock := lock.NewFileLock(resolvedConfigDir)
			return doStopServerWithFileLock(fileLock)
		},
	}

	cmd.Flags().StringVar(&localConfigDir, "config-dir", "",
		"configuration directory (specify which server instance to stop, overrides global --config-dir)")

	return cmd
}

// resolveConfigDirFromCmd resolves the config directory from command flags.
// It checks for a local --config-dir flag and uses that if present, otherwise falls back to the provided default.
func resolveConfigDirFromCmd(cmd *cobra.Command, defaultConfigDir string) (string, error) {
	localConfigDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return "", fmt.Errorf("failed to get config-dir flag: %w", err)
	}

	if localConfigDir != "" {
		return localConfigDir, nil
	}

	return defaultConfigDir, nil
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

// StatusCommand represents the status command
func StatusCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check server status and configuration",
		Long: `Display the current status of the Tingly Box server and
show configuration information including number of providers and server port.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			providers := appManager.ListProviders()
			appConfig := appManager.AppConfig()
			fileLock := lock.NewFileLock(appConfig.ConfigDir())
			serverRunning := fileLock.IsLocked()
			globalConfig := appConfig.GetGlobalConfig()

			fmt.Println("=== Tingly Box Status ===")
			fmt.Printf("Server Status: ")
			if serverRunning {
				fmt.Printf("Running\n")
				port := appConfig.GetServerPort()
				scheme := "http"
				fmt.Printf("Port: %d\n", port)
				fmt.Printf("OpenAI Style API Endpoint: "+openAIEndpointTpl+"\n", scheme, port)
				fmt.Printf("Anthropic Style API Endpoint: "+anthropicEndpointTpl+"\n", scheme, port)
				if globalConfig.HasUserToken() {
					fmt.Printf("Web UI: "+webUILoginTpl+"\n", scheme, port, globalConfig.GetUserToken())
				} else {
					fmt.Printf("Web UI: "+webUITpl+"\n", scheme, port)
				}
			} else {
				fmt.Printf("Stopped\n")
			}

			fmt.Printf("\nAuthentication:\n")
			if globalConfig.HasModelToken() {
				fmt.Printf("  Model API Key: Configured (sk-tingly- format)\n")
			} else {
				fmt.Printf("  Model API Key: Not configured (will auto-generate on start)\n")
			}

			fmt.Printf("\nConfigured Providers: %d\n", len(providers))
			if len(providers) > 0 {
				fmt.Println("Providers:")
				for _, provider := range providers {
					status := "Disabled"
					if provider.Enabled {
						status = "Enabled"
					}
					fmt.Printf("  - %s (%s) [%s]: %s\n", provider.Name, provider.APIBase, provider.APIStyle, status)
				}
			}

			// Show rules
			cfg := appConfig.GetGlobalConfig()
			rules := cfg.Rules
			fmt.Printf("\nConfigured Rules: %d\n", len(rules))
			if len(rules) > 0 {
				fmt.Println("Rules:")
				for _, rule := range rules {
					status := "Inactive"
					if rule.Active {
						status = "Active"
					}
					serviceCount := len(rule.GetServices())
					fmt.Printf("  - %s -> %s: %s (%d services)\n", rule.RequestModel, rule.ResponseModel, status, serviceCount)
				}
			}

			return nil
		},
	}

	return cmd
}

// RestartCommand represents the restart server command
func RestartCommand(appManager *AppManager) *cobra.Command {
	var flags options.StartFlags
	var localConfigDir string

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the Tingly Box server",
		Long: `Restart the running Tingly Box HTTP server.
This command will stop the current server (if running) and start a new instance.
The restart is graceful - ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve config directory from global and local flags
			resolvedConfigDir, err := resolveConfigDirFromCmd(cmd, appManager.AppConfig().ConfigDir())
			if err != nil {
				return err
			}

			// Use default app manager if config dir matches, otherwise create new one
			var targetManager *AppManager
			if resolvedConfigDir == appManager.AppConfig().ConfigDir() {
				targetManager = appManager
			} else {
				targetManager, err = CreateAppManagerForDir(resolvedConfigDir)
				if err != nil {
					return err
				}
			}

			opts := options.ResolveStartOptions(cmd, flags, targetManager.AppConfig())

			// Use resolved config dir for file lock
			fileLock := lock.NewFileLock(resolvedConfigDir)
			wasRunning := fileLock.IsLocked()

			if wasRunning {
				fmt.Println("Stopping current server...")
				if err := stopServerWithFileLock(fileLock); err != nil {
					return fmt.Errorf("failed to stop server: %w", err)
				}
				fmt.Println("Server stopped successfully")

				// Give a moment for cleanup
				time.Sleep(1 * time.Second)
			} else {
				fmt.Println("Server was not running, starting it...")
			}

			// Start new server
			return startServer(targetManager, opts)
		},
	}

	options.AddStartFlags(cmd, &flags)
	cmd.Flags().StringVar(&localConfigDir, "config-dir", "",
		"configuration directory (overrides global --config-dir)")
	return cmd
}

// OpenCommand represents the open web UI command
func OpenCommand(appManager *AppManager) *cobra.Command {
	var flags options.StartFlags

	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open the Tingly Box web UI",
		Long: `Open the Tingly Box web UI in your default browser.
If the server is not running, it will be started first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := options.ResolveStartOptions(cmd, flags, appManager.AppConfig())
			appConfig := appManager.AppConfig()
			fileLock := lock.NewFileLock(appConfig.ConfigDir())

			if fileLock.IsLocked() {
				// Server is running, just open the browser
				port := appConfig.GetServerPort()
				globalConfig := appConfig.GetGlobalConfig()
				scheme := "http"
				if opts.HTTPS.Enabled {
					scheme = "https"
				}

				// Resolve host for display
				host := opts.Host
				if host == "" {
					host = "localhost"
				}
				resolvedHost := network.ResolveHost(host)

				// Build web UI URL
				webUIURL := fmt.Sprintf("%s://%s:%d/", scheme, resolvedHost, port)
				if globalConfig.HasUserToken() {
					webUIURL = fmt.Sprintf("%s://%s:%d/login/%s", scheme, resolvedHost, port, globalConfig.GetUserToken())
				}

				fmt.Printf("Opening web UI: %s\n", webUIURL)
				return openBrowserURL(webUIURL)
			}

			// Server is not running, start it
			fmt.Println("Server is not running, starting it...")
			return startServer(appManager, opts)
		},
	}

	options.AddStartFlags(cmd, &flags)
	return cmd
}
