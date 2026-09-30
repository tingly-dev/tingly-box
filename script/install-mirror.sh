#!/usr/bin/env bash
# Mirror-aware variant of install.sh for mainland China networks.
#
# Defaults:
#   nvm        cloned from gitee.com/mirrors/nvm
#   node       fetched from npmmirror.com (Taobao)
#   npm reg    registry.npmmirror.com (Taobao)
#
# Override via env vars:
#   NVM_GITEE_REPO   git URL for the nvm mirror
#   NVM_VERSION      nvm tag/branch to checkout
#   NODE_VERSION     node version (e.g. 20, --lts)
#   NODE_MIRROR      NVM_NODEJS_ORG_MIRROR target
#   NPM_REGISTRY     npm registry to write into ~/.npmrc

set -euo pipefail

NVM_VERSION="${NVM_VERSION:-v0.40.1}"
NVM_GITEE_REPO="${NVM_GITEE_REPO:-https://gitee.com/mirrors/nvm.git}"
NODE_VERSION="${NODE_VERSION:---lts}"
NODE_MIRROR="${NODE_MIRROR:-https://npmmirror.com/mirrors/node/}"
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

NVM_DIR="${NVM_DIR:-$HOME/.nvm}"

log() { printf '\033[1;34m[install-mirror]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[install-mirror]\033[0m %s\n' "$*" >&2; }

if [ -z "${BASH_VERSION:-}" ]; then
  err "this script requires bash; re-run with: bash $0"
  exit 2
fi

install_nvm() {
  if [ -s "$NVM_DIR/nvm.sh" ]; then
    log "nvm already installed at $NVM_DIR, skipping"
    return
  fi
  log "cloning nvm ${NVM_VERSION} from ${NVM_GITEE_REPO}"
  git clone --depth 1 --branch "$NVM_VERSION" "$NVM_GITEE_REPO" "$NVM_DIR"
  ensure_profile_snippet
}

# ensure_profile_snippet appends NVM_DIR sourcing to the user's shell rc so
# subsequent shells pick up nvm. nvm's official install.sh does this — since
# we're cloning manually, we replicate it. Idempotent: grep before append.
ensure_profile_snippet() {
  local snippet
  snippet="$(cat <<'EOF'
# nvm (added by tingly-box install-mirror.sh)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"
EOF
  )"
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    [ -f "$rc" ] || continue
    if ! grep -q 'NVM_DIR="$HOME/.nvm"' "$rc"; then
      log "appending nvm init to $rc"
      printf '\n%s\n' "$snippet" >> "$rc"
    fi
  done
}

load_nvm() {
  export NVM_DIR
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
