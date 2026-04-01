#!/bin/bash
# Tingly Box Status Line Integration for Claude Code
# https://code.claude.com/docs/en/statusline.md
#
# This script integrates Tingly Box proxy status with Claude Code's status line.
# It sends Claude Code's context info to Tingly Box and receives a pre-rendered status line.
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
TINGLY_SCENARIO="${TINGLY_SCENARIO:-claude_code}"

# Read Claude Code JSON from stdin
CC_INPUT=$(cat)

# Send to Tingly Box and get rendered status line
# The server handles combining Claude Code info with Tingly Box current request
echo "$CC_INPUT" | curl -s -X POST \
	-H "Content-Type: application/json" \
	-d @- \
	"${TINGLY_API_URL}/tingly/${TINGLY_SCENARIO}/statusline" 2>/dev/null || echo "⚠ Tingly Box service stopped"

# We may config more like below
# echo -e

# GREEN='\033[32m'
# YELLOW='\033[33m'
# RESET='\033[0m'

# # Convert git SSH URL to HTTPS
# REMOTE=$(git remote get-url origin 2>/dev/null | sed 's/git@github.com:/https:\/\/github.com\//' | sed 's/\.git$//')

# if git rev-parse --git-dir > /dev/null 2>&1; then
#     BRANCH=$(git branch --show-current 2>/dev/null)
#     STAGED=$(git diff --cached --numstat 2>/dev/null | wc -l | tr -d ' ')
#     MODIFIED=$(git diff --numstat 2>/dev/null | wc -l | tr -d ' ')

#     GIT_STATUS=""
#     [ "$STAGED" -gt 0 ] && GIT_STATUS="${GREEN}+${STAGED}${RESET}"
#     [ "$MODIFIED" -gt 0 ] && GIT_STATUS="${GIT_STATUS}${YELLOW}~${MODIFIED}${RESET}"

#     if [ -n "$REMOTE" ]; then
#         REPO_NAME=$(basename "$REMOTE")
#         # OSC 8 format: \e]8;;URL\a then TEXT then \e]8;;\a
#         # printf %b interprets escape sequences reliably across shells
#         # printf '%b' "📁 ${DIR##*/} | 🌿 $BRANCH $GIT_STATUS | 🔗 \e]8;;${REMOTE}\a${REPO_NAME}\e]8;;\a\n"
#         printf '%b' "📁 ${DIR##*/} | 🌿 $BRANCH $GIT_STATUS | 🔗 ${REMOTE}\a\n"
#     else
#         echo -e "📁 ${DIR##*/} | 🌿 $BRANCH $GIT_STATUS"
#     fi
# else
#     echo "📁 ${DIR##*/}"
# fi
