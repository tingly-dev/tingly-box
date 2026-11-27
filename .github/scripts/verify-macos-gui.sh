#!/usr/bin/env bash
set -euo pipefail

# Build locally with the release Taskfile; never create or upload a release.
: "${RUNNER_TEMP:?RUNNER_TEMP must name a temporary directory}"
export FRONTEND_PREBUILT=true
task darwin:package ARCH=arm64 VERSION=v0.0.0 2>&1 | tee "$RUNNER_TEMP/macos-package.log"

# A successful link with a newer deployment target in some objects is not
# sufficient evidence that the app supports its declared macOS minimum.
if grep -E 'built for newer.*macOS' "$RUNNER_TEMP/macos-package.log"; then
  echo '::error::macOS object deployment targets exceed the link target'
  exit 1
fi

bundle=bin/TinglyBox.app
binary="$bundle/Contents/MacOS/tingly-box-gui"
codesign --verify --deep --strict --verbose=2 "$bundle"
test "$(lipo -archs "$binary")" = arm64
test "$(plutil -extract LSMinimumSystemVersion raw -o - "$bundle/Contents/Info.plist")" = 12.0.0
xcrun vtool -show-build "$binary" | tee "$RUNNER_TEMP/macos-build-version.txt"
minimum=$(awk '$1 == "minos" { print $2 }' "$RUNNER_TEMP/macos-build-version.txt")
case "$minimum" in
  12.0|12.0.0) ;;
  *) echo "::error::Expected macOS 12.0 deployment target, got $minimum"; exit 1 ;;
esac

# This bundle does not ship third-party dylibs. Reject developer-machine or
# unresolved @rpath dependencies instead of shipping a binary that only links
# on the build runner.
otool -L "$binary" | tee "$RUNNER_TEMP/macos-dylibs.txt"
awk 'NR > 1 { print $1 }' "$RUNNER_TEMP/macos-dylibs.txt" | while IFS= read -r library; do
  case "$library" in
    /System/Library/*|/usr/lib/*) ;;
    *) echo "::error::Unexpected non-system dependency: $library"; exit 1 ;;
  esac
done

# Match release-gui.yml's archive flags and check the extracted app, too.
archive="$RUNNER_TEMP/tingly-box-gui-macos-arm64.zip"
ditto -c -k --norsrc --noextattr --keepParent "$bundle" "$archive"
unzip -q "$archive" -d "$RUNNER_TEMP/macos-gui-unpacked"
codesign --verify --deep --strict --verbose=2 "$RUNNER_TEMP/macos-gui-unpacked/TinglyBox.app"
