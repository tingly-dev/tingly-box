package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"tingly-box/internal/config"
	"tingly-box/internal/manager"
	"tingly-box/internal/server"
	serverconfig "tingly-box/internal/server/config"
	"tingly-box/internal/util/daemon"
	"tingly-box/internal/util/lock"
	"tingly-box/internal/util/network"
)

const (
	// URL templates for displaying to users
	webUITpl             = "%s://localhost:%d/home"
	webUITokenTpl        = "%s://localhost:%d/home?token=%s"
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
	fmt.Println("\nYou can access the service at:")
	if cfg.GlobalConfig.HasUserToken() {
		fmt.Printf("  Web UI:       "+webUITokenTpl+"\n", scheme, cfg.Port, cfg.GlobalConfig.GetUserToken())
	} else {
		fmt.Printf("  Web UI:       "+webUITpl+"\n", scheme, cfg.Port)
	}
	fmt.Printf("  OpenAI API:   "+openAIEndpointTpl+"\n", scheme, cfg.Port)
	fmt.Printf("  Anthropic API: "+anthropicEndpointTpl+"\n", scheme, cfg.Port)

	if cfg.IsDaemon {
		fmt.Println("\nServer is running in background. Use 'tingly-box stop' to stop.")
	}
}

// stopServerWithFileLock is implemented in platform-specific files:
// - server_windows.go for Windows (uses process.Kill())
// - server_unix.go for Unix-like systems (uses SIGTERM/SIGKILL)
//
// resolveStartOptions resolves start options from CLI flags and config
// This is shared between start and restart commands
type startFlags struct {
	port                 int
	host                 string
	enableUI             bool
	enableDebug          bool
	enableOpenBrowser    bool
	enableStyleTransform bool
	daemon               bool
	logFile              string
	promptRestart        bool
	https                bool
	httpsCertDir         string
	httpsRegen           bool
}

// addStartFlags adds all start-related flags to a command
// This is shared between start and restart commands
func addStartFlags(cmd *cobra.Command, flags *startFlags) {
	cmd.Flags().IntVarP(&flags.port, "port", "p", 0, "Server port (default: from config or 12580)")
	cmd.Flags().StringVar(&flags.host, "host", "localhost", "Server host")
	cmd.Flags().BoolVarP(&flags.enableUI, "ui", "u", true, "Enable web UI (default: true)")
	cmd.Flags().BoolVar(&flags.enableDebug, "debug", false, "Enable debug mode including gin, low level logging and so on (default: false)")
	cmd.Flags().BoolVar(&flags.enableOpenBrowser, "browser", true, "Auto-open browser when server starts (default: true)")
	cmd.Flags().BoolVar(&flags.enableStyleTransform, "adapter", true, "Enable API style transformation (default: true)")
	cmd.Flags().BoolVar(&flags.daemon, "daemon", false, "Run as daemon in background (default: false)")
	cmd.Flags().StringVar(&flags.logFile, "log-file", "", "Log file path for daemon mode (default: ~/.tingly-box/tingly-box.log)")
	cmd.Flags().BoolVar(&flags.promptRestart, "prompt-restart", false, "Prompt to restart if server is already running (default: false)")
	cmd.Flags().BoolVar(&flags.https, "https", false, "Enable HTTPS mode with self-signed certificate (default: false)")
	cmd.Flags().StringVar(&flags.httpsCertDir, "https-cert-dir", "", "Certificate directory for HTTPS (default: ~/.tingly-box/certs/)")
	cmd.Flags().BoolVar(&flags.httpsRegen, "https-regen", false, "Regenerate HTTPS certificate (default: false)")
}

func resolveStartOptions(cmd *cobra.Command, flags startFlags, appConfig *config.AppConfig) startServerOptions {
	// Apply priority: CLI flag > Config > Default
	resolvedDebug := flags.enableDebug
	if !cmd.Flags().Changed("debug") {
		resolvedDebug = appConfig.GetDebug()
	}

	resolvedOpenBrowser := flags.enableOpenBrowser
	if !cmd.Flags().Changed("browser") {
		resolvedOpenBrowser = appConfig.GetOpenBrowser()
	}

	resolvedPort := flags.port
	if resolvedPort == 0 {
		resolvedPort = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(flags.port)
	}

	return startServerOptions{
		Host:              flags.host,
		Port:              resolvedPort,
		EnableUI:          flags.enableUI,
		EnableDebug:       resolvedDebug,
		EnableAdaptor:     flags.enableStyleTransform,
		EnableOpenBrowser: resolvedOpenBrowser,
		Daemon:            flags.daemon,
		LogFile:           flags.logFile,
		PromptRestart:     flags.promptRestart,
		HTTPS: struct {
			Enabled    bool
			CertDir    string
			Regenerate bool
		}{
			Enabled:    flags.https,
			CertDir:    flags.httpsCertDir,
			Regenerate: flags.httpsRegen,
		},
	}
}

