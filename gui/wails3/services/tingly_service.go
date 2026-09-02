package services

import (
	"context"
	"fmt"
	"log"
	"net/http"

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

	// openMainWindowFn is set by main (see run.go's useWebSystray) so the
	// frontend's tray hub panel can open the main app window via a direct
	// bound method call (OpenMainWindow) instead of a fire-and-forget
	// Events.Emit round trip. TinglyService lives in this package and can't
	// import main (main already imports this package), hence the callback.
	openMainWindowFn func(path string)
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
	// GUI-only route: lets the tray hub panel and a second GUI launch nudge
	// this instance to show its main window over plain HTTP — see run.go's
	// notifyRunningGUI and frontend HubPage.tsx. Registered before Start so
	// the route exists by the time the listener serves. A CLI server never
	// registers this route, so the nudge 404s there. Registered directly on
	// the engine (not the /api/v1 group) to skip that group's middleware;
	// the /api/v1 prefix keeps it reachable from the webview, whose asset
	// middleware only forwards /api and /tingly to Gin (see app.go).
	s.GetGinEngine().POST("/api/v1/gui/open", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+s.GetUserAuthToken() {
			c.Status(http.StatusForbidden)
			return
		}
		s.OpenMainWindow(c.Query("path"))
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	s.Start(ctx)

	// Store the application instance for later use
	s.app = application.Get()

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

// SetOpenMainWindowHandler wires the tray hub panel's "open the main app
// window" action to main's window management. Called once from
// useWebSystray after both the service and the windows exist.
func (s *TinglyService) SetOpenMainWindowHandler(fn func(path string)) {
	s.openMainWindowFn = fn
}

// OpenMainWindow shows/maximises the main app window at path, creating it on
// first use. Exposed as a bound method (rather than an Events.Emit) so the
// tray hub panel gets a direct, awaitable call instead of a fire-and-forget
// event - simpler to reason about and to debug than a pub/sub round trip.
func (s *TinglyService) OpenMainWindow(path string) {
	if s.openMainWindowFn != nil {
		s.openMainWindowFn(path)
	}
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
