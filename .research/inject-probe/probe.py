"""Mid-turn message injection probe: real `claude` CLI against a scripted
Anthropic Messages API that records what each model request contains.

    python3 probe.py <scenario> [outdir]

Scenarios (see .research/mid-turn-injection.md for what each answers):
  tool    inject while a Bash tool call is running
  stream  inject while the model is still streaming text that ends in a tool call
  final   inject while the model streams its last text (no tool call follows)
  perm    inject while a tool permission prompt (can_use_tool) is unanswered
  multi   inject two messages during one tool call
  task    inject while a subagent (Task tool) runs

HOME is isolated, the API key is fake, and nothing leaves 127.0.0.1.
"""
import json, os, shutil, subprocess, sys, tempfile, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SC = sys.argv[1]
OUT = sys.argv[2] if len(sys.argv) > 2 else tempfile.mkdtemp(prefix="inject-probe-")
os.makedirs(OUT, exist_ok=True)
T0 = time.time()
REQS = []          # one summary per model request
LOCK = threading.Lock()
TOOLS = set()
MAIN_N = [0]       # count of main-loop requests (system prompt of the main agent)
SUB_N = [0]


def now():
    return round(time.time() - T0, 2)


def summarize(req):
    out = []
    for m in req.get("messages", []):
        c = m["content"]
        if isinstance(c, str):
            c = [{"type": "text", "text": c}]
        parts = []
        for b in c:
            t = b.get("type")
            if t == "text":
                parts.append("text:" + b["text"][:90].replace("\n", " "))
            elif t == "tool_use":
                parts.append("tool_use:" + b["name"])
            elif t == "tool_result":
                cc = b.get("content")
                if isinstance(cc, list):
                    cc = " ".join(x.get("text", "") for x in cc)
                parts.append("tool_result:" + str(cc)[:60].replace("\n", " "))
            else:
                parts.append(t)
        out.append(m["role"] + "[" + " | ".join(parts) + "]")
    return out


def has_inj(req):
    return "INJECT" in json.dumps(req.get("messages", []))


# --- scripts: return (blocks, chunk_delay) ----------------------------------
SLOW = "The quick brown fox jumps over the lazy dog. " * 3  # streamed in 12 chunks


def script(kind, n, req):
    """kind: main|sub ; n: 1-based request index of that kind."""
    if kind == "sub":
        if n == 1:
            return [("tool", "Bash", {"command": "sleep 6 && echo sub-done"})], 0
        return [("text", "SUBAGENT-FINISHED")], 0
    if SC == "tool" or SC == "multi":
        if n == 1:
            return [("text", "Running the first command."), ("tool", "Bash", {"command": "sleep 6 && echo step1"})], 0
        if n == 2:
            return [("tool", "Bash", {"command": "echo step2"})], 0
        return [("text", "FINAL-ANSWER")], 0
    if SC == "stream":
        if n == 1:
            return [("text", SLOW), ("tool", "Bash", {"command": "echo step1"})], 0.5
        if n == 2:
            return [("tool", "Bash", {"command": "echo step2"})], 0
        return [("text", "FINAL-ANSWER")], 0
    if SC == "final":
        if n == 1:
            return [("text", SLOW)], 0.5
        return [("text", "SECOND-TURN-ANSWER")], 0
    if SC == "perm":
        if n == 1:
            return [("tool", "Bash", {"command": "touch approved.txt && echo needs-approval"})], 0
        if n == 2:
            return [("tool", "Bash", {"command": "echo step2"})], 0
        return [("text", "FINAL-ANSWER")], 0
    if SC == "task":
        if n == 1:
            return [("tool", "Agent" if "Agent" in TOOLS else "Task", {"description": "slow helper", "prompt": "run the slow command", "subagent_type": "general-purpose", "run_in_background": False})], 0
        if n == 2:
            return [("tool", "Bash", {"command": "echo after-task"})], 0
        return [("text", "FINAL-ANSWER")], 0
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
        if "Bash" in tools and ("Agent" in tools or "Task" in tools):
            TOOLS.update(tools)
        sysp = req.get("system", "")
        sysp = sysp if isinstance(sysp, str) else "\n".join(b.get("text", "") for b in sysp)
        # side requests (title generation, classifiers) carry no tools
        if "Bash" not in tools:
            blocks, delay, kind, idx = [("text", "{}")], 0, "side", 0
        else:
            is_sub = "run the slow command" in json.dumps(req.get("messages", [])[:1])  # the subagent's own prompt
            kind = "sub" if is_sub else "main"
            with LOCK:
                cnt = SUB_N if is_sub else MAIN_N
                cnt[0] += 1
                idx = cnt[0]
            blocks, delay = script(kind, idx, req)
        with LOCK:
            REQS.append({"t": now(), "kind": kind, "idx": idx, "has_injected_text": has_inj(req), "messages": summarize(req)})
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
                t = c["text"]
                step = max(1, len(t) // 12) if delay else 12
                for j in range(0, len(t), step):
                    ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "text_delta", "text": t[j:j + step]}})
                    if delay:
                        time.sleep(delay)
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
os.makedirs(home)
os.makedirs(work, exist_ok=True)
env = {"HOME": home, "PATH": os.environ["PATH"], "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
       "ANTHROPIC_API_KEY": "sk-ant-fake", "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
mode = "default" if SC == "perm" else "bypassPermissions"
cmd = ["claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
       "--permission-mode", mode, "--permission-prompt-tool", "stdio", "--model", "demo-sonnet"]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(os.path.join(OUT, "stderr.txt"), "w"),
                     text=True, bufsize=1, cwd=work, env=env)
