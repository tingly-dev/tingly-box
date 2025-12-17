# Multi-stage build for Tingly Box
# Stage 1: Build
FROM golang:1.25-alpine AS builder

# Install git, nodejs, npm, pnpm, java, and other build dependencies
RUN apk add --no-cache git nodejs npm ca-certificates tzdata curl jq openjdk17-jre

# Install pnpm
RUN npm install -g pnpm

# Install Task (task runner)
RUN go install github.com/go-task/task/v3/cmd/task@latest

# Install openapi-generator-cli
RUN npm install -g @openapitools/openapi-generator-cli

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files for faster builds
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Now build using the created Taskfile
RUN CI=true task cli:build

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
     exec tingly start --port 8080"]

# Volumes for persistent data
VOLUME ["/app/.tingly-box", "/app/memory", "/app/logs"]