// doStopServer stops the running server
func doStopServer(appConfig *config.AppConfig) error {
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

// startServerOptions contains options for starting the server
type startServerOptions struct {
	Host              string
	Port              int
	EnableUI          bool
	EnableDebug       bool
	EnableAdaptor     bool
	EnableOpenBrowser bool
	Daemon            bool
	LogFile           string
	PromptRestart     bool
	HTTPS             struct {
		Enabled    bool
		CertDir    string
		Regenerate bool
	}
}

// startServer handles the server starting logic
func startServer(appConfig *config.AppConfig, opts startServerOptions) error {
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

	// Create rotating log writer
	logWriter := daemon.NewLogger(daemon.DefaultLogRotationConfig(logFile))

	// Set up logrus to write to both stdout and file with rotation
	if opts.Daemon {
		// In daemon mode, only write to file ba
		logrus.SetOutput(logWriter)
	} else {
		// In non-daemon mode, write to both stdout and file
		multiWriter := io.MultiWriter(os.Stdout, logWriter)
		logrus.SetOutput(multiWriter)
	}

	logrus.Infof("Logging to file: %s (with rotation)", logFile)

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

	// Create file lock
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	// Check if server is already running using file lock
	if fileLock.IsLocked() {
		fmt.Printf("Server is already running on port %d\n", port)
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

	serverManager := manager.NewServerManager(
		appConfig,
		manager.WithUI(opts.EnableUI),
		manager.WithAdaptor(opts.EnableAdaptor),
		manager.WithDebug(opts.EnableDebug),
		manager.WithOpenBrowser(opts.EnableOpenBrowser),
		manager.WithHost(opts.Host),
		manager.WithHTTPSEnabled(opts.HTTPS.Enabled),
		manager.WithHTTPSCertDir(opts.HTTPS.CertDir),
		manager.WithHTTPSRegenerate(opts.HTTPS.Regenerate),
	)

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
func StartCommand(appConfig *config.AppConfig) *cobra.Command {
	var flags startFlags

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tingly Box server",
		Long: `Start the Tingly Box HTTP server that provides the unified API endpoint.
The server will handle request routing to configured AI providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := resolveStartOptions(cmd, flags, appConfig)
			return startServer(appConfig, opts)
		},
	}

	addStartFlags(cmd, &flags)
	return cmd
}

// StopCommand represents the stop server command
func StopCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Tingly Box server",
		Long: `Stop the running Tingly Box HTTP server gracefully.
All ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doStopServer(appConfig)
		},
	}

	return cmd
}

// StatusCommand represents the status command
func StatusCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check server status and configuration",
		Long: `Display the current status of the Tingly Box server and
show configuration information including number of providers and server port.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			providers := appConfig.ListProviders()
			fileLock := lock.NewFileLock(appConfig.ConfigDir())
			serverRunning := fileLock.IsLocked()
			globalConfig := appConfig.GetGlobalConfig()

			fmt.Println("=== Tingly Box Status ===")
			fmt.Printf("Server Status: ")
			if serverRunning {
				fmt.Printf("Running\n")
				port := appConfig.GetServerPort()
				fmt.Printf("Port: %d\n", port)
				fmt.Printf("OpenAI Style API Endpoint: "+openAIEndpointTpl+"\n", port)
				fmt.Printf("Anthropic Style API Endpoint: "+anthropicEndpointTpl+"\n", port)
				fmt.Printf("Web UI: "+webUITpl+"\n", port)
				if globalConfig.HasUserToken() {
					fmt.Printf("UI Management Key: %s\n", globalConfig.GetUserToken())
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
func RestartCommand(appConfig *config.AppConfig) *cobra.Command {
	var flags startFlags

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the Tingly Box server",
		Long: `Restart the running Tingly Box HTTP server.
This command will stop the current server (if running) and start a new instance.
The restart is graceful - ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := resolveStartOptions(cmd, flags, appConfig)

			fileLock := lock.NewFileLock(appConfig.ConfigDir())
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
			return startServer(appConfig, opts)
		},
	}

	addStartFlags(cmd, &flags)
	return cmd
}
