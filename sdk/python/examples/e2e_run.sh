#!/usr/bin/env bash
# End-to-end proof, using NO network and NO API keys:
#   client → tb (rule `router`) → Python provider → classify
#          → srv.use("openai").ask(model=fast-model | strong-model)
#          → tb (that rule → vmodel provider) → answer → back to the client
#
# The two target rules are backed by DIFFERENT virtual models, so the response
# text proves which branch ran: a short prompt and a long one must come back
# from different upstreams. That is the assertion that makes this a test of
# dispatch rather than of plumbing.
#
# The point of this script is as much what it does NOT do. There is no
# registration call, no plugin endpoint, no manifest. Step 4 creates the
# provider with the ordinary POST /api/v2/providers — byte for byte what the
# Connect AI dialog sends when you add Ollama — because that is the only path
# that exists.
#
# Prereqs:
#   go build -o /tmp/tb_e2e ./cli/tingly-box      # the tb binary
#   pip install httpx openai anthropic             # SDK deps
# Run:  bash sdk/python/examples/e2e_run.sh
set -uo pipefail

TB=${TB_BIN:-/tmp/tb_e2e}
CFG=$(mktemp -d)
PORT=18901
BASE="http://127.0.0.1:$PORT"
SRV_PORT=8765
SDK=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export PYTHONPATH=$SDK
PY=${PYTHON_BIN:-python3}
FAILED=0

note() { echo "   $*"; }
check() { # check <label> <condition-result>
  if [[ "$2" == "0" ]]; then echo "   PASS  $1"; else echo "   FAIL  $1"; FAILED=1; fi
}

