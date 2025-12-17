# Docker Guide for Tingly Box

This guide explains how to use Tingly Box with Docker.

## Overview

We provide a single Docker setup with multiple use cases:

1. **Dockerfile** - Multi-stage build using Task task runner (includes both server and CLI)
2. **docker-compose.yml** - Complete setup with volumes for production and development

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Create data directories
mkdir -p data/{.tingly-box,logs,memory}

# Start the server
docker-compose up tingly-box

# Run in detached mode
docker-compose up -d tingly-box

# View logs
docker-compose logs -f tingly-box

# Stop the server
docker-compose down
```

### Manual Docker Usage

#### 1. Build and Run Server

```bash
# Build the image
docker build -t tingly-box:latest .

# Run the server
docker stop tingly-box && docker rm tingly-box
docker run -d \
--name tingly-box \
-p 8080:8080 \
-v $(pwd)/data/.tingly-box:/app/.tingly-box \
-v $(pwd)/data/logs:/app/logs \
-v $(pwd)/data/memory:/app/memory \
tingly-box:latest

```

#### 2. CLI Tool Usage (using same image)

```bash
# Build the image
docker build -t tingly-box:latest .

# Add a provider
docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest add openai https://api.openai.com/v1 sk-token

# List providers
docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest list

# Generate token
docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest token

# Run any other CLI command
docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest status
```

#### 3. Development Mode

For development, you can mount the source code and rebuild manually when needed:

```bash
# Run with source mounted (for development)
docker-compose --profile dev up tingly-box-dev

# Make changes to source code, then rebuild and restart:
docker-compose exec tingly-box-dev task cli:build
docker-compose restart tingly-box-dev

# Or run manually with mounted source
docker run -it --rm \
  -v $(pwd):/app \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  -p 8080:8080 \
  tingly-box:latest
```

## Configuration

### Environment Variables

- `TINGLY_PORT` - Server port (default: 8080)
- `TINGLY_HOST` - Server host (default: 0.0.0.0)
- `TINGLY_DEBUG` - Enable debug mode (default: false)

### Volume Mounts

- `/app/.tingly-box` - Encrypted configuration storage
- `/app/logs` - Server logs
- `/app/memory` - Operation history and statistics

## Example: Complete Setup

```bash
#!/bin/bash

# 1. Create directories
mkdir -p data/{.tingly-box,logs,memory}

# 2. Build and start server
docker-compose up -d tingly-box

# 3. Wait for server to start
sleep 5

# 4. Add providers
docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest add openai https://api.openai.com/v1 sk-your-token

docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest add anthropic https://api.anthropic.com sk-your-token

# 5. Generate token
TOKEN=$(docker run -it --rm \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  tingly-box:latest token | grep "sk-" | head -1)

# 6. Test the API
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello from Docker!"}]
  }'
```

## Production Tips

### Security

1. Use secrets for API tokens:
```yaml
services:
  tingly-box:
    environment:
      - OPENAI_TOKEN=${OPENAI_TOKEN}
      - ANTHROPIC_TOKEN=${ANTHROPIC_TOKEN}
```

2. Run as non-root user (included in Dockerfile)

3. Use read-only volumes where possible

### Performance

1. Set memory limits:
```yaml
services:
  tingly-box:
    deploy:
      resources:
        limits:
          memory: 512M
```

2. Use health checks (included in Dockerfile)

### Backup

Backup the `.tingly-box` directory regularly:
```bash
docker run --rm -v tingly-config:/data -v $(pwd):/backup \
  alpine tar czf /backup/tingly-config-backup.tar.gz -C /data .
```

## Troubleshooting

### Common Issues

1. **Port already in use**
   - Change the host port mapping
   - Example: `-p 9090:8080`

2. **Permission errors**
   - Ensure proper ownership of data directories
   - `sudo chown -R 1000:1000 data/`

3. **Configuration not persisting**
   - Check volume mounts
   - Ensure correct paths

### Debug Mode

Enable debug logging:
```bash
docker run -d \
  --name tingly-box-debug \
  -p 8080:8080 \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  -e TINGLY_DEBUG=true \
  tingly-box:latest

# View debug logs
docker logs -f tingly-box-debug
```

## Building for Different Platforms

```bash
# Build for ARM64 (Apple Silicon)
docker buildx build --platform linux/arm64 -t tingly-box:arm64 .

# Build for AMD64 (Intel/AMD)
docker buildx build --platform linux/amd64 -t tingly-box:amd64 .

# Build multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 -t tingly-box:latest .