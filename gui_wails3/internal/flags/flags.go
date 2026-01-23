package flags

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"
)

func init() {
	// Parse flags early before Wails initialization
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
}

const (
	DefaultPort = 12580
)

var (
	portFlag    = flag.Int("port", DefaultPort, "Port number for the server")
	debugFlag   = flag.Bool("debug", false, "Enable debug mode including gin, low level logging and so on")
	verboseFlag = flag.Bool("verbose", false, "Enable verbose logging")
)

// GetPort returns the port number from flags
func GetPort() int {
	return *portFlag
}

// GetDebug returns the debug flag value
func GetDebug() bool {
	return *debugFlag
}

// GetVerbose returns the verbose flag value
func GetVerbose() bool {
	return *verboseFlag
}
