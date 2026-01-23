package main

import (
	"fmt"
	"log"

	"github.com/tingly-dev/tingly-box/gui_wails3/internal/flags"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

func main() {
	// Get port from flag
	port := flags.GetPort()
	if port <= 0 || port > 65535 {
		log.Fatalf("Invalid port number: %d. Port must be between 1 and 65535.", port)
	}

	// Check if port is available before starting the app
	available, info := network.IsPortAvailableWithInfo("localhost", port)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", port, available, info)

	if !available {
		// Show error and exit
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", port, info))
		return
	}

	log.Printf("[Port Check] Port %d is available, starting slim application...", port)

	app := newSlimApp(port, flags.GetDebug())
	useSlimSystray(app)

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
