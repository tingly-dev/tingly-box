#!/usr/bin/env bash
# End-to-end test for the two tb connection modes and router_plugin.py's use
# of both, using NO network / API keys (vmodel providers only):
#
#   mode 1: scenario + rule(model)                — tb picks the service
#           (affinity/smart-routing/load-balancer; tier order here)
#   mode 2: scenario + rule(model) + pin_provider  — caller picks, but tb
#           only allows a provider already on that rule's own services
#
# router_plugin.py is a real, unmodified consumer of both: it resolves
# candidates via Client.rules (mode 1's information), then forwards with
# pin_provider (mode 2) so the provider it quota-checked is the one that
# actually serves the request. vmodel providers have no fetchable quota, so
# the pick is a deterministic tie-break here, not a real quota decision —
# this script proves the WIRING (rule resolution -> pin -> tb enforcement),
# not "quota routing found a numerically better answer" (that needs a real
# provider account, out of scope for a no-network e2e test).
#
# Prereqs:
#   go build -o /tmp/tb_e2e ./cli/tingly-box
#   pip install -e .   # from sdk/python (needs `tingly` importable)
# Run:  bash sdk/python/examples/e2e_run_pin.sh
set -uo pipefail

TB=${TB_BIN:-/tmp/tb_e2e}
CFG=$(mktemp -d)
PORT=18903
BASE="http://127.0.0.1:$PORT"
SDK=/home/user/tingly-box/sdk/python
TB_LOG=/tmp/tb_pin_e2e.log
export PYTHONPATH=$SDK

FAILED=0
pass() { echo "   PASS: $1"; }
fail() { echo "   FAIL: $1"; FAILED=1; }

cleanup() {
  [[ -n "${PLUG_PID:-}" ]] && kill "$PLUG_PID" 2>/dev/null
  [[ -n "${TB_PID:-}" ]] && kill "$TB_PID" 2>/dev/null
}
trap cleanup EXIT

echo "== 1. start tb (config-dir=$CFG, port=$PORT) =="
"$TB" --config-dir "$CFG" start --port "$PORT" --ui --browser=false >"$TB_LOG" 2>&1 &
TB_PID=$!
for i in $(seq 1 60); do
  curl -sf "$BASE/api/v1/info/health" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sf "$BASE/api/v1/info/health" >/dev/null || { echo "tb did not start"; tail -30 "$TB_LOG"; exit 1; }
echo "   tb healthy at $BASE"

CFGFILE=$(find "$CFG" -name 'config.json' | head -1)
UTOK=$(python3 -c "import json;d=json.load(open('$CFGFILE'));print(d.get('user_token') or d.get('UserToken',''))")
MTOK=$(python3 -c "import json;d=json.load(open('$CFGFILE'));print(d.get('model_token') or d.get('ModelToken',''))")
UADMIN=(-H "Authorization: Bearer $UTOK" -H "Content-Type: application/json")
UMODEL=(-H "Authorization: Bearer $MTOK" -H "Content-Type: application/json")

echo "== 2. create three vmodel providers (A, B, C — no network) =="
mk_provider() {
  curl -s "${UADMIN[@]}" -X POST "$BASE/api/v2/providers" -d "{
    \"name\":\"$1\",\"api_base\":\"vmodel://local\",\"api_style\":\"openai\",
    \"auth_type\":\"vmodel\",\"no_key_required\":true,\"enabled\":true}" \
    | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('data',{}).get('uuid') or d.get('uuid',''))"
}
PA=$(mk_provider vmodel-a)
PB=$(mk_provider vmodel-b)
PC=$(mk_provider vmodel-c)
echo "   A=$PA B=$PB C=$PC"

