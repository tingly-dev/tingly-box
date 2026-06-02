package command

import (
	"bufio"
	"os"

	"github.com/tingly-dev/tingly-box/internal/command/tui"
)

// ConfigProviderCmdKong groups provider operations under `config provider`.
// The default (no subcommand) drops into the provider sub-menu so it mirrors
// the interactive top-level config menu.
type ConfigProviderCmdKong struct {
	Interactive ConfigProviderInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive provider management'"`

	Add    ConfigProviderAddCmdKong    `kong:"cmd,help='Add a new provider'"`
	List   ConfigProviderListCmdKong   `kong:"cmd,help='List all providers'"`
	Delete ConfigProviderDeleteCmdKong `kong:"cmd,help='Delete a provider (interactive)'"`
	Update ConfigProviderUpdateCmdKong `kong:"cmd,help='Update a provider (interactive)'"`
	Get    ConfigProviderGetCmdKong    `kong:"cmd,help='Get provider details by UUID'"`
}

// ConfigProviderInteractiveCmdKong runs the provider interactive sub-menu.
type ConfigProviderInteractiveCmdKong struct{}

func (c *ConfigProviderInteractiveCmdKong) Run(appManager *AppManager) error {
	return tui.RunProviderMode(appManager)
}

// ConfigProviderAddCmdKong adds a new provider.
type ConfigProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (c *ConfigProviderAddCmdKong) Run(appManager *AppManager) error {
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

// ConfigProviderListCmdKong lists all providers.
type ConfigProviderListCmdKong struct{}

func (c *ConfigProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ConfigProviderDeleteCmdKong deletes a provider via interactive selection.
type ConfigProviderDeleteCmdKong struct{}

func (c *ConfigProviderDeleteCmdKong) Run(appManager *AppManager) error {
	return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigProviderUpdateCmdKong updates a provider via interactive selection.
type ConfigProviderUpdateCmdKong struct{}

func (c *ConfigProviderUpdateCmdKong) Run(appManager *AppManager) error {
	return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigProviderGetCmdKong displays a provider's details by UUID. Names are
// not unique (UUID is the PK), so we require UUID. Empty UUID drops to
// interactive selection.
type ConfigProviderGetCmdKong struct {
	UUID string `kong:"arg,optional,help='Provider UUID'"`
}

func (c *ConfigProviderGetCmdKong) Run(appManager *AppManager) error {
	if c.UUID == "" {
		return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
	}
	return runProviderGet(appManager, c.UUID)
}

