package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/server"

	"github.com/gin-gonic/gin"
)

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig  *config.AppConfig
	server     *server.Server
	pidManager *config.PIDManager
	enableUI   bool
}

// NewServerManager creates a new server manager with UI enabled by default
func NewServerManager(appConfig *config.AppConfig) *ServerManager {
	res := NewServerManagerWithOptions(appConfig, true)
	res.Setup(appConfig.GetServerPort())
	return res
}

// NewServerManagerWithOptions creates a new server manager with UI option
func NewServerManagerWithOptions(appConfig *config.AppConfig, enableUI bool) *ServerManager {
	res := &ServerManager{
		appConfig:  appConfig,
		pidManager: config.NewPIDManager(),
		enableUI:   enableUI,
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

	// Create server with UI option
	sm.server = server.NewServerWithOptions(sm.appConfig.GetGlobalConfig(), sm.enableUI)

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

	// Create PID file
	if err := sm.pidManager.CreatePIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	// Start server synchronously (blocking)
	fmt.Printf("Starting server on port %d...\n", sm.appConfig.GetServerPort())

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	return sm.server.Start(sm.appConfig.GetServerPort())
}

// StartWithPort sets up and starts the server in one call (legacy behavior)
func (sm *ServerManager) StartWithPort(port int) error {
	if err := sm.Setup(port); err != nil {
		return err
	}
	return sm.Start()
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

// Cleanup removes PID file
func (sm *ServerManager) Cleanup() {
	sm.pidManager.RemovePIDFile()
}

// IsRunning checks if the server is currently running
func (sm *ServerManager) IsRunning() bool {
	return sm.pidManager.IsRunning()
}
