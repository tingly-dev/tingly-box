//go:build !legacy

package command

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// StartCmdKong is the Kong version of start command
type StartCmdKong struct {
	Port                 int    `kong:"flag,name='port',short='p',help='Server port'"`
	Host                 string `kong:"flag,name='host',help='Server host'"`
	EnableUI             bool   `kong:"flag,name='ui',short='u',default='true',help='Enable web UI'"`
	EnableDebug          bool   `kong:"flag,name='debug',help='Enable debug mode'"`
	EnableOpenBrowser    bool   `kong:"flag,name='browser',default='true',help='Auto-open browser'"`
	EnableStyleTransform bool   `kong:"flag,name='adapter',default='true',help='Enable API style transform'"`
	Daemon               bool   `kong:"flag,name='daemon',help='Run as daemon'"`
	LogFile              string `kong:"flag,name='log-file',help='Log file path'"`
	PromptRestart        bool   `kong:"flag,name='prompt-restart',help='Prompt to restart if running'"`
	RecordMode           string `kong:"flag,name='record-mode',help='Record mode'"`
	RecordDir            string `kong:"flag,name='record-dir',help='Record directory'"`
	Expr                 string `kong:"flag,name='expr',help='Experimental features'"`
}

func (s *StartCmdKong) Run(appManager *AppManager) error {
	flags := options.StartFlags{
		Port:                 s.Port,
		Host:                 s.Host,
		EnableUI:             s.EnableUI,
		EnableDebug:          s.EnableDebug,
		EnableOpenBrowser:    s.EnableOpenBrowser,
		EnableStyleTransform: s.EnableStyleTransform,
		Daemon:               s.Daemon,
		LogFile:              s.LogFile,
		PromptRestart:        s.PromptRestart,
		RecordMode:           s.RecordMode,
		RecordDir:            s.RecordDir,
		Expr:                 s.Expr,
	}
	opts := options.ResolveStartOptions(newKongShimCmd(s.EnableDebug), flags, appManager.AppConfig())
	return startServer(appManager, opts)
}

// StopCmdKong is the Kong version of stop command
type StopCmdKong struct{}

func (s *StopCmdKong) Run(appManager *AppManager) error {
	return doStopServer(appManager)
}

// StatusCmdKong is the Kong version of status command
type StatusCmdKong struct{}

func (s *StatusCmdKong) Run(appManager *AppManager) error {
	return runStatusCmd(appManager)
}

// RestartCmdKong is the Kong version of restart command
type RestartCmdKong struct {
	StartCmdKong
}

func (r *RestartCmdKong) Run(appManager *AppManager) error {
	appConfig := appManager.AppConfig()
	fileLock := lock.NewFileLock(appConfig.ConfigDir())
	wasRunning := fileLock.IsLocked()

	if wasRunning {
		fmt.Println("Stopping current server...")
		if err := stopServerWithFileLock(fileLock); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
		fmt.Println("Server stopped successfully")
	} else {
		fmt.Println("Server was not running, starting it...")
	}

	flags := options.StartFlags{
		Port:                 r.Port,
		Host:                 r.Host,
		EnableUI:             r.EnableUI,
		EnableDebug:          r.EnableDebug,
		EnableOpenBrowser:    r.EnableOpenBrowser,
		EnableStyleTransform: r.EnableStyleTransform,
		Daemon:               r.Daemon,
		LogFile:              r.LogFile,
		PromptRestart:        r.PromptRestart,
		RecordMode:           r.RecordMode,
		RecordDir:            r.RecordDir,
		Expr:                 r.Expr,
	}
	opts := options.ResolveStartOptions(newKongShimCmd(r.EnableDebug), flags, appManager.AppConfig())
	return startServer(appManager, opts)
}

// newKongShimCmd builds a cobra.Command whose only purpose is to satisfy
// options.ResolveStartOptions, which probes cmd.Flags().Changed("debug") to
// give an explicit --debug priority over the config file value. Kong has
// already done flag parsing; we register --debug here and Set it iff the
// caller passed it, so Changed() returns the right answer.
func newKongShimCmd(debugSet bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("debug", false, "")
	if debugSet {
		_ = cmd.Flags().Set("debug", "true")
	}
	return cmd
}

// Build information set by cli/tingly-box/main_kong.go at startup.
var (
	BuildVersion   = "dev"
	BuildGitCommit = "unknown"
	BuildBuildTime = "unknown"
	BuildGoVersion = "unknown"
	BuildPlatform  = "unknown"
)

// VersionCmdKong is the Kong version of version command
type VersionCmdKong struct{}

func (v *VersionCmdKong) Run(appManager *AppManager) error {
	fmt.Printf("Tingly Box CLI\n")
	fmt.Printf("Version:    %s\n", BuildVersion)
	fmt.Printf("Git Commit: %s\n", BuildGitCommit)
	fmt.Printf("Build Time: %s\n", BuildBuildTime)
	fmt.Printf("Go Version: %s\n", BuildGoVersion)
	fmt.Printf("Platform:   %s\n", BuildPlatform)
	return nil
}

// runStatusCmd extracts status logic
func runStatusCmd(appManager *AppManager) error {
	providers := appManager.ListProviders()
	appConfig := appManager.AppConfig()
	fileLock := lock.NewFileLock(appConfig.ConfigDir())
	serverRunning := fileLock.IsLocked()
	globalConfig := appManager.GetGlobalConfig()

	fmt.Println("=== Tingly Box Status ===")
	fmt.Printf("Server Status: ")
	if serverRunning {
		fmt.Printf("Running\n")
		port := appConfig.GetServerPort()
		fmt.Printf("Port: %d\n", port)
		fmt.Printf("OpenAI Style API Endpoint: http://localhost:%d/tingly/openai/v1/chat/completions\n", port)
		fmt.Printf("Anthropic Style API Endpoint: http://localhost:%d/tingly/anthropic/v1/messages\n", port)
		if globalConfig.HasUserToken() {
			fmt.Printf("Web UI: http://localhost:%d/login/%s\n", port, globalConfig.GetUserToken())
		} else {
			fmt.Printf("Web UI: http://localhost:%d/\n", port)
		}
	} else {
		fmt.Printf("Stopped\n")
	}

	fmt.Printf("\nAuthentication:\n")
	if globalConfig.HasModelToken() {
		fmt.Printf("  Model API Key: Configured (sk-tingly- format)\n")
	} else {
		fmt.Printf("  Model API Key: Not configured (will auto-generate on start)\n")
	}

	fmt.Printf("\nConfigured Providers: %d\n", len(providers))
	if len(providers) > 0 {
		fmt.Println("Providers:")
		for _, provider := range providers {
			status := "Disabled"
			if provider.Enabled {
				status = "Enabled"
			}
			fmt.Printf("  - %s (%s) [%s]: %s\n", provider.Name, provider.APIBase, provider.APIStyle, status)
		}
	}

	cfg := appConfig.GetGlobalConfig()
	rules := cfg.Rules
	fmt.Printf("\nConfigured Rules: %d\n", len(rules))
	if len(rules) > 0 {
		fmt.Println("Rules:")
		for _, rule := range rules {
			status := "Inactive"
			if rule.Active {
				status = "Active"
			}
			serviceCount := len(rule.GetServices())
			fmt.Printf("  - %s -> %s: %s (%d services)\n", rule.RequestModel, rule.ResponseModel, status, serviceCount)
		}
	}

	return nil
}
