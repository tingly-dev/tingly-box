package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/server"
	"tingly-box/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ProxyService manages the web UI and HTTP server functionality
type ProxyService struct {
	appConfig     *config.AppConfig
	serverManager *utils.ServerManager
	httpServer    *server.Server
	shutdownChan  chan struct{}
	isRunning     bool
	app           *application.App
}

// NewUIService creates a new UI service instance
func NewUIService(port int) (*ProxyService, error) {
	appConfig, err := config.NewAppConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}

	serverManager := utils.NewServerManagerWithOptions(appConfig, true)
	serverManager.Setup(port)

	res := &ProxyService{
		appConfig:     appConfig,
		serverManager: serverManager,
		shutdownChan:  make(chan struct{}),
		isRunning:     false,
	}

	return res, nil
}

// Start starts the UI service
func (s *ProxyService) Start(ctx context.Context) error {
	waitStart := make(chan any)
	go func() {
		err := s.serverManager.Start()
		if err != nil {
			panic(err)
		}
		close(waitStart)
	}()
	//<-waitStart

	return nil
}

// Stop stops the UI service gracefully
func (s *ProxyService) Stop() error {
	if !s.isRunning {
		return nil
	}

	fmt.Println("Stopping UI service...")
	err := s.serverManager.Stop()
	s.isRunning = false

	// Close shutdown channel to notify any waiting goroutines
	close(s.shutdownChan)

	return err
}

func (s *ProxyService) GetGinEngine() *gin.Engine {
	return s.serverManager.GetGinEngine()
}

// ServeHTTP implements the http.Handler interface
func (s *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
	s.serverManager.ServeHTTP(w, r)
}

// ServiceStartup is called when the service starts
func (s *ProxyService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// Store the application instance for later use
	s.app = application.Get()

	// Register an event handler that can be triggered from the frontend
	s.app.Event.On("gin-api-event", func(event *application.CustomEvent) {
		// Log the event data
		s.app.Logger.Info("Received event from frontend", "data", event.Data)

		// Emit an event back to the frontend
		s.app.Event.Emit("gin-api-response",
			map[string]interface{}{
				"message": "Response from Gin API Service",
				"time":    time.Now().Format(time.RFC3339),
			},
		)
	})

	return nil
}

// ServiceShutdown is called when the service shuts down
func (s *ProxyService) ServiceShutdown(ctx context.Context) error {
	// Clean up resources if needed
	return nil
}
