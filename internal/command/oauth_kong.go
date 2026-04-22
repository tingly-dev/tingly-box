//go:build kong

package command

// OAuthCmdKong handles OAuth authentication
type OAuthCmdKong struct {
	// Positional argument (optional)
	Provider string `kong:"arg,optional,help='Provider type (e.g., claude_code, qwen_code, codex)'"`

	// Flags
	Name     string `kong:"flag,name='name',short='n',help='Custom name for the provider'"`
	Port     int    `kong:"flag,name='port',short='p',help='Callback server port'"`
	ProxyURL string `kong:"flag,name='proxy',short='x',help='Proxy URL for OAuth requests'"`
}

func (o *OAuthCmdKong) Run(appManager *AppManager) error {
	return runOAuthFlow(appManager.AppConfig(), o.Provider, o.Name, o.Port, o.ProxyURL)
}
