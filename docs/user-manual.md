# Tingly Box User Manual

## Table of Contents

1. [What is Tingly Box?](#what-is-tingly-box)
    - Core Features
    - Architecture Overview
    - Use Cases
2. [Installation](#installation)
    - Prerequisites
    - Building from Source
    - Installation Methods
    - Verification
3. [Integration with Claude CLI](#integration-with-claude-cli)
    - Direct API Integration
    - Configuration Examples
    - Best Practices
4. [Troubleshooting](#troubleshooting)
    - Common Issues
    - Debug Mode
    - Log Analysis
    - Performance Optimization

## 1. What is Tingly Box?

Tingly Box is a **provider-agnostic AI model proxy** that exposes a unified OpenAI-compatible API endpoint while routing requests to multiple configured AI providers. It consists of both a **CLI tool** for management and a **service** for handling requests, acting as a middleware layer between your applications and various AI service providers, allowing you to switch between providers seamlessly without changing your application code.

### Core Features

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

### Architecture Overview

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

### Key Components

1. **CLI Tool**: Command-line interface for server management and provider configuration
2. **HTTP Server**: Gin-based server handling API requests and forwarding
3. **Configuration System**: Encrypted provider configuration storage
4. **Authentication Layer**: JWT tokens for API access
5. **Request Router**: Configured routing to appropriate providers based on model names
6. **Format Adapter**: Converts between different API formats
7. **Model Manager**: Caches and manages available models

### Use Cases

- **Development**: Test different AI providers without changing code
- **Production**: High availability with automatic failover across providers with load balancing support
- **Cost Optimization**: Route requests to the most cost-effective provider
- **Vendor Lock-in Prevention**: Easily switch between providers
- **Unified Interface**: Standardize API access across teams

## 2. Installation

### Prerequisites

- **Go**: Version 1.21 or later
- **Node.js**: Version 18 or later (for web UI development)
- **Git**: For cloning the repository

### Building from Source

1. **Clone the Repository**

   ```bash
   git clone https://github.com/tingly-dev/tingly-box.git
   cd tingly-box
   ```

2. **Build the CLI Binary**

   ```bash
   # Build for your current platform
   go build ./cmd/tingly
   
   # Build for multiple platforms
   GOOS=linux GOARCH=amd64 go build ./cmd/tingly -o tingly-linux-amd64
   GOOS=windows GOARCH=amd64 go build ./cmd/tingly -o tingly-windows-amd64.exe
   GOOS=darwin GOARCH=amd64 go build ./cmd/tingly -o tingly-darwin-amd64
   GOOS=darwin GOARCH=arm64 go build ./cmd/tingly -o tingly-darwin-arm64
   ```

3. **Build the Frontend (Optional)**

   ```bash
   cd frontend
   npm install
   npm run build
   
   # For development
   npm run dev
   ```

4. **Install the Binary**

   ```bash
   # Move to system PATH
   sudo mv tingly /usr/local/bin/
   
   # Or add to your PATH in ~/.bashrc or ~/.zshrc
   export PATH="$PATH:/path/to/tingly-box"
   ```

### Installation Methods

#### Method 1: Direct Download

1. Download the latest release from the [GitHub Releases](https://github.com/tingly-dev/tingly-box/releases) page
2. Extract the archive
3. Move the binary to your PATH

#### Method 2: Package Manager (Future)

```bash
npm install tingly-box
```

### Verification

1. **Check Installation**

   ```bash
   tingly --help
   ```

2. **Verify Version**

   ```bash
   tingly version
   ```

3. **Test Basic Functionality**

   ```bash
   tingly status
   ```

## 3. Integration with Claude CLI

Tingly Box can be integrated with Claude CLI in multiple ways, depending on your use case.

### Method 1: Direct API Integration

Configure Claude CLI to use Tingly Box as its API endpoint:

1. **Start Tingly Box**

   ```bash
   # Start with default port (8080)
   tingly start
   
   # Start with custom port
   tingly start --port 9090
   
   # Start with adaptor enabled for format conversion
   tingly start --adaptor
   ```

2. **Configure Claude CLI**

   ```bash
   # Set Claude CLI to use Tingly Box endpoint
   export ANTHROPIC_API_URL="http://localhost:{port}/anthropic"
   export ANTHROPIC_API_KEY="{your-tingly-box-token}"
   export ANTHROPIC_MODEL="tingly"
   
   # or alternatively change your ~/.claude/settings.json
   ```json
   {
     "env": {
       "ANTHROPIC_AUTH_TOKEN": "{your-tingly-box-token}",
       "ANTHROPIC_BASE_URL": "http://localhost:8080/anthropic",
       "ANTHROPIC_MODEL": "tingly"
     }
   }
   ```

3. **Generate Access Token**

   ```bash
   # Display UI management key
   tingly token
   ```


### Configuration Examples

#### Basic OpenAI Provider

```bash
# Add OpenAI provider
tingly add openai https://api.openai.com/v1 sk-your-openai-token

# Add Anthropic provider
tingly add anthropic https://api.anthropic.com sk-your-anthropic-token

# Add custom provider
tingly add custom-provider https://your-api.example.com/v1 your-api-token --api-style openai
```

#### Advanced Configuration

```bash
# List all providers
tingly list

# Remove provider
tingly delete openai

# Add provider with updated settings
tingly add openai https://api.openai.com/v1 new-token

# Check server status
tingly status
```

### Best Practices

1. **Security**
    - Always use HTTPS in production
    - Regularly rotate your API tokens
    - Use environment variables for sensitive data
    - Enable authentication for all endpoints

2. **Performance**
    - Enable model caching to reduce API calls
    - Use streaming for long responses
    - Configure appropriate timeouts
    - Monitor memory usage

3. **Reliability**
    - Configure multiple providers for failover
    - Implement retry logic in your application
    - Monitor server logs for errors
    - Set up health checks

4. **Development**
    - Use the development mode for frontend changes
    - Enable debug logging during development
    - Test with different providers
    - Validate API responses

### Example Integration with Claude CLI

```bash
# 1. Start Tingly Box with adaptor
tingly start --adaptor --port 8080

# 2. Add your providers
tingly add openai https://api.openai.com/v1 sk-your-openai-key
tingly add anthropic https://api.anthropic.com sk-your-anthropic-key

# 3. Generate tokens
tingly token  # This will display UI management key and model API key

# 4. Configure Claude CLI
export OPENAI_API_BASE="http://localhost:8080/openai/v1"
export OPENAI_API_KEY="sk-tingly-model-token"

# 5. Use Claude CLI normally
claude "Generate a Python function for data analysis"
```

## 4. Troubleshooting

### Common Issues

#### 1. Server Won't Start

**Problem**: Server fails to start with port error

```bash
Error: listen tcp :8080: bind: address already in use
```

**Solution**:

```bash
# Check what's using the port
lsof -i :8080

# Kill the process
kill -9 <PID>

# Or use a different port
tingly start --port 9090
```

#### 2. Authentication Failures

**Problem**: 401 Unauthorized errors

```bash
{"error":{"message":"Invalid token","type":"authentication_error"}}
```

**Solution**:

```bash
# Display your tokens
tingly token

# Check token validity
curl -H "Authorization: Bearer your-token" http://localhost:8080/health

# Verify token type
# Use 'user' token for management API
# Use 'model' token for chat completions
```

#### 3. Provider Connection Issues

**Problem**: Cannot connect to provider

```bash
{"error":{"message":"Failed to forward request","type":"api_error"}}
```

**Solution**:

```bash
# Check provider configuration
tingly list

# Test provider connection
curl -H "Authorization: Bearer provider-token" https://api.provider.com/v1/models

# Delete and re-add provider with new configuration
tingly delete provider-name
tingly add provider-name https://api.provider.com/v1 new-token
```

#### 4. Format Conversion Errors

**Problem**: Request format conversion fails

```bash
{"error":{"message":"Request format adaptation is disabled","type":"adapter_disabled"}}
```

**Solution**:

```bash
# Start server with adaptor enabled, note that it's experimental for now
tingly start --adaptor

# Or use native format
# Use /anthropic/v1/messages for Anthropic providers
# Use /openai/v1/chat/completions for OpenAI providers
```

#### 5. Model Not Found

**Problem**: Model not available error

```bash
{"error":{"message":"Model not found: gpt-4","type":"invalid_request_error"}}
```

**Solution**:

```bash
# List all available models
curl -H "Authorization: Bearer your-token" http://localhost:8080/v1/models

# Check model mapping
# The model name in your request should match the provider's model name
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
# Start with verbose logging
tingly start --verbose

# Check logs
tail -f ~/.tingly-box/logs/server.log

# Enable debug mode
export TINGLY_DEBUG=true
tingly start
```

### Log Analysis

Common log locations:

- Server logs: `~/.tingly-box/logs/server.log`
- Config file: `~/.tingly-box/config.json`
- Global config: `~/.tingly-box/global.json`

Key log patterns to watch:

- `[ADAPTOR]` - Format conversion messages
- `[AUTH]` - Authentication events
- `[FORWARD]` - Request forwarding
- `[ERROR]` - Error messages

### Performance Optimization

1. **Reduce Latency**

   ```bash
   # Use local providers when possible
   # Configure appropriate timeouts
   ```

2. **Memory Management**

   ```bash
   # Monitor memory usage
   ps aux | grep tingly
   
   # Restart server to clear cache if needed
   tingly restart
   ```

3. **Connection Optimization**

   ```bash
   # Use HTTP/2 when supported by providers
   # Enable response compression in provider settings
   # Configure appropriate timeouts in your application
   ```

### Getting Help

1. **Built-in Help**

   ```bash
   tingly --help
   tingly <command> --help
   ```

2. **Community Support**

    - GitHub Issues: https://github.com/tingly-dev/tingly-box/issues
    - Discussions: https://github.com/tingly-dev/tingly-box/discussions

3. **Debug Information Collection**

   ```bash
   # View current configuration
   tingly list
   
   # Check server status
   tingly status
   
   # Test with debug mode
   tingly start --verbose > debug.log 2>&1
   ```

### Frequently Asked Questions

**Q: Can I use multiple providers simultaneously?**
A: Yes, Tingly Box supports multiple providers and can route requests based on model names.

**Q: How do I migrate from direct API usage?**
A: Simply change your API base URL to point to Tingly Box and update your authentication token.

**Q: Is my provider data secure?**
A: Yes, all provider configurations are store locally and never get uploaded.

**Q: Can I use Tingly Box in production?**
A: Yes, Tingly Box is production-ready with proper error handling, logging, and security features.

**Q: How do I update provider configurations?**
A: Delete and re-add the provider with new settings using `tingly delete` and `tingly add`, or use the web UI to modify provider settings.

**Q: What happens if a provider is down?**
A: Tingly Box will return an error if the selected provider is unavailable. Configure multiple providers for redundancy.

---

For more information, visit the [GitHub repository](https://github.com/tingly-dev/tingly-box).