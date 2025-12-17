# Tingly Box

A **provider-agnostic AI model proxy** that exposes a unified OpenAI-compatible API endpoint while routing requests to multiple configured AI providers. It consists of both a **CLI tool** for management and a **service** for handling requests, acting as a middleware layer between your applications and various AI service providers.

## Core Features

**1. Multi-Provider Support with Unified API**
   - Connect to OpenAI, Anthropic, and custom AI providers simultaneously
   - Single OpenAI-compatible endpoint for all providers
   - Switch between providers without changing application code
   - Provider-agnostic architecture prevents vendor lock-in

**2. Config-Based Request Forwarding**
   - Route requests to specific providers based on model configuration
   - Pooled sharing & Load balancing between provider endpoints (same/different providers, same/different models, or different tokens)
   - Real-time streaming support for chat completions
   - Format adaptation between OpenAI and Anthropic APIs (experimental)

**3. User-Friendly Management UI**
   - Intuitive web interface for provider configuration
   - Visual dashboard for monitoring and status
   - Simple token and provider management
   - Easy provider addition and removal

## Quick Start

### Prerequisites

- **Go**: Version 1.21 or later
- **Node.js**: Version 18 or later (for web UI development)

### Installation

```bash
# Build the CLI binary
go build ./cmd/tingly

# Move to system PATH
sudo mv tingly /usr/local/bin/

# Or add to your PATH in ~/.bashrc or ~/.zshrc
export PATH="$PATH:/path/to/tingly-box"
```

### Basic Usage

```bash
# 1. Add AI providers
./tingly add openai https://api.openai.com/v1 sk-your-openai-token
./tingly add anthropic https://api.anthropic.com sk-your-anthropic-token

# 2. Generate access token
./tingly token

# 3. Start the server
./tingly start --port 8080

# 4. Use the unified API
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello, world!"}]
  }'
```

## Documentation

📖 **[Complete User Manual](docs/user-manual.md)**

The comprehensive user manual covers:

1. **What is Tingly Box?** - Architecture, features, and use cases
2. **Installation** - Detailed setup instructions for all platforms
3. **Integration with Claude CLI** - Step-by-step configuration guide
4. **Troubleshooting** - Common issues and solutions

## Key CLI Commands

### Provider Management
```bash
./tingly add <name> <api-base> <token>      # Add new provider
./tingly list                               # List all providers
./tingly delete <name>                      # Remove provider
./tingly token                              # Generate JWT token
```

### Server Management
```bash
./tingly start [--port <port>]              # Start server (default: 8080)
./tingly stop                               # Stop server
./tingly restart [--port <port>]            # Restart server
./tingly status                             # Check server status
```

### Additional Features
```bash
./tingly ui                                 # Open web interface
./tingly shell                              # Interactive mode
./tingly completion <shell>                 # Generate shell completions
```

## Architecture Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Application   │───▶│   Tingly Box     │───▶│  AI Providers   │
│   (Claude CLI)  │    │   (Proxy Server) │    │ (OpenAI, Anth.) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌──────────────┐
                       │   Web UI     │
                       │ (Management) │
                       └──────────────┘
```

## Use Cases

- **Development**: Test different AI providers without changing code
- **Production**: High availability with automatic failover across providers with load balancing support
- **Cost Optimization**: Route requests to the most cost-effective provider
- **Vendor Lock-in Prevention**: Easily switch between providers
- **Unified Interface**: Standardize API access across teams

## API Endpoints

- `GET /health` - Health check
- `POST /token` - Generate JWT token
- `POST /v1/chat/completions` - OpenAI-compatible chat completions
- `POST /anthropic/v1/messages` - Anthropic-compatible messages API
- `GET /v1/models` - List available models

## Configuration

- **Config**: `~/.tingly-box/config.json` (encrypted)
- **Global**: `~/.tingly-box/global.json` (JWT secrets, tokens)
- **Logs**: `~/.tingly-box/logs/server.log`
- **Memory**: `memory/` directory for operation history

## Development

```bash
# Run all tests
go test ./...

# Run integration tests
go test ./tests -v

# Run with coverage
go test -cover ./...

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build ./cmd/tingly -o tingly-linux-amd64
GOOS=windows GOARCH=amd64 go build ./cmd/tingly -o tingly-windows-amd64.exe
GOOS=darwin GOARCH=amd64 go build ./cmd/tingly -o tingly-darwin-amd64
GOOS=darwin GOARCH=arm64 go build ./cmd/tingly -o tingly-darwin-arm64

# Build frontend
cd frontend
npm install
npm run build

# Development mode
cd frontend
npm run dev
```

## Project Structure

```
├── cmd/tingly/              # CLI entry point (main.go uses Cobra)
├── internal/
│   ├── auth/                # JWT token management
│   ├── cli/                 # CLI commands (add, list, start, stop, etc.)
│   ├── config/              # Configuration management (encrypted JSON storage)
│   ├── memory/              # Operation history and statistics logging
│   └── server/              # HTTP server with Gin framework
├── frontend/                # React/TypeScript web UI (Material-UI)
├── tests/                   # Integration tests
├── docs/                    # Documentation
└── wails3/                  # Desktop GUI (experimental)
```

## License

MIT License

---

For detailed information, please refer to the **[Complete User Manual](docs/user-manual.md)**.