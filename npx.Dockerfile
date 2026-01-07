# NPX-based lightweight Docker image for Tingly Box
# This image uses npm to install tingly-box globally, resulting in a smaller image size

ARG TINGLY_VERSION=latest
FROM node:20-alpine

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata su-exec

# Update npm to latest version
RUN npm install -g npm@latest

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

# Expose the default port
EXPOSE 12580

# Environment variables for configuration
ENV TINGLY_PORT=12580
ENV TINGLY_HOST=0.0.0.0
ENV TINGLY_VERSION="${TINGLY_VERSION}"

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD tingly-box status || exit 1

# Default command: install and run tingly-box via npm
CMD ["sh", "-c", "echo '======================================' && \
     echo '  Tingly Box is starting up...' && \
     echo '  Installing version:' ${TINGLY_VERSION} && \
     echo '  Web UI will be available at:' && \
     echo '  http://localhost:12580/dashboard?user_auth_token=tingly-box-user-token' && \
     echo '======================================' && \
     npm install -g tingly-box@${TINGLY_VERSION} && \
     exec tingly-box start --host 0.0.0.0 --port 12580"]

# Volumes for persistent data
VOLUME ["/app/.tingly-box", "/app/memory", "/app/logs"]
