#!/bin/bash

# Download and extract tingly-box binaries from GitHub release for local testing
# Usage: ./download.sh <tag>
# Example: ./download.sh v1.6.1

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if tag is provided
if [ -z "$1" ]; then
    echo -e "${RED}Error: Please provide a release tag${NC}"
    echo "Usage: $0 <tag>"
    echo "Example: $0 v1.6.1"
    exit 1
fi

TAG="$1"
BASE_URL="https://github.com/tingly-dev/tingly-box/releases/download"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${SCRIPT_DIR}/bin"

# Platforms to download (matching the CI build)
PLATFORMS=(
    "darwin-arm64"
    "darwin-amd64"
    "linux-amd64"
    "linux-arm64"
    "windows-amd64"
)

echo -e "${GREEN}📦 Downloading tingly-box binaries for ${TAG}${NC}"
echo ""

# Create bin directories
for platform in "${PLATFORMS[@]}"; do
    mkdir -p "${BIN_DIR}/${platform}"
done

# Download and extract each platform
for platform in "${PLATFORMS[@]}"; do
    # Determine zip filename
    IFS='-' read -r OS ARCH <<< "$platform"

    # Map platform names to release zip naming
    if [[ "$OS" == "darwin" ]]; then
        RELEASE_OS="macos"
    elif [[ "$OS" == "linux" ]]; then
        RELEASE_OS="linux"
    elif [[ "$OS" == "windows" ]]; then
        RELEASE_OS="windows"
    fi

    if [[ "$ARCH" == "amd64" ]]; then
        RELEASE_ARCH="amd64"
    elif [[ "$ARCH" == "arm64" ]]; then
        RELEASE_ARCH="arm64"
    fi

    ZIP_FILE="tingly-box-${RELEASE_OS}-${RELEASE_ARCH}.zip"
    ZIP_URL="${BASE_URL}/${TAG}/${ZIP_FILE}"
    DEST_DIR="${BIN_DIR}/${platform}"

    echo -e "${YELLOW}⬇️  Downloading ${ZIP_FILE}...${NC}"

    # Download to temp file
    TEMP_ZIP="/tmp/${ZIP_FILE}"

    if curl -fL --progress-bar "${ZIP_URL}" -o "${TEMP_ZIP}"; then
        echo -e "${GREEN}✅ Downloaded ${ZIP_FILE}${NC}"

        # Extract zip
        echo -e "${YELLOW}📦 Extracting to ${DEST_DIR}...${NC}"

        if [[ "$OS" == "windows" ]]; then
            BINARY_NAME="tingly-box.exe"
        else
            BINARY_NAME="tingly-box"
        fi

        # Use unzip to extract the binary
        unzip -q -o "${TEMP_ZIP}" -d "${DEST_DIR}" 2>/dev/null || true

        # The zip might contain a directory structure, find and move the binary
        if [[ -f "${DEST_DIR}/${BINARY_NAME}" ]]; then
            # Binary is already in place
            :
        elif [[ -f "${DEST_DIR}/tingly-box" ]]; then
            # On non-windows, binary might be named tingly-box
            if [[ "$OS" != "windows" ]]; then
                :
            fi
        else
            # Try to find the binary in subdirectories
            FOUND=$(find "${DEST_DIR}" -name "${BINARY_NAME}" -o -name "tingly-box" | head -1)
            if [[ -n "$FOUND" ]]; then
                mv "$FOUND" "${DEST_DIR}/${BINARY_NAME}"
                # Clean up any extracted directories
                find "${DEST_DIR}" -type d -empty -delete 2>/dev/null || true
            fi
        fi

        # Make binary executable on Unix
        if [[ "$OS" != "windows" ]]; then
            chmod +x "${DEST_DIR}/tingly-box" 2>/dev/null || true
        fi

        echo -e "${GREEN}✅ Extracted ${platform}${NC}"
        rm -f "${TEMP_ZIP}"
    else
        echo -e "${RED}❌ Failed to download ${ZIP_FILE}${NC}"
        echo -e "${RED}   URL: ${ZIP_URL}${NC}"
        exit 1
    fi

    echo ""
done

# Verify all binaries were downloaded
echo -e "${GREEN}🔍 Verifying downloaded binaries...${NC}"
MISSING=0

for platform in "${PLATFORMS[@]}"; do
    IFS='-' read -r OS ARCH <<< "$platform"

    if [[ "$OS" == "windows" ]]; then
        BINARY_NAME="tingly-box.exe"
    else
        BINARY_NAME="tingly-box"
    fi

    BINARY_PATH="${BIN_DIR}/${platform}/${BINARY_NAME}"

    if [[ -f "${BINARY_PATH}" ]]; then
        SIZE=$(du -h "${BINARY_PATH}" | cut -f1)
        echo -e "${GREEN}✅ ${platform}: ${SIZE}${NC}"
    else
        echo -e "${RED}❌ ${platform}: Missing binary${NC}"
        MISSING=1
    fi
done

echo ""

if [[ $MISSING -eq 0 ]]; then
    echo -e "${GREEN}✅ All binaries downloaded successfully!${NC}"
    echo ""
    echo "You can now test locally:"
    echo "  cd ${SCRIPT_DIR}"
    echo "  node bin.js start"
else
    echo -e "${RED}❌ Some binaries are missing${NC}"
    exit 1
fi
