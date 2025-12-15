package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig *config.AppConfig
	server    *Server
	useUI     bool
	enableAdaptor bool
	status    string
	sync.Mutex
}

// NewServerManager creates a new server manager with UI enabled by default
func NewServerManager(appConfig *config.AppConfig) *ServerManager {
	res := NewServerManagerWithOptions(appConfig, true, false)
	res.Setup(appConfig.GetServerPort())
	return res
}

// NewServerManagerWithOptions creates a new server manager with UI option
func NewServerManagerWithOptions(appConfig *config.AppConfig, useUI bool, enableAdaptor bool) *ServerManager {
	res := &ServerManager{
		appConfig:     appConfig,

		useUI:         useUI,
		enableAdaptor: enableAdaptor,
	}
	res.Setup(appConfig.GetServerPort())
	return res
}

func (sm *ServerManager) GetGinEngine() *gin.Engine {
	return sm.server.GetRouter()
}

func (sm *ServerManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
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

	// Create server with UI and adaptor options
	sm.server = NewServerWithAllOptions(sm.appConfig.GetGlobalConfig(), sm.useUI, sm.enableAdaptor)

	// Set global server instance for web UI control
	SetGlobalServer(sm.server)

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
