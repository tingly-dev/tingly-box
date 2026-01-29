package command

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server"
)

const StopTimeout = time.Second

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig  *config.AppConfig
	server     *server.Server
	serverOpts []server.ServerOption
	status     string
	sync.Mutex
}

// NewServerManager creates a new server manager.
// opts are server options passed directly to the underlying server.
func NewServerManager(appConfig *config.AppConfig, opts ...server.ServerOption) *ServerManager {
	sm := &ServerManager{
		appConfig:  appConfig,
		serverOpts: opts,
	}
	_ = sm.Setup(appConfig.GetServerPort())
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

	// Build server options with version and pass through all provided options
	opts := append([]server.ServerOption{
		server.WithVersion(sm.appConfig.GetVersion()),
	}, sm.serverOpts...)

	sm.server = server.NewServer(sm.appConfig.GetGlobalConfig(), opts...)

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
