#!/bin/bash
# Enterprise module test runner
# Runs all enterprise module tests with coverage

set -e

echo "═══════════════════════════════════════════════════════════════"
echo "        ENTERPRISE MODULE TEST RUNNER                          "
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$REPO_ROOT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test packages
TEST_PACKAGES=(
    "./internal/enterprise/auth"
    "./internal/enterprise/user"
    "./internal/enterprise/token"
    "./internal/enterprise/rbac"
)

PASSED=0
FAILED=0

# Run tests for each package
for pkg in "${TEST_PACKAGES[@]}"; do
    echo "Testing: $pkg"
    echo "----------------------------------------"

    if go test -v "$pkg" 2>&1; then
        echo -e "${GREEN}✓ PASSED${NC}"
        ((PASSED++))
    else
        echo -e "${RED}✗ FAILED${NC}"
        ((FAILED++))
    fi
    echo ""
done

# Summary
echo "═══════════════════════════════════════════════════════════════"
echo "                        TEST SUMMARY                               "
echo "═══════════════════════════════════════════════════════════════"
echo "Total Packages: $((PASSED + FAILED))"
echo -e "Passed:         ${GREEN}$PASSED${NC}"
echo -e "Failed:         ${RED}$FAILED${NC}"
echo ""

if [ $FAILED -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Some tests failed. Please review the output above.${NC}"
    exit 1
else
    echo -e "${GREEN}✓ All tests passed!${NC}"
fi
