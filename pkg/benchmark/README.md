# Benchmark Package

A comprehensive benchmark package for testing LLM API proxies with support for OpenAI and Anthropic compatible endpoints.

## Features

- **Mock Server**: Configurable mock server that simulates OpenAI and Anthropic API endpoints
- **Benchmark Client**: High-performance client for load testing services
- **Flexible Configuration**: JSON-based configuration for custom responses, delays, and behavior
- **Multiple Providers**: Support for both OpenAI and Anthropic API formats
- **Gin Framework**: Built on Gin HTTP framework for better performance and extensibility
- **Detailed Metrics**: Comprehensive performance metrics including response times, throughput, and error rates

## Quick Start

### 1. Create a Configuration

```bash
# Create a sample configuration file
go run pkg/benchmark/examples/server/main.go config.json
```

### 2. Start the Mock Server

```bash
# Edit the config.json file as needed, then run:
go run pkg/benchmark/examples/server/main.go config.json
```

### 3. Run Benchmarks

```bash
# Run benchmark tests
go run pkg/benchmark/examples/client/main.go
```

## Configuration

The mock server uses a JSON configuration file to define its behavior:

```json
{
  "port": 8080,
  "provider": "openai",
  "models": {
    "defaultList": [
      {
        "id": "gpt-3.5-turbo",
        "object": "model",
        "created": 1677610602,
        "owned_by": "openai"
      }
    ]
  },
  "chat": {
    "defaultResponses": [
      {
        "id": "chatcmpl-123",
        "object": "chat.completion",
        "created": 1677652288,
        "model": "gpt-3.5-turbo",
        "choices": [
          {
            "index": 0,
            "message": {
              "role": "assistant",
              "content": "Hello! This is a mock response."
            },
            "finish_reason": "stop"
          }
        ],
        "usage": {
          "prompt_tokens": 10,
          "completion_tokens": 9,
          "total_tokens": 19
        }
      }
    ],
    "loopResponses": true,
    "delayMs": 100
  },
  "message": {
    "defaultResponses": [...],
    "loopResponses": true,
    "delayMs": 150
  }
}
```

### Configuration Options

- **port**: Server port (default: 8080)
- **provider**: API provider type ("openai" or "anthropic")
- **models.defaultList**: List of models to return from `/v1/models`
- **chat.defaultResponses**: Array of chat completion responses to cycle through
- **chat.loopResponses**: Whether to loop through responses (default: true)
- **chat.delayMs**: Artificial delay in milliseconds for chat responses
- **message**: Similar configuration for Anthropic message endpoints

## Architecture

The mock server is built using the Gin HTTP framework, providing:
- High performance and low memory footprint
- Flexible routing with middleware support
- Built-in CORS support for cross-origin requests
- Easy extensibility for new endpoints and features

## API Endpoints

### OpenAI Compatible Endpoints

- `GET /v1/models` - List available models
- `POST /v1/chat/completions` - Create chat completion
- `GET /openai/v1/models` - OpenAI-specific models endpoint
- `POST /openai/v1/chat/completions` - OpenAI-specific chat endpoint

### Anthropic Compatible Endpoints

- `POST /v1/messages` - Create message
- `POST /anthropic/v1/messages` - Anthropic-specific message endpoint

## Usage Examples

### Using the Mock Server Directly

```go
package main

import (
    "log"
    "your-project/pkg/benchmark"
)

func main() {
    // Create a mock server with custom options
    server := benchmark.NewMockServer(
        benchmark.WithPort(8080),
        benchmark.WithOpenAIDefaults(),
        benchmark.WithChatDelay(100),
    )

    // Start the server
    log.Printf("Starting mock server on port %d", server.Port())
    if err := server.Start(); err != nil {
        log.Fatal(err)
    }
}
```

### Using the Benchmark Client

```go
package main

import (
    "fmt"
    "time"
    "your-project/pkg/benchmark"
)

func main() {
    // Create client
    client := benchmark.NewBenchmarkClient(&benchmark.BenchmarkOptions{
        BaseURL:  "http://localhost:8080",
        Provider: "openai",
        APIKey:   "sk-test-key",
        Timeout:  30 * time.Second,
        MaxConns: 100,
    })

    // Test models endpoint
    result, err := client.TestModelsEndpoint(10, 100)
    if err != nil {
        panic(err)
    }
    result.PrintSummary()

    // Test chat completions
    messages := []benchmark.ChatMessage{
        {Role: "user", Content: "Hello, how are you?"},
    }
    result, err = client.TestChatEndpoint("gpt-3.5-turbo", messages, 20, 200)
    if err != nil {
        panic(err)
    }
    result.PrintSummary()
}
```

### Running Tests

```bash
# Run all tests
go test ./pkg/benchmark/...

# Run with verbose output
go test -v ./pkg/benchmark/...

# Run benchmarks
go test -bench=. ./pkg/benchmark/...
```

## Metrics

The benchmark client provides comprehensive performance metrics:

- **Total Requests**: Number of requests sent
- **Success/Failed Requests**: Success and failure counts
- **Response Times**: Average, minimum, and maximum response times
- **Throughput**: Requests per second
- **Error Rate**: Percentage of failed requests
- **Status Code Distribution**: Breakdown by HTTP status codes
- **Total Bytes**: Total amount of data transferred

## Contributing

When adding new features:

1. Ensure backward compatibility
2. Add appropriate tests
3. Update documentation
4. Follow Go conventions and best practices

## License

This package is part of the tingly-box project.