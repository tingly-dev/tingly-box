package command

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	obs2 "github.com/tingly-dev/tingly-box/pkg/obs"

	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/daemon"
	"github.com/tingly-dev/tingly-box/pkg/lock"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

// ============== Kong Command Structures ==============

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
	RecordMode string `kong:"flag,name='record-mode',help='Record mode'"`
	RecordDir  string `kong:"flag,name='record-dir',help='Record directory'"`
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
	}
	opts := options.ResolveStartOptions(newKongShimCmd(r.EnableDebug), flags, appManager.AppConfig())
	return startServer(appManager, opts)
}

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

// SwaggerCmdKong generates OpenAPI schema
type SwaggerCmdKong struct {
	Output string `kong:"flag,name='output',short='o',help='Output file path'"`
	Stdout bool   `kong:"flag,name='stdout',help='Write to stdout'"`
}

func (s *SwaggerCmdKong) Run(appManager *AppManager) error {
	return runSwagger(appManager, s.Output, s.Stdout)
}

// Build information set by cli/tingly-box/main_kong.go at startup.
var (
	BuildVersion   = "dev"
	BuildGitCommit = "unknown"
	BuildBuildTime = "unknown"
	BuildGoVersion = "unknown"
	BuildPlatform  = "unknown"
)

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

	return options.StartServerOptions{
		Host:              start.Host,
		Port:              resolvedPort,
		EnableUI:          start.EnableUI,
		EnableDebug:       resolvedDebug,
		EnableOpenBrowser: start.EnableOpenBrowser,
		Daemon:            start.Daemon,
		LogFile:           start.LogFile,
		PromptRestart:     start.PromptRestart,
		RecordMode:        start.RecordMode,
		RecordDir:         resolvedRecordDir,
	}
}

// runSwagger extracts swagger logic from SwaggerCommand
func runSwagger(appManager *AppManager, output string, stdout bool) error {
	cfg := appManager.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	json, err := server.GenerateOpenAPI(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate OpenAPI schema: %w", err)
	}

	if stdout {
		fmt.Println(json)
	} else {
		if output == "" {
			output = "openapi.json"
		}
		if err := os.WriteFile(output, []byte(json), 0644); err != nil {
			return fmt.Errorf("failed to write to file %s: %w", output, err)
		}
		fmt.Fprintf(os.Stderr, "OpenAPI schema written to: %s\n", output)
	}

	return nil
}

// ============== Business Logic Functions ==============

const (
	// URL templates for displaying to users
	webUITpl             = "http://localhost:%d/"
	webUILoginTpl        = "http://localhost:%d/login/%s"
	openAIEndpointTpl    = "http://localhost:%d/tingly/openai/v1/chat/completions"
	anthropicEndpointTpl = "http://localhost:%d/tingly/anthropic/v1/messages"
)

// BannerConfig holds configuration for banner display
type BannerConfig struct {
	Port         int
	Host         string
	EnableUI     bool
	GlobalConfig *serverconfig.Config
	IsDaemon     bool
}

// printBanner prints the server access banner
func printBanner(cfg BannerConfig) {
	if !cfg.EnableUI {
		resolvedHost := network.ResolveHost(cfg.Host)
		fmt.Printf("API endpoint: http://%s:%d/v1/chat/completions\n", resolvedHost, cfg.Port)
		return
	}

	const (
		primary   = lipgloss.Color("#3B82F6")
		success   = lipgloss.Color("#06B6D4")
		muted     = lipgloss.Color("#64748B")
		highlight = lipgloss.Color("#60A5FA")
	)

	labelStyle := lipgloss.NewStyle().Foreground(muted).Width(14).Align(lipgloss.Right)
	urlStyle := lipgloss.NewStyle().Foreground(success)
	tokenStyle := lipgloss.NewStyle().Foreground(highlight)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primary)

	var lines []string
	addLine := func(label, value string, valueStyle lipgloss.Style) {
		lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render(label), valueStyle.Render(value)))
	}

	if cfg.GlobalConfig.HasUserToken() {
		addLine("Web UI", fmt.Sprintf("http://localhost:%d/login/%s", cfg.Port, cfg.GlobalConfig.GetUserToken()), urlStyle)
	} else {
		addLine("Web UI", fmt.Sprintf("http://localhost:%d/", cfg.Port), urlStyle)
	}
	addLine("OpenAI API", fmt.Sprintf("http://localhost:%d/tingly/openai/v1/chat/completions", cfg.Port), urlStyle)
	addLine("Anthropic API", fmt.Sprintf("http://localhost:%d/tingly/anthropic/v1/messages", cfg.Port), urlStyle)

	if cfg.GlobalConfig.HasUserToken() {
		lines = append(lines, "")
		addLine("Login Token", cfg.GlobalConfig.GetUserToken(), tokenStyle)
	}

	// Build title with product name and version
	titleText := titleStyle.Render(fmt.Sprintf("Tingly Box - %s — Access Information", BuildVersion))

	// Compute visual width for centering the title
	maxWidth := lipgloss.Width(titleText)
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}

	title := lipgloss.PlaceHorizontal(maxWidth, lipgloss.Center, titleText)
	allLines := append([]string{title, ""}, lines...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(1, 2)

	fmt.Println()
	fmt.Println(box.Render(strings.Join(allLines, "\n")))
	fmt.Println()

	if cfg.IsDaemon {
		fmt.Println("Server is running in background. Use 'tingly-box stop' to stop.")
	}
}

