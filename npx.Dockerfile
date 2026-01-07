# NPX-based lightweight Docker image for Tingly Box
# This image uses npm to install tingly-box globally, resulting in a smaller image size

ARG TINGLY_VERSION=latest
FROM node:20-slim

# Expose the default port
EXPOSE 12580

# Environment variables for configuration
ENV TINGLY_PORT=12580
ENV TINGLY_HOST=0.0.0.0

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd -r tingly && \
    useradd -r -g tingly tingly

# Update npm to latest version (as root)
RUN npm install -g npm@latest

# Install tingly-box globally during build (as root)
RUN npm install -g tingly-box@${TINGLY_VERSION}

# Grant tingly user access to npm global directories and cache
RUN chown -R tingly:tingly /usr/local/lib/node_modules /usr/local/bin /root/.npm

# Set working directory
WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /app/.tingly-box /app/memory /app/logs && \
    chown -R tingly:tingly /app

RUN mkdir /home/tingly && chown -R tingly:tingly /home/tingly/

# Switch to non-root user
USER tingly

RUN tingly-box version

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD tingly-box version || exit 1

# Default command: run tingly-box
CMD ["sh", "-c", "echo '======================================' && \
     echo '  Tingly Box is starting up...' && \
     echo '  Installing version:' ${TINGLY_VERSION} && \
     echo '  Web UI will be available at:' && \
     echo '  http://localhost:12580/dashboard?user_auth_token=tingly-box-user-token' && \
     echo '======================================' && \
     exec tingly-box start --host 0.0.0.0 --port 12580"]

# Volumes for persistent data
VOLUME ["/app/.tingly-box", "/app/memory", "/app/logs"]
