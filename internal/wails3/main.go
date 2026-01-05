package main

import (
	"fmt"
	"log"

	"tingly-box/internal/util"

	"github.com/gin-gonic/gin"
)

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	// application.RegisterEvent[string]("time")
}

const (
	DefaultPort = 12580
)

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {
	gin.SetMode(gin.ReleaseMode)

	// Check if port is available before starting the app
	available, info := util.IsPortAvailableWithInfo("localhost", DefaultPort)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", DefaultPort, available, info)

	if !available {
		// Create a minimal error-only app
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or change the port in settings.\n\nDetails: %s", DefaultPort, info))
		return
	}

	log.Printf("[Port Check] Port %d is available, starting application...", DefaultPort)

	app := newApp()
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
