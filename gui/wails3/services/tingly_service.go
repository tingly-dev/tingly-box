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
	exportpkg "github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// Wails discovers lifecycle hooks through optional interfaces, so a signature
// drift compiles but silently stops the hook from being called. These
// assertions turn that into a build error.
var (
	_ application.ServiceStartup  = (*TinglyService)(nil)
	_ application.ServiceShutdown = (*TinglyService)(nil)
	_ http.Handler                = (*TinglyService)(nil)
)

// TinglyService manages the web UI and HTTP server functionality
type TinglyService struct {
	appManager    *command.AppManager
	serverManager *command.ServerManager
	app           *application.App
}

// NewTinglyServiceWithServerManager creates a new UI service instance with a pre-configured ServerManager
func NewTinglyServiceWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager) *TinglyService {
	res := &TinglyService{
		appManager:    appManager,
		serverManager: serverManager,
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

// ServiceShutdown is called when the service shuts down. The signature must
// stay parameterless: wails discovers this hook via the optional
// application.ServiceShutdown interface (ServiceShutdown() error), and a
// mismatched signature compiles fine but is silently never called.
func (s *TinglyService) ServiceShutdown() error {
	// Clean up resources if needed
	return nil
}

// ============
// Configuration Accessors
// ============

func (s *TinglyService) GetUserAuthToken() string {
	token := s.appManager.GetGlobalConfig().GetUserToken()
	logrus.Debugf("Getting auth token %s\n", token)
	return token
}

func (s *TinglyService) GetPort() int {
	port := s.appManager.GetGlobalConfig().GetServerPort()
	logrus.Debugf("Getting port %d\n", port)
	return port
}

// ChoosePath opens a native file dialog and returns a selected file or directory path.
func (s *TinglyService) ChoosePath() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("application is not ready")
	}

	return s.app.Dialog.OpenFile().
		SetTitle("Choose File or Directory").
		CanChooseFiles(true).
		CanChooseDirectories(true).
		ShowHiddenFiles(true).
		PromptForSingleSelection()
}

// ============
// Provider Management (exposed to GUI)
// ============

// ListProviders returns all configured providers
func (s *TinglyService) ListProviders() []*typ.Provider {
	return usecase.NewProviderUseCase(s.appManager.GetGlobalConfig()).List().Providers
}

// AddProvider adds a new AI provider
func (s *TinglyService) AddProvider(name, apiBase, token, apiStyle string) (string, error) {
	result, err := usecase.NewProviderUseCase(s.appManager.GetGlobalConfig()).Add(usecase.CreateProviderRequest{
		Name: name, APIBase: apiBase, Token: token, APIStyle: protocol.APIStyle(apiStyle),
	})
	if err != nil {
		return "", fmt.Errorf("failed to add provider: %w", err)
	}
	return result.Provider.UUID, nil
}

// DeleteProvider removes an AI provider by UUID.
func (s *TinglyService) DeleteProvider(uuid string) error {
	if err := usecase.NewProviderUseCase(s.appManager.GetGlobalConfig()).Delete(usecase.DeleteProviderRequest{UUID: uuid}); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}

// GetProvider returns a provider by UUID.
func (s *TinglyService) GetProvider(uuid string) (*typ.Provider, error) {
	return s.appManager.GetGlobalConfig().GetProviderByUUID(uuid)
}

// ============
// Rule Management (exposed to GUI)
// ============

// ListRules returns all configured rules
func (s *TinglyService) ListRules() []typ.Rule {
	return usecase.NewRuleUseCase(s.appManager.GetGlobalConfig()).List().Rules
}

// ImportRule imports providers from JSONL/base64 export data. Despite the
// name (kept for call-site compatibility), only providers are imported —
// dataio export/import no longer carries rule data.
func (s *TinglyService) ImportRule(data string) (*command.ImportResult, error) {
	return command.ImportProviders(s.appManager.GetGlobalConfig(), data, exportpkg.FormatAuto, command.ImportOptions{
		Quiet: true,
	})
}
