#!/bin/bash
set -e

# skill.sh - Install the sdlc skill into .claude/skills/ on demand
# Usage: ./skill.sh

REPO_URL="https://github.com/FFengIll/sdlc.git"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")"/.. && pwd)"
TARGET_DIR="$SCRIPT_DIR/.claude/skills/sdlc"

echo "Installing sdlc skill ..."
echo "  from: $REPO_URL"
echo "  to:   $TARGET_DIR"

# Clean existing
mkdir -p "$TARGET_DIR"

# Clone to temp
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

git clone --depth 1 "$REPO_URL" "$TEMP_DIR/sdlc"

# Copy skill contents
cp "$TEMP_DIR/sdlc/SKILL.md" "$TARGET_DIR/"
cp "$TEMP_DIR/sdlc/sdlc.md" "$TARGET_DIR/" 2>/dev/null || true

cp -R "$TEMP_DIR/sdlc/action" "$TARGET_DIR/" 2>/dev/null || true
cp -R "$TEMP_DIR/sdlc/workflow" "$TARGET_DIR/" 2>/dev/null || true

echo ""
echo "sdlc skill installed successfully."
echo "Use with: /sdlc"
