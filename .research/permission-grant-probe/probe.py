"""Permission-grant probe: does a permission rule pushed live into a running
`claude` (apply_flag_settings) let the model's own next Bash call run, in auto
mode with a classifier that blocks everything? Real CLI against a scripted
Anthropic Messages API.

    python3 probe.py <scenario> [outdir]

Scenarios (see ../permission-grant-probe.md; outdir defaults to a temp dir):
  control   auto mode, no rule: the classifier blocks the command
  early     auto mode, rule pushed mid-turn before the call: does it run, is the classifier asked?
  late      auto mode, call blocked, rule pushed, same call again: does the retry run?
  autorule  auto mode, autoMode.allow prose pushed mid-turn (no permission rule)
  interp    auto mode, rule for an interpreter command (python3 -c ...) pushed mid-turn
  default   default mode: can_use_tool answered allow + addRules(session); second call asks again?
  bypass    bypassPermissions with a deny-nothing setup: control for the classifier

HOME is isolated, the API key is fake, and nothing leaves 127.0.0.1.
"""
import json, os, shutil, subprocess, sys, tempfile, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SC = sys.argv[1]
OUT = sys.argv[2] if len(sys.argv) > 2 else tempfile.mkdtemp(prefix="grant-probe-")
os.makedirs(OUT, exist_ok=True)
T0 = time.time()
LOCK = threading.Lock()
MAIN_N = [0]
LOG = []        # model requests and CLI io, in order
CLASSIFIER = []  # classifier requests seen
CMD = "./tool.sh PROBE-RAN"
RULE = "Bash(./tool.sh *)"
if SC == "interp":
    CMD = "python3 -c \"open('ran.txt','w').write('PROBE-RAN')\""
    RULE = "Bash(python3 -c *)"
if SC == "interpexact":
    CMD = "python3 -c \"open('ran.txt','w').write('PROBE-RAN')\""
    RULE = "Bash(" + CMD + ")"
if os.environ.get("PROBE_CMD"):
    CMD = os.environ["PROBE_CMD"]
    RULE = os.environ.get("PROBE_RULE") or "Bash(" + CMD + ")"
MODEL = os.environ.get("PROBE_MODEL", "demo-sonnet")
PUSHED = threading.Event()


def now():
    return round(time.time() - T0, 2)


def note(d, kind="NOTE"):
    LOG.append(f"{now():6.2f} {kind} {d}")


def last_result(req):
    for m in reversed(req.get("messages", [])):
        c = m["content"]
        if isinstance(c, list):
            for b in c:
                if b.get("type") == "tool_result":
                    cc = b.get("content")
                    if isinstance(cc, list):
                        cc = " ".join(x.get("text", "") for x in cc)
                    return str(cc)[:160].replace("\n", " ")
    return ""


def script(n):
    """The main loop: (blocks, delay before answering)."""
    bash = ("tool", "Bash", {"command": CMD, "description": "probe"})
    if SC in ("control", "bypass"):
        return ([bash], 0) if n == 1 else ([("text", "DONE")], 0)
    if SC in ("early", "autorule", "interp", "interpexact"):
        # The host pushes the grant while request 1 is held (mid-turn), as
        # request_permission would between the card and the next call.
        if n == 1:
            return [bash], 4
        return [("text", "DONE")], 0
    if SC in ("late", "default"):
        if n in (1, 2):
            return [bash], (3 if n == 2 else 0)
        return [("text", "DONE")], 0
    return [("text", "?")], 0


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
        tools = {t.get("name") for t in req.get("tools") or []}
        sysp = req.get("system", "")
        sysp = sysp if isinstance(sysp, str) else "\n".join(b.get("text", "") for b in sysp)
        delay = 0
        if "Bash" in tools:
            with LOCK:
                MAIN_N[0] += 1
                idx = MAIN_N[0]
            blocks, delay = script(idx)
            note(f"main#{idx} last_tool_result={last_result(req)!r}", "REQ ")
        else:
            body = json.dumps(req)
            is_cls = "PROBE-RAN" in body and ("block" in sysp.lower() or "monitor" in sysp.lower() or "classif" in sysp.lower())
            if is_cls:
                CLASSIFIER.append(req)
                note(f"classifier request (tools={sorted(tools)}) -> block", "REQ ")
                with open(os.path.join(OUT, f"classifier-{len(CLASSIFIER)}.json"), "w") as f:
                    json.dump(req, f, indent=1)
                if tools:
                    tname = sorted(tools)[0]
                    blocks = [("tool", tname, {"thinking": "probe", "shouldBlock": True, "reason": "probe blocks everything"})]
                else:
                    blocks = [("text", "<block>yes</block><reason>probe blocks everything</reason>")]
            else:
                blocks = [("text", "{}")]
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
        if not req.get("stream"):
            return self.send_json(200, {"id": mid, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "stop_sequence": None, "usage": {"input_tokens": 100, "output_tokens": 20}})
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
    f.write("#!/bin/sh\necho \"$1\" > ran.txt\n")
