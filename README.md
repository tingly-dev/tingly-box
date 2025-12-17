# Tingly Box

[![](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![](https://img.shields.io/badge/React-19+-blue.svg)](https://reactjs.org)
[![](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)

A **provider-agnostic AI model proxy** that exposes a unified OpenAI-compatible API endpoint while routing requests to multiple configured AI providers.

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Node.js 18+ (for web UI development)

### Installation

```bash
# Build from source
git clone https://github.com/tingly-dev/tingly-box.git
cd tingly-box
go build ./cmd/tingly-box

# Or with Docker
docker build -t tingly-box:latest .
```

### Basic Usage

```bash
# Add providers
./tingly-box add openai https://api.openai.com/v1 sk-your-token
./tingly-box add anthropic https://api.anthropic.com sk-your-token

# Start server
./tingly-box start --port 8080

# Generate token
./tingly-box token

# Use API
curl -X POST http://localhost:8080/openai/v1/chat/completions \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}'
```

## 📌 Key Features

- **Multi-Provider Support** - OpenAI, Anthropic, and custom providers
- **Unified API** - Single OpenAI-compatible endpoint
- **Load Balancing** - Distribute requests across providers
- **Web Management UI** - Intuitive provider configuration
- **JWT Authentication** - Secure token-based access

## 🐳 Docker Deployment

```bash
# Build image
docker build -t tingly-box:latest .

# Run container
docker run -d \
  --name tingly-box \
  -p 8080:8080 \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  -v $(pwd)/data/logs:/app/logs \
  -v $(pwd)/data/memory:/app/memory \
  tingly-box:latest
```

## 🔧 CLI Commands

```bash
# Provider management
tingly-box add <name> <api-base> <token>      # Add provider
tingly-box list                               # List providers
tingly-box delete <name>                      # Remove provider

# Server management
tingly-box start [--port <port>]              # Start server
tingly-box stop                               # Stop server
tingly-box restart [--port <port>]            # Restart server
tingly-box status                             # Check status

# Utilities
tingly-box token                              # Generate JWT token
tingly-box ui                                 # Open web interface
```

## 📚 Documentation

- **[User Manual](docs/user-manual.md)** - Detailed guide for installation, configuration, and troubleshooting

## 🔌 API Endpoints

### OpenAI-Compatible
- `POST /openai/v1/chat/completions` - Chat completions
- `GET /v1/models` - List models

### Anthropic-Compatible
- `POST /anthropic/v1/messages` - Messages API
- `GET /anthropic/v1/models` - List models

### Management
- `GET /health` - Health check
- `GET /api/providers` - List providers
- `POST /api/providers` - Add provider

## 🏗️ Architecture

```
Application → Tingly Box → AI Providers
               ↓
           Web Management UI
```

## 📁 Project Structure

```
tingly-box/
├── cmd/tingly-box/      # CLI entry point
├── internal/
│   ├── auth/            # JWT authentication
│   ├── cli/             # CLI commands
│   ├── config/          # Configuration management
│   └── server/          # HTTP server
├── frontend/            # React web UI
├── docs/                # Documentation
└── Dockerfile           # Container definition
```

## 🧪 Development

```bash
# Run tests
go test ./...

# Build frontend
cd frontend && pnpm install && pnpm build

# Development mode
cd frontend && pnpm dev
```

## 📄 License

Apache 2.0

---

For detailed setup instructions and troubleshooting, see the **[User Manual](docs/user-manual.md)**.