package command

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

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

func (c *ProviderAddCmdKong) Run(appManager *AppManager) error {
	args := []string{}
	if c.Name != "" {
		args = append(args, c.Name)
	}
	if c.BaseURL != "" {
		args = append(args, c.BaseURL)
	}
	if c.Token != "" {
		args = append(args, c.Token)
	}
	if c.APIStyle != "" {
		args = append(args, c.APIStyle)
	}
	return runAdd(appManager, args)
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
