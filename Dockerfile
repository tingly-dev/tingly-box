# Multi-stage build for Tingly Box
# Stage 1: Build
FROM golang:1.25-alpine AS builder

# Install git, nodejs, npm, pnpm, java, gcc (for CGO), and other build dependencies
RUN apk add --no-cache git nodejs npm ca-certificates tzdata curl jq openjdk17-jre gcc musl-dev

# Install pnpm
RUN npm install -g pnpm

# Install Task (task runner)
RUN go install github.com/go-task/task/v3/cmd/task@latest

# Install openapi-generator-cli
RUN npm install -g @openapitools/openapi-generator-cli

# Pre-download openapi-generator JAR to avoid network issues during build
RUN mkdir -p /usr/local/lib/node_modules/@openapitools/openapi-generator-cli/versions && \
    curl -fsSL --retry 3 --retry-delay 2 \
    https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/7.17.0/openapi-generator-cli-7.17.0.jar \
    -o /usr/local/lib/node_modules/@openapitools/openapi-generator-cli/versions/7.17.0.jar

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Copy the entire source code (including submodule if initialized)
COPY . .

# Ensure openai-go submodule exists (clone if user hasn't initialized submodules)
RUN if [ ! -f libs/openai-go/go.mod ]; then \
      rm -rf libs/openai-go && \
      git clone -b fork --depth 1 https://github.com/tingly-dev/openai-go.git libs/openai-go; \
    fi

RUN if [ ! -f libs/anthropic-sdk-go/go.mod ]; then \
      rm -rf libs/anthropic-sdk-go && \
      git clone -b fork --depth 1 https://github.com/tingly-dev/anthropic-sdk-go.git libs/anthropic-sdk-go; \
    fi

# Download dependencies (must be after source copy due to local replace directive)
RUN go mod download

# Now build using the created Taskfile
RUN CGO_ENABLED=1 CI=true task cli:build

# Rename binary to expected name
RUN mv ./build/tingly-box ./tingly

# Stage 2: Runtime
FROM alpine:latest

# Install ca-certificates for HTTPS requests and su-exec for running as non-root
RUN apk --no-cache add ca-certificates su-exec tzdata

# Create app user
RUN addgroup -S tingly && \
    adduser -S -G tingly tingly

# Set the Current Working Directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/tingly /usr/local/bin/tingly

# Create necessary directories with proper permissions
RUN mkdir -p /app/.tingly-box /app/memory /app/logs && \
    chown -R tingly:tingly /app

# Switch to non-root user
USER tingly

# Expose port
EXPOSE 8080

# Environment variables for configuration
ENV TINGLY_PORT=8080
ENV TINGLY_HOST=0.0.0.0

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD tingly status || exit 1

# Default command (server mode)
CMD ["sh", "-c", "echo '======================================' && \
     echo '  Tingly Box is starting up...' && \
     echo '  Web UI will be available at:' && \
     echo '  http://localhost:8080/dashboard?user_auth_token=tingly-box-user-token' && \
     echo '======================================' && \
     rm -f /app/.tingly-box/tingly-server.pid && \
     exec tingly start --host 0.0.0.0 --port 8080"]

# Volumes for persistent data
VOLUME ["/app/.tingly-box", "/app/memory", "/app/logs"]