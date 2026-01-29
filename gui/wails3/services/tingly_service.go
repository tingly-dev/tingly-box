package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TinglyService manages the web UI and HTTP server functionality
type TinglyService struct {
	appManager    *command.AppManager
	serverManager *command.ServerManager
	httpServer    *server.Server
	shutdownChan  chan struct{}
	isRunning     bool
	app           *application.App
}

// NewTinglyService creates a new UI service instance
func NewTinglyService(configDir string, port int, debug bool) (*TinglyService, error) {
	// Create AppManager
	appManager, err := command.NewAppManager(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create app manager: %w", err)
	}

	// Set port
	if err := appManager.SetServerPort(port); err != nil {
		return nil, fmt.Errorf("failed to set server port: %w", err)
	}

	serverManager := command.NewServerManager(
		appManager.AppConfig(),
		server.WithDebug(debug),
		server.WithUI(true),
		server.WithAdaptor(true),
		server.WithOpenBrowser(false), // GUI doesn't need browser auto-open
	)

	res := &TinglyService{
		appManager:    appManager,
		serverManager: serverManager,
		shutdownChan:  make(chan struct{}),
		isRunning:     false,
	}

	log.Printf("config file: %s\n", appManager.AppConfig().GetGlobalConfig().ConfigFile)

	return res, nil
}

// NewTinglyServiceWithServerManager creates a new UI service instance with a pre-configured ServerManager
func NewTinglyServiceWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager) *TinglyService {
	res := &TinglyService{
		appManager:    appManager,
		serverManager: serverManager,
		shutdownChan:  make(chan struct{}),
		isRunning:     false,
	}

	log.Printf("config file: %s\n", appManager.AppConfig().GetGlobalConfig().ConfigFile)

	return res
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

// ============
// Configuration Accessors
// ============

func (s *TinglyService) GetUserAuthToken() string {
	logrus.Debugf("Getting auth token %s\n", s.appManager.GetUserToken())
	return s.appManager.GetUserToken()
}

func (s *TinglyService) GetPort() int {
	logrus.Debugf("Getting port %d\n", s.appManager.GetServerPort())
	return s.appManager.GetServerPort()
}

// ============
// Provider Management (exposed to GUI)
// ============

// ListProviders returns all configured providers
func (s *TinglyService) ListProviders() []*typ.Provider {
	return s.appManager.ListProviders()
}

// AddProvider adds a new AI provider
func (s *TinglyService) AddProvider(name, apiBase, token, apiStyle string) error {
	return s.appManager.AddProvider(name, apiBase, token, protocol.APIStyle(apiStyle))
}

// DeleteProvider removes an AI provider by name
func (s *TinglyService) DeleteProvider(name string) error {
	return s.appManager.DeleteProvider(name)
}

// GetProvider returns a provider by name
func (s *TinglyService) GetProvider(name string) (*typ.Provider, error) {
	return s.appManager.GetProvider(name)
}

// ============
// Rule Management (exposed to GUI)
// ============

// ListRules returns all configured rules
func (s *TinglyService) ListRules() []typ.Rule {
	return s.appManager.ListRules()
}

// ImportRule imports a rule from JSONL format
func (s *TinglyService) ImportRule(data string) (*command.ImportResult, error) {
	return s.appManager.ImportRuleFromJSONL(data, command.ImportOptions{
		OnProviderConflict: "use",
		OnRuleConflict:     "skip",
		Quiet:              true,
	})
}
