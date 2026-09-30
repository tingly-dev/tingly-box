#!/usr/bin/env bash
# Install nvm + LTS Node + tingly-box from official sources.
# For mainland China users, prefer install-mirror.sh.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tingly-dev/tingly-box/main/script/install.sh | bash
#   bash script/install.sh

set -euo pipefail

NVM_VERSION="${NVM_VERSION:-v0.40.1}"
NODE_VERSION="${NODE_VERSION:---lts}"

log() { printf '\033[1;34m[install]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[install]\033[0m %s\n' "$*" >&2; }

# nvm needs bash/zsh; refuse plain sh so users get a clear message rather
# than a confusing array-syntax error 100 lines into the install.
if [ -z "${BASH_VERSION:-}" ]; then
  err "this script requires bash; re-run with: bash $0"
  exit 2
fi

install_nvm() {
  if [ -s "${NVM_DIR:-$HOME/.nvm}/nvm.sh" ]; then
    log "nvm already installed at ${NVM_DIR:-$HOME/.nvm}, skipping"
    return
  fi
  log "installing nvm ${NVM_VERSION}"
  curl -fsSL "https://raw.githubusercontent.com/nvm-sh/nvm/${NVM_VERSION}/install.sh" | bash
}

load_nvm() {
  export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
  # shellcheck disable=SC1091
  . "$NVM_DIR/nvm.sh"
}

install_node() {
  log "installing node (${NODE_VERSION})"
  nvm install "$NODE_VERSION"
  nvm use "$NODE_VERSION" >/dev/null
  log "node $(node -v) / npm $(npm -v)"
}

install_tingly_box() {
  log "installing tingly-box via npm"
  npm install -g tingly-box
  log "installed: $(tingly-box version 2>/dev/null || echo 'tingly-box')"
}

install_nvm
load_nvm
install_node
install_tingly_box

log "done. open a new shell or run: . \"\$NVM_DIR/nvm.sh\""
