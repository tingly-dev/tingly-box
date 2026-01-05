package main

import (
	"flag"
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

var (
	portFlag    = flag.Int("port", DefaultPort, "Port number for the server")
	debugFlag   = flag.Bool("debug", false, "Enable debug mode including gin, low level logging and so on")
	verboseFlag = flag.Bool("verbose", false, "Enable verbose logging")
)

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {
	flag.Parse()

	// Set gin mode based on debug flag
	if *debugFlag {
		gin.SetMode(gin.DebugMode)
		log.Printf("[Debug] Debug mode enabled")
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Verbose logging
	if *verboseFlag {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
		log.Printf("[Verbose] Verbose logging enabled")
	}

	// Get port from flag
	port := *portFlag
	if port <= 0 || port > 65535 {
		log.Fatalf("Invalid port number: %d. Port must be between 1 and 65535.", port)
	}

	// Check if port is available before starting the app
	available, info := util.IsPortAvailableWithInfo("", port)
	log.Printf("[Port Check] Port %d: available=%v, info=%s", port, available, info)

	if !available {
		// Create a minimal error-only app
		runErrorApp(fmt.Sprintf("Port %d is already in use.\n\nPlease close the application using this port or use a different port with --port.\n\nDetails: %s", port, info))
		return
	}

	log.Printf("[Port Check] Port %d is available, starting application...", port)

	app := newApp(port, *debugFlag)
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
