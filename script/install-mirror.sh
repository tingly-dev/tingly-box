#!/usr/bin/env bash
# Mirror-aware variant of install.sh for mainland China networks.
#
# Mirrors used (override via env vars):
#   GH_PROXY     github.com / raw.githubusercontent.com proxy
#   NODE_MIRROR  nvm's NVM_NODEJS_ORG_MIRROR target
#   NPM_REGISTRY npm registry
#
# Usage:
#   bash script/install-mirror.sh

set -euo pipefail

NVM_VERSION="${NVM_VERSION:-v0.40.1}"
NODE_VERSION="${NODE_VERSION:---lts}"
GH_PROXY="${GH_PROXY:-https://gh-proxy.com}"
NODE_MIRROR="${NODE_MIRROR:-https://npmmirror.com/mirrors/node/}"
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

log() { printf '\033[1;34m[install-mirror]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[install-mirror]\033[0m %s\n' "$*" >&2; }

if [ -z "${BASH_VERSION:-}" ]; then
  err "this script requires bash; re-run with: bash $0"
  exit 2
fi

install_nvm() {
  if [ -s "${NVM_DIR:-$HOME/.nvm}/nvm.sh" ]; then
    log "nvm already installed at ${NVM_DIR:-$HOME/.nvm}, skipping"
    return
  fi
  log "installing nvm ${NVM_VERSION} via ${GH_PROXY}"
  # nvm's installer fetches its own files from raw.githubusercontent.com via
  # NVM_SOURCE; route those through the same proxy so both the bootstrap and
  # the per-file fetch succeed in restricted networks.
  local installer="${GH_PROXY}/https://raw.githubusercontent.com/nvm-sh/nvm/${NVM_VERSION}/install.sh"
  NVM_SOURCE="${GH_PROXY}/https://github.com/nvm-sh/nvm.git" \
    curl -fsSL "$installer" | bash
}

load_nvm() {
  export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
  # shellcheck disable=SC1091
  . "$NVM_DIR/nvm.sh"
}

install_node() {
  log "installing node (${NODE_VERSION}) from ${NODE_MIRROR}"
  NVM_NODEJS_ORG_MIRROR="$NODE_MIRROR" nvm install "$NODE_VERSION"
  nvm use "$NODE_VERSION" >/dev/null
  log "node $(node -v) / npm $(npm -v)"
}

configure_npm_registry() {
  log "setting npm registry to ${NPM_REGISTRY}"
  npm config set registry "$NPM_REGISTRY"
}

install_tingly_box() {
  log "installing tingly-box via npm (registry=${NPM_REGISTRY})"
  npm install -g tingly-box
  log "installed: $(tingly-box version 2>/dev/null || echo 'tingly-box')"
}

install_nvm
load_nvm
install_node
configure_npm_registry
install_tingly_box

log "done. open a new shell or run: . \"\$NVM_DIR/nvm.sh\""
