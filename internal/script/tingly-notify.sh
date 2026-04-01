#!/bin/bash
# Tingly Box Notify Hook for Claude Code
#
# Claude Code hooks pass context via stdin as JSON:
#   {
#     "session_id": "...",
#     "transcript_path": "...",
#     "cwd": "...",
#     "permission_mode": "default",
#     "hook_event_name": "Stop|Notification",
#     "stop_hook_active": false,
#     "last_assistant_message": "..."
#   }
#
# This script forwards the event to Tingly Box for desktop notification.
#
# Usage (from Claude Code settings.json hooks):
#   {
#     "hooks": {
#       "Notification": [{
#         "matcher": "permission",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-notify.sh" }]
#       }],
#       "Stop": [{
#         "matcher": "",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-notify.sh" }]
#       }]
#     }
#   }

set -e

CC_INPUT=$(cat)

TINGLY_API_URL="${TINGLY_API_URL:-http://localhost:12580}"
TINGLY_SCENARIO="${TINGLY_SCENARIO:-claude_code}"

# Forward the full Claude Code hook input to Tingly Box
echo "$CC_INPUT" | curl -s -X POST \
  -H "Content-Type: application/json" \
  -d @- \
  "${TINGLY_API_URL}/tingly/${TINGLY_SCENARIO}/notify" 2>/dev/null || true
