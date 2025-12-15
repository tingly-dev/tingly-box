package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"tingly-box/internal/server"

	"tingly-box/internal/config"

	"github.com/spf13/cobra"
)

// UICommand represents the ui command
func UICommand(appConfig *config.AppConfig) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Open the Tingly Box web UI",
		Long: `Open the Tingly Box web interface in your default browser.
If the server is not running, it will be started first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverManager := server.NewServerManager(appConfig)

			// Check if server is already running
			serverRunning := serverManager.IsRunning()

			if !serverRunning {
				fmt.Println("Server is not running, starting it...")

				// Set port if provided
				if port > 0 {
					if err := appConfig.SetServerPort(port); err != nil {
						return fmt.Errorf("failed to set server port: %w", err)
					}
				}

				// Start server in background
				go func() {
					newServerManager := server.NewServerManagerWithOptions(appConfig, true, false)
					if err := newServerManager.Start(); err != nil {
						fmt.Printf("Failed to start server: %v\n", err)
					}
				}()

				// Give server a moment to start
				fmt.Println("Waiting for server to start...")
			}

			// Get server port
			serverPort := appConfig.GetServerPort()
			if port > 0 {
				serverPort = port
			}

			// Construct web UI URL
			webUIURL := fmt.Sprintf("http://localhost:%d", serverPort)

			// Open browser
			if err := openBrowser(webUIURL); err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}

			if !serverRunning {
				fmt.Printf("Server started and web UI opened at %s\n", webUIURL)
			} else {
				fmt.Printf("Web UI opened at %s\n", webUIURL)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (default: 8080 or configured port)")
	return cmd
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)

	return exec.Command(cmd, args...).Start()
}
