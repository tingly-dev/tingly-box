package utils

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/server"
)

// ServerManager manages the HTTP server lifecycle
type ServerManager struct {
	appConfig  *config.AppConfig
	server     *server.Server
	pidManager *config.PIDManager
}

// NewServerManager creates a new server manager
func NewServerManager(appConfig *config.AppConfig) *ServerManager {
	return &ServerManager{
		appConfig:  appConfig,
		pidManager: config.NewPIDManager(),
	}
}

// Start starts the server
func (sm *ServerManager) Start(port int) error {
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

	// Create server
	sm.server = server.NewServer(sm.appConfig)

	// Create PID file
	if err := sm.pidManager.CreatePIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	// Start server synchronously (blocking)
	fmt.Printf("Starting server on port %d...\n", sm.appConfig.GetServerPort())

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- sm.server.Start(sm.appConfig.GetServerPort())
	}()

	// Wait for either server error or shutdown signal
	select {
	case err := <-serverErr:
		// Server stopped with error
		sm.Cleanup()
		return fmt.Errorf("server stopped unexpectedly: %w", err)
	case <-sigChan:
		// Received shutdown signal
		fmt.Println("\nReceived shutdown signal")
		return sm.Stop()
	}
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