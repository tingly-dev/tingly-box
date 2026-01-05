package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tingly-box/internal/lock"

	"tingly-box/internal/config"
	"tingly-box/internal/manager"
	"tingly-box/internal/server"
	"tingly-box/internal/util"

	"github.com/spf13/cobra"
)

const (
	// URL templates for displaying to users
	webUITpl             = "http://localhost:%d/home"
	webUITokenTpl        = "http://localhost:%d/home?token=%s"
	openAIEndpointTpl    = "http://localhost:%d/tingly/openai/v1/chat/completions"
	anthropicEndpointTpl = "http://localhost:%d/tingly/anthropic/v1/messages"
)

// stopServer stops the running server using the file lock
func stopServer(fileLock *lock.FileLock) error {
	return stopServerWithFileLock(fileLock)
}

// stopServerWithFileLock handles the server stopping logic
func stopServerWithFileLock(fileLock *lock.FileLock) error {
	// Get PID from lock file
	pid, err := fileLock.GetPID()
	if err != nil {
		return fmt.Errorf("lock file does not exist or is invalid: %w", err)
	}

	// Find and signal the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send shutdown signal: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 30; i++ { // Wait up to 30 seconds
		if !fileLock.IsLocked() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	// If still running, force kill
	fmt.Println("Server didn't stop gracefully, force killing...")
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to force kill process: %w", err)
	}

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
}

// startServer handles the server starting logic
func startServer(appConfig *config.AppConfig, opts startServerOptions) error {
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
		globalConfig := appConfig.GetGlobalConfig()
		fmt.Printf("Server is already running on port %d\n", port)
		fmt.Println("\nYou can access the service at:")
		if globalConfig.HasUserToken() {
			fmt.Printf("  Web UI:       "+webUITokenTpl+"\n", port, globalConfig.GetUserToken())
		} else {
			fmt.Printf("  Web UI:       "+webUITpl+"\n", port)
		}
		fmt.Printf("  OpenAI API:   "+openAIEndpointTpl+"\n", port)
		fmt.Printf("  Anthropic API: "+anthropicEndpointTpl+"\n", port)
		fmt.Println("\nTip: Use 'tingly-box stop' to stop the running server first")
		return nil
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

	if !opts.EnableUI {
		// Resolve host for display
		resolvedHost := util.ResolveHost(opts.Host)
		fmt.Printf("API endpoint: http://%s:%d/v1/chat/completions\n", resolvedHost, port)
	} else {
		// Show all access URLs when UI is enabled
		globalConfig := appConfig.GetGlobalConfig()
		fmt.Println("\nYou can access the service at:")
		if globalConfig.HasUserToken() {
			fmt.Printf("  Web UI:       "+webUITokenTpl+"\n", port, globalConfig.GetUserToken())
		} else {
			fmt.Printf("  Web UI:       "+webUITpl+"\n", port)
		}
		fmt.Printf("  OpenAI API:   "+openAIEndpointTpl+"\n", port)
		fmt.Printf("  Anthropic API: "+anthropicEndpointTpl+"\n", port)
		fmt.Println("\nPress Ctrl+C to stop the server")
	}

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
	var port int
	var enableUI bool
	var enableDebug bool
	var enableOpenBrowser bool
	var host string
	var enableStyleTransform bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tingly Box server",
		Long: `Start the Tingly Box HTTP server that provides the unified API endpoint.
The server will handle request routing to configured AI providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Apply priority: CLI flag > Config > Default
			resolvedDebug := enableDebug
			if !cmd.Flags().Changed("debug") {
				resolvedDebug = appConfig.GetDebug()
			}

			resolvedOpenBrowser := enableOpenBrowser
			if !cmd.Flags().Changed("browser") {
				resolvedOpenBrowser = appConfig.GetOpenBrowser()
			}

			resolvedPort := port
			if resolvedPort == 0 {
				resolvedPort = appConfig.GetServerPort()
			} else {
				appConfig.SetServerPort(port)
			}

			return startServer(appConfig, startServerOptions{
				Host:              host,
				Port:              resolvedPort,
				EnableUI:          enableUI,
				EnableDebug:       resolvedDebug,
				EnableAdaptor:     enableStyleTransform,
				EnableOpenBrowser: resolvedOpenBrowser,
			})
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (default: from config or 12580)")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().BoolVarP(&enableUI, "ui", "u", true, "Enable web UI (default: true)")
	cmd.Flags().BoolVar(&enableDebug, "debug", false, "Enable debug mode including gin, low level logging and so on (default: false)")
	cmd.Flags().BoolVar(&enableOpenBrowser, "browser", true, "Auto-open browser when server starts (default: true)")
	cmd.Flags().BoolVar(&enableStyleTransform, "adapter", true, "Enable API style transformation (default: true)")
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
	var port int
	var host string
	var debug bool
	var openBrowser bool

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the Tingly Box server",
		Long: `Restart the running Tingly Box HTTP server.
This command will stop the current server (if running) and start a new instance.
The restart is graceful - ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Apply priority: CLI flag > Config > Default
			resolvedDebug := debug
			if !cmd.Flags().Changed("debug") {
				resolvedDebug = appConfig.GetDebug()
			}

			resolvedOpenBrowser := openBrowser
			if !cmd.Flags().Changed("browser") {
				resolvedOpenBrowser = appConfig.GetOpenBrowser()
			}

			resolvedPort := port
			if resolvedPort == 0 {
				resolvedPort = appConfig.GetServerPort()
			} else {
				appConfig.SetServerPort(port)
			}

			if err := appConfig.SetServerPort(resolvedPort); err != nil {
				return fmt.Errorf("failed to set server port: %w", err)
			}

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

			// Start new server using non-blocking mode
			return startServer(appConfig, startServerOptions{
				Host:              host,
				Port:              resolvedPort,
				EnableUI:          true,
				EnableDebug:       resolvedDebug,
				EnableOpenBrowser: resolvedOpenBrowser,
			})
		},
	}

	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (default: from config or 12580)")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode (default: from config or false)")
	cmd.Flags().BoolVar(&openBrowser, "browser", true, "Auto-open browser when server starts (default: true)")
	return cmd
}
