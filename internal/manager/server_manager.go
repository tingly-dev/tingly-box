package manager

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"tingly-box/internal/config"
	"tingly-box/internal/server"
)

const StopTimeout = time.Second * 10

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig *config.AppConfig
	server    *server.Server

	host              string
	enableUI          bool
	enableAdaptor     bool
	enableDebug       bool
	enableOpenBrowser bool
	httpsEnabled      bool
	httpsCertDir      string
	httpsRegenerate   bool
	status            string
	sync.Mutex
}

// ServerManagerOption defines a functional option for ServerManager
type ServerManagerOption func(*ServerManager)

// WithUI enables or disables the UI for the server manager
func WithUI(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.enableUI = enabled
	}
}

// WithAdaptor enables or disables the adaptor for the server manager
func WithAdaptor(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.enableAdaptor = enabled
	}
}

// WithDebug enables or disables the debug mode for the server manager
func WithDebug(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.enableDebug = enabled
	}
}

// WithOpenBrowser enables or disables automatic browser opening
func WithOpenBrowser(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.enableOpenBrowser = enabled
	}
}

func WithHost(host string) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.host = host
	}
}

// WithHTTPSEnabled sets HTTPS enabled flag
func WithHTTPSEnabled(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.httpsEnabled = enabled
	}
}

// WithHTTPSCertDir sets HTTPS certificate directory
func WithHTTPSCertDir(certDir string) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.httpsCertDir = certDir
	}
}

// WithHTTPSRegenerate sets HTTPS certificate regenerate flag
func WithHTTPSRegenerate(regenerate bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.httpsRegenerate = regenerate
	}
}

// NewServerManager creates a new server manager with default options (UI enabled, adapter enabled)
func NewServerManager(appConfig *config.AppConfig, opts ...ServerManagerOption) *ServerManager {
	// Default options
	sm := &ServerManager{
		appConfig:         appConfig,
		enableUI:          true, // Default: UI enabled
		enableAdaptor:     true, // Default: adapter enabled
		enableOpenBrowser: true, // Default: browser auto-open enabled
	}

	// Apply provided options
	for _, opt := range opts {
		opt(sm)
	}

	sm.Setup(appConfig.GetServerPort())
	return sm
}

func (sm *ServerManager) GetGinEngine() *gin.Engine {
	return sm.server.GetRouter()
}

func (sm *ServerManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin engine
	sm.server.GetRouter().ServeHTTP(w, r)
}

// Setup creates and configures the server without starting it
func (sm *ServerManager) Setup(port int) error {

	// Check if already running
	if sm.IsRunning() {
		return fmt.Errorf("server is already running")
	}

	// Set port if provided
	if port > 0 {
		if err := sm.appConfig.SetServerPort(port); err != nil {
			return fmt.Errorf("failed to set server port: %w", err)
		}
	}

	if sm.enableDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create server with UI and adaptor options
	sm.server = server.NewServer(
		sm.appConfig.GetGlobalConfig(),
		server.WithUI(sm.enableUI),
		server.WithAdaptor(sm.enableAdaptor),
		server.WithOpenBrowser(sm.enableOpenBrowser),
		server.WithHost(sm.host),
		server.WithHTTPSEnabled(sm.httpsEnabled),
		server.WithHTTPSCertDir(sm.httpsCertDir),
		server.WithHTTPSRegenerate(sm.httpsRegenerate),
		server.WithVersion(sm.appConfig.GetVersion()),
	)

	// Set global server instance for web UI control
	server.SetGlobalServer(sm.server)

	return nil
}

// Start starts the server (requires Setup to be called first)
func (sm *ServerManager) Start() error {
	if sm.server == nil {
		return fmt.Errorf("server not initialized, call Setup() first")
	}

	// Check if already running
	if sm.IsRunning() {
		return fmt.Errorf("server is already running")
	}

	if sm.enableDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	err := sm.server.Start(sm.appConfig.GetServerPort())
	if err != nil {
		return err
	}

	sm.Lock()
	defer sm.Unlock()
	sm.status = "Running"
	return nil
}

// Stop stops the server gracefully
func (sm *ServerManager) Stop() error {
	if sm.server == nil {
		sm.Cleanup()
		return nil
	}

	fmt.Println("Stopping server...")
	ctx, cancel := context.WithTimeout(context.Background(), StopTimeout)
	defer cancel()

	if err := sm.server.Stop(ctx); err != nil {
		fmt.Printf("Error stopping server: %v\n", err)
	} else {
		fmt.Println("Server stopped successfully")
	}

	sm.Cleanup()
	return nil
}

func (sm *ServerManager) Cleanup() {
}

// IsRunning checks if the server is currently running
func (sm *ServerManager) IsRunning() bool {
	return sm.status == "Running"
}
