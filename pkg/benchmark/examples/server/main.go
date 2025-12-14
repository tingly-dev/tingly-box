package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"tingly-box/pkg/benchmark"
)

func main() {
	// Parse command line flags
	port := flag.Int("port", benchmark.DefaultPort, "Port to run the mock server on")
	flag.Parse()

	// Also support environment variable
	if envPort := os.Getenv("MOCK_SERVER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	// Create mock server with both OpenAI and Anthropic support
	server := benchmark.NewMockServer(
		benchmark.WithBothDefaults(),
		benchmark.WithPort(*port),
		benchmark.WithChatResponseContent("Hello from the OpenAI benchmark server! This is a custom response."),
		benchmark.WithMessageResponseContent("Hello from the Anthropic benchmark server! This is a custom response."),
		benchmark.WithChatDelay(benchmark.DefaultChatDelayMs/2), // Use half of default for faster response
		benchmark.WithMessageDelay(benchmark.DefaultMessageDelayMs/2), // Use half of default for faster response
		benchmark.WithRandomDelay(benchmark.DefaultRandomDelayMin, benchmark.DefaultRandomDelayMax),
	)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nShutting down server...")
		if err := server.Stop(); err != nil {
			log.Printf("Error stopping server: %v", err)
		}
		os.Exit(0)
	}()

	// Start server
	fmt.Printf("Starting mock server on port %d\n", server.Port())
	fmt.Printf("\nUsage:\n")
	fmt.Printf("  %s [-port PORT]\n", os.Args[0])
	fmt.Printf("  or set MOCK_SERVER_PORT environment variable\n")
	fmt.Printf("\nEndpoints:\n")
	fmt.Printf("OpenAI:\n")
	fmt.Printf("  - http://localhost:%d/openai/v1/models\n", server.Port())
	fmt.Printf("  - http://localhost:%d/openai/v1/chat/completions\n", server.Port())
	fmt.Printf("\nAnthropic:\n")
	fmt.Printf("  - http://localhost:%d/anthropic/v1/messages\n", server.Port())
	fmt.Printf("\nGeneric (backward compatibility, defaults to OpenAI):\n")
	fmt.Printf("  - http://localhost:%d/v1/models\n", server.Port())
	fmt.Printf("  - http://localhost:%d/v1/chat/completions\n", server.Port())
	fmt.Println("\nPress Ctrl+C to stop")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}