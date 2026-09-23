# Lightweight runtime image installed from the published npm package.
ARG TINGLY_VERSION=latest
FROM node:20-slim

# A build ARG declared before FROM is not visible inside the stage unless it is
# redeclared. Keep it in the runtime environment as useful image metadata too.
ARG TINGLY_VERSION
ENV TINGLY_VERSION=${TINGLY_VERSION} \
    TINGLY_PORT=12580 \
    TINGLY_HOST=0.0.0.0 \
    TINGLY_DEBUG=false

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system tingly \
    && useradd --system --gid tingly --home-dir /app tingly \
    && mkdir -p /app/.tingly-box \
    && chown -R tingly:tingly /app

# Install the requested release directly with npm. pm2-runtime becomes PID 1
# later and forwards container signals to the foreground Tingly Box process.
RUN npm install --global \
      npm@10.8.2 \
      pm2@7.0.1 \
      "tingly-box@${TINGLY_VERSION}" \
    && npm cache clean --force

WORKDIR /app
ENV HOME=/app

# Run the installed CLI as the same unprivileged user used at runtime. This
# materializes and executes the packaged platform binary. Exact semver builds
# additionally assert the version; dist-tags such as the default `latest`
# cannot be compared literally and only require a valid version response.
RUN VERSION_OUTPUT="$(su tingly -c "tingly-box version")" \
    && printf '%s\n' "$VERSION_OUTPUT" \
    && INSTALLED_VERSION="$(printf '%s\n' "$VERSION_OUTPUT" | awk '$1 == "Version:" { print $2 }')" \
    && test -n "$INSTALLED_VERSION" \
    && case "${TINGLY_VERSION#v}" in \
         [0-9]*.[0-9]*.[0-9]*) test "$INSTALLED_VERSION" = "v${TINGLY_VERSION#v}" ;; \
         *) true ;; \
       esac

# This small entrypoint remains necessary for bind mounts. Docker creates a
# new host bind mount with the host user's ownership; it must be made writable
# before the process drops to the unprivileged tingly user.
COPY build/docker/docker-entrypoint-npm.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

EXPOSE 12580
VOLUME ["/app/.tingly-box"]

HEALTHCHECK --interval=30s --timeout=10s --retries=3 CMD ["tingly-box", "version"]

# PM2 supervises the actual foreground server instead of starting a second
# `pm2 logs` process. `--` separates PM2 flags from Tingly Box arguments.
CMD ["sh", "-c", "DEBUG_ARGS=''; [ \"${TINGLY_DEBUG}\" = 'true' ] && DEBUG_ARGS='--verbose --debug'; exec pm2-runtime start /usr/local/bin/tingly-box --name tingly-box -- start --no-daemon --host \"${TINGLY_HOST}\" --port \"${TINGLY_PORT}\" ${DEBUG_ARGS}"]
