#!/bin/bash
# Tingly Box Notify Hook for Claude Code (Push Only)
#
# This script handles PUSH-ONLY notifications that do NOT require user approval.
# It forwards the event to Tingly Box for desktop or IM delivery and exits immediately.
#
# Supported events:
#   - Stop (task completion notification)
#   - PostToolUse (tool finished notification)
#   - Notification with "completion" or other non-permission messages
#
# For INTERACTIVE approval hooks, use tingly-im-hook.sh instead.
#
# Usage (from Claude Code settings.json hooks):
#   {
#     "hooks": {
#       "Stop": [{
#         "matcher": "",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-notify.sh" }]
#       }],
#       "Notification": [{
#         "matcher": "completion",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-notify.sh" }]
#       }]
#     }
#   }

set -u

CC_INPUT=$(cat)

TINGLY_API_URL="${TINGLY_API_URL:-http://localhost:12580}"
TINGLY_SCENARIO="${TINGLY_SCENARIO:-claude_code}"

# Forward the full Claude Code hook input to Tingly Box.
# This is fire-and-forget: we don't care about the response.
# The server will deliver desktop notification or forward to IM if configured.
echo "$CC_INPUT" | curl -s -X POST \
  -H "Content-Type: application/json" \
  -d @- \
  "${TINGLY_API_URL}/tingly/${TINGLY_SCENARIO}/notify" 2>/dev/null || true

exit 0
