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

# Read Claude Code JSON from stdin
CC_INPUT=$(cat)

# Send to Tingly Box and get rendered status line
# The server handles combining Claude Code info with Tingly Box current request
echo "$CC_INPUT" | curl -s -X POST \
	-H "Content-Type: application/json" \
	-d @- \
	"${TINGLY_API_URL}/tingly/claude_code/statusline" 2>/dev/null || echo "Tingly Box unavailable"
