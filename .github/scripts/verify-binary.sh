#!/usr/bin/env bash
# Verify tingly-box release binary zips.
#
# Each tingly-box-<platform>.zip must be a valid zip containing the expected
# binary (tingly-box / tingly-box.exe) in the correct executable format for
# its platform, so a mis-packaged build fails in CI instead of on user
# machines (a wrong-arch tingly-box.exe surfaces on Windows as "This app
# can't run on your PC").
#
# Usage:
#   verify-binary.sh <dir> [platform ...]
#
#   <dir>       directory containing tingly-box-<platform>.zip files
#   [platform]  platforms that MUST be present and valid (e.g. windows-amd64);
#               when omitted, every tingly-box-<platform>.zip found in <dir>
#               is verified (GUI zips are skipped — different layout).
set -euo pipefail

DIR="${1:?usage: verify-binary.sh <dir> [platform ...]}"
shift || true

expected_pattern() {
  case "$1" in
    linux-amd64)   echo 'ELF 64-bit.*x86-64' ;;
    linux-arm64)   echo 'ELF 64-bit.*aarch64' ;;
    macos-amd64)   echo 'Mach-O 64-bit.*x86_64' ;;
    macos-arm64)   echo 'Mach-O 64-bit.*arm64' ;;
    windows-amd64) echo 'PE32\+ executable.*x86-64' ;;
    windows-arm64) echo 'PE32\+ executable.*(aarch64|ARM64)' ;;
    *) return 1 ;;
  esac
}

verify_zip() {
  local platform="$1"
  local zip="$DIR/tingly-box-${platform}.zip"
  local pattern binary tmp path info

  if ! pattern="$(expected_pattern "$platform")"; then
    echo "❌ $platform: unknown platform (no expected format defined)"
    return 1
  fi
  if [ ! -f "$zip" ]; then
    echo "❌ $platform: zip not found: $zip"
    return 1
  fi

  binary="tingly-box"
  case "$platform" in windows-*) binary="tingly-box.exe" ;; esac

  if ! unzip -t -q "$zip" >/dev/null 2>&1; then
    echo "❌ $platform: zip integrity check failed"
    return 1
  fi

  tmp="$(mktemp -d)"
  unzip -o -q "$zip" -d "$tmp"
  path="$(find "$tmp" -type f -name "$binary" | head -1)"
  if [ -z "$path" ]; then
    echo "❌ $platform: binary '$binary' not found in zip"
    rm -rf "$tmp"
    return 1
  fi

  info="$(file -b "$path")"
  if ! echo "$info" | grep -Eiq "$pattern"; then
    echo "❌ $platform: unexpected binary format"
    echo "   got:      $info"
    echo "   expected: $pattern"
    rm -rf "$tmp"
    return 1
  fi

  echo "✅ $platform: $info"
  rm -rf "$tmp"
}

if [ "$#" -gt 0 ]; then
  platforms=("$@")
else
  platforms=()
  for zip in "$DIR"/tingly-box-*.zip; do
    [ -e "$zip" ] || continue
    name="$(basename "$zip" .zip)"
    name="${name#tingly-box-}"
    case "$name" in gui-*) continue ;; esac
    platforms+=("$name")
  done
  if [ "${#platforms[@]}" -eq 0 ]; then
    echo "❌ no tingly-box-*.zip files found in $DIR"
    exit 1
  fi
fi

failures=0
for platform in "${platforms[@]}"; do
  verify_zip "$platform" || failures=$((failures + 1))
done

echo ""
if [ "$failures" -gt 0 ]; then
  echo "❌ $failures of ${#platforms[@]} platform(s) failed verification"
  exit 1
fi
echo "✅ all ${#platforms[@]} platform(s) verified"
