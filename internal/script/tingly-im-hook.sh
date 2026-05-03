#!/bin/bash
# Tingly Box IM Hook for Claude Code
#
# This script handles INTERACTIVE hooks that require user approval via IM.
# It posts the hook event and long-polls until the user responds.
#
# Supported events:
#   - PreToolUse (all tool calls requiring permission)
#   - AskUserQuestion
#   - Notification with "permission" or "approve" keywords
#
# Usage (from Claude Code settings.json hooks):
#   {
#     "hooks": {
#       "PreToolUse": [{
#         "matcher": "",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-im-hook.sh" }]
#       }],
#       "Notification": [{
#         "matcher": "permission",
#         "hooks": [{ "type": "command", "command": "~/.claude/tingly-im-hook.sh" }]
#       }]
#     }
#   }

set -u

CC_INPUT=$(cat)

TINGLY_API_URL="${TINGLY_API_URL:-http://localhost:12580}"
TINGLY_SCENARIO="${TINGLY_SCENARIO:-claude_code}"
TINGLY_HOOK_POLL_SECONDS="${TINGLY_HOOK_POLL_SECONDS:-45}"
TINGLY_HOOK_TOTAL_BUDGET_SECONDS="${TINGLY_HOOK_TOTAL_BUDGET_SECONDS:-300}"

# json_field <key> < json   -> echoes the string value of the top-level
# key (or empty if missing). Avoids a hard jq dependency.
json_field() {
  local key="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r --arg k "$key" '.[$k] // empty' 2>/dev/null
  else
    # Best-effort grep fallback for our own server response shape.
    sed -n "s/.*\"${key}\":\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
  fi
}

# Extract a nested .decision JSON object as a single line. Falls back to
# an empty body when jq is unavailable so we never block Claude.
json_decision() {
  if command -v jq >/dev/null 2>&1; then
    jq -c '.decision // empty' 2>/dev/null
  else
    cat
  fi
}

POST_BODY=$(mktemp)
trap 'rm -f "$POST_BODY"' EXIT

printf '%s' "$CC_INPUT" >"$POST_BODY"

# POST the hook event. -w '%{http_code}' lets us split body from status
# without having to invoke curl twice.
RESP=$(curl -sS -o - -w '\n%{http_code}' \
  -X POST \
  -H 'Content-Type: application/json' \
  --data-binary "@$POST_BODY" \
  "${TINGLY_API_URL}/tingly/${TINGLY_SCENARIO}/notify" 2>/dev/null) || {
  # Network failure: do not block Claude.
  exit 0
}

HTTP_CODE="${RESP##*$'\n'}"
BODY="${RESP%$'\n'*}"

# Only handle 202 (interactive); everything else exits immediately
case "$HTTP_CODE" in
  202)
    : # interactive — proceed to long-poll
    ;;
  200)
    # Push delivered (no IM binding configured); continue without blocking.
    exit 0
    ;;
  404)
    # No scenario binding configured; let Claude proceed.
    exit 0
    ;;
  *)
    # Unexpected error; log to stderr but never deny.
    printf 'tingly-im-hook: HTTP %s from notify endpoint\n' "$HTTP_CODE" >&2
    exit 0
    ;;
esac

REQUEST_ID=$(printf '%s' "$BODY" | json_field request_id)
WAIT_URL=$(printf '%s' "$BODY" | json_field wait_url)
if [ -z "$REQUEST_ID" ] || [ -z "$WAIT_URL" ]; then
  printf 'tingly-im-hook: malformed interactive response\n' >&2
  exit 0
fi

START_TS=$(date +%s)
while :; do
  NOW=$(date +%s)
  ELAPSED=$((NOW - START_TS))
  if [ "$ELAPSED" -ge "$TINGLY_HOOK_TOTAL_BUDGET_SECONDS" ]; then
    # Total budget exceeded — server should have already emitted a
    # fallback decision via 410, but if we got here with no answer we
    # let Claude proceed without blocking.
    exit 0
  fi

  WAIT_RESP=$(curl -sS -o - -w '\n%{http_code}' \
    --max-time "$TINGLY_HOOK_POLL_SECONDS" \
    "${TINGLY_API_URL}${WAIT_URL}?timeout=${TINGLY_HOOK_POLL_SECONDS}s" 2>/dev/null) || {
    # Transient network error; back off briefly and retry.
    sleep 1
    continue
  }
  WAIT_CODE="${WAIT_RESP##*$'\n'}"
  WAIT_BODY="${WAIT_RESP%$'\n'*}"

  case "$WAIT_CODE" in
    200)
      # answered or cancelled — both carry a `decision` we forward.
      DECISION=$(printf '%s' "$WAIT_BODY" | json_decision)
      if [ -n "$DECISION" ] && [ "$DECISION" != "null" ]; then
        printf '%s\n' "$DECISION"
      fi
      exit 0
      ;;
    410)
      # Timeout fallback: server has computed a policy decision.
      DECISION=$(printf '%s' "$WAIT_BODY" | json_decision)
      if [ -n "$DECISION" ] && [ "$DECISION" != "null" ]; then
        printf '%s\n' "$DECISION"
      fi
      exit 0
      ;;
    504)
      # Long-poll timed out without an answer; reconnect.
      continue
      ;;
    404)
      # Request unknown server-side; let Claude proceed.
      exit 0
      ;;
    *)
      printf 'tingly-im-hook: HTTP %s from wait endpoint\n' "$WAIT_CODE" >&2
      exit 0
      ;;
  esac
done
