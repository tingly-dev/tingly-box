#!/usr/bin/env bash
set -euo pipefail

# Script to commit and push built packages to a different repository
# Usage: ./commit-packages-to-branch.sh [branch-name] [target-repo]

# Get the script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Get version from environment or use default
VERSION="${VERSION:-$(date +%Y%m%d-%H%M%S)}"
BRANCH_NAME="${1:-packages-${VERSION}}"
TARGET_REPO="${2:-tingly-dev/tingly-box-release}"

echo "📦 Committing packages to branch: $BRANCH_NAME"
echo "🎯 Target repository: $TARGET_REPO"

# Change to project root
cd "$PROJECT_ROOT"

# Check if dist directory exists
if [ ! -d "dist" ]; then
    echo "❌ Error: dist directory not found at $PROJECT_ROOT/dist"
    echo "Please build the packages first using build-executables.sh"
    exit 1
fi

# Create a temporary directory for the target repository
TEMP_DIR="/tmp/tingly-box-release-$(date +%s)"
echo "📁 Creating temporary directory: $TEMP_DIR"
mkdir -p "$TEMP_DIR"

# Clone the target repository (or initialize if it doesn't exist)
echo "🔄 Cloning target repository..."

# Check if target repo is SSH or HTTPS
if [[ "$TARGET_REPO" == git@github.com:* ]]; then
    REPO_URL="$TARGET_REPO"
    REPO_NAME=$(echo "$TARGET_REPO" | sed 's|git@github.com:||')
else
    REPO_URL="https://github.com/$TARGET_REPO.git"
    REPO_NAME="$TARGET_REPO"
fi

# Set up SSH command to use the specific key
export GIT_SSH_COMMAND="ssh -i ~/.ssh/id_ed25519 -o StrictHostKeyChecking=no"

# Test SSH connection first
if [[ "$TARGET_REPO" == git@github.com:* ]] || [[ "$TARGET_REPO" == */* ]]; then
    # If it's a repo name (owner/repo), construct SSH URL
    if [[ "$TARGET_REPO" == */* ]] && [[ "$TARGET_REPO" != git@* ]]; then
        REPO_URL="git@github.com:$TARGET_REPO.git"
        REPO_NAME="$TARGET_REPO"
    fi

#    # Test SSH connection
#    if ! ssh -i ~/.ssh/id_ed25519 -T git@github.com 2>&1 | grep -q "successfully authenticated"; then
#        echo "❌ Error: SSH authentication failed"
#        echo "Public key fingerprint:"
#        ssh-keygen -lf ~/.ssh/id_ed25519.pub
#        echo ""
#        echo "Please ensure:"
#        echo "1. The SSH key is properly configured in GitHub repository secrets"
#        echo "2. The corresponding public key is added as a deploy key to the target repository"
#        exit 1
#    fi
fi

# Try to clone the repository
if git ls-remote "$REPO_URL" &>/dev/null; then
    echo "✅ Repository accessible: $REPO_NAME"
    # Use the specific SSH key for cloning
    GIT_SSH_COMMAND="ssh -i ~/.ssh/id_ed25519 -o StrictHostKeyChecking=no" git clone "$REPO_URL" "$TEMP_DIR"
else
    echo "❌ Error: Target repository does not exist or is not accessible"
    echo "Repository URL: $REPO_URL"
    echo ""
    echo "Please ensure:"
    echo "1. The repository exists at https://github.com/$REPO_NAME"
    echo "2. You have the correct SSH key configured in your secrets"
    echo "3. The SSH key has the necessary permissions"
    exit 1
fi

# Change to the target repository directory
cd "$TEMP_DIR"

# Ensure all git commands use the specific SSH key
export GIT_SSH_COMMAND="ssh -i ~/.ssh/id_ed25519 -o StrictHostKeyChecking=no"

# Create a new orphan branch
echo "🌳 Creating new orphan branch: $BRANCH_NAME"
git checkout --orphan "$BRANCH_NAME" 2>/dev/null || {
    # Branch might already exist, switch to it
    git checkout "$BRANCH_NAME" 2>/dev/null || true
}

# Remove all files from the working directory (except .git)
git rm -rf . > /dev/null 2>&1 || true

# Create a simple README for the packages branch
cat > README.md << EOF
# Built Packages

This branch contains the built packages for Tingly Box version ${VERSION}.

## Directory Structure

- \`dist/\` - Contains all built executables for different platforms

## Platform Support

The following platforms are supported:

- Darwin (macOS) - amd64, arm64
- Linux - amd64, arm64
- Windows - amd64

Built on: $(date -u)
Commit: ${GITHUB_SHA:-local}
EOF

# Add the README
git add README.md

# Copy the dist directory contents from the source project
if [ -n "$(ls -A "$PROJECT_ROOT/dist")" ]; then
    echo "📋 Adding built packages from $PROJECT_ROOT/dist/"
    cp -r "$PROJECT_ROOT"/dist/* ./

    # Add all package files
    git add .
else
    echo "⚠️ Warning: dist directory is empty"
fi

# Commit the changes
echo "💾 Committing packages to branch $BRANCH_NAME"
git commit -m "Add built packages for version ${VERSION}

Platform support:
- Darwin (macOS) - amd64, arm64
- Linux - amd64, arm64
- Windows - amd64

Built on: $(date -u)
"

# Push the new branch to the target repository
echo "🚀 Pushing branch $BRANCH_NAME to $REPO_URL"
git push -u origin "$BRANCH_NAME"

# Cleanup temporary directory
echo "🧹 Cleaning up temporary directory"
rm -rf "$TEMP_DIR"

echo "✅ Successfully committed and pushed packages to:"
echo "   Repository: $REPO_NAME"
echo "   Branch: $BRANCH_NAME"
echo ""
echo "To use this branch:"
if [[ "$REPO_URL" == git@github.com:* ]]; then
    echo "  git clone $REPO_URL.git"
else
    echo "  git clone $REPO_URL"
fi
echo "  cd $(basename $REPO_NAME .git)"
echo "  git checkout $BRANCH_NAME"
echo "  # Packages are available in the dist/ subdirectories"