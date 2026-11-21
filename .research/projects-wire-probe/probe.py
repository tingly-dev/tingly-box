"""What a real Projects run sends to its model: the real tingly-project server
and the real `claude` CLI against a scripted Anthropic Messages API that logs
every request (headers that could identify the conversation, tools, system,
first/last messages, the ids of earlier assistant turns).

    python3 probe.py <tingly-project binary> <outdir> [--no-hint-headers] [--subagent]

The script is the e2e-demo flow cut to one thread: the coordinator starts
"Add CHANGELOG", the thread writes CHANGELOG.md, asks to commit (an approval
the probe allows), commits and reports; the coordinator answers the report.
With --subagent the thread first runs one Agent call. Everything runs under
an isolated HOME and CLAUDE_CONFIG_DIR with a fake key; nothing leaves
127.0.0.1. Findings: ../projects-wire-probe.md.
"""
import json, os, shutil, subprocess, sys, threading, time, urllib.request, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN, OUT = sys.argv[1], os.path.abspath(sys.argv[2])
HINT = "--no-hint-headers" not in sys.argv
SUB = "--subagent" in sys.argv
os.makedirs(OUT, exist_ok=True)
T0 = time.time()
LOCK = threading.Lock()
LOG = open(os.path.join(OUT, "requests.jsonl"), "w")
SEQ = [0]
HEADERS = ["x-claude-code-request-class", "x-claude-code-agent-type", "x-claude-code-session-id",
           "x-claude-code-prompt-id", "x-client-request-id", "x-app", "user-agent", "anthropic-beta"]
TITLE = "Add CHANGELOG"
SUBPROMPT = "List the files in the current directory and say how many there are."


def text_of(content):
    if isinstance(content, str):
        return content
    return "\n".join(b.get("text", "") for b in content if b.get("type") == "text")


def system_text(req):
    s = req.get("system", "")
    return s if isinstance(s, str) else "\n".join(b.get("text", "") for b in s)


def blocks(msg):
    c = msg.get("content")
    return [{"type": "text", "text": c}] if isinstance(c, str) else c


def last_assistant_ids(req):
    for m in reversed(req.get("messages", [])):
        if m["role"] == "assistant":
            ids = [b.get("id") for b in blocks(m) if b.get("type") == "tool_use"]
            txt = [b.get("text", "")[:60] for b in blocks(m) if b.get("type") == "text"]
            return {"tool_use_ids": ids, "text": txt}
    return None


def tool_results(msg):
    return [b for b in blocks(msg) if b.get("type") == "tool_result"]


# --- the script: which conversation is this, what comes next ---------------
def classify(req, hdr):
    tools = {t.get("name") for t in req.get("tools") or []}
    sysp = system_text(req)
    first = text_of(req["messages"][0]["content"]) if req.get("messages") else ""
    if any(t.endswith("__start_thread") for t in tools):
        return "coordinator"
    if SUBPROMPT in first:
        return "subagent"
    if f"Your task: {TITLE}" in sysp and tools:
        return "thread"
    return "other"


