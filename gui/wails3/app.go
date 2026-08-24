package main

import (
	"log"
	"net/http"
	"path"
	"strings"
	"time"

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

// newSlimAppWithServerManager creates a new slim GUI app with a pre-configured ServerManager
// This is in the main package because the slim package has its own main() and cannot be imported
func newSlimAppWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager, debug bool) *application.App {
	// Create UI service with existing serverManager
	tinglyService := services.NewTinglyServiceWithServerManager(appManager, serverManager)

	// Create a new Wails application for slim version
	// Now with embedded UI assets for the local window
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(tinglyService),
		},
		// No Assets handler - slim version opens browser instead
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			ActivationPolicy: application.ActivationPolicyAccessory, // Tray-only: no dock icon, no default window
		},
		Windows: application.WindowsOptions{},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "tingly-box.slim.single-instance",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// Just focus/notify - slim version has no window to restore
				log.Printf("Second instance launch detected: %v", data)
			},
			AdditionalData: map[string]string{
				"launchtime": time.Now().Local().String(),
			},
			ExitCode:      0,
			EncryptionKey: [32]byte([]byte("Ml!Zjj@Lfw#Wqq$Wxb%Mjy^&*()_+1234567890-=")[:32]),
		},
	})

	// IMPORTANT: Set up systray after creating the app
	useSlimSystray(app, tinglyService)

	return app
}
