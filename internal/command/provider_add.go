package command

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/command/tui"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// runAdd handles `config provider add`. Same all-or-nothing shape as
// `config rule add`: all four positional args runs non-interactively (CI);
// anything else opens the TUI's Provider mode (Add) rather than a bespoke
// bufio flow that mixed given args with prompts for the rest — the
// prior behavior let something you typed silently vanish behind the
// following prompts, and duplicated what the TUI already does properly.
func runAdd(appManager *AppManager, args []string) error {
	if len(args) >= 4 {
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
	if len(args) > 0 {
		return fmt.Errorf("partial arguments supplied; for CI mode pass all four: name, base-url, token, api-style. For interactive use, run with no arguments")
	}
	if err := requireTTY("pass all four positional args for non-interactive mode: name, base-url, token, api-style, or use 'tingly-box tui' for interactive setup"); err != nil {
		return err
	}
	return tui.RunProviderMode(appManager.GetGlobalConfig())
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
