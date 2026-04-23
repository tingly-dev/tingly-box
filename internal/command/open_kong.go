//go:build kong

package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/feature"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

// OpenCmdKong opens the web UI
type OpenCmdKong struct {
	// Embed StartCmdKong to share server flags
	StartCmdKong
}

func (o *OpenCmdKong) Run(appManager *AppManager) error {
	opts := resolveStartCmdKongOptions(&o.StartCmdKong, appManager.AppConfig())
	appConfig := appManager.AppConfig()
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	if fileLock.IsLocked() {
		port := appConfig.GetServerPort()
		globalConfig := appManager.GetGlobalConfig()

		host := opts.Host
		if host == "" {
			host = "localhost"
		}
		resolvedHost := network.ResolveHost(host)

		webUIURL := fmt.Sprintf("http://%s:%d/", resolvedHost, port)
		if globalConfig.HasUserToken() {
			webUIURL = fmt.Sprintf("http://%s:%d/login/%s", resolvedHost, port, globalConfig.GetUserToken())
		}

		fmt.Printf("Opening web UI: %s\n", webUIURL)
		return openBrowserURL(webUIURL)
	}

	fmt.Println("Server is not running, starting it...")
	return startServer(appManager, opts)
}

// resolveStartCmdKongOptions converts StartCmdKong to StartServerOptions
func resolveStartCmdKongOptions(start *StartCmdKong, appConfig *config.AppConfig) options.StartServerOptions {
	resolvedDebug := start.EnableDebug
	if !start.EnableDebug {
		resolvedDebug = appConfig.GetDebug()
	}

	resolvedPort := start.Port
	if resolvedPort == 0 {
		resolvedPort = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(start.Port)
	}

	resolvedRecordDir := start.RecordDir
	if resolvedRecordDir == "" {
		resolvedRecordDir = appConfig.ConfigDir() + "/record"
	}

	experimentalFeatures := feature.ParseFeatures(start.Expr)

	return options.StartServerOptions{
		Host:                 start.Host,
		Port:                 resolvedPort,
		EnableUI:             start.EnableUI,
		EnableDebug:          resolvedDebug,
		EnableOpenBrowser:    start.EnableOpenBrowser,
		Daemon:               start.Daemon,
		LogFile:              start.LogFile,
		PromptRestart:        start.PromptRestart,
		RecordMode:           start.RecordMode,
		RecordDir:            resolvedRecordDir,
		ExperimentalFeatures: experimentalFeatures,
	}
}
