package main

import (
	"fmt"
	"log"

	"github.com/tingly-dev/tingly-box/gui_wails3/internal/flags"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	// application.RegisterEvent[string]("time")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
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
		// Create a minimal error-only app
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", port, info))
		return
	}

	log.Printf("[Port Check] Port %d is available, starting application...", port)

	app := newApp(port, flags.GetDebug())
	useWindows(app)
	useSystray(app)

	// Create a goroutine that emits an event containing the current time every second.
	// The frontend can listen to this event and update the UI accordingly.
	// go func() {
	// 	for {
	// 		now := time.Now().Format(time.RFC1123)
	// 		app.Event.Emit("time", now)
	// 		time.Sleep(time.Second)
	// 	}
	// }()

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
