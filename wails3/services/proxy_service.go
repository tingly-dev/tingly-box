package services

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
	"tingly-box/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// UIService manages the web UI and HTTP server functionality
type UIService struct {
	appConfig     *config.AppConfig
	serverManager *utils.ServerManager
	httpServer    *server.Server
	shutdownChan  chan struct{}
	isRunning     bool
	app           *application.App
}

// NewUIService creates a new UI service instance
func NewUIService() (*UIService, error) {
	appConfig, err := config.NewAppConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}

	enableUI := true
	appConfig.SetServerPort(8080)
	//port := appConfig.GetServerPort()

	serverManager := utils.NewServerManagerWithOptions(appConfig, enableUI)

	res := &UIService{
		appConfig:     appConfig,
		serverManager: serverManager,
		shutdownChan:  make(chan struct{}),
		isRunning:     false,
	}

	waitStart := make(chan any)
	go func() {
		err := res.Start(context.Background())
		if err != nil {
			panic(err)
		}
		close(waitStart)
	}()
	<-waitStart

	return res, nil
}

// Start starts the UI service
func (s *UIService) Start(ctx context.Context) error {
	if s.isRunning {
		return fmt.Errorf("UI service is already running")
	}

	port := s.appConfig.GetServerPort()
	fmt.Printf("Starting UI service on port %d\n", port)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	err := s.serverManager.Setup(8080)

	go s.serverManager.Start()

	s.isRunning = true
	return err
}

// Stop stops the UI service gracefully
func (s *UIService) Stop() error {
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

func (s *UIService) GetGin() *gin.Engine {
	return s.serverManager.GetGin()
}

// ServeHTTP implements the http.Handler interface
func (s *UIService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
	s.serverManager.ServeHTTP(w, r)
}

// ServiceStartup is called when the service starts
func (s *UIService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
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
func (s *UIService) ServiceShutdown(ctx context.Context) error {
	// Clean up resources if needed
	return nil
}
