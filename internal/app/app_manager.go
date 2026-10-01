package app

import (
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/appconfig"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/lock"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// AppManager is the command process host: it owns AppConfig and server
// lifecycle. Domain behavior belongs in internal/usecase rather than here.
//
// AppConfig is built lazily, on the first call that needs it. Building it is
// not free of side effects: appconfig.NewAppConfig creates the config
// directory tree, opens tingly.db (SQLite, WAL) through db.NewStoreManager
// and runs every store's AutoMigrate — in other words, it turns this process
// into a client of the server's database. A command that only needs the
// config directory (lock file, port file, log paths) must be able to run
// without any of that: `version` is what Docker's HEALTHCHECK runs every
// 30s, and `mcp-builtin` is a subprocess the running server itself spawns.
// Both used to open the database as a side effect of being dispatched, and
// on a Docker Desktop bind mount — where SQLite's cross-process locking is
// unreliable — that second writer corrupted a user's usage table (#1912).
//
// So the rule is: ConfigDir and GetRuntimeServerPort never open the
// database; AppConfig and GetGlobalConfig do, once, on demand.
type AppManager struct {
	configDir string
	version   string

	once      sync.Once
	appConfig *appconfig.AppConfig
	initErr   error
	// onInitError is called when the lazy build of AppConfig fails. The CLI
	// installs a handler that prints the error and exits, which is what the
	// eager construction in main() used to do. A handler that returns leaves
	// AppConfig returning nil, so a caller that must keep going should check.
	onInitError func(error)
}

// NewAppManager creates an AppManager and builds its AppConfig immediately.
// An empty configDir means the default directory.
func NewAppManager(configDir string) (*AppManager, error) {
	am := NewLazyAppManager(configDir, nil)
	if _, err := am.init(); err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}
	return am, nil
}

// NewAppManagerWithConfig creates a new AppManager over an existing AppConfig.
func NewAppManagerWithConfig(appConfig *appconfig.AppConfig) *AppManager {
	am := &AppManager{configDir: appConfig.ConfigDir(), appConfig: appConfig}
	am.once.Do(func() {})
	return am
}

// NewLazyAppManager creates an AppManager that builds its AppConfig on first
// use (AppConfig / GetGlobalConfig). An empty configDir means the default
// directory. onInitError, if non-nil, is called when that build fails; the
// CLI uses it to print the error and exit with the same message the eager
// path printed before.
func NewLazyAppManager(configDir string, onInitError func(error)) *AppManager {
	if configDir == "" {
		configDir = constant.GetTinglyConfDir()
	}
	return &AppManager{configDir: configDir, onInitError: onInitError}
}

// SetVersion records the build version, applied to AppConfig when it is
// built (or immediately if it already is).
func (am *AppManager) SetVersion(version string) {
	am.version = version
	if am.appConfig != nil {
		am.appConfig.SetVersion(version)
	}
}

// ConfigDir returns the configuration directory. It never builds AppConfig
// and never touches the database.
func (am *AppManager) ConfigDir() string {
	return am.configDir
}

// Initialized reports whether AppConfig has been built (successfully).
func (am *AppManager) Initialized() bool {
	return am.appConfig != nil
}

func (am *AppManager) init() (*appconfig.AppConfig, error) {
	am.once.Do(func() {
		cfg, err := appconfig.NewAppConfig(appconfig.WithConfigDir(am.configDir))
		if err != nil {
			am.initErr = err
			return
		}
		if am.version != "" {
			cfg.SetVersion(am.version)
		}
		am.appConfig = cfg
	})
	return am.appConfig, am.initErr
}

// AppConfig returns the underlying AppConfig, building it on first call.
// This opens the database; see the type comment.
func (am *AppManager) AppConfig() *appconfig.AppConfig {
	cfg, err := am.init()
	if err != nil && am.onInitError != nil {
		am.onInitError(err)
	}
	return cfg
}

// GetGlobalConfig returns the global configuration manager, building
// AppConfig on first call.
func (am *AppManager) GetGlobalConfig() *serverconfig.Config {
	cfg := am.AppConfig()
	if cfg == nil {
		return nil
	}
	return cfg.GetGlobalConfig()
}

// ============
// Server Management
// ============

// StartServerAt initializes and starts the in-process server used by the TUI.
func (am *AppManager) StartServerAt(port int) error {
	serverManager := NewServerManager(am.AppConfig())
	if err := serverManager.Setup(port); err != nil {
		return err
	}
	return serverManager.Start()
}

// ============
// Configuration Accessors
// ============

// GetRuntimeServerPort returns the port the running server is actually
// listening on. The server port is intentionally not persisted in the config
// file, so a server started with --port would be invisible to other CLI
// processes; the server therefore records its port in a runtime port file
// next to the PID lock. When the server is running (lock held) and the port
// file is readable, that port wins; otherwise this falls back to the
// configured port.
//
// Neither the lock nor the port file needs AppConfig, and the configured
// port is not stored in config.json (it is the build default unless set
// in-process), so this never opens the database — `stop`, `log` and `open`
// on a running server rely on that.
func (am *AppManager) GetRuntimeServerPort() int {
	fileLock := lock.NewFileLock(am.configDir)
	if fileLock.IsLocked() {
		if port, err := fileLock.ReadPort(); err == nil {
			return port
		}
	}
	if am.appConfig != nil {
		return am.appConfig.GetServerPort()
	}
	return constant.DefaultServerPort
}
