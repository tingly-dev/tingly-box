#!/usr/bin/env bash
# End-to-end demo of tingly-project against a scripted model: no API key needed.
# Starts the mock model and the server, creates a project on a fresh git repo
# and sends one message. Then open the printed URL (or run drive.cjs).
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=${WORK:-$(mktemp -d -t tingly-project-demo.XXXX)}
MOCK_PORT=${MOCK_PORT:-12581}
PORT=${PORT:-12590}
API=http://127.0.0.1:$PORT/api/v1

[ -x "$ROOT/bin/tingly-project" ] || (cd "$ROOT" && task build)

# A small repository for the threads to work in.
mkdir -p "$WORK/demo-app" "$WORK/home"
cd "$WORK/demo-app"
git init -q -b main
printf 'package main\n\nfunc main() {}\n' > main.go
printf 'module demo\n\ngo 1.22\n' > go.mod
git config user.name demo && git config user.email demo@example.com
git add . && git commit -qm init

python3 "$HERE/mock_anthropic.py" "$MOCK_PORT" "$WORK/mock.log" > "$WORK/mock.out" 2>&1 &
echo $! > "$WORK/mock.pid"
# A clean environment so the Claude Code CLI uses only the demo settings.
env -i PATH="$PATH" HOME="$WORK/home" LANG=C.UTF-8 NO_PROXY='*' no_proxy='*' \
  "$ROOT/bin/tingly-project" --port "$PORT" --data-dir "$WORK/data" > "$WORK/server.log" 2>&1 &
echo $! > "$WORK/server.pid"
for _ in $(seq 40); do curl -sf "$API/health" > /dev/null && break; sleep 0.25; done

json() { curl -sf -H 'content-type: application/json' "$@"; }
json -X PUT "$API/settings" -d "{\"api_style\":\"anthropic\",\"base_url\":\"http://127.0.0.1:$MOCK_PORT\",\"api_key\":\"demo-key\",\"default_model\":\"demo-sonnet\"}" > /dev/null
PID=$(json -X POST "$API/projects" -d "{\"name\":\"Demo App\",\"goal\":\"Ship the demo app v0.1\",\"repos\":[\"$WORK/demo-app\"]}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
json -X POST "$API/projects/$PID/messages" -d '{"content":"两件事：1) 加一个 CHANGELOG；2) 加一个 Greet(name) 辅助函数"}' > /dev/null

cat <<MSG
Demo running.
  Open:  http://127.0.0.1:$PORT/project/$PID
  Repo:  $WORK/demo-app
  Logs:  $WORK/server.log, $WORK/mock.log
  Stop:  kill \$(cat $WORK/server.pid $WORK/mock.pid)
MSG
