package command

import (
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
)

// StartFlagsKong is the Kong-tagged twin of options.StartFlags (cobra
// version, still used by internal/command/server.go's cobra shim). Embedded
// into GUICmdKong/SlimCmdKong/TrayCmdKong so `gui`/`slim`/`tray` share one
// flag set — mirrors internal/command.StartCmdKong, the CLI's equivalent.
type StartFlagsKong struct {
	Port              int    `kong:"flag,name='port',short='p',help='Server port (default: from config or 12580)'"`
	Host              string `kong:"flag,name='host',default='localhost',help='Server host'"`
	EnableUI          bool   `kong:"flag,name='ui',short='u',default='true',help='Enable web UI'"`
	EnableDebug       bool   `kong:"flag,name='debug',help='Enable debug mode'"`
	EnableOpenBrowser bool   `kong:"flag,name='browser',default='true',help='Auto-open browser when server starts'"`
	Daemon            bool   `kong:"flag,name='daemon',hidden,help='Unused in GUI mode'"`
	LogFile           string `kong:"flag,name='log-file',hidden,help='Unused in GUI mode'"`
	PromptRestart     bool   `kong:"flag,name='prompt-restart',hidden,help='Unused in GUI mode'"`
}

// resolve mirrors internal/command's resolveStartCmdKongOptions: same
// fallback rules (explicit --debug wins, else config's debug setting; a
// zero --port falls back to config, a non-zero one persists to config), just
// without a *cobra.Command to check Flags().Changed on — Kong parses these
// directly, so a set-but-false --debug can't be told apart from "not passed"
// the way cobra's Changed() can. Existing behavior for GUI/CLI alike, not a
// regression from this migration.
func (f StartFlagsKong) resolve(appConfig *config.AppConfig) options.StartServerOptions {
	resolvedDebug := f.EnableDebug
	if !f.EnableDebug {
		resolvedDebug = appConfig.GetDebug()
	}

	resolvedPort := f.Port
	if resolvedPort == 0 {
		resolvedPort = appConfig.GetServerPort()
	} else {
		_ = appConfig.SetServerPort(f.Port)
	}

	return options.StartServerOptions{
		Host:              f.Host,
		Port:              resolvedPort,
		EnableUI:          f.EnableUI,
		EnableDebug:       resolvedDebug,
		EnableOpenBrowser: f.EnableOpenBrowser,
		Daemon:            f.Daemon,
		LogFile:           f.LogFile,
		PromptRestart:     f.PromptRestart,
		RecordDir:         appConfig.ConfigDir() + "/record",
	}
}
