# Docker Guide for Tingly Box

This guide explains how to use Tingly Box with Docker.

## Overview

Two Dockerfiles live under `build/docker/`:

1. **`docker.build.Dockerfile`** - Multi-stage image built directly from the Go and frontend source.
2. **`docker.npm.Dockerfile`** - Lightweight image that installs the published `tingly-box` package directly with npm. This is used by the release pipeline and `docker-compose.yml`.

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
  -v "$(pwd)/tingly-data:/app/.tingly-box" \
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
- `TINGLY_DEBUG` - Enable debug mode (npm image only, default: `false`)

### Volume Mounts

Config, memory, logs and the database all live under a single directory tree
(see `internal/config/app_config.go`), so only one bind mount is needed:

- `docker.build.Dockerfile`: `/home/tingly/.tingly-box`
- `docker.npm.Dockerfile` / `docker-compose.yml`: `/app/.tingly-box`

#### Docker Desktop (macOS / Windows): prefer a named volume

The data directory holds a SQLite database in WAL mode. SQLite relies on
file locks and a shared-memory file (`tingly.db-shm`) that host-directory
bind mounts on Docker Desktop (virtiofs / gRPC-FUSE) do not honour reliably,
and SQLite's own documentation rules WAL out on such filesystems. On Linux
hosts a bind mount is a plain local directory and is fine.

On Docker Desktop, keep the database on a named volume, which lives inside
the Docker VM's own filesystem:

```bash
docker volume create tingly-data
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v tingly-data:/app/.tingly-box \
  ghcr.io/tingly-dev/tingly-box
```

To copy an existing bind-mounted directory into the volume, stop the
container first so the WAL is checkpointed, then:

```bash
docker run --rm -v "$(pwd)/tingly-data:/src:ro" -v tingly-data:/dst alpine \
  sh -c 'cp -a /src/. /dst/'
```

Whatever the mount type, only the server process should open the database.
The images' health checks probe `/api/v1/info/health` over HTTP for that
reason, and `tingly-box version` never opens the data directory; do not add
a cron or sidecar that runs other `tingly-box` subcommands against a live
server's data directory.

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
     `/app/.tingly-box` for the npm image / Compose).

4. **`database disk image is malformed` / usage dashboard returns HTTP 500**
   - The SQLite file was written by more than one process, or lives on a
     filesystem whose locking SQLite cannot trust — on Docker Desktop this is
     a host-directory bind mount (see *Docker Desktop: prefer a named volume*
     above). Images before this note also ran a `tingly-box` CLI process as
     the container health check, which opened the database every 30s.
   - To recover, stop the container and rebuild the database from whatever
     is still readable:
     ```bash
     cd <data-dir>/db
     sqlite3 tingly.db ".recover" | sqlite3 tingly.recovered.db
     sqlite3 tingly.recovered.db "PRAGMA integrity_check"
     mv tingly.db tingly.db.corrupt && rm -f tingly.db-wal tingly.db-shm
     mv tingly.recovered.db tingly.db
     ```
     Rows on damaged pages are lost; everything else (providers, rules,
     tokens, the remaining usage history) comes back. Then move the data
     directory to a named volume before starting the container again.

## Building for Different Platforms

```bash
# Build for ARM64 (Apple Silicon)
docker buildx build --platform linux/arm64 -f build/docker/docker.build.Dockerfile -t tingly-box:arm64 .

# Build for AMD64 (Intel/AMD)
docker buildx build --platform linux/amd64 -f build/docker/docker.build.Dockerfile -t tingly-box:amd64 .

# Build multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 -f build/docker/docker.build.Dockerfile -t tingly-box:latest .
```
