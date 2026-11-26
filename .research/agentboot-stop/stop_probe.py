"""Stop probe: what does an `interrupt` control request do to a running
`claude` process, its in-flight turn and the background task it started?

Real Claude Code CLI (2.1.259 here) against a scripted Anthropic Messages API
on 127.0.0.1; isolated HOME; fake key. Nothing leaves the machine.

    python3 stop_probe.py [outdir]

Script:
  turn 1  the model starts a *background* Bash task (`sleep 8; echo survived > bg.txt`),
          the CLI answers with a task id and the model says STARTED.
  turn 2  the user asks for a long foreground command (`sleep 30`); while it runs
          the host sends the same `interrupt` control request agentboot's
          Conductor sends on Stop.
  then    the host asks the process (a) whether it is still alive (set_model),
          (b) whether it takes another user message (turn 3), and checks after
          the sleep whether bg.txt exists and whether a task_notification for
          the background task arrived.

Questions answered:
  Q1 does interrupt end only the turn, keeping the process? (turn-3 answer)
  Q2 does the background task started in an earlier turn survive interrupt?
  Q3 does the process report the interrupted turn's result?
"""
import json, os, shutil, subprocess, sys, tempfile, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

OUT = sys.argv[1] if len(sys.argv) > 1 else tempfile.mkdtemp(prefix="stop-probe-")
os.makedirs(OUT, exist_ok=True)
T0 = time.time()
LOCK = threading.Lock()
MAIN_N = [0]
LOG = []
MODEL = os.environ.get("PROBE_MODEL", "demo-sonnet")
RESULT_N = [0]
TURN2_SENT = threading.Event()


def now():
    return round(time.time() - T0, 2)


def note(d, kind="NOTE"):
    LOG.append(f"{now():6.2f} {kind} {d}")


def last_tool_result(req):
    for m in reversed(req.get("messages", [])):
        c = m["content"]
        if isinstance(c, list):
            for b in c:
                if b.get("type") == "tool_result":
                    cc = b.get("content")
                    if isinstance(cc, list):
                        cc = " ".join(x.get("text", "") for x in cc)
                    return str(cc)[:200].replace("\n", " ")
    return ""


def script(n):
    """The main loop: (blocks, delay before answering)."""
    if n == 1:
        return [("tool", "Bash", {"command": "sleep 8; echo survived > bg.txt",
                                  "description": "background probe", "run_in_background": True})], 0
    if n == 2:
        return [("text", "STARTED")], 0
    if n == 3:
        return [("tool", "Bash", {"command": "sleep 30", "description": "long foreground probe"})], 0
    return [("text", "TURN2-END")], 0


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
        note(f"main#{idx} last_tool_result={last_tool_result(req)!r}", "REQ ")
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
env = {"HOME": home, "PATH": os.environ["PATH"], "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
       "ANTHROPIC_API_KEY": "sk-ant-fake", "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
cmd = ["claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
       "--permission-mode", "bypassPermissions", "--permission-prompt-tool", "stdio", "--model", MODEL]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(os.path.join(OUT, "stderr.txt"), "w"),
                     text=True, bufsize=1, cwd=work, env=env)
STATE = {"turn1_done": False, "task_id": None, "task_notes": [], "interrupt_ack": None, "turn2_result": None}


def send(o):
    p.stdin.write(json.dumps(o) + "\n")
    p.stdin.flush()
    note(json.dumps(o)[:200], "SEND")


