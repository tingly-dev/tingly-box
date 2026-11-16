#!/usr/bin/env bash
# Regression guard for #1662: a freshly `mkdir`ed host directory bind-mounted
# into a Tingly Box image, owned by the invoking host user (not the image's
# non-root "tingly", UID/GID 999), must come up without a manual `chown`.
#
# Runs <image> against exactly that kind of directory and fails if the
# container dies, its logs mention a permission error, or the entrypoint
# never actually writes into the mount (memory/logs/db all nest under it,
# see internal/config/app_config.go).
#
# Usage:
#   docker-smoke-test.sh <image> <container-data-dir>
#
#   <image>              image tag to run, e.g. tingly-box-smoke:build
#   <container-data-dir> bind-mount target inside the container, e.g.
#                        /app/.tingly-box for both image variants
set -euo pipefail

IMAGE="${1:?usage: docker-smoke-test.sh <image> <container-data-dir>}"
DATA_DIR="${2:?usage: docker-smoke-test.sh <image> <container-data-dir>}"

# The entrypoint's `chown -R` reassigns the bind-mounted directory itself
# (not just its contents) to the container's "tingly" (UID/GID 999) — that's
# the fix working as intended, but it also means the *host* user that
# created the directory (this script, when not root) can no longer `ls` or
# `rm` a 0700 dir it no longer owns. Use sudo for anything touching the
# mount after the container has run, same as an operator would.
priv() {
    if [ "$(id -u)" != "0" ] && command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        "$@"
    fi
}

HOST_DIR="$(mktemp -d)"
CONTAINER="tingly-box-smoke-$$"

cleanup() {
    docker logs "$CONTAINER" 2>&1 | tail -100 || true
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    priv rm -rf "$HOST_DIR"
}
trap cleanup EXIT

echo "== host dir before run =="
ls -ld "$HOST_DIR"

# Deliberately NOT chowned — this is the exact shape of the bug: a plain
# `mkdir` on the host, owned by whoever ran it (the CI runner user here,
# UID/GID 999 the container's "tingly" in the reported issue).
docker run -d --name "$CONTAINER" -p 12580:12580 \
    -v "$HOST_DIR:$DATA_DIR" \
    "$IMAGE"

echo "== waiting for the entrypoint to fix ownership and the app to write into the mount =="
ok=0
for _ in $(seq 1 30); do
    if ! docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q true; then
        echo "FAIL: container exited early" >&2
        break
    fi
    if docker logs "$CONTAINER" 2>&1 | grep -qi "permission denied"; then
        echo "FAIL: 'permission denied' in container logs" >&2
        break
    fi
    # Anything landing in the mount (config.json, memory/, logs/, db/) proves
    # the entrypoint's chown + privilege-drop actually let the app write here.
    if [ -n "$(priv find "$HOST_DIR" -mindepth 1 -print -quit 2>/dev/null)" ]; then
        ok=1
        break
    fi
    sleep 2
done

echo "== host dir after run =="
priv ls -la "$HOST_DIR"

if [ "$ok" != "1" ]; then
    echo "FAIL: nothing was written to $HOST_DIR (bind-mounted at $DATA_DIR) within the timeout" >&2
    exit 1
fi

echo "OK: $IMAGE wrote into a bind mount it did not own at container start"

# The entrypoint must resolve the same HOME/data path after dropping privileges.
docker run --rm "$IMAGE" sh -c 'test "$HOME" = /app'

# Recreate the container against the same storage; an operator's existing file
# must remain readable and unchanged after ownership adjustment.
docker stop "$CONTAINER" >/dev/null
docker rm "$CONTAINER" >/dev/null
# shellcheck disable=SC2016 # Expanded by the privileged child shell.
priv sh -c 'printf "%s" persistence-sentinel > "$1/persistence-sentinel"' sh "$HOST_DIR"
docker run --rm -v "$HOST_DIR:$DATA_DIR" "$IMAGE" sh -c \
    'test "$(cat /app/.tingly-box/persistence-sentinel)" = persistence-sentinel'

# Old source-image mounts must fail explicitly, even when currently empty.
if legacy_output="$(docker run --rm -v "$HOST_DIR:/home/tingly/.tingly-box" "$IMAGE" true 2>&1)"; then
    echo "FAIL: legacy mount was silently accepted" >&2
    exit 1
fi
printf '%s\n' "$legacy_output" | grep -q "ERROR: legacy data path"
priv test -f "$HOST_DIR/persistence-sentinel"
echo "OK: unified HOME, recreation persistence and legacy-mount rejection"
