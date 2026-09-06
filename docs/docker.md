# Docker Guide for Tingly Box

This guide explains how to use Tingly Box with Docker.

## Overview

Two Dockerfiles live under `build/docker/`:

1. **`docker.build.Dockerfile`** - Multi-stage build from source (Go + frontend). This is what the published `ghcr.io/tingly-dev/tingly-box` images are built from.
2. **`docker.npx.Dockerfile`** - Lightweight image that installs the published `tingly-box` npm package. Used by `docker-compose.yml`.

Both run as a non-root `tingly` user and both ship an entrypoint that fixes up
ownership of the bind-mounted data directory at container start, so a plain
`mkdir` + bind mount works without a manual `chown` on the host.

## Quick Start

### Using the published image

```bash
mkdir tingly-data
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v "$(pwd)/tingly-data:/home/tingly/.tingly-box" \
  ghcr.io/tingly-dev/tingly-box
```

Open `http://localhost:12580` in your browser (the container logs print the
full login URL).

Image tags mirror the npm dist-tags: `latest` tracks the newest stable
release, `rc` tracks the newest pre-release (e.g. `v1.2.3-rc1`), and every
release is also available under its exact version tag (`v1.2.3`). Pre-releases
never move `latest`.

### Using Docker Compose

```bash
# Start the server
docker-compose -f build/docker/docker-compose.yml up -d tingly-box

# View logs
docker-compose -f build/docker/docker-compose.yml logs -f tingly-box

# Stop the server
docker-compose -f build/docker/docker-compose.yml down
```

Compose creates `build/docker/data/.tingly-box` for you on first `up`; no
manual `mkdir` or `chown` is needed.

### Manual Docker Usage (build from source)

```bash
# Build the image
docker build -f build/docker/docker.build.Dockerfile -t tingly-box:latest .

# Run the server
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v "$(pwd)/data/.tingly-box:/home/tingly/.tingly-box" \
  tingly-box:latest

# CLI usage against the same data directory
docker run -it --rm \
  -v "$(pwd)/data/.tingly-box:/home/tingly/.tingly-box" \
  tingly-box:latest tingly list
```

## Configuration

### Environment Variables

- `TINGLY_PORT` - Server port (default: `12580`)
- `TINGLY_HOST` - Server host (default: `0.0.0.0`)
- `TINGLY_DEBUG` - Enable debug mode (npx image only, default: `false`)

### Volume Mounts

Config, memory, logs and the database all live under a single directory tree
(see `internal/config/app_config.go`), so only one bind mount is needed:

- `docker.build.Dockerfile`: `/home/tingly/.tingly-box`
- `docker.npx.Dockerfile` / `docker-compose.yml`: `/app/.tingly-box`

### Running as a specific host UID/GID

The entrypoint only fixes ownership when the container starts as root (the
default). If you explicitly run with `docker run --user <uid>:<gid>`, make
sure that UID/GID already owns the mounted directory on the host — the
entrypoint leaves an explicit `--user` untouched.

## Production Tips

### Security

1. Use secrets/env files for API tokens rather than baking them into the image.
2. The image already runs as a non-root user by default.
3. Use read-only volumes where possible.

### Performance

Set memory limits in `docker-compose.yml`:
```yaml
services:
  tingly-box:
    deploy:
      resources:
        limits:
          memory: 512M
```

### Backup

Back up the `.tingly-box` directory regularly, e.g.:
```bash
tar czf tingly-config-backup.tar.gz -C data .tingly-box
```

## Troubleshooting

### Common Issues

1. **Port already in use**
   - Change the host port mapping, e.g. `-p 12581:12580`.

2. **Permission errors on the bind mount**
   - The image's entrypoint chowns the mounted directory to the container's
     `tingly` user automatically on startup as long as the container runs as
     root (the default). If you still see `permission denied`, check
     whether you passed `--user`, or whether the mount is on a filesystem
     that doesn't support `chown` (e.g. some network/FUSE mounts).

3. **Configuration not persisting**
   - Check the volume mount path matches the image you're running
     (`/home/tingly/.tingly-box` for the source-build image,
     `/app/.tingly-box` for the npx image / Compose).

## Building for Different Platforms

```bash
# Build for ARM64 (Apple Silicon)
docker buildx build --platform linux/arm64 -f build/docker/docker.build.Dockerfile -t tingly-box:arm64 .

# Build for AMD64 (Intel/AMD)
docker buildx build --platform linux/amd64 -f build/docker/docker.build.Dockerfile -t tingly-box:amd64 .

# Build multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 -f build/docker/docker.build.Dockerfile -t tingly-box:latest .
```
