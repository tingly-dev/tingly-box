#!/bin/bash

set -e

# Script to extract version from npx/package.json
# This script outputs the version for GitHub Actions

# Check if package.json exists
if [ ! -f "npx/package.json" ]; then
    echo "Error: npx/package.json not found"
    exit 1
fi

# Extract version using node
VERSION=$(node -p "require('./npx/package.json').version")

# Validate version format
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
    echo "Error: Invalid version format: $VERSION"
    exit 1
fi

# Extract tag from package.json name
PACKAGE_NAME=$(node -p "require('./npx/package.json').name")

# Create full tag name (e.g., npx-v1.0.0)
FULL_TAG="npx-v$VERSION"

# Output for GitHub Actions
echo "version=$VERSION" >> $GITHUB_OUTPUT
echo "full-tag=$FULL_TAG" >> $GITHUB_OUTPUT
echo "package-name=$PACKAGE_NAME" >> $GITHUB_OUTPUT

echo "Extracted version: $VERSION"
echo "Full tag: $FULL_TAG"
echo "Package name: $PACKAGE_NAME"