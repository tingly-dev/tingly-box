package main

import (
	"log"
	"time"

	"tingly-box/internal/util"
	"tingly-box/wails3/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	AppName        = "Model Box"
	AppDescription = "A proxy server for AI model APIs with web UI"
)

var App *application.App
var uiService *services.ProxyService

func newApp() *application.App {
	// Create UI service
	home, err := util.GetUserPath()
	uiService, err = services.NewUIService(home, 12580)
	if err != nil {
		log.Fatalf("Failed to create UI service: %v", err)
	}

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Services' is a lis t of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	embdHandler := application.AssetFileServerFS(assets)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(&services.GreetService{}),
			application.NewService(uiService),
		},
		Assets: application.AssetOptions{
			Handler: embdHandler,
		},
		//Assets: application.AssetOptions{
		//	Middleware: func(next http.Handler) http.Handler {
		//		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//			// Let Wails handle the `/wails/` route
		//			if strings.HasPrefix(r.URL.Path, "/wails") {
		//				// Let API handle everything else
		//				next.ServeHTTP(w, r)
		//				return
		//			}
		//
		//			if strings.HasPrefix(r.URL.Path, "/api") {
		//				// Let API handle everything else
		//				ginEngine.ServeHTTP(w, r)
		//				return
		//			}
		//
		//			// Let Wails handle the `/wails/` route
		//			if strings.HasPrefix(r.URL.Path, "/dashboard") {
		//				next.ServeHTTP(w, r)
		//				return
		//			}
		//
		//			embdHandler.ServeHTTP(w, r)
		//			return
		//		})
		//	},
		//},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "tingly-model-box.single-instance",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if WindowMain != nil {
					WindowMain.EmitEvent("secondInstanceLaunched", data)
					WindowMain.Restore()
					WindowMain.Focus()
				}
			},
			AdditionalData: map[string]string{
				"launchtime": time.Now().Local().String(),
			},
			ExitCode:      0,
			EncryptionKey: [32]byte([]byte("Ml!Zjj@Lfw#Wqq$Wxb%Mjy^&*()_+1234567890-=")[:32]),
		},
	})
	return app
}
