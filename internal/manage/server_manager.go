package manage

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/server"

	"github.com/gin-gonic/gin"
)

const StopTimeout = time.Second * 10

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig     *config.AppConfig
	server        *server.Server
	enableUI      bool
	enableAdaptor bool
	enableDebug   bool
	status        string
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

// WithAdaptor enables or disables the adaptor for the server manager
func WithDebug(enabled bool) ServerManagerOption {
	return func(sm *ServerManager) {
		sm.enableDebug = enabled
	}
}

// NewServerManager creates a new server manager with default options (UI enabled, adaptor disabled)
func NewServerManager(appConfig *config.AppConfig, opts ...ServerManagerOption) *ServerManager {
	// Default options
	sm := &ServerManager{
		appConfig:     appConfig,
		enableUI:      true,  // Default: UI enabled
		enableAdaptor: false, // Default: adaptor disabled
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

	sm.appConfig.GetGlobalConfig().SetDebug(sm.enableDebug)

	// Create server with UI and adaptor options
	sm.server = server.NewServerWithAllOptions(sm.appConfig.GetGlobalConfig(), sm.enableUI, sm.enableAdaptor)

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

	// Start server synchronously (blocking)
	fmt.Printf("Starting server on port %d...\n", sm.appConfig.GetServerPort())

	gin.SetMode(gin.ReleaseMode)
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