echo "== 3. rule 'tiered-model': A@tier0, B@tier1 (mode 1 target) =="
echo "   (request_model is the client-facing name; each service's own 'model'"
echo "    must be 'echo-model' — the only mock ID the no-network vmodel backend knows)"
curl -s "${UADMIN[@]}" -X POST "$BASE/api/v1/rule" -d "{
  \"scenario\":\"experiment\",\"request_model\":\"tiered-model\",\"active\":true,
  \"lb_tactic\":{\"type\":\"tier\",\"params\":{}},
  \"services\":[{\"provider\":\"$PA\",\"model\":\"echo-model\",\"weight\":1,\"active\":true,\"tier\":0},
                {\"provider\":\"$PB\",\"model\":\"echo-model\",\"weight\":1,\"active\":true,\"tier\":1}]}" \
  | python3 -c "import sys,json;d=json.load(sys.stdin);print('   rule created:', d.get('success'))"

echo "== 4. rules 'sonnet1'->A and 'sonnet2'->B, single service each =="
echo "   (router_plugin.py's default CANDIDATE_MODELS — unmodified file, real names)"
mk_pinned_rule() {
  curl -s "${UADMIN[@]}" -X POST "$BASE/api/v1/rule" -d "{
    \"scenario\":\"experiment\",\"request_model\":\"$1\",\"active\":true,
    \"lb_tactic\":{\"type\":\"random\",\"params\":{}},
    \"services\":[{\"provider\":\"$2\",\"model\":\"echo-model\",\"weight\":1,\"active\":true}]}" \
    | python3 -c "import sys,json;d=json.load(sys.stdin);print('   rule created:', d.get('success'))"
}
mk_pinned_rule sonnet1 "$PA"
mk_pinned_rule sonnet2 "$PB"

echo "== 5. MODE 1 (scenario+rule): unpinned call -> tb picks tier0 = A =="
SEL=$(curl -s "${UMODEL[@]}" -H "X-Tingly-Debug-Routing: 1" -D - -o /dev/null \
  -X POST "$BASE/tingly/experiment/v1/chat/completions" \
  -d '{"model":"tiered-model","messages":[{"role":"user","content":"hi"}]}' \
  | grep -i "x-tingly-selected-provider-uuid" | tr -d '\r' | awk '{print $2}')
[[ "$SEL" == "$PA" ]] && pass "unpinned call selected tier0 (A=$PA)" || fail "unpinned call selected '$SEL', expected A=$PA"

echo "== 6. MODE 2 (scenario+rule+pin): pin to B overrides tier order =="
SEL=$(curl -s "${UMODEL[@]}" -H "X-Tingly-Debug-Routing: 1" -H "X-Tingly-Pin-Provider: $PB" -D - -o /dev/null \
  -X POST "$BASE/tingly/experiment/v1/chat/completions" \
  -d '{"model":"tiered-model","messages":[{"role":"user","content":"hi"}]}' \
  | grep -i "x-tingly-selected-provider-uuid" | tr -d '\r' | awk '{print $2}')
[[ "$SEL" == "$PB" ]] && pass "pinned call selected B ($PB) despite tier0=A" || fail "pinned call selected '$SEL', expected B=$PB"

echo "== 7. MODE 2 scoping: pin to C (not on this rule) is rejected =="
ERR=$(curl -s "${UMODEL[@]}" -X POST "$BASE/tingly/experiment/v1/chat/completions" -H "X-Tingly-Pin-Provider: $PC" -d '{
  "model":"tiered-model","messages":[{"role":"user","content":"hi"}]}')
echo "$ERR" | grep -q "not an active service" && pass "pin to unrelated provider C rejected" || fail "expected rejection, got: $ERR"

echo "== 8. SDK-level: Client.ask(pin_provider=) round-trips through the real gateway =="
python3 - "$BASE" "$UTOK" "$PB" <<'PY' && pass "Client.ask(pin_provider=) completed" || { echo "   FAIL: SDK pin_provider call raised"; FAILED=1; }
import sys
import tingly
base, admin_token, want_provider = sys.argv[1], sys.argv[2], sys.argv[3]
tb = tingly.connect(base_url=base, token=admin_token, scenario="experiment")
text = tb.ask("hi", model="tiered-model", pin_provider=want_provider)
assert isinstance(text, str) and text, f"expected non-empty text, got {text!r}"
PY

echo "== 9. router_plugin.py: real run — resolves sonnet1/sonnet2 via Client.rules,"
echo "   picks by quota (tied here, no live quota source), forwards with pin_provider =="
TINGLY_BOX_URL="$BASE" TINGLY_BOX_TOKEN="$UTOK" \
  python3 "$SDK/examples/router_plugin.py" >/tmp/router_e2e.log 2>&1 &
PLUG_PID=$!
for i in $(seq 1 40); do
  curl -sf "http://127.0.0.1:8768/health" >/dev/null 2>&1 && break
  sleep 0.3
done
curl -sf "http://127.0.0.1:8768/health" >/dev/null || { fail "router plugin did not start"; cat /tmp/router_e2e.log; }

for i in $(seq 1 20); do
  curl -s "${UADMIN[@]}" "$BASE/api/v2/plugins" | grep -q '"router"' && break
  sleep 0.3
done

RESP=$(curl -s "${UMODEL[@]}" -X POST "$BASE/tingly/experiment/v1/chat/completions" -d '{
  "model":"plugin/router","messages":[{"role":"user","content":"what is 2+2?"}]}')
echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print('   response:', d.get('choices',[{}])[0].get('message',{}).get('content', d))" \
  && pass "model=plugin/router answered" || fail "router call failed: $RESP"

echo "== 10. confirm the router's FORWARDED call actually used a provider pin (tb log) =="
grep -q "source=provider_pin" "$TB_LOG" \
  && pass "tb log shows a provider_pin-sourced selection (router's forwarded call)" \
  || fail "no provider_pin selection found in $TB_LOG"

kill -KILL "$PLUG_PID" 2>/dev/null
PLUG_PID=""

echo "== done: $([[ $FAILED -eq 0 ]] && echo "ALL PASSED" || echo "SOME FAILED — see above") =="
exit $FAILED