cleanup() {
  [[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null
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
note "tb healthy at $BASE"

# Tokens are generated fresh per config-dir; read them from the config file.
CFGFILE=$(find "$CFG" -name 'config.json' | head -1)
UTOK=$($PY -c "import json;d=json.load(open('$CFGFILE'));print(d.get('user_token') or d.get('UserToken',''))")
MTOK=$($PY -c "import json;d=json.load(open('$CFGFILE'));print(d.get('model_token') or d.get('ModelToken',''))")
note "user token: ${UTOK:0:16}…   model token: ${MTOK:0:16}…"

UADMIN=(-H "Authorization: Bearer $UTOK" -H "Content-Type: application/json")
UMODEL=(-H "Authorization: Bearer $MTOK" -H "Content-Type: application/json")

echo "== 2. create the vmodel provider (in-process synthetic backend, no network) =="
note "AuthType=vmodel — a provider whose code is compiled into tb"
VUUID=$(curl -s "${UADMIN[@]}" -X POST "$BASE/api/v2/providers" -d '{
  "name":"vmodel-echo","api_base":"vmodel://local","api_style":"openai",
  "auth_type":"vmodel","no_key_required":true,"enabled":true}' \
  | $PY -c "import sys,json;d=json.load(sys.stdin);print(d.get('data',{}).get('uuid') or d.get('uuid',''))")
note "vmodel provider uuid: $VUUID"

echo "== 3. create the two DISPATCH TARGET rules under the openai scenario =="
note "these are the rules the Python provider fans out to. Different vmodels"
note "back them, so the answer text identifies which branch ran."
make_rule() { # make_rule <request_model> <vmodel>
  curl -s "${UADMIN[@]}" -X POST "$BASE/api/v1/rule" -d "{
    \"scenario\":\"openai\",\"request_model\":\"$1\",\"active\":true,
    \"lb_tactic\":{\"type\":\"random\",\"params\":{}},
    \"services\":[{\"provider\":\"$VUUID\",\"model\":\"$2\",\"weight\":1,\"active\":true}]}" \
    | $PY -c "import sys,json;d=json.load(sys.stdin);print('   rule', '$1', '->', '$2', ':', d.get('success'))"
}
make_rule fast-model   echo-model      # echoes the prompt back
make_rule strong-model virtual-gpt-4   # returns a fixed, recognisably different text

echo "== 4. start the Python server — it registers NOTHING =="
TINGLY_BOX_URL="$BASE" TINGLY_BOX_TOKEN="$UTOK" \
  $PY "$SDK/examples/e2e_server.py" >/tmp/py_server_e2e.log 2>&1 &
SRV_PID=$!
for i in $(seq 1 40); do
  curl -sf "http://127.0.0.1:$SRV_PORT/health" >/dev/null 2>&1 && break
  sleep 0.3
done
curl -sf "http://127.0.0.1:$SRV_PORT/health" >/dev/null || { echo "server did not start"; cat /tmp/py_server_e2e.log; exit 1; }
note "serving on http://127.0.0.1:$SRV_PORT (both /v1/messages and /v1/chat/completions)"

# tb's model-list refresh uses this; it is why the model id never has to be typed.
MODELS=$(curl -s "http://127.0.0.1:$SRV_PORT/v1/models")
echo "$MODELS" | grep -q '"router"'; check "GET /v1/models advertises router" $?

echo "== 5. add it to tb as an ORDINARY provider =="
note "POST /api/v2/providers — exactly what Connect AI → Self-hosted sends."
note "No plugin endpoint is involved because none exists."
PUUID=$(curl -s "${UADMIN[@]}" -X POST "$BASE/api/v2/providers" -d "{
  \"name\":\"router\",\"api_base\":\"http://127.0.0.1:$SRV_PORT\",
  \"api_style\":\"anthropic\",\"auth_type\":\"api_key\",
  \"no_key_required\":true,\"enabled\":true}" \
  | $PY -c "import sys,json;d=json.load(sys.stdin);print(d.get('data',{}).get('uuid') or d.get('uuid',''))")
note "python provider uuid: $PUUID"
[[ -n "$PUUID" ]]; check "provider created via the generic endpoint" $?

# supports_models_endpoint in the provider template means tb refreshes the
# model list off the server itself — the id is discovered, never typed.
curl -s "${UADMIN[@]}" -X POST "$BASE/api/v2/provider-models/$PUUID" -d '{}' | grep -q '"router"'
check "tb's model-list refresh discovered the model" $?

echo "== 6. bind the inbound rule to it =="
note "one model id in — `router` — which fans out to the two rules from step 3"
curl -s "${UADMIN[@]}" -X POST "$BASE/api/v1/rule" -d "{
  \"scenario\":\"openai\",\"request_model\":\"router\",\"active\":true,
  \"lb_tactic\":{\"type\":\"random\",\"params\":{}},
  \"services\":[{\"provider\":\"$PUUID\",\"model\":\"router\",\"weight\":1,\"active\":true}]}" \
  | $PY -c "import sys,json;d=json.load(sys.stdin);print('   rule created:', d.get('success'))"

# ask <prompt> -> the assistant's DECODED text.
#
# Decoded, not the raw body, deliberately: Go's encoding/json HTML-escapes `>`
# to \u003e, so grepping the wire bytes for a marker like "fast->fast-model"
# silently never matches even when the routing is correct. Assert on the value,
# not on its transport encoding.
ask() {
  curl -s "${UMODEL[@]}" -X POST "$BASE/tingly/openai/v1/chat/completions" \
    -d "$($PY -c "import json,sys;print(json.dumps({'model':'router','messages':[{'role':'user','content':sys.argv[1]}]}))" "$1")" \
  | $PY -c "import json,sys
d = json.load(sys.stdin)
try:
    print(d['choices'][0]['message']['content'])
except (KeyError, IndexError, TypeError):
    print('NO_CONTENT: ' + json.dumps(d)[:300])"
}

echo "== 7. DISPATCH: a short prompt must take the fast branch =="
note "client speaks OpenAI → tb calls the Python provider as Anthropic →"
note "handler classifies → calls BACK into tb against the fast-model rule"
SHORT_OUT=$(ask "hi")
note "answer: $SHORT_OUT"
echo "$SHORT_OUT" | grep -q "routed:fast->fast-model"; check "short prompt routed to fast-model" $?
echo "$SHORT_OUT" | grep -q "Echo:"; check "  ...and reached the echo vmodel behind it" $?

echo "== 8. DISPATCH: a long prompt must take the strong branch =="
LONG_OUT=$(ask "$($PY -c "print('please analyse this at length. ' * 20)")")
note "answer: ${LONG_OUT:0:90}…"
echo "$LONG_OUT" | grep -q "routed:strong->strong-model"; check "long prompt routed to strong-model" $?
echo "$LONG_OUT" | grep -q "virtual GPT-4"; check "  ...and reached the OTHER vmodel behind it" $?

# The pair above is the actual claim: one inbound model id, two different
# upstreams chosen by the provider's own logic, each through a real tb rule.
if echo "$SHORT_OUT" | grep -q "virtual GPT-4" || echo "$LONG_OUT" | grep -q "Echo:"; then
  check "branches are genuinely distinct" 1
else
  check "branches are genuinely distinct" 0
fi

echo "== 9. no separate lifecycle: SIGKILL the server =="
note "the provider is a normal DB row, so it stays listed like any other"
kill -KILL "$SRV_PID" 2>/dev/null
SRV_PID=""
sleep 0.5
curl -s "${UADMIN[@]}" "$BASE/api/v2/providers" | grep -q '"router"'
check "provider still listed after the process died" $?

echo "== 10. client call against the dead provider =="
note "liveness is the SAME per-service circuit breaker every provider gets."
note "No fallback tier is configured on this rule, so the request just errors —"
note "add a tier-1 real model and this would tier-failover instead."
note "-> $(ask "still there?" | head -c 200)"

echo "== 11. restart the server; the provider row is untouched =="
TINGLY_BOX_URL="$BASE" TINGLY_BOX_TOKEN="$UTOK" \
  $PY "$SDK/examples/e2e_server.py" >/tmp/py_server_e2e_2.log 2>&1 &
SRV_PID=$!
for i in $(seq 1 40); do
  curl -sf "http://127.0.0.1:$SRV_PORT/health" >/dev/null 2>&1 && break
  sleep 0.3
done
COUNT=$(curl -s "${UADMIN[@]}" "$BASE/api/v2/providers" \
  | $PY -c "import sys,json; d=json.load(sys.stdin); print(sum(1 for p in (d.get('data') or []) if p.get('name')=='router'))")
note "providers named router after restart: $COUNT (expect 1)"
[[ "$COUNT" == "1" ]]; check "no duplicate provider (nothing re-registers)" $?

ask "hi" | grep -q "routed:fast->fast-model"
check "traffic resumes with no reconfiguration" $?

echo
if [[ "$FAILED" == "0" ]]; then echo "== ALL CHECKS PASSED =="; else echo "== SOME CHECKS FAILED =="; fi
exit "$FAILED"
