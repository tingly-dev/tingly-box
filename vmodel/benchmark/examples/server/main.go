// Stand-alone benchmark mock server: starts a local HTTP server backed by
// the production virtualmodel registries (with their default mock models
// pre-registered) so external benchmark drivers can hit a realistic vmodel
// surface over loopback.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
)

func main() {
	port := flag.Int("port", benchmark.DefaultPort, "Port to run the mock server on")
	flag.Parse()

	if envPort := os.Getenv("MOCK_SERVER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	addr := fmt.Sprintf(":%d", *port)
	srv, err := benchmark.NewLocalServer(addr)
	if err != nil {
		log.Fatalf("start benchmark server: %v", err)
	}
	defer srv.Close()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nShutting down server...")
		_ = srv.Close()
		os.Exit(0)
	}()

	fmt.Printf("Starting mock server on port %d\n", srv.Port())
	fmt.Printf("\nUsage:\n")
	fmt.Printf("  %s [-port PORT]\n", os.Args[0])
	fmt.Printf("  or set MOCK_SERVER_PORT environment variable\n")
	fmt.Printf("\nEndpoints (mounted under /v1, /openai/v1, /anthropic/v1):\n")
	fmt.Printf("  GET  /v1/models\n")
	fmt.Printf("  POST /v1/chat/completions\n")
	fmt.Printf("  POST /v1/messages\n")

	// Block forever until the goroutine above calls os.Exit.
	select {}
}
