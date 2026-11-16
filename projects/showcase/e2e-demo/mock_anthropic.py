"""Scripted Anthropic Messages API for demoing tingly-project offline.

Coordinator turns (system prompt starts "You are the coordinator") and
Claude Code thread turns (requests carrying the Write tool) follow a fixed
script; anything else gets a short text reply.
"""
import json, re, sys, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Usage: python3 mock_anthropic.py PORT [LOGFILE]
LOG = open(sys.argv[2] if len(sys.argv) > 2 else "/dev/null", "a")

TASKS = {
    "Add CHANGELOG": ("CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- Initial release of the demo app.\n"),
    "Add greeting helper": ("greet.go", 'package main\n\n// Greet returns a friendly greeting for name.\nfunc Greet(name string) string { return "Hello, " + name + "!" }\n'),
}

def text_of(content):
    if isinstance(content, str):
        return content
    return "\n".join(b.get("text", "") for b in content if b.get("type") == "text")

def system_text(req):
    s = req.get("system", "")
    return s if isinstance(s, str) else "\n".join(b.get("text", "") for b in s)

def tool_results(msg):
    c = msg.get("content")
    return [] if isinstance(c, str) else [b for b in c if b.get("type") == "tool_result"]

def coordinator(req):
    msgs = req["messages"]
    last = msgs[-1]
    if tool_results(last):
        return [("text", "好的，我开了两个线程并行处理：**Add CHANGELOG** 和 **Add greeting helper**。它们各自在独立的 worktree 分支上工作，完成或需要你确认时我会告诉你。")]
    t = text_of(last["content"])
    if "Coordinator check-in" in t:
        if "waiting" in t or "NEEDS" in t or "permission" in t.lower():
            return [("text", "有线程在等你：它想执行 `git commit`，请到线程里批准。")]
        return [("text", "线程已完成并提交到自己的分支，可以在 Overview 的 *Ready for review* 里查看 diff 并合并。")]
    return [
        ("tool", "start_thread", {"title": "Add CHANGELOG", "task": "Create CHANGELOG.md with an Unreleased section and commit it."}),
        ("tool", "start_thread", {"title": "Add greeting helper", "task": "Add a Greet(name) helper in greet.go and commit it."}),
    ]

def thread(req):
    sysp = system_text(req)
    title = next((k for k in TASKS if f"Your task: {k}" in sysp), "Add CHANGELOG")
    fname, body = TASKS[title]
    m = re.search(r"Primary working directory: (\S+)|Working directory: (\S+)", sysp)
    cwd = (m.group(1) or m.group(2)) if m else "."
    n = sum(len(tool_results(x)) for x in req["messages"] if x["role"] == "user")
    if n == 0:
        return [("text", f"I'll create {fname}."), ("tool", "Write", {"file_path": f"{cwd}/{fname}", "content": body})]
    if n == 1:
        return [("text", "Now commit it to the thread branch."),
                ("tool", "Bash", {"command": f"git add {fname} && git commit -m '{title}'", "description": f"Commit {fname}"})]
    return [("text", f"Done. Added `{fname}` and committed it on this thread's branch.\n\nVerified: `git log -1` shows the commit; the file contains the expected content.")]

def generic(req):
    return [("text", "Demo")]

def pick(req):
    sysp = system_text(req)
    tools = {t.get("name") for t in req.get("tools") or []}
    if sysp.lstrip().startswith("You are the coordinator") or "start_thread" in tools:
        return "coordinator", coordinator(req)
    if "Write" in tools and "You are a thread in the project" in sysp:
        return "thread", thread(req)
    return "other", generic(req)

class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def log_message(self, *a): pass

    def send_json(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        if self.path.split("?")[0].endswith("/v1/models"):
            return self.send_json(200, {"data": [{"id": "demo-sonnet", "type": "model", "display_name": "Demo Sonnet", "created_at": "2026-01-01T00:00:00Z"}], "has_more": False, "first_id": "demo-sonnet", "last_id": "demo-sonnet"})
        self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": self.path}})

    def do_HEAD(self):
        self.send_response(200); self.send_header("content-length", "0"); self.end_headers()

    def do_POST(self):
        n = int(self.headers.get("content-length") or 0)
        req = json.loads(self.rfile.read(n) or b"{}")
        path = self.path.split("?")[0]
        if path.endswith("/count_tokens"):
            return self.send_json(200, {"input_tokens": 100})
        if not path.endswith("/v1/messages"):
            return self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": path}})
        kind, blocks = pick(req)
        LOG.write(f"{kind}: {[b[1] if b[0]=='tool' else b[1][:50] for b in blocks]}\n"); LOG.flush()
        model = req.get("model", "demo-sonnet")
        mid = "msg_" + uuid.uuid4().hex[:20]
        stop = "tool_use" if any(b[0] == "tool" for b in blocks) else "end_turn"
        content = []
        for b in blocks:
            if b[0] == "text":
                content.append({"type": "text", "text": b[1]})
            else:
                content.append({"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20], "name": b[1], "input": b[2]})
        usage = {"input_tokens": 120, "output_tokens": 40}
        if not req.get("stream"):
            return self.send_json(200, {"id": mid, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "stop_sequence": None, "usage": usage})
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("cache-control", "no-cache")
        self.send_header("connection", "close")
        self.end_headers()
        def ev(name, data):
            self.wfile.write(f"event: {name}\ndata: {json.dumps(data)}\n\n".encode()); self.wfile.flush()
        ev("message_start", {"type": "message_start", "message": {"id": mid, "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 120, "output_tokens": 1}}})
        for i, c in enumerate(content):
            if c["type"] == "text":
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "text", "text": ""}})
                t = c["text"]
                for j in range(0, len(t), 12):
                    ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "text_delta", "text": t[j:j+12]}})
            else:
                ev("content_block_start", {"type": "content_block_start", "index": i, "content_block": {"type": "tool_use", "id": c["id"], "name": c["name"], "input": {}}})
                ev("content_block_delta", {"type": "content_block_delta", "index": i, "delta": {"type": "input_json_delta", "partial_json": json.dumps(c["input"])}})
            ev("content_block_stop", {"type": "content_block_stop", "index": i})
        ev("message_delta", {"type": "message_delta", "delta": {"stop_reason": stop, "stop_sequence": None}, "usage": {"output_tokens": 40}})
        ev("message_stop", {"type": "message_stop"})
        self.close_connection = True

ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
