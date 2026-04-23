//go:build kong

package command

// ProviderCmdKong is the Kong version of provider command
type ProviderCmdKong struct {
	Add  ProviderAddCmdKong  `kong:"cmd,help='Add a new provider'"`
	List ProviderListCmdKong `kong:"cmd,help='List all providers'"`
}

func (p *ProviderCmdKong) Run(appManager *AppManager) error {
	return runProviderInteractiveMode(appManager)
}

// ProviderAddCmdKong adds a new provider
type ProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (p *ProviderAddCmdKong) Run(appManager *AppManager) error {
	args := []string{}
	if p.Name != "" {
		args = append(args, p.Name)
	}
	if p.BaseURL != "" {
		args = append(args, p.BaseURL)
	}
	if p.Token != "" {
		args = append(args, p.Token)
	}
	if p.APIStyle != "" {
		args = append(args, p.APIStyle)
	}
	return runAdd(appManager, args)
}

// ProviderListCmdKong lists all providers
type ProviderListCmdKong struct{}

func (p *ProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}
