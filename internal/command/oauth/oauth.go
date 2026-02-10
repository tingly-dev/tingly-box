package oauth

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/config"
)

// OAuthCommand represents the oauth command group
func OAuthCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oauth",
		Short: "OAuth authentication for AI providers",
		Long: `OAuth authentication commands for AI providers.
Supports qwen_code, codex, claude_code, and antigravity providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Add subcommands
	cmd.AddCommand(AddCommand(appConfig))
	cmd.AddCommand(ListCommand())

	return cmd
}

// supportedProviders returns the list of supported OAuth providers
func supportedProviders() []ProviderInfo {
	return []ProviderInfo{
		{
			Type:        "qwen_code",
			DisplayName: "Qwen",
			Description: "Qwen AI (Device Code flow - requires manual code entry)",
		},
		{
			Type:        "codex",
			DisplayName: "Codex",
			Description: "OpenAI Codex CLI (PKCE flow - requires port 1455)",
		},
		{
			Type:        "claude_code",
			DisplayName: "Claude Code",
			Description: "Anthropic Claude Code (PKCE flow)",
		},
		{
			Type:        "antigravity",
			DisplayName: "Antigravity",
			Description: "Antigravity AI (Google OAuth with PKCE)",
		},
	}
}

// ProviderInfo holds information about an OAuth provider
type ProviderInfo struct {
	Type        string
	DisplayName string
	Description string
}

// getProviderInfo returns provider info by type
func getProviderInfo(providerType string) *ProviderInfo {
	for _, p := range supportedProviders() {
		if p.Type == providerType {
			return &p
		}
	}
	return nil
}

// isProviderSupported checks if a provider is supported
func isProviderSupported(providerType string) bool {
	return getProviderInfo(providerType) != nil
}

// getProviderConfig returns OAuth configuration for a provider
func getProviderConfig(providerType string) (*ProviderOAuthConfig, error) {
	switch providerType {
	case "qwen_code":
		return &ProviderOAuthConfig{
			Type:         "qwen_code",
			DisplayName:  "Qwen",
			APIBase:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIStyle:     "openai",
			OAuthMethod:  "device_code",
			NeedsPort1455: false,
		}, nil
	case "codex":
		return &ProviderOAuthConfig{
			Type:         "codex",
			DisplayName:  "Codex",
			APIBase:      "https://api.openai.com/v1",
			APIStyle:     "openai",
			OAuthMethod:  "pkce",
			NeedsPort1455: true,
		}, nil
	case "claude_code":
		return &ProviderOAuthConfig{
			Type:         "claude_code",
			DisplayName:  "Claude Code",
			APIBase:      "https://api.anthropic.com/v1",
			APIStyle:     "anthropic",
			OAuthMethod:  "pkce",
			NeedsPort1455: false,
		}, nil
	case "antigravity":
		return &ProviderOAuthConfig{
			Type:         "antigravity",
			DisplayName:  "Antigravity",
			APIBase:      "https://api.antigravity.com/v1",
			APIStyle:     "openai",
			OAuthMethod:  "pkce",
			NeedsPort1455: false,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}
}

// ProviderOAuthConfig holds OAuth configuration for a provider
type ProviderOAuthConfig struct {
	Type         string
	DisplayName  string
	APIBase      string
	APIStyle     string
	OAuthMethod  string // "pkce" or "device_code"
	NeedsPort1455 bool
}
