package command

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// runAdd handles `provider add`. All four positional args are required —
// this is a pure CI command with no interactive fallback of any kind
// (unlike an earlier revision, this isn't gated on TTY presence either):
// picking values interactively is `tingly-box tui` or Web UI work.
func runAdd(appManager *AppManager, args []string) error {
	if len(args) != 4 {
		return fmt.Errorf("all four positional args are required: name, base-url, token, api-style; for interactive setup use 'tingly-box tui' or the Web UI")
	}
	var apiStyle APIStyle
	switch strings.ToLower(args[3]) {
	case "openai":
		apiStyle = protocol.APIStyleOpenAI
	case "anthropic":
		apiStyle = protocol.APIStyleAnthropic
	default:
		return fmt.Errorf("invalid API style '%s'. Supported values: openai, anthropic", args[3])
	}
	return addProviderCI(appManager, args[0], args[1], args[2], apiStyle)
}

// addProviderCI adds a provider without prompting. Used when every required
// field is provided on the command line — typical for scripts and CI.
func addProviderCI(appManager *AppManager, name, apiBase, token string, apiStyle APIStyle) error {
	res, err := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).Add(usecase.CreateProviderRequest{
		Name: name, APIBase: apiBase, Token: token, APIStyle: apiStyle,
	})
	if err != nil {
		return fmt.Errorf("failed to add provider: %w", err)
	}
	fmt.Printf("✓ Provider '%s' added (uuid: %s, style: %s)\n", name, res.Provider.UUID, apiStyle)
	return nil
}
