package main

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"

	services2 "github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"

	assets "github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

const (
	AppName        = "Tingly Box"
	AppDescription = "A proxy server for AI model APIs"
)

var App *application.App
var tinglyService *services2.TinglyService

func newApp(port int, debug bool) *application.App {
	// Create UI service
	home, err := fs.GetUserPath()
	if err != nil {
		log.Fatal(err)
	}
	configDir := filepath.Join(home, ".tingly-box")
	tinglyService, err = services2.NewTinglyService(configDir, port, debug)
	if err != nil {
		log.Fatalf("Failed to create UI service: %v", err)
	}

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Services' is a lis t of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	// embdHandler := application.AssetFileServerFS(assets.GUIDistAssets)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(&services2.GreetService{}),
			application.NewService(tinglyService),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{},
		//SingleInstance: &application.SingleInstanceOptions{
		//	UniqueID: "tingly-box.single-instance",
		//	OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
		//		if WindowMain != nil {
		//			WindowMain.EmitEvent("secondInstanceLaunched", data)
		//			WindowMain.Restore()
		//			WindowMain.Focus()
		//		}
		//	},
		//	AdditionalData: map[string]string{
		//		"launchtime": time.Now().Local().String(),
		//	},
		//	ExitCode:      0,
		//	EncryptionKey: [32]byte([]byte("Ml!Zjj@Lfw#Wqq$Wxb%Mjy^&*()_+1234567890-=")[:32]),
		//},
	})
	return app
}

// newAppWithServerManager creates a new full GUI app with a pre-configured ServerManager
func newAppWithServerManager(appManager *command.AppManager, serverManager *command.ServerManager, debug bool) *application.App {
	// Create UI service with existing serverManager
	tinglyService = services2.NewTinglyServiceWithServerManager(appManager, serverManager)

	// Create a new Wails application by providing the necessary options.

	embdHandler := application.AssetFileServerFS(assets.GUIDistAssets)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(&services2.GreetService{}),
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

					embdHandler.ServeHTTP(w, r)
				})
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{},
		//SingleInstance: &application.SingleInstanceOptions{
		//	UniqueID: "tingly-box.single-instance",
		//	OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
		//		if WindowMain != nil {
		//			WindowMain.EmitEvent("secondInstanceLaunched", data)
		//			WindowMain.Restore()
		//			WindowMain.Focus()
		//		}
		//	},
		//	AdditionalData: map[string]string{
		//		"launchtime": time.Now().Local().String(),
		//	},
		//	ExitCode:      0,
		//	EncryptionKey: [32]byte([]byte("Ml!Zjj@Lfw#Wqq$Wxb%Mjy^&*()_+1234567890-=")[:32]),
		//},
	})

	// IMPORTANT: Set up windows and systray after creating the app
	useWindows(app)
	useSystray(app)

	return app
}