os.chmod(os.path.join(work, "tool.sh"), 0o755)
env = {"HOME": home, "PATH": os.environ["PATH"], "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
       "ANTHROPIC_API_KEY": "sk-ant-fake", "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
mode = {"default": "default", "bypass": "bypassPermissions"}.get(SC, "auto")
cmd = ["claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
       "--permission-mode", mode, "--permission-prompt-tool", "stdio", "--model", MODEL]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(os.path.join(OUT, "stderr.txt"), "w"),
                     text=True, bufsize=1, cwd=work, env=env)
RESULTS = [0]


def send(o):
    p.stdin.write(json.dumps(o) + "\n")
    p.stdin.flush()
    note(json.dumps(o)[:200], "SEND")


def push(settings):
    send({"type": "control_request", "request_id": "push-" + uuid.uuid4().hex[:6], "request": {"subtype": "apply_flag_settings", "settings": settings}})


def grant():
    if SC == "autorule":
        push({"autoMode": {"allow": ["The user approved running `" + CMD + "` in this project; allow exactly that command."]}})
    else:
        rules = [RULE] + ["Bash(" + x + ")" for x in os.environ.get("PROBE_EXTRA", "").split("\n") if x]
        push({"permissions": {"allow": rules}})
    PUSHED.set()


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
            full = json.dumps(e["message"]["content"])
            d = full[:220]
            if SC == "late" and not PUSHED.is_set() and '"is_error": true' in full:
                grant()
        elif ty == "result":
            d = str(e.get("result"))[:80]
            RESULTS[0] += 1
        elif ty == "control_request":
            r = e["request"]
            d = r.get("subtype", "") + " " + str(r.get("tool_name", ""))
            if r.get("subtype") == "can_use_tool":
                resp = {"behavior": "allow", "updatedInput": r.get("input")}
                if SC == "default":
                    resp["updatedPermissions"] = [{"type": "addRules", "rules": [{"toolName": "Bash", "ruleContent": RULE[5:-1]}], "behavior": "allow", "destination": "session"}]
                send({"type": "control_response", "response": {"subtype": "success", "request_id": e["request_id"], "response": resp}})
        elif ty == "control_response":
            d = json.dumps(e.get("response"))[:200]
        elif ty == "system":
            d = str(e.get("subtype"))
        note(ty + " " + d, "RECV")


threading.Thread(target=reader, daemon=True).start()
send({"type": "user", "message": {"role": "user", "content": "Run the probe command."}})
if SC in ("early", "autorule", "interp", "interpexact"):
    time.sleep(2)  # request 1 is held for 4s: the push lands mid-turn
    grant()

deadline = time.time() + 60
while time.time() < deadline and RESULTS[0] < 1:
    time.sleep(0.5)
time.sleep(1)
try:
    p.stdin.close()
except Exception:
    pass
try:
    p.wait(10)
except Exception:
    p.kill()

ran = os.path.exists(os.path.join(work, "ran.txt"))
print(f"=== scenario {SC} (CLI {subprocess.run(['claude', '--version'], capture_output=True, text=True).stdout.strip()}, model {MODEL})")
print("\n".join(LOG))
print(f"--- command ran: {ran}; classifier requests: {len(CLASSIFIER)}; main requests: {MAIN_N[0]}")
