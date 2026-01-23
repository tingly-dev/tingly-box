package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server"
)

// TinglyService manages the web UI and HTTP server functionality
type TinglyService struct {
	appConfig     *config.AppConfig
	serverManager *command.ServerManager
	httpServer    *server.Server
	shutdownChan  chan struct{}
	isRunning     bool
	app           *application.App
}

// NewTinglyService creates a new UI service instance
func NewTinglyService(configDir string, port int, debug bool) (*TinglyService, error) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}

	appConfig.SetServerPort(port)

	serverManager := command.NewServerManager(
		appConfig,
		command.WithUI(true),
		command.WithAdaptor(true),
		command.WithDebug(debug),
		command.WithOpenBrowser(false), // GUI doesn't need browser auto-open
	)

	res := &TinglyService{
		appConfig:     appConfig,
		serverManager: serverManager,
		shutdownChan:  make(chan struct{}),
		isRunning:     false,
	}

	log.Printf("config file: %s\n", appConfig.GetGlobalConfig().ConfigFile)

	return res, nil
}

// Start starts the UI service synchronously and returns any error
func (s *TinglyService) Start(ctx context.Context) error {
	go func() {
		err := s.serverManager.Start()
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

// Stop stops the UI service gracefully
func (s *TinglyService) Stop() error {
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

func (s *TinglyService) GetGinEngine() *gin.Engine {
	return s.serverManager.GetGinEngine()
}

// ServeHTTP implements the http.Handler interface
func (s *TinglyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
	s.serverManager.ServeHTTP(w, r)
}

// ServiceStartup is called when the service starts
func (s *TinglyService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.Start(ctx)

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
func (s *TinglyService) ServiceShutdown(ctx context.Context) error {
	// Clean up resources if needed
	return nil
}

func (s *TinglyService) GetUserAuthToken() string {
	fmt.Println("Getting auth token")
	return s.appConfig.GetGlobalConfig().GetUserToken()
}

func (s *TinglyService) GetPort() int {
	fmt.Println("Getting port")
	return s.appConfig.GetGlobalConfig().GetServerPort()
}
