"""End to end: one request_permission, one yes, and the thread's own Bash call
runs, in auto mode, with a classifier that blocks everything.

    python3 e2e.py CLAUDE_CLI TINGLY_PROJECT [early|late|deny ...]

Real Projects server + real Claude Code against a scripted model. The thread
(a project thread in a git repo, permission mode auto, model
claude-sonnet-4-6 so auto mode holds) runs ./tool.sh, which the classifier
blocks (the mock answers it with nothing it can parse, which blocks):

  early  request_permission(command) first, the user allows, then Bash
  late   Bash first (blocked), request_permission(command), allow, Bash again
  deny   request_permission(command), the user denies, then Bash anyway

The probe answers every card: allow for request_permission (deny in deny),
deny for anything else, and counts them. Everything lives in a temporary
directory with its own HOME; nothing leaves 127.0.0.1.
"""
import json, os, socket, subprocess, sys, tempfile, time, urllib.error, urllib.request
from pathlib import Path

MOCK = r'''
import json, sys, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
SC = sys.argv[2]
CMD = "./tool.sh PROBE-RAN"
def results(m):
    c = m.get("content")
    return [] if isinstance(c, str) else [b for b in c if b.get("type") == "tool_result"]
def pick(req):
    names = [t.get("name", "") for t in req.get("tools") or []]
    start = next((n for n in names if n.endswith("__start_thread")), None)
    if start:  # the coordinator: start the thread once
        done = sum(len(results(m)) for m in req["messages"] if m["role"] == "user")
        return [("tool", start, {"title": "Run the probe tool", "task": "run the probe tool"})] if done == 0 else [("text", "Started.")]
    if "Write" not in names:  # a side call: title, summary, or the auto-mode classifier
        if "PROBE-RAN" in json.dumps(req):
            with open(sys.argv[3], "a") as f:
                f.write("classifier\n")
        return [("text", "ok")]
    ask = next((n for n in names if n.endswith("__request_permission")), None)
    if not ask:
        return [("text", "TOOLS MISSING")]
    done = sum(len(results(m)) for m in req["messages"] if m["role"] == "user")
    bash = ("tool", "Bash", {"command": CMD, "description": "probe"})
    perm = ("tool", ask, {"action": "run the probe tool", "reason": "the probe needs it", "command": CMD})
    plan = {"early": [perm, bash], "deny": [perm, bash], "late": [bash, perm, bash]}[SC]
    if done < len(plan):
        return [plan[done]]
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


def run(claude, app, sc):
    with tempfile.TemporaryDirectory(prefix="grant-e2e-", ignore_cleanup_errors=True) as temp:
        temp = Path(temp)
        repo = temp / "repo"
        repo.mkdir()
        (repo / "tool.sh").write_text('#!/bin/sh\necho "$1" > ran.txt\necho ran\n')
        os.chmod(repo / "tool.sh", 0o755)
        for cmd in (["git", "init", "-q", "-b", "main"], ["git", "add", "tool.sh"], ["git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"]):
            subprocess.run(cmd, cwd=repo, check=True)
        (temp / "mock.py").write_text(MOCK)
        cls_log = temp / "classifier.log"
        cls_log.write_text("")
        mp, sp = port(), port()
        (temp / "home").mkdir()
        env = {"PATH": os.environ["PATH"], "HOME": str(temp / "home"), "CLAUDE_CLI_PATH": claude, "CLAUDE_CONFIG_DIR": str(temp / "cc"),
               "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
               "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost", "TMPDIR": str(temp)}
        with (temp / "server.log").open("w+") as slog:
            model = subprocess.Popen([sys.executable, str(temp / "mock.py"), str(mp), sc, str(cls_log)])
            server = subprocess.Popen([app, "--port", str(sp), "--data-dir", str(temp / "data")], env=env, stdout=slog, stderr=subprocess.STDOUT)
            base = f"http://127.0.0.1:{sp}/api/v1"
            cards = []
            try:
                for _ in range(100):
                    try:
                        call(base + "/health")
                        break
                    except Exception:
                        time.sleep(.1)
                call(base + "/settings", "PUT", {"api_style": "anthropic", "base_url": f"http://127.0.0.1:{mp}", "api_key": "k", "default_model": "claude-sonnet-4-6"})
                proj = call(base + "/projects", "POST", {"name": "Probe", "repos": [str(repo)]})
                call(base + f"/projects/{proj['id']}", "PATCH", {"thread_permission_mode": "auto"})
                proj = call(base + f"/projects/{proj['id']}")
                main = proj.get("main_thread_id") or proj.get("project", {}).get("main_thread_id")
                call(base + f"/threads/{main}/messages", "POST", {"content": "start the probe thread"})
                tid = None
                for _ in range(200):
                    lst = call(base + f"/projects/{proj.get('id') or proj['project']['id']}/threads")
                    ths = lst if isinstance(lst, list) else lst.get("threads", [])
                    ws = [t for t in ths if t["id"] != main]
                    if ws:
                        tid = ws[0]["id"]
                        break
                    time.sleep(.1)
                seen = []
                for _ in range(600):
                    seen = call(base + f"/threads/{tid}/events?limit=300")["events"]
                    if any("PROBE DONE" in (e.get("content") or "") for e in seen) or any(e.get("kind") == "error" for e in seen):
                        break
                    for e in call(base + f"/threads/{tid}/requests")["requests"]:
                        p = json.loads(e["payload"])
                        ours = p.get("tool", "").endswith("__request_permission")
                        allow = ours and sc != "deny"
                        cards.append((p.get("tool"), "allow" if allow else "deny"))
                        call(base + f"/threads/{tid}/respond", "POST", {"request_id": p["request_id"], "allow": allow})
                    time.sleep(.1)
                return {
                    "cards": cards,
                    "ran": any(temp.rglob("ran.txt")),
                    "classifier_calls": len(cls_log.read_text().split()),
                    "tool_results": [(e.get("content") or "")[:110].replace("\n", " ") for e in seen if e.get("kind") == "tool_result"],
                    "notes": [e.get("content") for e in seen if e.get("kind") == "system" and e.get("content")],
                    "errors": [e.get("content") for e in seen if e.get("kind") == "error"],
                    "mode": [e.get("content") for e in seen if "mode" in (e.get("content") or "").lower()][:2],
                }
            finally:
                server.terminate()
                model.terminate()
                server.wait(timeout=10)
                model.wait(timeout=10)


if __name__ == "__main__":
    claude, app = map(os.path.abspath, sys.argv[1:3])
    for sc in (sys.argv[3:] or ["early", "late", "deny"]):
        r = run(claude, app, sc)
        print(f"== {sc}: ran={r['ran']} cards={r['cards']} classifier_calls={r['classifier_calls']} errors={r['errors']}")
        for t in r["tool_results"]:
            print("    tool_result:", t)
        for n in r["notes"]:
            print("    note:", n)
