package command

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// APIStyle is re-exported from internal/protocol for the CLI prompts.
type APIStyle = protocol.APIStyle

// ProviderCmdKong is the top-level `provider` command: a pure CI surface for
// managing providers by UUID/flags, one operation per invocation, never
// interactive. Browsing and picking a provider to edit/delete is TUI/Web UI
// work (`tingly-box tui`) — this command family intentionally has no picker
// of its own to duplicate that.
type ProviderCmdKong struct {
	List   ProviderListCmdKong   `kong:"cmd,name='list',default='1',help='List all providers'"`
	Add    ProviderAddCmdKong    `kong:"cmd,help='Add a new provider (CI: pass all four positional args)'"`
	Get    ProviderGetCmdKong    `kong:"cmd,help='Get provider details by UUID'"`
	Update ProviderUpdateCmdKong `kong:"cmd,help='Update one or more fields on an existing provider'"`
	Delete ProviderDeleteCmdKong `kong:"cmd,help='Delete a provider (-y required)'"`
}

// ProviderAddCmdKong adds a new provider. All four positional args runs
// non-interactively; anything else is a clear error — see runAdd.
type ProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

// Run is a pure CI command with no interactive fallback of any kind: all
// four fields are required, or it fails with a clear error naming what to
// pass instead — picking values interactively is `tingly-box tui` or Web UI
// work.
func (c *ProviderAddCmdKong) Run(appManager *AppManager) error {
	if c.Name == "" || c.BaseURL == "" || c.Token == "" || c.APIStyle == "" {
		return fmt.Errorf("all four positional args are required: name, base-url, token, api-style; for interactive setup use 'tingly-box tui' or the Web UI")
	}
	var apiStyle APIStyle
	switch strings.ToLower(c.APIStyle) {
	case "openai":
		apiStyle = protocol.APIStyleOpenAI
	case "anthropic":
		apiStyle = protocol.APIStyleAnthropic
	default:
		return fmt.Errorf("invalid API style '%s'. Supported values: openai, anthropic", c.APIStyle)
	}
	return addProviderCI(appManager, c.Name, c.BaseURL, c.Token, apiStyle)
}

// ProviderListCmdKong lists all providers.
type ProviderListCmdKong struct{}

func (c *ProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ProviderGetCmdKong displays a provider's details by UUID. Names are not
// unique (UUID is the PK), so UUID is required — no picker to fall back to.
type ProviderGetCmdKong struct {
	UUID string `kong:"arg,required,help='Provider UUID'"`
}

func (c *ProviderGetCmdKong) Run(appManager *AppManager) error {
	return runProviderGet(appManager, c.UUID)
}

// ProviderUpdateCmdKong updates one or more fields on an existing provider.
// Only fields whose flag was actually passed change; the rest are left as-is
// (fetched from the current record first). At least one field flag is
// required — otherwise there is nothing to do.
type ProviderUpdateCmdKong struct {
	UUID     string `kong:"arg,required,help='Provider UUID'"`
	Name     string `kong:"flag,name='name',help='New name'"`
	BaseURL  string `kong:"flag,name='base-url',help='New API base URL'"`
	Token    string `kong:"flag,name='token',help='New API token'"`
	APIStyle string `kong:"flag,name='api-style',help='New API style: openai or anthropic'"`
	ProxyURL string `kong:"flag,name='proxy-url',help='New proxy URL'"`
}

func (c *ProviderUpdateCmdKong) Run(appManager *AppManager) error {
	if c.Name == "" && c.BaseURL == "" && c.Token == "" && c.APIStyle == "" && c.ProxyURL == "" {
		return fmt.Errorf("nothing to update; pass at least one of --name, --base-url, --token, --api-style, --proxy-url")
	}

	providerUC := usecase.NewProviderUseCase(appManager.GetGlobalConfig())
	result, err := providerUC.Get(usecase.GetProviderRequest{UUID: c.UUID})
	if err != nil {
		return err
	}
	p := result.Provider

	name, apiBase, token, proxyURL := p.Name, p.APIBase, p.Token, p.ProxyURL
	apiStyle := p.APIStyle
	if c.Name != "" {
		name = c.Name
	}
	if c.BaseURL != "" {
		apiBase = c.BaseURL
	}
	if c.Token != "" {
		token = c.Token
	}
	if c.ProxyURL != "" {
		proxyURL = c.ProxyURL
	}
	if c.APIStyle != "" {
		switch strings.ToLower(c.APIStyle) {
		case "openai":
			apiStyle = protocol.APIStyleOpenAI
		case "anthropic":
			apiStyle = protocol.APIStyleAnthropic
		default:
			return fmt.Errorf("invalid API style '%s'. Supported values: openai, anthropic", c.APIStyle)
		}
	}

	if _, err := providerUC.Update(usecase.UpdateProviderRequest{
		UUID:     c.UUID,
		Name:     name,
		APIBase:  apiBase,
		Token:    token,
		APIStyle: apiStyle,
		ProxyURL: proxyURL,
	}); err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}
	fmt.Printf("✓ Provider '%s' updated (uuid: %s)\n", name, c.UUID)
	return nil
}