def answer(kind, req):
    msgs = [m for m in req["messages"] if m["role"] != "system"]  # 2.1.259 puts role=system entries in messages
    n = sum(len(tool_results(m)) for m in msgs if m["role"] == "user")
    last = msgs[-1]
    if kind == "coordinator":
        if tool_results(last):
            return [("text", "Started one thread: Add CHANGELOG.")]
        tail = [b.get("text", "") for b in blocks(last) if b.get("type") == "text"][-1:]
        if tail and 'kind="check-in"' in tail[0]:
            return [("text", "Add CHANGELOG is committed on its branch and waits for review.")]
        return [("tool", "mcp__tingly-projects__start_thread", {"title": TITLE, "task": "Create CHANGELOG.md with an Unreleased section and commit it."})]
    if kind == "subagent":
        if n == 0:
            return [("tool", "Bash", {"command": "ls | wc -l", "description": "Count files"})]
        return [("text", "SUBAGENT-DONE: counted the files.")]
    if kind == "thread":
        sysp = system_text(req)
        cwd = sysp.split("in your own worktree (", 1)[1].split(")", 1)[0] if "in your own worktree (" in sysp else "."
        steps = []
        if SUB:
            tools = {t.get("name") for t in req.get("tools") or []}
            steps.append([("tool", "Agent" if "Agent" in tools else "Task", {"description": "count files", "prompt": SUBPROMPT, "subagent_type": "general-purpose"})])
        steps += [
            [("text", "I'll create CHANGELOG.md."), ("tool", "Write", {"file_path": f"{cwd}/CHANGELOG.md", "content": "# Changelog\n\n## Unreleased\n"})],
            [("text", "Now commit it."), ("tool", "Bash", {"command": "git add CHANGELOG.md && git commit -qm 'Add CHANGELOG'", "description": "Commit CHANGELOG.md"})],
            [("text", "Added CHANGELOG.md and committed it; git log -1 shows the commit.")],
        ]
        return steps[min(n, len(steps) - 1)]
    return [("text", "{}")]


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

    def record(self, entry):
        with LOCK:
            SEQ[0] += 1
            entry = {"seq": SEQ[0], "t": round(time.time() - T0, 2), **entry}
            LOG.write(json.dumps(entry, ensure_ascii=False) + "\n")
            LOG.flush()

    def do_GET(self):
        self.record({"method": "GET", "path": self.path})
        if self.path.split("?")[0].endswith("/v1/models"):
            return self.send_json(200, {"data": [{"id": "demo-sonnet", "type": "model", "display_name": "Demo", "created_at": "2026-01-01T00:00:00Z"}], "has_more": False, "first_id": "demo-sonnet", "last_id": "demo-sonnet"})
        self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": self.path}})

    def do_HEAD(self):
        self.record({"method": "HEAD", "path": self.path})
        self.send_response(200)
        self.send_header("content-length", "0")
        self.end_headers()

    def do_POST(self):
        n = int(self.headers.get("content-length") or 0)
        raw = self.rfile.read(n) or b"{}"
        req = json.loads(raw)
        path = self.path.split("?")[0]
        hdr = {h: self.headers.get(h) for h in HEADERS if self.headers.get(h) is not None}
        if not path.endswith("/v1/messages"):
            self.record({"method": "POST", "path": self.path, "headers": hdr})
            if path.endswith("/count_tokens"):
                return self.send_json(200, {"input_tokens": 100})
            return self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": path}})
        kind = classify(req, hdr)
        out = answer(kind, req)
        msgs = req.get("messages", [])
        meta = req.get("metadata", {}).get("user_id")
        try:
            meta = json.loads(meta)
        except Exception:
            pass
        with LOCK:
            seq = SEQ[0] + 1
        with open(os.path.join(OUT, f"body-{seq:03d}.json"), "wb") as f:
            f.write(raw)
        self.record({
            "method": "POST", "path": self.path, "kind": kind, "headers": hdr, "model": req.get("model"),
            "stream": req.get("stream"), "n_tools": len(req.get("tools") or []),
            "tools_sample": sorted(t.get("name") for t in req.get("tools") or [])[:60],
            "system_head": system_text(req)[:160], "system_len": len(system_text(req)),
            "first_user": text_of(msgs[0]["content"])[-200:] if msgs else None,
            "last_user": next((text_of(m["content"])[-200:] for m in reversed(msgs) if m["role"] == "user"), None),
            "roles": [m["role"] for m in msgs],
            "n_messages": len(msgs), "last_assistant": last_assistant_ids(req),
            "metadata_user_id": meta, "max_tokens": req.get("max_tokens"),
            "reply": [b[1] if b[0] == "tool" else b[1][:60] for b in out],
        })
        model = req.get("model", "demo-sonnet")
        mid = "msg_" + uuid.uuid4().hex[:20]
        stop = "tool_use" if any(b[0] == "tool" for b in out) else "end_turn"
        content = []
        for b in out:
            if b[0] == "text":
                content.append({"type": "text", "text": b[1]})
            else:
                content.append({"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20], "name": b[1], "input": b[2]})
        usage = {"input_tokens": 120, "output_tokens": 40}
        if not req.get("stream"):
            return self.send_json(200, {"id": mid, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "stop_sequence": None, "usage": usage})
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("connection", "close")
        self.end_headers()

        def ev(name, data):
            self.wfile.write(f"event: {name}\ndata: {json.dumps(data)}\n\n".encode())
            self.wfile.flush()

        ev("message_start", {"type": "message_start", "message": {"id": mid, "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 120, "output_tokens": 1}}})
        for i, c in enumerate(content):
            if c["type"] == "text":
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "text", "text": ""}})
                ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "text_delta", "text": c["text"]}})
            else:
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "tool_use", "id": c["id"], "name": c["name"], "input": {}}})
                ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "input_json_delta", "partial_json": json.dumps(c["input"])}})
            ev("content_block_stop", {"type": "content_block_stop", "index": i})
        ev("message_delta", {"type": "message_delta", "delta": {"stop_reason": stop, "stop_sequence": None}, "usage": {"output_tokens": 40}})
        ev("message_stop", {"type": "message_stop"})
        self.close_connection = True


srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
threading.Thread(target=srv.serve_forever, daemon=True).start()
mock = f"http://127.0.0.1:{srv.server_address[1]}"

# --- an isolated world -------------------------------------------------------
home, data, repo = (os.path.join(OUT, d) for d in ("home", "data", "repo"))
for d in (home, data, repo):
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
os.makedirs(os.path.join(home, ".claude"))
with open(os.path.join(home, ".claude.json"), "w") as f:
    json.dump({"hasCompletedOnboarding": True}, f)


def git(*a):
    subprocess.run(["git", *a], cwd=repo, check=True, capture_output=True)


git("init", "-q", "-b", "main")
with open(os.path.join(repo, "README.md"), "w") as f:
    f.write("demo\n")
git("-c", "user.name=demo", "-c", "user.email=demo@example.com", "add", ".")
git("-c", "user.name=demo", "-c", "user.email=demo@example.com", "commit", "-qm", "init")

port = 12700 + os.getpid() % 200
env = {"PATH": os.environ["PATH"], "HOME": home, "CLAUDE_CONFIG_DIR": os.path.join(home, ".claude"),
       "LANG": "C.UTF-8", "NO_PROXY": "*", "no_proxy": "*", "GIT_AUTHOR_NAME": "demo", "GIT_AUTHOR_EMAIL": "demo@example.com",
       "GIT_COMMITTER_NAME": "demo", "GIT_COMMITTER_EMAIL": "demo@example.com",
       "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1", "DISABLE_ERROR_REPORTING": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
if HINT:
    env["CLAUDE_CODE_GATEWAY_HINT_HEADERS"] = "1"
server = subprocess.Popen([BIN, "serve", "--port", str(port), "--data-dir", data], env=env,
                          stdout=open(os.path.join(OUT, "server.log"), "w"), stderr=subprocess.STDOUT)
API = f"http://127.0.0.1:{port}/api/v1"


def call(method, path, body=None):
    req = urllib.request.Request(API + path, method=method, data=None if body is None else json.dumps(body).encode(),
                                 headers={"content-type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read() or b"null")


def wait(pred, what, timeout=120):
    end = time.time() + timeout
    while time.time() < end:
        try:
            v = pred()
            if v:
                return v
        except Exception:
            pass
        time.sleep(0.3)
    raise SystemExit(f"timed out waiting for {what}")


result = {"hint_headers": HINT, "subagent": SUB}
try:
    wait(lambda: call("GET", "/health"), "server")
    call("PUT", "/settings", {"api_style": "anthropic", "base_url": mock, "api_key": "sk-ant-fake-probe", "default_model": "demo-sonnet"})
    p = call("POST", "/projects", {"name": "Probe", "goal": "See what Projects sends", "repos": [repo]})
    call("POST", f"/threads/{p['main_thread_id']}/messages", {"content": "Add a CHANGELOG please."})
    th = wait(lambda: next((t for t in call("GET", f"/projects/{p['id']}/threads")["threads"] if t["title"] == TITLE), None), "thread")
    req = wait(lambda: (call("GET", f"/threads/{th['id']}/requests")["requests"] or [None])[0], "approval")
    result["approval"] = {"content": req["content"], "payload": json.loads(req.get("payload") or "{}")}
    call("POST", f"/threads/{th['id']}/respond", {"request_id": json.loads(req["payload"])["request_id"], "allow": True, "scope": "once"})
    wait(lambda: call("GET", f"/threads/{th['id']}")["thread"]["status"] == "ready", "thread ready")
    time.sleep(8)  # let the coordinator answer the report and any trailing requests arrive
    result["thread"] = call("GET", f"/threads/{th['id']}")["thread"]
    result["conversation"] = [(e["kind"], e["content"][:120]) for e in call("GET", f"/threads/{p['main_thread_id']}/events")["events"]]
finally:
    server.terminate()
    try:
        server.wait(10)
    except Exception:
        server.kill()
    with open(os.path.join(OUT, "result.json"), "w") as f:
        json.dump(result, f, indent=1, ensure_ascii=False)
    print(json.dumps({k: result.get(k) for k in ("hint_headers", "subagent", "approval")}, ensure_ascii=False)[:600])
