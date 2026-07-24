#!/usr/bin/env bash
# End-to-end demo of the full hub, using NO network / API keys:
#   client → tb (rule plugin/rag-demo) → plugin → plugin.use("experiment")
#          → tb (rule echo-model → vmodel) → echoed text → back to client
#
# Prereqs:
#   go build -o /tmp/tb_e2e ./cli/tingly-box      # the tb binary
#   pip install httpx openai                       # SDK transitive + a transport
# Run:  bash sdk/python/examples/e2e_run.sh
set -uo pipefail

TB=${TB_BIN:-/tmp/tb_e2e}
CFG=$(mktemp -d)
PORT=18901
BASE="http://127.0.0.1:$PORT"
SDK=/home/user/tingly-box/sdk/python
export PYTHONPATH=$SDK

cleanup() {
  [[ -n "${PLUG_PID:-}" ]] && kill "$PLUG_PID" 2>/dev/null
  [[ -n "${TB_PID:-}" ]] && kill "$TB_PID" 2>/dev/null
}
trap cleanup EXIT

echo "== 1. start tb (config-dir=$CFG, port=$PORT) =="
"$TB" --config-dir "$CFG" start --port "$PORT" --ui --browser=false >/tmp/tb_e2e.log 2>&1 &
TB_PID=$!
for i in $(seq 1 60); do
  curl -sf "$BASE/api/v1/info/health" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sf "$BASE/api/v1/info/health" >/dev/null || { echo "tb did not start"; tail -25 /tmp/tb_e2e.log; exit 1; }
echo "   tb healthy at $BASE"

# Tokens are generated fresh per config-dir; read them from the config file.
CFGFILE=$(find "$CFG" -name 'config.json' | head -1)
echo "   config file: $CFGFILE"
UTOK=$(python3 -c "import json,sys;d=json.load(open('$CFGFILE'));print(d.get('user_token') or d.get('UserToken',''))")
MTOK=$(python3 -c "import json,sys;d=json.load(open('$CFGFILE'));print(d.get('model_token') or d.get('ModelToken',''))")
echo "   user token:  ${UTOK:0:16}…   model token: ${MTOK:0:16}…"

UADMIN=(-H "Authorization: Bearer $UTOK" -H "Content-Type: application/json")
UMODEL=(-H "Authorization: Bearer $MTOK" -H "Content-Type: application/json")

echo "== 2. create vmodel provider (echo backend, no network) =="
VRESP=$(curl -s "${UADMIN[@]}" -X POST "$BASE/api/v2/providers" -d '{
  "name":"vmodel-echo","api_base":"vmodel://local","api_style":"openai",
  "auth_type":"vmodel","no_key_required":true,"enabled":true}')
VUUID=$(echo "$VRESP" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('data',{}).get('uuid') or d.get('uuid',''))")
echo "   vmodel provider uuid: $VUUID"

echo "== 3. create echo-model rule under experiment scenario =="
curl -s "${UADMIN[@]}" -X POST "$BASE/api/v1/rule" -d "{
  \"scenario\":\"experiment\",\"request_model\":\"echo-model\",\"active\":true,
  \"lb_tactic\":{\"type\":\"random\",\"params\":{}},
  \"services\":[{\"provider\":\"$VUUID\",\"model\":\"echo-model\",\"weight\":1,\"active\":true}]}" \
  | python3 -c "import sys,json;d=json.load(sys.stdin);print('   rule created:', d.get('success'), d.get('data',{}).get('uuid',''))"

echo "== 4. start the plugin — it registers with tb once on startup =="
echo "   (serve() → POST /api/v2/plugins, an idempotent upsert-by-name; no heartbeat)"
TINGLY_BOX_URL="$BASE" TINGLY_BOX_TOKEN="$UTOK" \
  python3 "$SDK/examples/e2e_plugin.py" >/tmp/plugin_e2e.log 2>&1 &
PLUG_PID=$!
for i in $(seq 1 40); do
  curl -sf "http://127.0.0.1:8765/health" >/dev/null 2>&1 && break
  sleep 0.3
done
curl -sf "http://127.0.0.1:8765/health" >/dev/null || { echo "plugin did not start"; cat /tmp/plugin_e2e.log; exit 1; }

echo "== 5. tb sees the plugin provider (GET /api/v2/plugins) =="
for i in $(seq 1 20); do
  LIST=$(curl -s "${UADMIN[@]}" "$BASE/api/v2/plugins")
  echo "$LIST" | grep -q 'rag-demo' && break
  sleep 0.3
done
echo "$LIST" | python3 -m json.tool

echo "== 6. CLIENT CALL: model=plugin/rag-demo through tb =="
echo "   (client → tb → plugin → tb echo-model → back)"
curl -s "${UMODEL[@]}" -X POST "$BASE/tingly/experiment/v1/chat/completions" -d '{
  "model":"plugin/rag-demo",
  "messages":[{"role":"user","content":"What is tingly-box?"}]}' | python3 -m json.tool

echo "== plugin log tail =="
tail -6 /tmp/plugin_e2e.log

echo "== 7. NO SEPARATE LIFECYCLE: kill the plugin (simulated crash) =="
echo "   (hard SIGKILL, no graceful shutdown — the provider is a normal DB row,"
echo "    same as any other provider, so it does NOT disappear from the list)"
kill -KILL "$PLUG_PID" 2>/dev/null
PLUG_PID=""
LIST=$(curl -s "${UADMIN[@]}" "$BASE/api/v2/plugins")
echo "   GET /api/v2/plugins still shows it: $LIST"

echo "== 8. client call after the plugin is dead =="
echo "   (liveness is the SAME per-service circuit breaker every provider gets;"
echo "    with no fallback tier configured on this rule the request just errors —"
echo "    add a tier-1 real model to the rule and this would tier-failover instead)"
curl -s "${UMODEL[@]}" -X POST "$BASE/tingly/experiment/v1/chat/completions" -d '{
  "model":"plugin/rag-demo",
  "messages":[{"role":"user","content":"still there?"}]}' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); e=d.get('error', d); print('   ->', (json.dumps(e) if isinstance(e,dict) else str(e))[:200])"

echo "== 9. restart the plugin → re-register upserts the SAME provider (no duplicate) =="
TINGLY_BOX_URL="$BASE" TINGLY_BOX_TOKEN="$UTOK" \
  python3 "$SDK/examples/e2e_plugin.py" >/tmp/plugin_e2e_2.log 2>&1 &
PLUG_PID=$!
for i in $(seq 1 40); do
  curl -sf "http://127.0.0.1:8765/health" >/dev/null 2>&1 && break
  sleep 0.3
done
COUNT=$(curl -s "${UADMIN[@]}" "$BASE/api/v2/plugins" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(1 for p in d['data'] if p['name']=='rag-demo'))")
echo "   plugin providers named rag-demo after restart: $COUNT (expect 1)"

echo "== done =="
