package command

import (
	"github.com/tingly-dev/tingly-box/internal/command/oauth"
)

// OAuthCommand returns the oauth command group
func OAuthCommand(appManager interface{}) interface{} {
	// Extract the AppConfig from AppManager
	if am, ok := appManager.(*AppManager); ok {
		return oauth.OAuthCommand(am.AppConfig())
	}
	// Otherwise assume it's already an AppConfig (shouldn't happen)
	return oauth.OAuthCommand(appManager.(*AppManager).AppConfig())
}
