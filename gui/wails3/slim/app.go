package main

import (
	"log"
	"path/filepath"
	"time"

	"github.com/tingly-dev/tingly-box/gui/wails3/services"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tingly-dev/tingly-box/pkg/fs"
)

const (
	AppName        = "Tingly Box"
	AppDescription = "A proxy server for AI model APIs"
)

var tinglyService *services.TinglyService

func newSlimApp(port int, debug bool) *application.App {
	// Create UI service
	home, err := fs.GetUserPath()
	if err != nil {
		log.Fatal(err)
	}
	configDir := filepath.Join(home, ".tingly-box")
	tinglyService, err = services.NewTinglyService(configDir, port, debug)
	if err != nil {
		log.Fatalf("Failed to create UI service: %v", err)
	}

	// Create a new Wails application for slim version (no embedded UI)
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Services: []application.Service{
			application.NewService(&services.GreetService{}),
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
	return app
}
