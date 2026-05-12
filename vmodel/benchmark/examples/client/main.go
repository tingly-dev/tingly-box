// Stand-alone benchmark client driver: starts an in-process LocalServer
// (vmodel-backed) and drives it with the BenchmarkClient against both the
// OpenAI Chat and Anthropic Messages routes, printing a metrics summary.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
)

const (
	DefaultMaxTokens     = 1000
	DefaultConcurrency   = 25
	DefaultTotalRequests = 150
)

func main() {
	port := flag.Int("port", 0, "Listen port (0 picks an ephemeral port)")
	flag.Parse()

	addr := ""
	if *port > 0 {
		addr = fmt.Sprintf(":%d", *port)
	}
	srv, err := benchmark.NewLocalServer(addr)
	if err != nil {
		log.Fatalf("start benchmark server: %v", err)
	}
	defer srv.Close()

	fmt.Printf("=== Multi-Provider vmodel Benchmark ===\n")
	fmt.Printf("Server listening on %s\n\n", srv.BaseURL())

	// OpenAI side: hit /openai/v1/{models,chat/completions}
	fmt.Println("1. OpenAI endpoints (/openai/v1/*)")
	driveOpenAI(srv.BaseURL() + "/openai")

	// Anthropic side: hit /anthropic/v1/messages
	fmt.Println("\n2. Anthropic endpoints (/anthropic/v1/*)")
	driveAnthropic(srv.BaseURL() + "/anthropic")
}

func driveOpenAI(baseURL string) {
	client := benchmark.NewBenchmarkClient(&benchmark.BenchmarkOptions{
		BaseURL:  baseURL,
		Timeout:  10 * time.Second,
		Provider: "openai",
		MaxConns: 100,
	})

	fmt.Println("\n  a) /v1/models")
	if r, err := client.TestModelsEndpoint(10, 100); err != nil {
		log.Printf("     models test failed: %v", err)
	} else {
		r.PrintSummary()
	}

	fmt.Println("\n  b) /v1/chat/completions")
	messages := []map[string]interface{}{
		{"role": "user", "content": "Hello! Please respond with a short message."},
	}
	if r, err := client.TestChatEndpoint("virtual-gpt-4", messages, 30, 300); err != nil {
		log.Printf("     chat test failed: %v", err)
	} else {
		r.PrintSummary()
	}
}

func driveAnthropic(baseURL string) {
	client := benchmark.NewBenchmarkClient(&benchmark.BenchmarkOptions{
		BaseURL:  baseURL,
		Timeout:  10 * time.Second,
		Provider: "anthropic",
		MaxConns: 80,
	})

	fmt.Println("\n  a) /v1/messages")
	messages := []map[string]interface{}{
		{"role": "user", "content": "Hello! Please respond with a short message."},
	}
	if r, err := client.TestMessagesEndpoint("virtual-claude-3", messages, DefaultMaxTokens, DefaultConcurrency, DefaultTotalRequests); err != nil {
		log.Printf("     messages test failed: %v", err)
	} else {
		r.PrintSummary()
	}
}