// ProviderDeleteCmdKong deletes a provider. -y/--yes is required: this
// command never prompts, so the flag is the only way to confirm intent.
type ProviderDeleteCmdKong struct {
	UUID string `kong:"arg,required,help='Provider UUID'"`
	Yes  bool   `kong:"flag,name='yes',short='y',help='Confirm deletion (required — this command never prompts)'"`
}

func (c *ProviderDeleteCmdKong) Run(appManager *AppManager) error {
	if !c.Yes {
		return fmt.Errorf("pass -y/--yes to confirm deletion of provider %s — this command never prompts", c.UUID)
	}
	providerUC := usecase.NewProviderUseCase(appManager.GetGlobalConfig())
	result, err := providerUC.Get(usecase.GetProviderRequest{UUID: c.UUID})
	if err != nil {
		return err
	}
	if err := providerUC.Delete(usecase.DeleteProviderRequest{UUID: c.UUID}); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	fmt.Printf("✓ Provider '%s' deleted\n", result.Provider.Name)
	return nil
}

// ============== Provider operations ==============

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

// runProviderList lists all providers
func runProviderList(appManager *AppManager) error {
	providers := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).List().Providers

	if len(providers) == 0 {
		fmt.Println("No providers configured. Use 'provider add' to add a provider.")
		return nil
	}

	fmt.Println("\nAll Configured Providers")
	fmt.Println(strings.Repeat("-", 80))

	for i, provider := range providers {
		status := "❌ Disabled"
		if provider.Enabled {
			status = "✅ Enabled"
		}
		fmt.Printf("%d. %s\n", i+1, provider.Name)
		fmt.Printf("   UUID: %s\n", provider.UUID)
		fmt.Printf("   URL: %s\n", provider.APIBase)
		fmt.Printf("   Style: %s\n", provider.APIStyle)
		fmt.Printf("   Status: %s\n", status)
		fmt.Println(strings.Repeat("-", 80))
	}

	return nil
}

// runProviderGet displays provider details for the given UUID. Providers are
// keyed by UUID; names are not unique and must not be used as lookup keys.
func runProviderGet(appManager *AppManager, uuid string) error {
	result, err := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: uuid})
	if err != nil {
		return err
	}
	provider := result.Provider

	fmt.Println("\n🔍 Provider Details")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Name:      %s\n", provider.Name)
	fmt.Printf("UUID:      %s\n", provider.UUID)
	fmt.Printf("API Base:  %s\n", provider.APIBase)
	fmt.Printf("API Style: %s\n", provider.APIStyle)
	fmt.Printf("Enabled:   %v\n", provider.Enabled)
	fmt.Printf("Proxy URL: %s\n", provider.ProxyURL)
	fmt.Printf("Timeout:   %d seconds\n", provider.Timeout)

	if provider.Tags != nil && len(provider.Tags) > 0 {
		fmt.Printf("Tags:      %v\n", provider.Tags)
	}

	status := "❌ Disabled"
	if provider.Enabled {
		status = "✅ Enabled"
	}
	fmt.Printf("Status:    %s\n", status)
	fmt.Println(strings.Repeat("=", 60))

	return nil
}
