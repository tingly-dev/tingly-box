package options

import (
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/appconfig"
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
	ProjectsDir          string
}

// StartServerOptions contains resolved options for starting the server
type StartServerOptions struct {
	Host              string
	Port              int
	EnableUI          bool
	EnableDebug       bool
	EnableOpenBrowser bool
	Daemon            bool
	LogFile           string
	// RecordDir is where scenario recording sinks write when the scenario's
	// recording_v2 flag enables recording. Always the config-dir default —
	// recording enablement is flag-driven, not a CLI concern.
	RecordDir string
	// ProjectsDir is where Projects keeps its data: --projects-dir, else
	// "<config dir>-project", beside the config dir and never inside it.
	ProjectsDir string
}

// ResolveStartOptions resolves CLI flags with config file defaults
// Priority: CLI flag > Config > Default
func ResolveStartOptions(cmd *cobra.Command, flags StartFlags, appConfig *appconfig.AppConfig) StartServerOptions {
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

	// Priority: --projects-dir (remembered) > remembered value > default
	resolvedProjectsDir := flags.ProjectsDir
	if resolvedProjectsDir != "" {
		if err := appConfig.SetProjectsDir(resolvedProjectsDir); err != nil {
			logrus.Warnf("Failed to remember --projects-dir: %v", err)
		}
	} else if resolvedProjectsDir = appConfig.GetProjectsDir(); resolvedProjectsDir == "" {
		resolvedProjectsDir = DefaultProjectsDir(appConfig.ConfigDir())
	}

	return StartServerOptions{
		Host:              flags.Host,
		Port:              resolvedPort,
		EnableUI:          flags.EnableUI,
		EnableDebug:       resolvedDebug,
		EnableOpenBrowser: flags.EnableOpenBrowser,
		Daemon:            flags.Daemon,
		LogFile:           flags.LogFile,
		RecordDir:         appConfig.ConfigDir() + "/record",
		ProjectsDir:       resolvedProjectsDir,
	}
}

// DefaultProjectsDir is the Projects data directory of the instance whose
// config directory is configDir: a sibling named "<config dir>-project"
// (~/.tingly-box-project by default), so instances never share a database
// and a thread's working directory never sits next to the config.
func DefaultProjectsDir(configDir string) string {
	return filepath.Clean(configDir) + "-project"
}