// openBrowserURL opens the given URL in the default browser
func openBrowserURL(url string) error {
	return browser.OpenURL(url)
}

// resolveStartOptions is implemented in platform-specific files:
// - server_windows.go for Windows (uses process.Kill())
// - server_unix.go for Unix-like systems (uses SIGTERM/SIGKILL)

func doStopServer(appManager *AppManager) error {
	appConfig := appManager.AppConfig()
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	if !fileLock.IsLocked() {
		fmt.Println("Server is not running")
		return nil
	}

	fmt.Println("Stopping server...")
	if err := stopServerWithFileLock(fileLock); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	fmt.Println("Server stopped successfully")
	return nil
}

// startServer handles the server starting logic
func startServer(appManager *AppManager, opts options.StartServerOptions) error {
	return startServerWithHook(appManager, opts)
}

// startServerWithHook handles the server starting logic with optional setup hooks.
func startServerWithHook(appManager *AppManager, opts options.StartServerOptions, hooks ...func(*ServerManager) error) error {
	appConfig := appManager.AppConfig()

	// Set logrus level based on debug flag
	if opts.EnableDebug {
		appConfig.SetDebug(true)
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Info("Debug mode enabled - detailed logging will be shown")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	// Determine log file path - always use log file with rotation
	logFile := opts.LogFile
	if logFile == "" {
		// Default to config directory
		logFile = appConfig.ConfigDir() + "/log/tingly-box.log"
	}

	// Create multi-mode logger (text + JSON)
	multiLoggerCfg := obs2.DefaultMultiLoggerConfig(appConfig.ConfigDir())
	multiLoggerCfg.TextLogPath = logFile
	multiLogger, err := obs2.NewMultiLogger(multiLoggerCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize multi-mode logger: %w", err)
	}

	// Sync multiLogger level with logrus level based on debug flag
	if opts.EnableDebug {
		multiLogger.SetLevel(logrus.DebugLevel)
	}

	// Set up logrus to write to both stdout and file with rotation
	if opts.Daemon {
		// In daemon mode, only write to file
		logrus.SetOutput(multiLogger)
	} else {
		// In non-daemon mode, write to both stdout and file
		multiWriter := io.MultiWriter(os.Stdout, multiLogger)
		logrus.SetOutput(multiWriter)
	}

	// Add hook for JSON logging
	logrus.AddHook(obs2.NewMultiLoggerHook(multiLogger, nil))

	logrus.Infof("Logging to file: %s (with rotation)", logFile)
	logrus.Infof("JSON logging to: %s (for frontend/API)", multiLoggerCfg.JSONLogPath)

	var port int = opts.Port
	if port == 0 {
		port = appConfig.GetServerPort()
	} else {
		appConfig.SetServerPort(port)
	}

	// Check if port is available before proceeding
	if !network.IsPortAvailable(port) {
		return fmt.Errorf("port %d is already in use by another process", port)
	}

	// Create file lock
	fileLock := lock.NewFileLock(appConfig.ConfigDir())

	// Check if server is already running using file lock
	if fileLock.IsLocked() {
		logrus.Printf("Server is already running on port %d\n", port)
		printBanner(BannerConfig{
			Port:         port,
			Host:         opts.Host,
			EnableUI:     opts.EnableUI,
			GlobalConfig: appConfig.GetGlobalConfig(),
			IsDaemon:     false,
		})

		// If prompt-restart is enabled, ask user if they want to restart
		if opts.PromptRestart {
			fmt.Print("\nDo you want to restart the server? [y/N]: ")
			var response string
			fmt.Scanln(&response)

			// Check if user wants to restart
			if strings.ToLower(strings.TrimSpace(response)) == "y" || strings.ToLower(strings.TrimSpace(response)) == "yes" {
				fmt.Println("\nRestarting server...")
				// Stop the existing server first
				if err := stopServerWithFileLock(fileLock); err != nil {
					return fmt.Errorf("failed to stop existing server: %w", err)
				}
				// Give a moment for cleanup
				time.Sleep(1 * time.Second)
				// Continue to start the server (fall through to the rest of the function)
			} else {
				fmt.Println("\nRestart cancelled.")
				return nil
			}
		} else {
			fmt.Println("Tip: Use 'tingly-box restart' or 'npx tingly-box restart' to restart the server")
			fmt.Println("     Use 'tingly-box stop' or 'npx tingly-box stop' to stop it")
			return nil
		}
	}

	// Acquire lock before starting server
	if err := fileLock.TryLock(); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	fmt.Printf("Lock acquired: %s\n", fileLock.GetLockFilePath())

	serverManager := NewServerManager(
		appConfig,
		server.WithDebug(opts.EnableDebug),
		server.WithUI(opts.EnableUI),
		server.WithOpenBrowser(opts.EnableOpenBrowser),
		server.WithHost(opts.Host),
		server.WithRecordMode(obs.RecordMode(opts.RecordMode)),
		server.WithRecordDir(opts.RecordDir),
		server.WithMultiLogger(multiLogger),
	)

	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(serverManager); err != nil {
			fileLock.Unlock()
			return err
		}
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Print startup information before daemonizing
	fmt.Printf("Server starting on port %d...\n", port)

	printBanner(BannerConfig{
		Port:         port,
		Host:         opts.Host,
		EnableUI:     opts.EnableUI,
		GlobalConfig: appConfig.GetGlobalConfig(),
		IsDaemon:     false,
	})

	// Handle daemon mode - fork and detach after all messages are printed
	if opts.Daemon {
		if !daemon.IsDaemonProcess() {
			// Fork and detach - this call exits the parent process
			if err := daemon.Daemonize(); err != nil {
				fileLock.Unlock()
				return fmt.Errorf("failed to daemonize: %w", err)
			}
			// Daemonize() calls os.Exit(0), so we never reach here in parent
		}
		// Child process continues here
		fmt.Println("Server running in background (daemon mode)")
	}

	// Start server in goroutine to keep it non-blocking
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serverManager.Start()
	}()

	// Wait for either server error, shutdown signal, or web UI stop request
	select {
	case err := <-serverErr:
		// Release lock on error
		fileLock.Unlock()
		return fmt.Errorf("server stopped unexpectedly: %w", err)
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal, stopping server...")
		// Release lock on shutdown
		fileLock.Unlock()
		return serverManager.Stop()
	case <-server.GetShutdownChannel():
		fmt.Println("\nReceived stop request from web UI, stopping server...")
		// Release lock on shutdown
		fileLock.Unlock()
		return serverManager.Stop()
	}
}

// CreateAppManagerForDir creates a new AppManager for the specified config directory.
// This is used when a command specifies a different config directory than the global one.
func CreateAppManagerForDir(configDir string) (*AppManager, error) {
	// Create app config for the specified directory
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create app config for directory %s: %w", configDir, err)
	}
	return NewAppManagerWithConfig(appConfig), nil
}

// doStopServerWithFileLock stops the server using the provided file lock
func doStopServerWithFileLock(fileLock *lock.FileLock) error {
	if !fileLock.IsLocked() {
		fmt.Println("Server is not running")
		return nil
	}

	fmt.Println("Stopping server...")
	if err := stopServerWithFileLock(fileLock); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	fmt.Println("Server stopped successfully")
	return nil
}
