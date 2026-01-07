# NPX-based lightweight Docker image for Tingly Box
# This image uses npx to download the binary on first run, resulting in a smaller image size

FROM node:20-alpine

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata su-exec

# Create non-root user for security
RUN addgroup -S tingly && \
    adduser -S -G tingly tingly

# Set working directory
WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /app/.tingly-box /app/memory /app/logs && \
    chown -R tingly:tingly /app

# Switch to non-root user
USER tingly

# Set environment variables for npx cache directory
ENV NPX_CACHE_DIR=/app/.npx-cache

# Expose the default port
EXPOSE 12580

# Environment variables for configuration
ENV TINGLY_PORT=12580
ENV TINGLY_HOST=0.0.0.0

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD npx tingly-box@latest status || exit 1

# Default command: download and run tingly-box via npx
# The binary will be cached in ~/.npm/_npx for subsequent runs
CMD ["sh", "-c", "echo '======================================' && \
     echo '  Tingly Box (NPX) is starting up...' && \
     echo '  Web UI will be available at:' && \
     echo '  http://localhost:12580/dashboard?user_auth_token=tingly-box-user-token' && \
     echo '======================================' && \
     exec npx tingly-box@latest start --host 0.0.0.0 --port 12580"]

# Volumes for persistent data
VOLUME ["/app/.tingly-box", "/app/memory", "/app/logs"]
