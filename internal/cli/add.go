package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"tingly-box/internal/config"
)

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
)

// AddCommand represents the add provider command
func AddCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [name] [baseurl] [token] [api_style]",
		Short: "Add a new AI provider configuration",
		Long: `Add a new AI provider with name, API base URL, token, and optional API style.
You can provide the arguments as positional parameters:
  add openai https://api.openai.com/v1 your-token-here openai
  add anthropic https://api.anthropic.com your-token-here anthropic

The api_style parameter is optional and defaults to "openai".
Supported values: openai, anthropic

Or run the command without arguments for interactive mode.`,
		Args: cobra.MaximumNArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(appConfig, args)
		},
	}

	return cmd
}

// runAdd handles the provider addition process with both positional arguments and interactive mode
func runAdd(appConfig *config.AppConfig, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	var name, apiBase, token string
	var apiStyle APIStyle = APIStyleOpenAI // default to openai

	// Extract values from positional arguments if provided
	if len(args) > 0 {
		name = args[0]
	}
	if len(args) > 1 {
		apiBase = args[1]
	}
	if len(args) > 2 {
		token = args[2]
	}
	if len(args) > 3 {
		// Validate and set API style
		switch strings.ToLower(args[3]) {
		case "openai":
			apiStyle = APIStyleOpenAI
		case "anthropic":
			apiStyle = APIStyleAnthropic
		default:
			return fmt.Errorf("invalid API style '%s'. Supported values: openai, anthropic", args[3])
		}
	}

	// If we have all required arguments, skip interactive prompts
	if len(args) >= 3 {
		return addProviderWithConfirmation(appConfig, reader, name, apiBase, token, apiStyle)
	}

	// Interactive mode for missing values
	fmt.Println("Let's add a new AI provider configuration.")
	if len(args) > 0 {
		fmt.Printf("Using provided name: %s\n", name)
	}
	if len(args) > 1 {
		fmt.Printf("Using provided API base URL: %s\n", apiBase)
	}
	if len(args) > 2 {
		fmt.Printf("Using provided token: %s\n", maskToken(token))
	}
	if len(args) > 3 {
		fmt.Printf("Using provided API style: %s\n", apiStyle)
	}
	fmt.Println()

	// Get provider name (if not provided)
	if name == "" {
		var err error
		name, err = promptForInput(reader, "Enter provider name (e.g., openai, anthropic): ", true)
		if err != nil {
			return err
		}
	}

	// Check if provider already exists
	if existingProvider, err := appConfig.GetProvider(name); err == nil && existingProvider != nil {
		fmt.Printf("Provider '%s' already exists. Please use a different name or update the existing provider.\n", name)
		return fmt.Errorf("provider already exists")
	}

	// Get API base URL (if not provided)
	if apiBase == "" {
		var err error
		apiBase, err = promptForInput(reader, "Enter API base URL (e.g., https://api.openai.com/v1): ", true)
		if err != nil {
			return err
		}
	}

	// Get API token (if not provided)
	if token == "" {
		var err error
		token, err = promptForInput(reader, "Enter API token: ", true)
		if err != nil {
			return err
		}
	}

	// Get API style (if not provided)
	if len(args) < 4 {
		var err error
		apiStyle, err = promptForAPIStyle(reader, name, apiBase)
		if err != nil {
			return err
		}
	}

	return addProviderWithConfirmation(appConfig, reader, name, apiBase, token, apiStyle)
}

// addProviderWithConfirmation displays summary and adds the provider
func addProviderWithConfirmation(appConfig *config.AppConfig, reader *bufio.Reader, name, apiBase, token string, apiStyle APIStyle) error {
	// Display summary and get confirmation
	fmt.Println("\n--- Configuration Summary ---")
	fmt.Printf("Provider Name: %s\n", name)
	fmt.Printf("API Base URL: %s\n", apiBase)
	fmt.Printf("API Style: %s\n", apiStyle)
	fmt.Printf("Token: %s\n", maskToken(token))
	fmt.Println("---------------------------")

	confirmed, err := promptForConfirmation(reader, "Do you want to save this configuration? (Y/n): ")
	if err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Operation cancelled.")
		return nil
	}

	// Add the provider with API style
	if err := appConfig.AddProviderByName(name, apiBase, token); err != nil {
		return fmt.Errorf("failed to add provider: %w", err)
	}

	// Update the provider to set the API style
	if provider, err := appConfig.GetProvider(name); err == nil {
		provider.APIStyle = config.APIStyle(apiStyle)
		// Save the configuration
		if saveErr := appConfig.Save(); saveErr != nil {
			fmt.Printf("Warning: failed to save API style configuration: %v\n", saveErr)
		}
	}

	fmt.Printf("Successfully added provider '%s' with API style '%s'\n", name, apiStyle)
	return nil
}

// promptForInput prompts the user for input and returns the trimmed response
func promptForInput(reader *bufio.Reader, prompt string, required bool) (string, error) {
	for {
		fmt.Print(prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		if required && input == "" {
			fmt.Println("This field is required. Please enter a value.")
			continue
		}

		return input, nil
	}
}

// promptForConfirmation prompts the user for a yes/no confirmation
func promptForConfirmation(reader *bufio.Reader, prompt string) (bool, error) {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	// Default to Yes if user just presses Enter
	return input == "" || input == "y" || input == "yes", nil
}

// maskToken masks the API token for display purposes
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

// promptForAPIStyle prompts the user to select an API style with intelligent defaults
func promptForAPIStyle(reader *bufio.Reader, name, apiBase string) (APIStyle, error) {
	// Auto-detect API style based on name or URL
	var suggestedStyle APIStyle = APIStyleOpenAI
	var suggestion string

	lowerName := strings.ToLower(name)
	lowerURL := strings.ToLower(apiBase)

	if strings.Contains(lowerName, "anthropic") || strings.Contains(lowerName, "claude") ||
		strings.Contains(lowerURL, "anthropic") || strings.Contains(lowerURL, "claude") {
		suggestedStyle = APIStyleAnthropic
		suggestion = "anthropic"
	} else if strings.Contains(lowerName, "openai") || strings.Contains(lowerName, "gpt") ||
		strings.Contains(lowerURL, "openai") {
		suggestedStyle = APIStyleOpenAI
		suggestion = "openai"
	}

	fmt.Printf("\nSelect API style (default: %s):\n", suggestion)
	fmt.Println("1. openai - For OpenAI-compatible APIs")
	fmt.Println("2. anthropic - For Anthropic Claude API")
	fmt.Print("Enter choice (1-2) or press Enter for default: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return suggestedStyle, nil
	}

	switch input {
	case "1", "openai":
		return APIStyleOpenAI, nil
	case "2", "anthropic":
		return APIStyleAnthropic, nil
	default:
		fmt.Printf("Invalid choice '%s', using default: %s\n", input, suggestion)
		return suggestedStyle, nil
	}
}