EV = []   # CLI output events (type, detail)
pending_perm = []


def send(o):
    p.stdin.write(json.dumps(o) + "\n")
    p.stdin.flush()
    EV.append({"t": now(), "dir": "SEND", "d": json.dumps(o)[:150]})


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
        if ty == "assistant":
            d = json.dumps([(b.get("text") or b.get("name") or "")[:50] for b in e["message"]["content"]])
        elif ty == "user":
            d = json.dumps(e["message"]["content"])[:110]
        elif ty == "result":
            d = str(e.get("result"))[:80]
        elif ty == "control_request":
            d = e["request"].get("subtype", "") + " " + e["request"].get("tool_name", "")
            pending_perm.append(e)
        elif ty == "system":
            d = str(e.get("subtype"))
        EV.append({"t": now(), "dir": "RECV", "d": ty + " " + d})


threading.Thread(target=reader, daemon=True).start()


def user(text):
    send({"type": "user", "message": {"role": "user", "content": text}})


if SC == "multi":
    user("Run the slow command, then the next one.")
    time.sleep(3); user("INJECT-ONE: say ONE in your answer.")
    time.sleep(1.5); user("INJECT-TWO: say TWO in your answer.")
elif SC == "perm":
    user("Run the command.")
    time.sleep(2.5); user("INJECT-A: change of plan, say BANANA.")
    time.sleep(3)
    for e in pending_perm:  # approve the prompt that has been open for ~5s
        send({"type": "control_response", "response": {"subtype": "success", "request_id": e["request_id"], "response": {"behavior": "allow", "updatedInput": e["request"].get("input")}}})
        EV.append({"t": now(), "dir": "NOTE", "d": "approved prompt"})
else:
    user("Do the work.")
    time.sleep(3); user("INJECT-A: change of plan, say BANANA.")

deadline = time.time() + 40
while time.time() < deadline:
    time.sleep(0.5)
    n_results = sum(1 for e in EV if e["dir"] == "RECV" and e["d"].startswith("result"))
    if time.time() - T0 > 12 and p.poll() is None and n_results >= (2 if SC == "final" else 1) and time.time() - [e["t"] + T0 for e in EV][-1] > 3:
        break
try:
    p.stdin.close()
except Exception:
    pass
try:
    p.wait(10)
except Exception:
    p.kill()

print("=== scenario", SC, "(CLI", subprocess.run(["claude", "--version"], capture_output=True, text=True).stdout.strip() + ")")
print("--- CLI io timeline")
for e in EV:
    print(f'{e["t"]:6.2f} {e["dir"]} {e["d"]}')
print("--- model requests (main loop first-class)")
for r in REQS:
    if r["kind"] == "side":
        continue
    print(f'{r["t"]:6.2f} {r["kind"]}#{r["idx"]} injected_in_request={r["has_injected_text"]}')
    for m in r["messages"][-4:]:
        print("         ", m)
json.dump({"events": EV, "requests": REQS}, open(os.path.join(OUT, f"{SC}.json"), "w"), indent=1)