def reader():
    for line in p.stdout:
        try:
            e = json.loads(line)
        except Exception:
            continue
        ty = e.get("type")
        if ty in ("stream_event", "rate_limit_event", "autocompact_state"):
            continue
        d = ""
        if ty == "system" and e.get("subtype") == "init":
            d = "init permissionMode=" + str(e.get("permissionMode"))
        elif ty == "system" and e.get("subtype") in ("task_started", "task_notification", "background_tasks_changed"):
            d = json.dumps({k: v for k, v in e.items() if k not in ("uuid", "session_id")})[:300]
            if e.get("subtype") == "task_notification":
                STATE["task_notes"].append(e)
        elif ty == "assistant":
            d = json.dumps([(b.get("text") or b.get("name") or "")[:60] for b in e["message"]["content"]])[:200]
        elif ty == "user":
            d = json.dumps(e["message"]["content"])[:200]
        elif ty == "result":
            with LOCK:
                RESULT_N[0] += 1
            d = f"#{RESULT_N[0]} subtype={e.get('subtype')} is_error={e.get('is_error')} result={str(e.get('result'))[:60]}"
            if RESULT_N[0] == 1:
                STATE["turn1_done"] = True
            if RESULT_N[0] == 2:
                STATE["turn2_result"] = e
        elif ty == "control_request":
            r = e["request"]
            d = r.get("subtype", "") + " " + str(r.get("tool_name", ""))
            if r.get("subtype") == "can_use_tool":
                send({"type": "control_response", "response": {"subtype": "success", "request_id": e["request_id"], "response": {"behavior": "allow", "updatedInput": r.get("input")}}})
        elif ty == "control_response":
            resp = e.get("response") or {}
            d = json.dumps(resp)[:200]
            if resp.get("request_id") == "probe-interrupt":
                STATE["interrupt_ack"] = resp
        elif ty == "system":
            d = str(e.get("subtype"))
        note(ty + " " + d, "RECV")


threading.Thread(target=reader, daemon=True).start()

send({"type": "user", "message": {"role": "user", "content": "Start the background task."}})

# Wait for turn 1 (start the background task) to finish.
deadline = time.time() + 30
while time.time() < deadline and not STATE["turn1_done"]:
    time.sleep(0.2)
note(f"turn1 done={STATE['turn1_done']}")

# Turn 2: a long foreground command; interrupt it while it runs.
time.sleep(0.5)
send({"type": "user", "message": {"role": "user", "content": "Now run the long one."}})
time.sleep(2.5)
pid_alive_before = p.poll() is None
note(f"before interrupt: process alive={pid_alive_before}")
send({"type": "control_request", "request_id": "probe-interrupt", "request": {"subtype": "interrupt"}})

# Give the interrupt time to land.
time.sleep(2.0)
note(f"after interrupt: process alive={p.poll() is None} interrupt_ack={STATE['interrupt_ack']}")

# Is the process still usable? Ask it to switch model (a control request that
# only a live process answers), then send a third user message.
send({"type": "control_request", "request_id": "probe-alive", "request": {"subtype": "set_model", "model": MODEL}})
time.sleep(1.5)
n_before = MAIN_N[0]
send({"type": "user", "message": {"role": "user", "content": "Are you still there? Reply YES."}})
deadline = time.time() + 10
while time.time() < deadline and MAIN_N[0] == n_before:
    time.sleep(0.2)
note(f"after interrupt, another user message reached the model: {MAIN_N[0] > n_before} (main requests {n_before}->{MAIN_N[0]})")
time.sleep(3)

# The background task was `sleep 8`; give it time to finish and write the file.
deadline = time.time() + 12
while time.time() < deadline and not os.path.exists(os.path.join(work, "bg.txt")):
    time.sleep(0.3)
bg = os.path.exists(os.path.join(work, "bg.txt"))
note(f"background task output file exists={bg}")

alive = p.poll() is None
try:
    p.stdin.close()
except Exception:
    pass
try:
    p.wait(10)
except Exception:
    p.kill()

print(f"=== stop probe (CLI {subprocess.run(['claude', '--version'], capture_output=True, text=True).stdout.strip()}, model {MODEL})")
print("\n".join(LOG))
print("--- summary")
print(f"turn1 (background task started) done: {STATE['turn1_done']}")
print(f"turn2 result reported after interrupt: {STATE['turn2_result'] is not None} {str(STATE['turn2_result'].get('subtype') if STATE['turn2_result'] else '')}")
print(f"interrupt ack: {STATE['interrupt_ack']}")
print(f"process alive after interrupt: {alive}")
print(f"new user message reached the model after interrupt: {MAIN_N[0] > n_before}")
print(f"background task survived (bg.txt): {bg}")
print(f"task notifications: {len(STATE['task_notes'])}")
