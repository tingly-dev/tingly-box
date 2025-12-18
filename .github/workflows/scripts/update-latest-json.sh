#!/bin/bash

set -e

# Script to update LATEST.json in the main branch of the release repository
# This script should be run after creating a new packages branch

# Check if branch name is provided
if [ -z "$1" ]; then
    echo "Error: Branch name is required"
    echo "Usage: $0 <branch-name> [repository]"
    exit 1
fi

COMMIT_SHA="$1"
BRANCH_NAME="$2"
REPOSITORY="${3:-tingly-dev/tingly-box-release}"

# Validate branch name format
#if [[ ! "$BRANCH_NAME" =~ ^packages-[a-zA-Z0-9_-]+$ ]]; then
#    echo "Error: Invalid branch name format. Expected: packages-<suffix>"
#    exit 1
#fi

echo "Updating LATEST.json in main branch..."
echo "Branch: $BRANCH_NAME"
echo "Repository: $REPOSITORY"

# Clone the repository
TEMP_DIR=$(mktemp -d)
echo "Cloning repository to temporary directory: $TEMP_DIR"

git clone "git@github.com:$REPOSITORY.git" "$TEMP_DIR"
cd "$TEMP_DIR"

# Switch to main branch
git checkout main

# Create or update LATEST.json
if [ -f "LATEST.json" ]; then
    # Update existing file
    echo "Updating existing LATEST.json..."

    # Update or add packages_latest_branch
    if jq -e '.packages_latest_branch' LATEST.json >/dev/null 2>&1; then
        # Update existing packages_latest_branch
        jq --arg branch "$BRANCH_NAME" '.packages_latest_branch = $branch' LATEST.json > LATEST.tmp && mv LATEST.tmp LATEST.json
    else
        # Add packages_latest_branch
        jq --arg branch "$BRANCH_NAME" '. + {"packages_latest_branch": $branch}' LATEST.json > LATEST.tmp && mv LATEST.tmp LATEST.json
    fi

    # Update common fields
    jq --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
       --arg sha "$COMMIT_SHA" \
       '.updated_at = $timestamp | .packages_updated_at = $timestamp | .workflow_run = $workflow | .repository = $repo | .commit_sha = $sha' \
       LATEST.json > LATEST.tmp && mv LATEST.tmp LATEST.json
else
    # Create new file
    echo "Creating new LATEST.json..."
    cat > LATEST.json << EOF
{
  "packages_latest_branch": "$BRANCH_NAME",
  "updated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "packages_updated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "commit_sha": "$COMMIT_SHA"
}
EOF
fi

echo "Created/Updated LATEST.json with content:"
cat LATEST.json

# Add and commit the changes
git add LATEST.json
git commit -m "Update LATEST.json to point to packages branch $BRANCH_NAME

- Updated packages_latest_branch to: $BRANCH_NAME
- Updated at: $(date -u +%Y-%m-%dT%H:%M:%SZ)
- Commit: $COMMIT_SHA"

# Push changes to main branch
git push origin main

echo "✅ Successfully updated LATEST.json in main branch"
echo "📄 LATEST.json packages_latest_branch now points to: $BRANCH_NAME"

# Cleanup
cd ..
rm -rf "$TEMP_DIR"

echo "🧹 Cleaned up temporary directory"