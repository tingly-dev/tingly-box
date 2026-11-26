#!/usr/bin/env bash
# Builds the macvm guest image (.design/sandbox-macvm.md §5) on Linux (or in
# Docker): a kernel, a Debian root file system as erofs, and manifest.json.
#
#   build.sh [arm64|amd64] [outdir]      # default: the host's, ./out
#
# Needs: docker, mkfs.erofs (erofs-utils), go, and a kernel built with
# kernel.config (KERNEL=/path/to/Image to use one you have built; arm64 needs
# the uncompressed Image, amd64 a bzImage). Not run on CI yet.
set -euo pipefail

arch=${1:-$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')}
out=${2:-out}
here=$(cd "$(dirname "$0")" && pwd)
module=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
: "${KERNEL:?set KERNEL to a kernel built with $here/kernel.config}"

platform=linux/$arch
mkdir -p "$out"

# The guest's init.
(cd "$module" && GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -o "$work/tb-guest" ./vmguest/cmd/tb-guest)

# The userland: what a thread needs to run Claude Code and its tools.
cid=$(docker create --platform "$platform" debian:bookworm-slim sleep infinity)
trap 'docker rm -f "$cid" >/dev/null; rm -rf "$work"' EXIT
docker start "$cid" >/dev/null
docker exec "$cid" sh -c '
  apt-get update -qq &&
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
    bash coreutils findutils grep sed gawk git openssh-client ca-certificates curl ripgrep \
    iproute2 procps nodejs npm python3 build-essential jq &&
  npm install -g @anthropic-ai/claude-code &&
  mkdir -p /mnt /proc /sys /dev /tmp /run /work &&
  rm -rf /var/lib/apt/lists/*'
docker cp "$work/tb-guest" "$cid:/sbin/tb-guest"
docker exec "$cid" chmod 0755 /sbin/tb-guest
docker export "$cid" | (mkdir "$work/root" && tar -x -C "$work/root")

mkfs.erofs -zlz4hc "$out/rootfs.erofs" "$work/root"
cp "$KERNEL" "$out/kernel"

blob() { printf '{"size": %s, "sha256": "%s"}' "$(stat -c %s "$1" 2>/dev/null || stat -f %z "$1")" "$(sha256sum "$1" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$1" | cut -d' ' -f1)"; }
printf '{"version": 1, "arch": "%s", "kernel": %s, "rootfs": %s}\n' "$arch" "$(blob "$out/kernel")" "$(blob "$out/rootfs.erofs")" > "$out/manifest.json"
echo "image in $out; install it in the user's cache directory or point TB_SANDBOX_MACVM_IMAGE at it"
