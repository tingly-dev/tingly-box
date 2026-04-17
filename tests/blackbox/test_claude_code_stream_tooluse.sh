#!/usr/bin/env bash
set -euo pipefail

# Black-box stream test against an already running tingly-box instance.
#
# Defaults target the endpoint/key provided in the current debugging thread.
# You can override:
#   BASE_URL="http://localhost:8080/tingly/claude_code" \
#   API_KEY="..." \
#   bash tests/blackbox/test_claude_code_stream_tooluse.sh

BASE_URL="${BASE_URL:-http://localhost:8080/tingly/claude_code}"
API_KEY="${API_KEY:-tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjU2ODc4MywiaWF0IjoxNzY2NDgyMzgzfQ.Dp5YAV2ibWe2pYaO9sP2nzTAPTGOgNQ9ykHfz1QNs9c}"

if [[ -z "${API_KEY}" ]]; then
  echo "API_KEY is empty" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT
OUT_FILE="${TMP_DIR}/stream.sse"

echo "[info] sending stream request to: ${BASE_URL%/}/messages"

curl -sS -N \
  -X POST "${BASE_URL%/}/messages" \
  -H "content-type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -H "authorization: Bearer ${API_KEY}" \
  -H "x-api-key: ${API_KEY}" \
  --data-binary @- >"${OUT_FILE}" <<'JSON'
{
  "model": "tingly/cc",
  "max_tokens": 1024,
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "使用 ls 查看当前文件夹文件。先调用 bash，再在需要时调用 adviser。"
        }
      ]
    }
  ],
  "tools": [
    {
      "name": "bash",
      "description": "Run shell commands",
      "input_schema": {
        "type": "object",
        "properties": {
          "command": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["command", "description"]
      }
    },
    {
      "name": "tingly_box_mcp__builtin__advisor",
      "description": "consult advisor",
      "input_schema": {
        "type": "object",
        "properties": {
          "reason": { "type": "string" }
        },
        "required": ["reason"]
      }
    }
  ]
}
JSON

echo "[info] validating SSE payload"

python3 - "${OUT_FILE}" <<'PY'
import json
import re
import sys
from pathlib import Path

path = Path(sys.argv[1])
raw = path.read_text(encoding="utf-8", errors="replace")

if "invalid arguments" in raw.lower():
    print("[fail] found 'invalid arguments' in stream output", file=sys.stderr)
    sys.exit(1)

if "event:message_start" not in raw:
    print("[fail] missing event:message_start", file=sys.stderr)
    sys.exit(1)

has_bash_tool_use = False
has_bash_command = False
has_bash_description = False

for line in raw.splitlines():
    if not line.startswith("data:"):
        continue
    payload = line[5:].strip()
    if not payload or payload == "[DONE]":
        continue
    try:
        obj = json.loads(payload)
    except json.JSONDecodeError:
        continue

    content_block = obj.get("content_block") or {}
    if content_block.get("type") == "tool_use" and content_block.get("name") == "bash":
        has_bash_tool_use = True

    delta = obj.get("delta") or {}
    if delta.get("type") == "input_json_delta":
        partial_json = delta.get("partial_json", "")
        if isinstance(partial_json, str):
            if re.search(r'"command"\s*:', partial_json):
                has_bash_command = True
            if re.search(r'"description"\s*:', partial_json):
                has_bash_description = True

if not has_bash_tool_use:
    print("[fail] no bash tool_use block found in stream", file=sys.stderr)
    sys.exit(1)
if not has_bash_command or not has_bash_description:
    print("[fail] missing command/description in input_json_delta partial_json", file=sys.stderr)
    sys.exit(1)

print("[pass] stream contains bash tool_use with input_json_delta(command+description)")
PY

echo "[ok] black-box stream test passed"
