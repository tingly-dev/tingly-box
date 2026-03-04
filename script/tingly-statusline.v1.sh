#!/bin/bash
# Tingly Box Status Line Integration for Claude Code
# https://code.claude.com/docs/en/statusline.md
#
# This script integrates Tingly Box proxy status with Claude Code's status line.
# It combines Claude Code's context info with Tingly Box's current request info.
#
# Installation:
#   1. Copy this script to ~/.claude/tingly-statusline.sh
#   2. chmod +x ~/.claude/tingly-statusline.sh
#   3. Add to ~/.claude/settings.json:
#      {
#        "statusLine": {
#          "type": "command",
#          "command": "~/.claude/tingly-statusline.sh"
#        }
#      }

set -e

# Configuration
TINGLY_API_URL="${TINGLY_API_URL:-http://localhost:12580}"
CACHE_FILE="/tmp/tingly-statusline-cache"
CACHE_TTL=3  # seconds

# Colors
CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
DIM='\033[2m'
RESET='\033[0m'

# Check if cache is stale
cache_is_stale() {
    [ ! -f "$CACHE_FILE" ] || \
    [ $(($(date +%s) - $(stat -c %Y "$CACHE_FILE" 2>/dev/null || echo 0))) -gt $CACHE_TTL ]
}

# Fetch Tingly Box status with caching
fetch_tingly_status() {
    if cache_is_stale; then
        curl -s "${TINGLY_API_URL}/api/v1/current-request" 2>/dev/null > "$CACHE_FILE" || echo '{"success":false}' > "$CACHE_FILE"
    fi
    cat "$CACHE_FILE"
}

# Read Claude Code JSON from stdin
CC_INPUT=$(cat)

# Extract Claude Code fields
CC_MODEL=$(echo "$CC_INPUT" | jq -r '.model.display_name // "unknown"' 2>/dev/null || echo "unknown")
CC_PCT=$(echo "$CC_INPUT" | jq -r '.context_window.used_percentage // 0' 2>/dev/null | cut -d. -f1 || echo "0")
CC_COST=$(echo "$CC_INPUT" | jq -r '.cost.total_cost_usd // 0' 2>/dev/null || echo "0")

# Fetch Tingly Box status
TINGLY_STATUS=$(fetch_tingly_status)
TINGLY_MODEL=$(echo "$TINGLY_STATUS" | jq -r '.data.model // ""' 2>/dev/null || echo "")
TINGLY_PROVIDER=$(echo "$TINGLY_STATUS" | jq -r '.data.provider_name // ""' 2>/dev/null || echo "")

# Build context bar
BAR_WIDTH=8
FILLED=$((CC_PCT * BAR_WIDTH / 100))
EMPTY=$((BAR_WIDTH - FILLED))
BAR=""
[ "$FILLED" -gt 0 ] && BAR=$(printf "%${FILLED}s" | tr ' ' '▓')
[ "$EMPTY" -gt 0 ] && BAR="${BAR}$(printf "%${EMPTY}s" | tr ' ' '░')"

# Pick bar color based on context usage
if [ "$CC_PCT" -ge 90 ]; then
    BAR_COLOR="$RED"
elif [ "$CC_PCT" -ge 70 ]; then
    BAR_COLOR="$YELLOW"
else
    BAR_COLOR="$GREEN"
fi

# Build output line
OUTPUT="${CYAN}[${CC_MODEL}]${RESET}"

# Add Tingly Box info if available
if [ -n "$TINGLY_MODEL" ] && [ "$TINGLY_MODEL" != "null" ]; then
    OUTPUT="${OUTPUT} ${DIM}→${RESET} ${GREEN}${TINGLY_MODEL}@${TINGLY_PROVIDER}${RESET}"
fi

# Add context bar and cost
COST_FMT=$(printf '$%.2f' "$CC_COST")
echo -e "${OUTPUT} | ${BAR_COLOR}${BAR}${RESET} ${CC_PCT}% | ${YELLOW}${COST_FMT}${RESET}"