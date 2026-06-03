package options

import (
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/config"
)

// StartFlags holds flags for starting the server
type StartFlags struct {
	Port                 int
	Host                 string
	EnableUI             bool
	EnableDebug          bool
	EnableOpenBrowser    bool
	EnableStyleTransform bool
	Daemon               bool
	LogFile              string
	PromptRestart        bool
	RecordMode           string
	RecordDir            string
}

// StartServerOptions contains resolved options for starting the server
type StartServerOptions struct {
	Host                 string
	Port                 int
	EnableUI             bool
	EnableDebug          bool
	EnableOpenBrowser    bool
	Daemon               bool
	LogFile              string
	PromptRestart        bool
	RecordMode string
	RecordDir  string
}

// AddStartFlags adds all start-related flags to a command
// This is shared between start, restart, gui, and slim commands
func AddStartFlags(cmd *cobra.Command, flags *StartFlags) {
	cmd.Flags().IntVarP(&flags.Port, "port", "p", 0, "Server port (default: from config or 12580)")
	cmd.Flags().StringVar(&flags.Host, "host", "localhost", "Server host")
	cmd.Flags().BoolVarP(&flags.EnableUI, "ui", "u", true, "Enable web UI (default: true)")
	cmd.Flags().BoolVar(&flags.EnableDebug, "debug", false, "Enable debug mode including gin, low level logging and so on (default: false)")
	cmd.Flags().BoolVar(&flags.EnableOpenBrowser, "browser", true, "Auto-open browser when server starts (default: true)")
	cmd.Flags().BoolVar(&flags.EnableStyleTransform, "adapter", true, "Enable API style transformation (default: true)")
	cmd.Flags().BoolVar(&flags.Daemon, "daemon", false, "Run as daemon in background (default: false)")
	cmd.Flags().StringVar(&flags.LogFile, "log-file", "", "Log file path for daemon mode (default: ~/.tingly-box/tingly-box.log)")
	cmd.Flags().BoolVar(&flags.PromptRestart, "prompt-restart", false, "Prompt to restart if server is already running (default: false)")
	cmd.Flags().StringVar(&flags.RecordMode, "record-mode", "", "Record mode: empty=disabled, 'all'=record request+response, 'scenario'=all but for scenario only, 'response'=response only (default: disabled)")
	cmd.Flags().StringVar(&flags.RecordDir, "record-dir", "", "Record directory (default: ~/.tingly-box/record/)")
}

// ResolveStartOptions resolves CLI flags with config file defaults
// Priority: CLI flag > Config > Default
func ResolveStartOptions(cmd *cobra.Command, flags StartFlags, appConfig *config.AppConfig) StartServerOptions {
	// Apply priority: CLI flag > Config > Default
	resolvedDebug := flags.EnableDebug
	if !cmd.Flags().Changed("debug") {
		resolvedDebug = appConfig.GetDebug()
	}

	resolvedPort := flags.Port
	if resolvedPort == 0 {
		resolvedPort = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(flags.Port)
	}

	// Resolve record directory
	resolvedRecordDir := flags.RecordDir
	if resolvedRecordDir == "" {
		resolvedRecordDir = appConfig.ConfigDir() + "/record"
	}

	return StartServerOptions{
		Host:              flags.Host,
		Port:              resolvedPort,
		EnableUI:          flags.EnableUI,
		EnableDebug:       resolvedDebug,
		EnableOpenBrowser: flags.EnableOpenBrowser,
		Daemon:            flags.Daemon,
		LogFile:           flags.LogFile,
		PromptRestart:     flags.PromptRestart,
		RecordMode:        flags.RecordMode,
		RecordDir:         resolvedRecordDir,
	}
}
