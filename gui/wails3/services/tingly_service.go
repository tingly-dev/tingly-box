package services

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tingly-dev/tingly-box/internal/app"
)

// Wails discovers lifecycle hooks through optional interfaces, so a signature
// drift compiles but silently stops the hook from being called. These
// assertions turn that into a build error.
var (
	_ application.ServiceStartup  = (*TinglyService)(nil)
	_ application.ServiceShutdown = (*TinglyService)(nil)
	_ http.Handler                = (*TinglyService)(nil)
)

// TinglyService manages the web UI and HTTP server functionality.
//
// It is a Wails service, and Wails binds EVERY exported method (bar
// ServiceStartup/ServiceShutdown/ServeHTTP/ServiceName) for any script in
// the window to call by name. So the exported methods are exactly what the
// page uses (frontend/src/host/desktop.ts BOUND_METHODS) — pinned by
// TestBoundMethods — and everything main needs that the page must not call
// (starting the gateway, the gin engine) is unexported or a field.
type TinglyService struct {
	appManager    *app.AppManager
	serverManager *app.ServerManager
	app           *application.App

	// OpenMainWindowFn is set by main (systray.go's useSystray) so the hub
	// panel and the /api/v1/gui/open nudge can open the main app window.
	// TinglyService lives in this package and can't import main (main
	// already imports this package), hence the callback. A field rather than
	// a setter method so Wails does not bind it.
	OpenMainWindowFn func(path string)
}

// NewTinglyServiceWithServerManager creates a new UI service instance with a pre-configured ServerManager
func NewTinglyServiceWithServerManager(appManager *app.AppManager, serverManager *app.ServerManager) *TinglyService {
	res := &TinglyService{
		appManager:    appManager,
		serverManager: serverManager,
	}

	log.Printf("config file: %s\n", appManager.AppConfig().GetGlobalConfig().ConfigFile)

	return res
}

// start runs the gateway in the background. Unexported: a page script must
// never be able to start a second one (see the TinglyService doc).
func (s *TinglyService) start(ctx context.Context) error {
	go func() {
		err := s.serverManager.Start()
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (s *TinglyService) ginEngine() *gin.Engine {
	return s.serverManager.GetGinEngine()
}

// ServeHTTP implements the http.Handler interface
func (s *TinglyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// All requests go to the Gin router
	s.serverManager.ServeHTTP(w, r)
}

// ServiceStartup is called when the service starts
func (s *TinglyService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// Taken first: the routes below use it from request goroutines.
	wailsApp := application.Get()
	s.app = wailsApp

	// GUI-only route: lets the tray hub panel and a second GUI launch nudge
	// this instance to show its main window over plain HTTP — see run.go's
	// notifyRunningGUI and frontend HubPage.tsx. Registered before Start so
	// the route exists by the time the listener serves. A CLI server never
	// registers this route, so the nudge 404s there. Registered directly on
	// the engine (not the /api/v1 group) to skip that group's middleware;
	// the /api/v1 prefix keeps it reachable from the webview, whose asset
	// middleware only forwards /api and /tingly to Gin (see app.go).
	s.ginEngine().POST("/api/v1/gui/open", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+s.GetUserAuthToken() {
			c.Status(http.StatusForbidden)
			return
		}
		s.OpenMainWindow(c.Query("path"))
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Native save dialog for the desktop bridge's saveFile; see
	// save_file.go. Same token check as /gui/open.
	s.ginEngine().POST("/api/v1/gui/save", saveFileHandler(s.GetUserAuthToken, func(name string) (string, error) {
		return wailsApp.Dialog.SaveFile().
			SetFilename(name).
			CanCreateDirectories(true).
			PromptForSingleSelection()
	}))

	s.start(ctx)

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

// OpenMainWindow shows the main app window at path, creating it on first use.
// Bound for the hub panel as the fallback to its /api/v1/gui/open request.
func (s *TinglyService) OpenMainWindow(path string) {
	if s.OpenMainWindowFn != nil {
		s.OpenMainWindowFn(path)
	}
}
