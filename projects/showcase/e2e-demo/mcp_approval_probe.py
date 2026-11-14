"""Does a thread's own MCP tool run without an approval card? One run per permission mode.

Usage: [GITHUB_ACCESS=ask] [USER_ASK=tool,tool] python3 mcp_approval_probe.py CLAUDE_CLI TINGLY_PROJECT [MODE ...]

Real Claude Code against a scripted model: a folder session calls the Projects
MCP tool `view` (a read) and then `create_issue` (a write), and the probe prints
whether an `approval` entry appeared. Everything lives in a temporary data
directory; no upstream API key and no network are used. The folder has no GitHub
origin, so a tool that was allowed fails with "no origin remote": that answer is
the proof it ran, an approval card is the proof it did not.

GITHUB_ACCESS sets the project's thread_github_access (ask is the opt-out).
USER_ASK writes `ask` rules into the Claude config the CLI reads, which is what
a launch profile can do. See .design/github-tools.md ("Probe").
"""
import json, os, socket, subprocess, sys, tempfile, time, urllib.error, urllib.request
from pathlib import Path

MOCK = r'''
import json, sys, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
def results(m):
    c = m.get("content")
    return [] if isinstance(c, str) else [b for b in c if b.get("type") == "tool_result"]
def pick(req):
    names = [t.get("name", "") for t in req.get("tools") or []]
    if "Write" not in names:  # not the thread's session (a title or summary call)
        return [("text", "ok")]
    view = next((n for n in names if n.endswith("__view")), None)
    issue = next((n for n in names if n.endswith("__create_issue")), None)
    done = sum(len(results(m)) for m in req["messages"] if m["role"] == "user")
    if not view or not issue:
        return [("text", "TOOLS MISSING")]
    if done == 0:
        return [("tool", view, {"number": 1})]
    if done == 1:
        return [("tool", issue, {"title": "probe"})]
    return [("text", "PROBE DONE")]
class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def log_message(self, *a): pass
    def send_json(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code); self.send_header("content-type", "application/json"); self.send_header("content-length", str(len(b))); self.end_headers(); self.wfile.write(b)
    def do_GET(self):
        if self.path.split("?")[0].endswith("/v1/models"):
            return self.send_json(200, {"data": [{"id": "claude-sonnet-4-6", "type": "model", "display_name": "S", "created_at": "2026-01-01T00:00:00Z"}], "has_more": False, "first_id": "claude-sonnet-4-6", "last_id": "claude-sonnet-4-6"})
        self.send_json(404, {})
    def do_HEAD(self):
        self.send_response(200); self.send_header("content-length", "0"); self.end_headers()
    def do_POST(self):
        n = int(self.headers.get("content-length") or 0)
        req = json.loads(self.rfile.read(n) or b"{}")
        if self.path.split("?")[0].endswith("/count_tokens"):
            return self.send_json(200, {"input_tokens": 100})
        blocks = pick(req)
        mid = "msg_" + uuid.uuid4().hex[:20]
        stop = "tool_use" if any(b[0] == "tool" for b in blocks) else "end_turn"
        content = [{"type": "text", "text": b[1]} if b[0] == "text" else {"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20], "name": b[1], "input": b[2]} for b in blocks]
        model = req.get("model", "claude-sonnet-4-6")
        if not req.get("stream"):
            return self.send_json(200, {"id": mid, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "stop_sequence": None, "usage": {"input_tokens": 120, "output_tokens": 40}})
        self.send_response(200); self.send_header("content-type", "text/event-stream"); self.send_header("connection", "close"); self.end_headers()
        def ev(name, data):
            self.wfile.write(f"event: {name}\ndata: {json.dumps(data)}\n\n".encode()); self.wfile.flush()
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
ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
'''


def port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def call(url, method="GET", body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {url}: {e.code} {e.read().decode()[:300]}")


def run(claude, app, mode, github_access=None):
    # The CLI can still be writing its config as the run ends.
    with tempfile.TemporaryDirectory(prefix="mcp-approval-probe-", ignore_cleanup_errors=True) as temp:
        temp = Path(temp)
        repo = temp / "repo"
        repo.mkdir()
        for cmd in (["git", "init", "-q", "-b", "main"], ["git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"]):
            subprocess.run(cmd, cwd=repo, check=True)
        (temp / "mock.py").write_text(MOCK)
        mp, sp = port(), port()
        env = os.environ.copy()
        env.update({"CLAUDE_CLI_PATH": claude, "CLAUDE_CONFIG_DIR": str(temp / "cc"), "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1",
                    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost", "HOME": str(temp / "home")})
        (temp / "home").mkdir()
        if os.environ.get("USER_ASK"):
            (temp / "cc").mkdir()
            (temp / "cc" / "settings.json").write_text(json.dumps({"permissions": {"ask": os.environ["USER_ASK"].split(",")}}))
        with (temp / "server.log").open("w+") as slog:
            model = subprocess.Popen([sys.executable, str(temp / "mock.py"), str(mp)])
            server = subprocess.Popen([app, "--port", str(sp), "--data-dir", str(temp / "data")], env=env, stdout=slog, stderr=subprocess.STDOUT)
            base = f"http://127.0.0.1:{sp}/api/v1"
            try:
                for _ in range(100):
                    try:
                        call(base + "/health")
                        break
                    except Exception:
                        time.sleep(.1)
                call(base + "/settings", "PUT", {"api_style": "anthropic", "base_url": f"http://127.0.0.1:{mp}", "api_key": "k", "default_model": "claude-sonnet-4-6"})
                folder = call(base + "/projects", "POST", {"type": "folder", "path": str(repo)})
                if github_access:
                    call(base + f"/projects/{folder['id']}", "PATCH", {"thread_github_access": github_access})
                tid = call(base + f"/projects/{folder['id']}/threads", "POST", {"title": "Probe", "content": "call the tools", "permission_mode": mode})["id"]
                seen = []
                for _ in range(400):
                    seen = call(base + f"/threads/{tid}/events?limit=200")["events"]
                    if any("PROBE DONE" in (e.get("content") or "") for e in seen) or any(e.get("kind") == "error" for e in seen):
                        break
                    # A card is the finding; deny it so the run can finish.
                    for e in call(base + f"/threads/{tid}/requests")["requests"]:
                        call(base + f"/threads/{tid}/respond", "POST", {"request_id": json.loads(e["payload"])["request_id"], "allow": False})
                    time.sleep(.1)
                return {
                    "approvals": [json.loads(e.get("payload") or "{}").get("tool") for e in seen if e.get("kind") == "approval"],
                    "tool_results": [(e.get("content") or "")[:90] for e in seen if e.get("kind") == "tool_result"],
                    "errors": [e.get("content") for e in seen if e.get("kind") == "error"],
                }
            finally:
                server.terminate()
                model.terminate()
                server.wait(timeout=10)
                model.wait(timeout=10)


if __name__ == "__main__":
    claude, app = map(os.path.abspath, sys.argv[1:3])
    access = os.environ.get("GITHUB_ACCESS") or None
    for mode in (sys.argv[3:] or ["auto", "acceptEdits", "default"]):
        r = run(claude, app, mode, access)
        print(f"mode={mode:18} github_access={access or '-':6} cards={r['approvals']} errors={r['errors']}")
        for t in r["tool_results"]:
            print("    tool_result:", t)
