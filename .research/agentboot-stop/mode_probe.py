"""Permission-mode probe: does `set_permission_mode` sent mid-turn change the
mode of the *running* turn (its next tool call), without a restart?

Real Claude Code CLI (2.1.259 here) against a scripted Anthropic Messages API
on 127.0.0.1; isolated HOME; fake key. Nothing leaves the machine.

    python3 mode_probe.py control|tighten|widen [outdir]

All run in `--permission-mode default`, so each Bash call raises
`can_use_tool`. One turn makes two Bash calls; the first (`./tool.sh FIRST 3`)
runs 3 seconds, and the host changes the mode while it runs:

  control  no change: the second call raises can_use_tool too.
  tighten  set_permission_mode plan: plan mode must stop the second call.
  widen    set_permission_mode bypassPermissions: what does the CLI answer?

Answers: is a live `set_permission_mode` immediate for the turn in flight
(the premise of agentboot/engine's Engine.SetPermissionMode), and which
directions can it actually take?
"""
import json, os, shutil, subprocess, sys, tempfile, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SC = sys.argv[1]
OUT = sys.argv[2] if len(sys.argv) > 2 else tempfile.mkdtemp(prefix="mode-probe-")
os.makedirs(OUT, exist_ok=True)
T0 = time.time()
LOCK = threading.Lock()
MAIN_N = [0]
LOG = []
MODEL = os.environ.get("PROBE_MODEL", "demo-sonnet")
PERM = []          # (tool, command) of every can_use_tool
SWITCHED = threading.Event()
STATE = {"switch_ack": None, "denied": []}
TARGET = {"control": None, "tighten": "plan", "widen": "bypassPermissions"}[SC]


def now():
    return round(time.time() - T0, 2)


def note(d, kind="NOTE"):
    LOG.append(f"{now():6.2f} {kind} {d}")


def script(n):
    if n == 1:
        return [("tool", "Bash", {"command": "./tool.sh FIRST 3", "description": "first probe call"})], 0
    if n == 2:
        return [("tool", "Bash", {"command": "./tool.sh SECOND 0", "description": "second probe call"})], 0
    return [("text", "DONE")], 0


class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def send_json(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": self.path}})

    def do_POST(self):
        n = int(self.headers.get("content-length") or 0)
        req = json.loads(self.rfile.read(n) or b"{}")
        path = self.path.split("?")[0]
        if path.endswith("/count_tokens"):
            return self.send_json(200, {"input_tokens": 100})
        if not path.endswith("/v1/messages"):
            return self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": path}})
        with LOCK:
            MAIN_N[0] += 1
            idx = MAIN_N[0]
        blocks, delay = script(idx)
        note(f"main#{idx}", "REQ ")
        if delay:
            time.sleep(delay)
        model = req.get("model", "demo")
        mid = "msg_" + uuid.uuid4().hex[:20]
        stop = "tool_use" if any(b[0] == "tool" for b in blocks) else "end_turn"
        content = []
        for b in blocks:
            if b[0] == "text":
                content.append({"type": "text", "text": b[1]})
            else:
                content.append({"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20], "name": b[1], "input": b[2]})
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("connection", "close")
        self.end_headers()

        def ev(name, data):
            self.wfile.write(f"event: {name}\ndata: {json.dumps(data)}\n\n".encode())
            self.wfile.flush()

        ev("message_start", {"type": "message_start", "message": {"id": mid, "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 100, "output_tokens": 1}}})
        for i, c in enumerate(content):
            if c["type"] == "text":
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "text", "text": ""}})
                ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "text_delta", "text": c["text"]}})
            else:
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "tool_use", "id": c["id"], "name": c["name"], "input": {}}})
                ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "input_json_delta", "partial_json": json.dumps(c["input"])}})
            ev("content_block_stop", {"type": "content_block_stop", "index": i})
        ev("message_delta", {"type": "message_delta", "delta": {"stop_reason": stop, "stop_sequence": None}, "usage": {"output_tokens": 20}})
        ev("message_stop", {"type": "message_stop"})
        self.close_connection = True


srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
threading.Thread(target=srv.serve_forever, daemon=True).start()
port = srv.server_address[1]

home = os.path.join(OUT, "home")
work = os.path.join(OUT, "work")
shutil.rmtree(home, ignore_errors=True)
shutil.rmtree(work, ignore_errors=True)
os.makedirs(home)
os.makedirs(work)
with open(os.path.join(work, "tool.sh"), "w") as f:
    f.write('#!/bin/sh\necho "$1" > "ran-$1.txt"\nsleep "$2"\n')
os.chmod(os.path.join(work, "tool.sh"), 0o755)
env = {"HOME": home, "PATH": os.environ["PATH"], "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
       "ANTHROPIC_API_KEY": "sk-ant-fake", "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
cmd = ["claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
       "--permission-mode", "default", "--permission-prompt-tool", "stdio", "--model", MODEL]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(os.path.join(OUT, "stderr.txt"), "w"),
                     text=True, bufsize=1, cwd=work, env=env)
RESULTS = [0]


def send(o):
    p.stdin.write(json.dumps(o) + "\n")
    p.stdin.flush()
    note(json.dumps(o)[:200], "SEND")


def switch():
    time.sleep(0.5)  # let the first tool start
    send({"type": "control_request", "request_id": "probe-mode", "request": {"subtype": "set_permission_mode", "mode": TARGET}})
    SWITCHED.set()


def reader():
    for line in p.stdout:
        try:
            e = json.loads(line)
        except Exception:
            continue
        ty = e.get("type")
        if ty in ("stream_event", "rate_limit_event"):
            continue
        d = ""
        if ty == "system" and e.get("subtype") == "init":
            d = "init permissionMode=" + str(e.get("permissionMode"))
        elif ty == "assistant":
            d = json.dumps([(b.get("text") or b.get("name") or "")[:50] for b in e["message"]["content"]])
        elif ty == "user":
            block = e["message"]["content"]
            d = json.dumps(block)[:260]
            if isinstance(block, list) and block and block[0].get("is_error"):
                STATE["denied"].append(str(block[0].get("content"))[:120])
        elif ty == "result":
            RESULTS[0] += 1
            d = f"result subtype={e.get('subtype')} is_error={e.get('is_error')}"
        elif ty == "control_request":
            r = e["request"]
            d = r.get("subtype", "") + " " + str(r.get("tool_name", ""))
            if r.get("subtype") == "can_use_tool":
                cmd = (r.get("input") or {}).get("command", "")
                with LOCK:
                    PERM.append((r.get("tool_name"), cmd))
                send({"type": "control_response", "response": {"subtype": "success", "request_id": e["request_id"], "response": {"behavior": "allow", "updatedInput": r.get("input")}}})
                if TARGET and not SWITCHED.is_set():
                    threading.Thread(target=switch, daemon=True).start()
        elif ty == "control_response":
            resp = e.get("response") or {}
            d = json.dumps(resp)[:200]
            if resp.get("request_id") == "probe-mode":
                STATE["switch_ack"] = resp
        elif ty == "system":
            d = str(e.get("subtype"))
        note(ty + " " + d, "RECV")


threading.Thread(target=reader, daemon=True).start()
send({"type": "user", "message": {"role": "user", "content": "Run the two probe commands."}})

deadline = time.time() + 45
while time.time() < deadline and RESULTS[0] < 1:
    time.sleep(0.3)
time.sleep(1.5)
try:
    p.stdin.close()
except Exception:
    pass
try:
    p.wait(10)
except Exception:
    p.kill()

print(f"=== mode probe {SC} (CLI {subprocess.run(['claude', '--version'], capture_output=True, text=True).stdout.strip()}, model {MODEL})")
print("\n".join(LOG))
print("--- summary")
print(f"can_use_tool requests: {PERM}")
print(f"switch ack: {STATE['switch_ack']}")
print(f"FIRST ran: {os.path.exists(os.path.join(work, 'ran-FIRST.txt'))}; SECOND ran: {os.path.exists(os.path.join(work, 'ran-SECOND.txt'))}")
print(f"error tool_results: {STATE['denied']}")
