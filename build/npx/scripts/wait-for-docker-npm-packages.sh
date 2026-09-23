#!/usr/bin/env bash
set -euo pipefail

version="${1:?Usage: wait-for-docker-npm-packages.sh <npm-version>}"
version="${version#v}"
packages=(tingly-box @tingly-dev/tingly-box-linux-x64)

# Match the registry-visibility check before the CLI shim publish in npm.yml.
# Publishing can finish before npm's read replicas serve the exact version.
for attempt in $(seq 1 30); do
  all_ready=true
  for package in "${packages[@]}"; do
    if ! npm view "${package}@${version}" version >/dev/null 2>&1; then
      echo "⏳ ${package}@${version} is not visible on npm (attempt ${attempt}/30)"
      all_ready=false
    fi
  done

  if [ "$all_ready" = true ]; then
    echo "✅ CLI and Linux platform packages are visible on npm at ${version}"
    exit 0
  fi

  if [ "$attempt" -lt 30 ]; then sleep 10; fi
done

echo "::error::npm packages for ${version} were not visible after 30 checks"
exit 1
