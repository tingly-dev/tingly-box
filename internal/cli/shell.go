package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"tingly-box/internal/server"

	"tingly-box/internal/auth"
	"tingly-box/internal/config"
	"tingly-box/internal/memory"

	"github.com/spf13/cobra"
)

// ShellCommand represents the interactive CLI command
func ShellCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Enter shell interactive mode for managing Tingly Box",
		Long: `Enter an shell interactive command-line interface for managing Tingly Box.
This provides an easy-to-use menu system for all operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractiveMode(appConfig)
		},
	}

	return cmd
}

// runInteractiveMode starts the interactive CLI
func runInteractiveMode(appConfig *config.AppConfig) error {
	// Initialize memory logger
	logger, err := memory.NewMemoryLogger()
	if err != nil {
		fmt.Printf("Warning: Failed to initialize memory logger: %v\n", err)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		showMainMenu()
		fmt.Print("Select an option (1-9): ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("\n👋 Goodbye!")
				return nil
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			handleProviderManagement(appConfig, reader, logger)
		case "2":
			handleServerManagement(appConfig, reader, logger)
		case "3":
			handleViewProviders(appConfig)
		case "4":
			handleServerStatus(appConfig, logger)
		case "5":
			handleGenerateToken(appConfig, reader, logger)
		case "6":
			handleViewHistory(logger)
		case "7":
			handleSystemInfo(logger)
		case "8":
			handleGenerateExample(appConfig)
		case "9":
			fmt.Println("👋 Goodbye!")
			return nil
		default:
			fmt.Println("❌ Invalid choice. Please select 1-9.")
		}

		fmt.Println("\nPress Enter to continue...")
		_, _ = reader.ReadString('\n')
	}
}

// showMainMenu displays the main menu
func showMainMenu() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎯 Tingly Box - Interactive Management Console")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("1. 👥 Provider Management (Add/Delete)")
	fmt.Println("2. ⚡ Server Management (Start/Stop/Restart)")
	fmt.Println("3. 📋 View All Providers")
	fmt.Println("4. 📊 Server Status & Statistics")
	fmt.Println("5. 🔑 Generate Authentication Token")
	fmt.Println("6. 📜 View Operation History")
	fmt.Println("7. 💾 System Information")
	fmt.Println("8. 🚀 Generate API Example")
	fmt.Println("9. 🚪 Exit")
	fmt.Println(strings.Repeat("=", 60))
}

// handleProviderManagement handles provider management operations
func handleProviderManagement(appConfig *config.AppConfig, reader *bufio.Reader, logger *memory.MemoryLogger) {
	fmt.Println("\n👥 Provider Management")
	fmt.Println("1. Add Provider")
	fmt.Println("2. Delete Provider")
	fmt.Println("3. Back to Main Menu")

	fmt.Print("Select option: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	switch choice {
	case "1":
		addProviderInteractive(appConfig, reader, logger)
	case "2":
		deleteProviderInteractive(appConfig, reader, logger)
	case "3":
		return
	default:
		fmt.Println("❌ Invalid choice.")
	}
}

// addProviderInteractive adds a provider interactively
func addProviderInteractive(appConfig *config.AppConfig, reader *bufio.Reader, logger *memory.MemoryLogger) {
	fmt.Print("Enter provider name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(strings.TrimSuffix(name, "\n"))

	if name == "" {
		fmt.Println("❌ Provider name cannot be empty.")
		return
	}

	fmt.Print("Enter API base URL: ")
	apiBase, _ := reader.ReadString('\n')
	apiBase = strings.TrimSpace(strings.TrimSuffix(apiBase, "\n"))

	if apiBase == "" {
		fmt.Println("❌ API base URL cannot be empty.")
		return
	}

	fmt.Print("Enter API token: ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(strings.TrimSuffix(token, "\n"))

	if token == "" {
		fmt.Println("❌ API token cannot be empty.")
		return
	}

	// Ask for API style
	fmt.Println("\nSelect API style:")
	fmt.Println("1. openai - For OpenAI-compatible APIs")
	fmt.Println("2. anthropic - For Anthropic Claude API")
	fmt.Print("Enter choice (1-2, default: openai): ")

	styleInput, _ := reader.ReadString('\n')
	styleInput = strings.TrimSpace(strings.TrimSuffix(styleInput, "\n"))

	var apiStyle config.APIStyle = config.APIStyleOpenAI
	switch styleInput {
	case "2", "anthropic":
		apiStyle = config.APIStyleAnthropic
	case "1", "openai", "":
		apiStyle = config.APIStyleOpenAI
	default:
		fmt.Printf("Invalid choice '%s', using default: openai\n", styleInput)
		apiStyle = config.APIStyleOpenAI
	}

	if err := appConfig.AddProviderByName(name, apiBase, token); err != nil {
		fmt.Printf("❌ Failed to add provider: %v\n", err)
		if logger != nil {
			logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
				"name":     name,
				"api_base": apiBase,
			}, false, err.Error())
		}
		return
	}

	// Update the provider to set the API style
	if provider, err := appConfig.GetProvider(name); err == nil {
		provider.APIStyle = apiStyle
		// Save the configuration
		if saveErr := appConfig.Save(); saveErr != nil {
			fmt.Printf("Warning: failed to save API style configuration: %v\n", saveErr)
		}
	}

	fmt.Printf("✅ Provider '%s' added successfully with API style '%s'!\n", name, apiStyle)
	if logger != nil {
		logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
			"name":     name,
			"api_base": apiBase,
		}, true, "Provider added successfully")
	}
}

// deleteProviderInteractive deletes a provider interactively
func deleteProviderInteractive(appConfig *config.AppConfig, reader *bufio.Reader, logger *memory.MemoryLogger) {
	providers := appConfig.ListProviders()
	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return
	}

	fmt.Println("\n📋 Configured Providers:")
	for i, provider := range providers {
		fmt.Printf("%d. %s (%s)\n", i+1, provider.Name, provider.APIBase)
	}

	fmt.Print("Enter provider name or number to delete: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	var nameToDelete string
	if num, err := strconv.Atoi(choice); err == nil && num > 0 && num <= len(providers) {
		nameToDelete = providers[num-1].Name
	} else {
		nameToDelete = choice
	}

	fmt.Printf("Are you sure you want to delete provider '%s'? (y/N): ", nameToDelete)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.TrimSuffix(confirm, "\n"))

	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("❌ Deletion cancelled.")
		return
	}

	if err := appConfig.DeleteProvider(nameToDelete); err != nil {
		fmt.Printf("❌ Failed to delete provider: %v\n", err)
		if logger != nil {
			logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
				"name": nameToDelete,
			}, false, err.Error())
		}
		return
	}

	fmt.Printf("✅ Provider '%s' deleted successfully!\n", nameToDelete)
	if logger != nil {
		logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
			"name": nameToDelete,
		}, true, "Provider deleted successfully")
	}
}

// handleServerManagement handles server management operations
func handleServerManagement(appConfig *config.AppConfig, reader *bufio.Reader, logger *memory.MemoryLogger) {
	fmt.Println("\n⚡ Server Management")
	fmt.Println("1. Start Server")
	fmt.Println("2. Stop Server")
	fmt.Println("3. Restart Server")
	fmt.Println("4. Back to Main Menu")

	fmt.Print("Select option: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

	serverManager := server.NewServerManager(appConfig)

	switch choice {
	case "1":
		if serverManager.IsRunning() {
			fmt.Println("⚠️ Server is already running!")
			return
		}
		fmt.Print("Enter port (default 8080): ")
		portInput, _ := reader.ReadString('\n')
		portInput = strings.TrimSpace(strings.TrimSuffix(portInput, "\n"))

		port := 8080
		if portInput != "" {
			if p, err := strconv.Atoi(portInput); err == nil {
				port = p
			}
		}

		if err := serverManager.Start(); err != nil {
			fmt.Printf("❌ Failed to start server: %v\n", err)
			if logger != nil {
				logger.LogAction(memory.ActionStartServer, map[string]interface{}{
					"port": port,
				}, false, err.Error())
			}
			return
		}

		fmt.Printf("✅ Server started on port %d\n", port)
		if logger != nil {
			logger.LogAction(memory.ActionStartServer, map[string]interface{}{
				"port": port,
			}, true, "Server started successfully")
			logger.UpdateServerStatus(true, port, "0s", 0)
		}

	case "2":
		if !serverManager.IsRunning() {
			fmt.Println("⚠️ Server is not running!")
			return
		}

		if err := serverManager.Stop(); err != nil {
			fmt.Printf("❌ Failed to stop server: %v\n", err)
			if logger != nil {
				logger.LogAction(memory.ActionStopServer, nil, false, err.Error())
			}
			return
		}

		fmt.Println("✅ Server stopped successfully")
		if logger != nil {
			logger.LogAction(memory.ActionStopServer, nil, true, "Server stopped successfully")
			logger.UpdateServerStatus(false, 0, "", 0)
		}

	case "3":
		fmt.Print("Enter port for restart (default 8080): ")
		portInput, _ := reader.ReadString('\n')
		portInput = strings.TrimSpace(strings.TrimSuffix(portInput, "\n"))

		port := 8080
		if portInput != "" {
			if p, err := strconv.Atoi(portInput); err == nil {
				port = p
			}
		}

		wasRunning := serverManager.IsRunning()
		if wasRunning {
			serverManager.Cleanup()
			time.Sleep(1 * time.Second)
		}

		newServerManager := server.NewServerManager(appConfig)
		if err := newServerManager.Start(); err != nil {
			fmt.Printf("❌ Failed to restart server: %v\n", err)
			if logger != nil {
				logger.LogAction(memory.ActionRestartServer, map[string]interface{}{
					"port":        port,
					"was_running": wasRunning,
				}, false, err.Error())
			}
			return
		}

		fmt.Printf("✅ Server restarted on port %d\n", port)
		if logger != nil {
			logger.LogAction(memory.ActionRestartServer, map[string]interface{}{
				"port":        port,
				"was_running": wasRunning,
			}, true, "Server restarted successfully")
			logger.UpdateServerStatus(true, port, "0s", 0)
		}
	}
}

// handleViewProviders displays all providers
func handleViewProviders(appConfig *config.AppConfig) {
	providers := appConfig.ListProviders()

	fmt.Println("\n📋 All Configured Providers")
	fmt.Println(strings.Repeat("-", 50))

	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return
	}

	for i, provider := range providers {
		status := "❌ Disabled"
		if provider.Enabled {
			status = "✅ Enabled"
		}
		fmt.Printf("%d. %s\n", i+1, provider.Name)
		fmt.Printf("   URL: %s\n", provider.APIBase)
		fmt.Printf("   Status: %s\n", status)
		fmt.Println(strings.Repeat("-", 50))
	}
}

// handleServerStatus displays server status
func handleServerStatus(appConfig *config.AppConfig, logger *memory.MemoryLogger) {
	fmt.Println("\n📊 Server Status")
	fmt.Println(strings.Repeat("=", 50))

	serverManager := server.NewServerManager(appConfig)
	running := serverManager.IsRunning()

	status := "❌ Stopped"
	if running {
		status = "✅ Running"
	}

	fmt.Printf("Server Status: %s\n", status)
	fmt.Printf("Port: %d\n", appConfig.GetServerPort())

	if running {
		fmt.Printf("API Endpoint: http://localhost:%d/v1/chat/completions\n", appConfig.GetServerPort())
	}

	fmt.Println("\n📈 Configuration Statistics:")
	fmt.Printf("Configured Providers: %d\n", len(appConfig.ListProviders()))

	enabledCount := 0
	for _, provider := range appConfig.ListProviders() {
		if provider.Enabled {
			enabledCount++
		}
	}
	fmt.Printf("Enabled Providers: %d\n", enabledCount)

	if logger != nil {
		currentStatus := logger.GetCurrentStatus()
		fmt.Printf("Last Updated: %s\n", currentStatus.Timestamp.Format("2006-01-02 15:04:05"))
		if currentStatus.Running && currentStatus.Uptime != "" {
			fmt.Printf("Uptime: %s\n", currentStatus.Uptime)
		}

		stats := logger.GetActionStats()
		fmt.Println("\n📜 Operation Statistics:")
		for action, count := range stats {
			fmt.Printf("  %s: %d\n", action, count)
		}
	}
}

// handleGenerateToken generates a JWT token
func handleGenerateToken(appConfig *config.AppConfig, reader *bufio.Reader, logger *memory.MemoryLogger) {
	fmt.Print("Enter client ID (default: client): ")
	clientIDInput, _ := reader.ReadString('\n')
	clientIDInput = strings.TrimSpace(strings.TrimSuffix(clientIDInput, "\n"))

	clientID := "client"
	if clientIDInput != "" {
		clientID = clientIDInput
	}

	token := generateTokenForClient(appConfig, clientID)

	fmt.Println("\n🔑 Generated JWT Token:")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println(token)
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("\n📝 Usage in API requests:")
	fmt.Printf("Authorization: Bearer %s\n", token)

	if logger != nil {
		logger.LogAction(memory.ActionGenerateToken, map[string]interface{}{
			"client_id": clientID,
		}, true, "Token generated successfully")
	}
}

// generateTokenForClient generates JWT token for client
func generateTokenForClient(appConfig *config.AppConfig, clientID string) string {
	jwtManager := auth.NewJWTManager(appConfig.GetJWTSecret())
	token, _ := jwtManager.GenerateToken(clientID)
	return token
}

// handleViewHistory displays operation history
func handleViewHistory(logger *memory.MemoryLogger) {
	if logger == nil {
		fmt.Println("❌ History logging is not available.")
		return
	}

	fmt.Println("\n📜 Operation History")
	fmt.Println(strings.Repeat("=", 60))

	history := logger.GetHistory(20) // Show last 20 entries

	if len(history) == 0 {
		fmt.Println("❌ No history available.")
		return
	}

	for _, entry := range history {
		status := "✅"
		if !entry.Success {
			status = "❌"
		}

		fmt.Printf("%s %s - %s\n", entry.Timestamp.Format("15:04:05"), status, entry.Action)
		if entry.Message != "" {
			fmt.Printf("    %s\n", entry.Message)
		}
		fmt.Println()
	}
}

// handleSystemInfo displays system information
func handleSystemInfo(logger *memory.MemoryLogger) {
	fmt.Println("\n💾 System Information")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("📁 Memory Files:")
	fmt.Println("  - memory/history.json (operation history)")
	fmt.Println("  - memory/status.json (server status)")

	if logger != nil {
		stats := logger.GetActionStats()
		fmt.Printf("\n📊 Total Operations: %d\n", len(logger.GetHistory(1000)))

		fmt.Println("\n📈 Action Breakdown:")
		for action, count := range stats {
			fmt.Printf("  %s: %d\n", action, count)
		}
	}

	fmt.Println("\n🔧 Configuration:")
	homeDir, _ := os.UserHomeDir()
	fmt.Printf("  Config File: %s/%s/config.enc\n", homeDir, config.ConfigDirName)
}

// handleGenerateExample generates and displays API example
func handleGenerateExample(appConfig *config.AppConfig) {
	fmt.Println("\n🚀 API Usage Example")
	fmt.Println(strings.Repeat("=", 60))

	token := generateTokenForClient(appConfig, "example")
	port := appConfig.GetServerPort()

	fmt.Printf("🔑 Generated Token: %s\n", token)
	fmt.Printf("🌐 Server Endpoint: http://localhost:%d/v1/chat/completions\n\n", port)

	fmt.Println("📋 Example cURL Command:")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("curl -X POST http://localhost:%d/v1/chat/completions \\\n", port)
	fmt.Println("  -H \"Content-Type: application/json\" \\")
	fmt.Printf("  -H \"Authorization: Bearer %s\" \\\n", token)
	fmt.Println("  -d '{")
	fmt.Println("    \"model\": \"gpt-3.5-turbo\",")
	fmt.Println("    \"messages\": [")
	fmt.Println("      {\"role\": \"user\", \"content\": \"Hello, how are you?\"}")
	fmt.Println("    ]")
	fmt.Println("  }'")
	fmt.Println(strings.Repeat("-", 40))
}
