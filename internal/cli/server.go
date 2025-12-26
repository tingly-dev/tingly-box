package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/manager"
	"tingly-box/internal/server"
	"tingly-box/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

// stopServer stops the running server using the PID manager
func stopServer(pidManager *config.PIDManager) error {
	return stopServerWithPIDManager(pidManager)
}

// stopServerWithPIDManager handles the server stopping logic
func stopServerWithPIDManager(pidManager *config.PIDManager) error {
	// Get PID from manager
	pid, err := pidManager.GetPID()
	if err != nil {
		return fmt.Errorf("PID file does not exist or is invalid: %w", err)
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
		if !pidManager.IsRunning() {
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
	Host          string
	Port          int
	EnableUI      bool
	EnableDebug   bool
	EnableAdaptor bool
}

// startServer handles the server starting logic
func startServer(appConfig *config.AppConfig, opts startServerOptions) error {
	gin.SetMode(gin.ReleaseMode)

	var port int = opts.Port
	if port == 0 {
		port = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(port)
	}

	// Create PID manager
	pidManager := config.NewPIDManager(appConfig.ConfigDir())

	// Check if server is already running using PID manager
	if pidManager.IsRunning() {
		fmt.Printf("Server is already running on port %d\n", port)
		return nil
	}

	// Create PID file before starting server
	if err := pidManager.CreatePIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	serverManager := manager.NewServerManager(
		appConfig,
		manager.WithUI(opts.EnableUI),
		manager.WithAdaptor(opts.EnableAdaptor),
		manager.WithDebug(opts.EnableDebug),
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
	}

	// Wait for either server error, shutdown signal, or web UI stop request
	select {
	case err := <-serverErr:
		// Clean up PID file on error
		pidManager.RemovePIDFile()
		return fmt.Errorf("server stopped unexpectedly: %w", err)
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal, stopping server...")
		// Clean up PID file on shutdown
		pidManager.RemovePIDFile()
		return serverManager.Stop()
	case <-server.GetShutdownChannel():
		fmt.Println("\nReceived stop request from web UI, stopping server...")
		// Clean up PID file on shutdown
		pidManager.RemovePIDFile()
		return serverManager.Stop()
	}
}

// StartCommand represents the start server command
func StartCommand(appConfig *config.AppConfig) *cobra.Command {
	var port int
	var enableUI bool
	var enableDebug bool
	var host string
	var enableStyleTransform bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tingly Box server",
		Long: `Start the Tingly Box HTTP server that provides the unified API endpoint.
The server will handle request routing to configured AI providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startServer(appConfig, startServerOptions{
				Host:          host,
				Port:          port,
				EnableUI:      enableUI,
				EnableDebug:   enableDebug,
				EnableAdaptor: enableStyleTransform,
			})
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 12580, "Server port (default: 12580)")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().BoolVarP(&enableUI, "ui", "u", true, "Enable web UI (default: true)")
	cmd.Flags().BoolVar(&enableDebug, "debug", false, "Enable debug mode including gin, low level logging and so on (default: false)")
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
			pidManager := config.NewPIDManager(appConfig.ConfigDir())

			if !pidManager.IsRunning() {
				fmt.Println("Server is not running")
				return nil
			}

			fmt.Println("Stopping server...")
			if err := stopServerWithPIDManager(pidManager); err != nil {
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
			pidManager := config.NewPIDManager(appConfig.ConfigDir())
			serverRunning := pidManager.IsRunning()
			globalConfig := appConfig.GetGlobalConfig()

			fmt.Println("=== Tingly Box Status ===")
			fmt.Printf("Server Status: ")
			if serverRunning {
				fmt.Printf("Running\n")
				port := appConfig.GetServerPort()
				fmt.Printf("Port: %d\n", port)
				fmt.Printf("OpenAI Style API Endpoint: http://localhost:%d/openai/v1/chat/completions\n", port)
				fmt.Printf("Anthropic Style API Endpoint: http://localhost:%d/anthropic/v1/messages\n", port)
				fmt.Printf("Web UI: http://localhost:%d/dashboard\n", port)
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

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the Tingly Box server",
		Long: `Restart the running Tingly Box HTTP server.
This command will stop the current server (if running) and start a new instance.
The restart is graceful - ongoing requests will be completed before shutdown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := appConfig.SetServerPort(port); err != nil {
				return fmt.Errorf("failed to set server port: %w", err)
			}

			pidManager := config.NewPIDManager(appConfig.ConfigDir())
			wasRunning := pidManager.IsRunning()

			if wasRunning {
				fmt.Println("Stopping current server...")
				if err := stopServerWithPIDManager(pidManager); err != nil {
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
				Host:        host,
				Port:        port,
				EnableUI:    true,
				EnableDebug: true,
			})
		},
	}

	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().IntVarP(&port, "port", "p", 12580, "Server port (default: 12580)")
	return cmd
}
