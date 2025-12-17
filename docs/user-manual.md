# Tingly Box User Manual

## Table of Contents

1. [Installation Guide](#installation-guide)
2. [Configuration](#configuration)
3. [Integration with AI Applications](#integration-with-ai-applications)
4. [Advanced Features](#advanced-features)
5. [Troubleshooting](#troubleshooting)
6. [FAQ](#faq)

## Installation Guide

### Prerequisites

- **Go**: Version 1.21 or later
- **Node.js**: Version 18 or later (for web UI development only)
- **Git**: For cloning the repository

### Building from Source

#### 1. Clone the Repository

```bash
git clone https://github.com/tingly-dev/tingly-box.git
cd tingly-box
```

#### 2. Build the CLI Binary

```bash
# Build for your current platform
go build ./cmd/tingly-box

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build ./cmd/tingly-box -o tingly-linux-amd64
GOOS=windows GOARCH=amd64 go build ./cmd/tingly-box -o tingly-windows-amd64.exe
GOOS=darwin GOARCH=amd64 go build ./cmd/tingly-box -o tingly-darwin-amd64
GOOS=darwin GOARCH=arm64 go build ./cmd/tingly-box -o tingly-darwin-arm64
```

#### 3. Install the Binary

```bash
# Move to system PATH (Linux/macOS)
sudo mv tingly-box /usr/local/bin/

# Or add to your PATH in ~/.bashrc or ~/.zshrc
export PATH="$PATH:/path/to/tingly-box"

# Windows: Add to PATH or move to a directory in PATH
```

#### 4. Build the Frontend (Optional)

The frontend is pre-built in the binary, but you can rebuild it:

```bash
cd frontend
pnpm install
pnpm build

# For development
pnpm dev
```

### Docker Installation

#### Build Docker Image

```bash
docker build -t tingly-box:latest .
```

#### Run Container

```bash
# Create data directories
mkdir -p data/.tingly-box data/logs data/memory

# Run container
docker run -d \
  --name tingly-box \
  -p 8080:8080 \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  -v $(pwd)/data/logs:/app/logs \
  -v $(pwd)/data/memory:/app/memory \
  tingly-box:latest
```

#### Docker Compose

```yaml
version: '3.8'
services:
  tingly-box:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data/.tingly-box:/app/.tingly-box
      - ./data/logs:/app/logs
      - ./data/memory:/app/memory
    restart: unless-stopped
```

### Verification

```bash
# Check installation
tingly-box --help

# Verify version (if implemented)
tingly-box version

# Test basic functionality
tingly-box status
```

## Configuration

### Adding Providers

#### OpenAI Provider

```bash
tingly-box add openai https://api.openai.com/v1 sk-your-openai-token
```

#### Anthropic Provider

```bash
tingly-box add anthropic https://api.anthropic.com sk-your-anthropic-token
```

#### Custom Provider

```bash
# OpenAI-compatible custom provider
tingly-box add custom-openai https://your-api.example.com/v1 your-token --api-style openai

# Anthropic-compatible custom provider
tingly-box add custom-anthropic https://your-api.example.com your-token --api-style anthropic
```

### Managing Providers

```bash
# List all configured providers
tingly-box list

# Delete a provider
tingly-box delete provider-name

# Update a provider (delete and re-add)
tingly-box delete provider-name
tingly-box add provider-name https://api.provider.com/v1 new-token
```

### Understanding Provider Configuration

Each provider has the following attributes:
- **Name**: Unique identifier for the provider
- **API Base**: Base URL for the provider's API
- **API Style**: Either `openai` or `anthropic` (affects request format)
- **Token**: Authentication token
- **Enabled**: Whether the provider is active

### Authentication Tokens

Tingly Box uses JWT-based authentication with two types of tokens:

1. **User Token**: For management operations (add/delete providers)
2. **Model Token**: For API access (chat completions)

```bash
# Generate tokens
tingly-box token

# Output will show both tokens:
# User Token: tingly-user-xxxxx (for management)
# Model Token: sk-tingly-model-xxxxx (for API access)
```

## Integration with AI Applications

### Method 1: OpenAI SDK Integration

Configure your application to use Tingly Box as if it were OpenAI:

```python
# Python example with openai library
import openai

client = openai.OpenAI(
    api_key="sk-tingly-model-token",
    base_url="http://localhost:8080/openai/v1"
)

response = client.chat.completions.create(
    model="gpt-3.5-turbo",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### Method 2: Anthropic SDK Integration

```python
# Python example with anthropic library
import anthropic

client = anthropic.Anthropic(
    api_key="sk-tingly-model-token",
    base_url="http://localhost:8080/anthropic/v1"
)

response = client.messages.create(
    model="claude-3-sonnet-20240229",
    max_tokens=1000,
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### Method 3: cURL Integration

```bash
# OpenAI-compatible endpoint
curl -X POST http://localhost:8080/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-tingly-model-token" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Anthropic-compatible endpoint
curl -X POST http://localhost:8080/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-tingly-model-token" \
  -d '{
    "model": "claude-3-sonnet-20240229",
    "max_tokens": 1000,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Method 4: Claude CLI Integration

Configure Claude CLI to use Tingly Box:

```bash
# Method A: Environment variables
export OPENAI_API_BASE="http://localhost:8080/openai/v1"
export OPENAI_API_KEY="sk-tingly-model-token"

# Method B: Settings file (~/.claude/settings.json)
{
  "api": {
    "baseUrl": "http://localhost:8080/openai/v1",
    "apiKey": "sk-tingly-model-token"
  }
}
```

## Advanced Features

### Load Balancing

Configure multiple providers with the same models to enable load balancing:

```bash
# Add multiple providers with overlapping models
tingly-box add openai-us https://api.openai.com/v1 sk-us-token
tingly-box add openai-eu https://api.openai.com/v1 sk-eu-token

# Requests will be distributed between providers
```

### Model Fetching

Tingly Box can automatically fetch available models from providers:

```bash
# Fetch models from a specific provider
curl -X POST http://localhost:8080/api/provider-models/openai \
  -H "Authorization: Bearer tingly-user-token"

# List all available models
curl -X GET http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-tingly-model-token"
```

### Web Management UI

Access the web interface for visual management:

```bash
# Open web UI
tingly-box ui

# Or manually open in browser
# http://localhost:8080/dashboard?user_auth_token=tingly-user-token
```

The web UI provides:
- Provider management (add/edit/delete)
- Model browsing
- Request monitoring
- Configuration viewing

### Configuration Files

All configurations are stored in `~/.tingly-box/`:

- `config.json`: Encrypted provider configurations
- `global.json`: JWT secrets and tokens
- `logs/server.log`: Server logs
- `memory/`: Operation history

Note: Configuration files are encrypted for security.

## Troubleshooting

### Common Issues

#### 1. Port Already in Use

```bash
Error: listen tcp :8080: bind: address already in use
```

**Solutions:**
```bash
# Check what's using the port
lsof -i :8080  # macOS/Linux
netstat -ano | findstr :8080  # Windows

# Kill the process
kill -9 <PID>  # macOS/Linux

# Or use a different port
tingly-box start --port 9090
```

#### 2. Authentication Failures

```bash
{"error":{"message":"Invalid token","type":"authentication_error"}}
```

**Solutions:**
```bash
# Regenerate tokens
tingly-box token

# Verify token type
# - Use user token (tingly-user-xxxx) for management APIs
# - Use model token (sk-tingly-model-xxxx) for chat APIs
```

#### 3. Provider Connection Issues

```bash
{"error":{"message":"Failed to forward request","type":"api_error"}}
```

**Solutions:**
```bash
# Check provider configuration
tingly-box list

# Test provider directly
curl -H "Authorization: Bearer provider-token" \
  https://api.provider.com/v1/models

# Verify provider token validity
```

#### 4. Docker Volume Permissions

```bash
Error: permission denied while trying to connect to Docker daemon
```

**Solutions:**
```bash
# Fix volume permissions on host
sudo chown -R $USER:$USER data/

# Or run with proper user mapping
docker run -u $(id -u):$(id -g) ...
```

### Debug Mode

Enable verbose logging:

```bash
# Start with verbose output
tingly-box start --verbose

# Check logs in real-time
tail -f ~/.tingly-box/logs/server.log

# Enable debug environment variable
export TINGLY_DEBUG=true
tingly-box start
```

### Performance Issues

1. **High Memory Usage**
   - Restart server: `tingly-box restart`
   - Monitor with: `ps aux | grep tingly-box`

2. **Slow Response Times**
   - Check provider latency
   - Enable HTTP/2 if supported
   - Configure appropriate timeouts

3. **Connection Timeouts**
   - Increase timeout in provider configuration
   - Check network connectivity to providers

### Getting Help

1. **Built-in Help**
   ```bash
   tingly-box --help
   tingly-box <command> --help
   ```

2. **Health Check**
   ```bash
   curl http://localhost:8080/health
   ```

3. **Debug Information**
   ```bash
   # View configuration
   tingly-box list

   # Check server status
   tingly-box status

   # Test with verbose mode
   tingly-box start --verbose
   ```

## FAQ

**Q: Can I use multiple providers simultaneously?**
A: Yes, you can configure multiple providers and route requests based on model names.

**Q: How do I backup my configuration?**
A: Backup the `~/.tingly-box/` directory. 

**Q: Is my data secure?**
A: Yes, all configurations are stored locally.

**Q: Can I run Tingly Box behind a reverse proxy?**
A: Yes, ensure you configure proper headers (X-Forwarded-For, etc.).

**Q: How do I update provider tokens?**
A: Delete and re-add the provider with the new token, or use the web UI.

**Q: What happens if a provider is down?**
A: Requests to that provider will fail. Configure multiple providers for redundancy.

---

For more information, visit the [GitHub repository](https://github.com/tingly-dev/tingly-box).