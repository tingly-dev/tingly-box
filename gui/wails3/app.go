package main

import (
	"net/http"
	"path"
	"strings"

	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"

	assets "github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/command"
)

const (
	AppName        = "Tingly Box"
	AppDescription = "A proxy server for AI model APIs"
)

var App *application.App
var tinglyService *services.TinglyService

// newAppWithServerManager creates a new GUI app with a pre-configured ServerManager.
// macActivationPolicy controls dock visibility: use ActivationPolicyRegular for full GUI
// (dock icon visible) or ActivationPolicyAccessory for tray-only (no dock icon).
func newAppWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager, debug bool, macActivationPolicy application.ActivationPolicy) *application.App {
	// Create UI service with existing serverManager
	tinglyService = services.NewTinglyServiceWithServerManager(appManager, serverManager)

	// Create a new Wails application by providing the necessary options.
	embdHandler := application.AssetFileServerFS(assets.GUIDistAssets)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(tinglyService),
		},
		Assets: application.AssetOptions{
			Handler: tinglyService.GetGinEngine(),
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

					// Wails internal routes - let Wails handle them
					if strings.HasPrefix(r.URL.Path, "/wails") {
						next.ServeHTTP(w, r)
						return
					}

					// API routes - forward to Gin engine (via TinglyService)
					if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/tingly") {
						tinglyService.ServeHTTP(w, r)
						return
					}

					// SPA fallback: client-side routes like /login/<token> have no
					// matching file in the embedded dist, and the wails asset file
					// server would 404 (blank window). Rewrite document navigations
					// to "/" so index.html is served; BrowserRouter still sees the
					// original route from the webview location. Keyed off the Accept
					// header so extension-less vite dev-server module requests such
					// as /@vite/client (Accept: */*) pass through untouched.
					if r.Method == http.MethodGet && path.Ext(r.URL.Path) == "" &&
						strings.Contains(r.Header.Get("Accept"), "text/html") {
						r.URL.Path = "/"
					}

					embdHandler.ServeHTTP(w, r)
				})
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			ActivationPolicy: macActivationPolicy,
		},
		Windows: application.WindowsOptions{},
	})

	return app
}
