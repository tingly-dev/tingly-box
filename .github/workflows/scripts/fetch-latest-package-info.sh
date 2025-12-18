#!/bin/bash

set -e

# Script to fetch LATEST.json from release repository and extract latest package info
# This script outputs the latest package branch and version for GitHub Actions

REPOSITORY="${1:-tingly-dev/tingly-box-release}"
TEMP_DIR=$(mktemp -d)

echo "Fetching LATEST.json from $REPOSITORY..."

# Clone the repository
git clone "git@github.com:$REPOSITORY.git" "$TEMP_DIR"
cd "$TEMP_DIR"

# Switch to main branch
git checkout main

# Check if LATEST.json exists
if [ ! -f "LATEST.json" ]; then
    echo "Error: LATEST.json not found in main branch of $REPOSITORY"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "Found LATEST.json:"
cat LATEST.json

# Extract packages_latest_branch
PACKAGES_BRANCH=$(jq -r '.packages_latest_branch // empty' LATEST.json)

if [ -z "$PACKAGES_BRANCH" ]; then
    echo "Error: packages_latest_branch not found in LATEST.json"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "Extracted packages branch: $PACKAGES_BRANCH"

## Validate branch name format
#if [[ ! "$PACKAGES_BRANCH" =~ ^packages-[a-zA-Z0-9_-]+$ ]]; then
#    echo "Error: Invalid packages branch format: $PACKAGES_BRANCH"
#    rm -rf "$TEMP_DIR"
#    exit 1
#fi

# Extract version from branch name
# Handle both formats: packages-v1.0.0 and packages-20241218-123456
if [[ "$PACKAGES_BRANCH" =~ ^v([0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?)$ ]]; then
    VERSION="${BASH_REMATCH[1]}"
    echo "Extracted version from versioned branch: $VERSION"
elif [[ "$PACKAGES_BRANCH" =~ ^manual-([0-9]{8}-[0-9]{6})$ ]]; then
    # For manual branches, use current date or a default version
    VERSION="$(date +%Y.%m.%d)"
    echo "Using date-based version for manual branch: $VERSION"
else
    # Fallback: use current date
    VERSION="$(date +%Y.%m.%d)"
    echo "Using date-based version as fallback: $VERSION"
fi

# Get the current NPX version from npx/package.json
if [ -f "../../../npx/package.json" ]; then
    CURRENT_NPX_VERSION=$(node -p "require('../../../npx/package.json').version")
    echo "Current NPX version: $CURRENT_NPX_VERSION"
else
    echo "Warning: Could not find npx/package.json, using extracted version"
    CURRENT_NPX_VERSION="$VERSION"
fi

# Determine if we should use the extracted version or current NPX version
# If NPX version is newer (lexicographically), use it; otherwise use the package version
if [ "$VERSION" != "$(date +%Y.%m.%d)" ]; then
    # This is a versioned branch, compare versions
    if [[ "$VERSION" > "$CURRENT_NPX_VERSION" ]]; then
        FINAL_VERSION="$VERSION"
    else
        FINAL_VERSION="$CURRENT_NPX_VERSION"
    fi
else
    # This is a manual branch, always increment NPX version
    FINAL_VERSION="$CURRENT_NPX_VERSION"
fi

# Output for GitHub Actions
echo "packages_branch=$PACKAGES_BRANCH" >> $GITHUB_OUTPUT
echo "packages_version=$VERSION" >> $GITHUB_OUTPUT
echo "npx_version=$FINAL_VERSION" >> $GITHUB_OUTPUT
echo "repository=$REPOSITORY" >> $GITHUB_OUTPUT

echo "Package branch: $PACKAGES_BRANCH"
echo "Package version: $VERSION"
echo "NPX version to use: $FINAL_VERSION"

# Cleanup
cd /
rm -rf "$TEMP_DIR"

echo "✅ Successfully extracted latest package information"