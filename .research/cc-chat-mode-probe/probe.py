"""Probe the Claude Code CLI flags Chat v1 relies on, with the real `claude`
binary against a scripted Messages API. Nothing leaves 127.0.0.1.

    python3 probe.py <outdir>

Checks (see ../../.design/chat-mode.md §7 U1-U3, U8, U17):
  tools      --tools "Read,Glob,Grep" makes the request carry exactly those
             three tools; --tools "" carries none.
  restricted --restricted confines Read to the working directory: an absolute
             path outside, ../ and a symlink pointing outside all fail.
  safe       --safe-mode does not load CLAUDE.md, settings hooks or skills;
             control run without it proves the hook would otherwise fire.
  image      a user message content block of type image reaches the API as-is.
  configdir  transcripts land under CLAUDE_CONFIG_DIR, not the real ~/.claude.
  system     --system-prompt replaces the built-in system prompt.

Results: ../cc-chat-mode-probe.md
"""
import base64, json, os, shutil, subprocess, sys, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

OUT = os.path.abspath(sys.argv[1])
os.makedirs(OUT, exist_ok=True)
CLAUDE = shutil.which("claude") or "claude"
T0 = time.time()
LOCK = threading.Lock()
LOG = open(os.path.join(OUT, "requests.jsonl"), "w")
BODIES = os.path.join(OUT, "bodies")
os.makedirs(BODIES, exist_ok=True)
SEQ = [0]

PNG = base64.b64encode(bytes.fromhex(
    "89504e470d0a1a0a0000000d4948445200000001000000010806000000"
    "1f15c4890000000d49444154789c6360000002000100ffff030000060005"
    "57bfabd40000000049454e44ae426082")).decode()


def text_of(content):
    if isinstance(content, str):
        return content
    return "\n".join(b.get("text", "") for b in content if isinstance(b, dict) and b.get("type") == "text")


def system_text(req):
    s = req.get("system", "")
    return s if isinstance(s, str) else "\n".join(b.get("text", "") for b in s if isinstance(b, dict))


def first_user(req):
    for m in req.get("messages", []):
        if m.get("role") == "user":
            return text_of(m.get("content"))
    return ""


def scenario(req):
    text = first_user(req)
    for name in ("TOOLS-EMPTY", "TOOLS-PROBE", "RESTRICT-PROBE", "SAFE-PROBE", "IMAGE-PROBE", "SYSTEM-PROBE"):
        if name in text:
            return name
    return "other"


# Each scenario: how many model calls so far, and what to answer.
def answer(kind, req):
    tools = {t.get("name") for t in req.get("tools") or []}
    last = next((m for m in reversed(req.get("messages", [])) if m.get("role") == "user"), {})
    if kind in ("TOOLS-PROBE", "TOOLS-EMPTY", "SYSTEM-PROBE", "IMAGE-PROBE"):
        return [("text", f"SEE-TOOLS {sorted(tools)}")]
    if kind == "SAFE-PROBE":
        # One Read so a PreToolUse hook would fire if one were loaded.
        last = next((m for m in reversed(req.get("messages", [])) if m.get("role") == "user"), {})
        content = last.get("content")
        if any(isinstance(b, dict) and b.get("type") == "tool_result" for b in (content if isinstance(content, list) else [])):
            return [("text", "SAFE-DONE")]
        return [("tool", "Read", {"file_path": "notes.txt"})]
    if kind == "RESTRICT-PROBE":
        # Ask the model to Read one path given in the prompt, then report the
        # result it got. The path is after "READ:" in the first user message.
        first = first_user(req)
        target = first.split("READ:", 1)[1].split("\n", 1)[0].strip() if "READ:" in first else "."
        content = last.get("content")
        results = [b for b in (content if isinstance(content, list) else []) if isinstance(b, dict) and b.get("type") == "tool_result"]
        if results:
            return [("text", "RESULT " + json.dumps(results[-1].get("content"))[:800])]
        return [("tool", "Read", {"file_path": target})]
    return [("text", "ok")]


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
        if not path.endswith("/v1/messages"):
            self.record({"method": "POST", "path": self.path})
            if path.endswith("/count_tokens"):
                return self.send_json(200, {"input_tokens": 100})
            return self.send_json(404, {"type": "error", "error": {"type": "not_found_error", "message": path}})
        kind = scenario(req)
        out = answer(kind, req)
        with LOCK:
            seq = SEQ[0] + 1
        with open(os.path.join(BODIES, f"body-{seq:03d}.json"), "wb") as f:
            f.write(raw)
        tools = sorted(t.get("name") for t in req.get("tools") or [])
        first = first_user(req)
        content = next((m.get("content") for m in req.get("messages", []) if m.get("role") == "user"), None)
        blocks = None
        if isinstance(content, list):
            blocks = [{"type": b.get("type"), "media_type": (b.get("source") or {}).get("media_type"),
                       "data_len": len((b.get("source") or {}).get("data") or "")} for b in content if isinstance(b, dict)]
        self.record({
            "kind": kind, "model": req.get("model"), "tools": tools, "n_tools": len(tools),
            "system_len": len(system_text(req)), "system_head": system_text(req)[:120],
            "first_user": first[:160], "user_blocks": blocks,
            "msg_roles": [m.get("role") for m in req.get("messages", [])],
            "reply": [b[1][:80] if b[0] == "text" else b[1] for b in out],
        })
        model = req.get("model", "demo-sonnet")
        mid = "msg_" + uuid.uuid4().hex[:20]
        stop = "tool_use" if any(b[0] == "tool" for b in out) else "end_turn"
        body = []
        for b in out:
            if b[0] == "text":
                body.append({"type": "text", "text": b[1]})
            else:
                body.append({"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20], "name": b[1], "input": b[2]})
        usage = {"input_tokens": 120, "output_tokens": 40}
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("connection", "close")
        self.end_headers()

        def ev(name, data):
            self.wfile.write(f"event: {name}\ndata: {json.dumps(data)}\n\n".encode())
            self.wfile.flush()

        ev("message_start", {"type": "message_start", "message": {"id": mid, "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 120, "output_tokens": 1}}})
        for i, c in enumerate(body):
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
BASE = f"http://127.0.0.1:{srv.server_address[1]}"

root = os.path.join(OUT, "world")
home = os.path.join(root, "home")
chat = os.path.join(root, "chat")
outside = os.path.join(root, "outside")
for d in (root, home, chat, outside):
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
config = os.path.join(home, ".claude")
os.makedirs(config)
with open(os.path.join(home, ".claude.json"), "w") as f:
    json.dump({"hasCompletedOnboarding": True, "hasTrustDialogAccepted": True}, f)

# Fixtures that --safe-mode must ignore.
SECRET = os.path.join(outside, "secret.txt")
with open(SECRET, "w") as f:
    f.write("OUTSIDE-SECRET-42\n")
with open(os.path.join(root, "escape.txt"), "w") as f:
    f.write("ESCAPE-VIA-DOTDOT-7\n")
with open(os.path.join(chat, "notes.txt"), "w") as f:
    f.write("NOTES-OK\n")
os.symlink(SECRET, os.path.join(chat, "link.txt"))
with open(os.path.join(chat, "CLAUDE.md"), "w") as f:
    f.write("CLAUDE_MD_MARKER_ABC this text must not reach the model under --safe-mode.\n")
with open(os.path.join(config, "CLAUDE.md"), "w") as f:
    f.write("USER_CLAUDE_MD_MARKER_DEF must not load.\n")
hook_marker = os.path.join(root, "hook-ran")
with open(os.path.join(config, "settings.json"), "w") as f:
    json.dump({"hooks": {"PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": f"touch {hook_marker}"}]}]}}, f)

env = {"PATH": os.environ["PATH"], "HOME": home, "CLAUDE_CONFIG_DIR": config,
       "LANG": "C.UTF-8", "NO_PROXY": "*", "no_proxy": "*",
       "ANTHROPIC_BASE_URL": BASE, "ANTHROPIC_AUTH_TOKEN": "sk-ant-fake-probe",
       "ANTHROPIC_API_KEY": "",
       "DISABLE_TELEMETRY": "1", "DISABLE_AUTOUPDATER": "1", "DISABLE_ERROR_REPORTING": "1",
       "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}

SYS = ("You are the chat assistant in Tingly Box. You may read files in this chat's folder. "
       "You cannot write, run commands or use the web. SYSTEMMARKER_XYZ")
READONLY = ["--tools", "Read,Glob,Grep", "--allowedTools", "Read,Glob,Grep",
            "--restricted", "--safe-mode", "--strict-mcp-config", "--permission-mode", "default"]
results = {}


def run(name, flags, prompt, image=None, timeout=120):
    msg = {"type": "user", "message": {"role": "user", "content": prompt}}
    if image:
        msg["message"]["content"] = [{"type": "text", "text": prompt},
                                     {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": image}}]
    cmd = [CLAUDE, "--print", "", "--output-format", "stream-json", "--verbose",
           "--input-format", "stream-json", "--system-prompt", SYS] + flags
    t = time.time()
    p = subprocess.run(cmd, input=(json.dumps(msg) + "\n").encode(), cwd=chat, env=env,
                       capture_output=True, timeout=timeout)
    results[name] = {"rc": p.returncode, "secs": round(time.time() - t, 1),
                     "stdout_tail": p.stdout.decode(errors="replace")[-500:],
                     "stderr_tail": p.stderr.decode(errors="replace")[-400:]}


try:
    run("tools", READONLY, "TOOLS-PROBE: report which tools you have.")
    run("tools_empty", ["--tools", "", "--restricted", "--safe-mode", "--strict-mcp-config", "--permission-mode", "default"],
        "TOOLS-EMPTY: report which tools you have.")
    for label, target in (("abs", SECRET), ("dotdot", "../escape.txt"), ("symlink", "link.txt"), ("inside", "notes.txt")):
        run("restrict_" + label, READONLY, f"RESTRICT-PROBE\nREAD:{target}")
    if os.path.exists(hook_marker):
        os.remove(hook_marker)
    run("safe", READONLY, "SAFE-PROBE: say hi.")
    results["safe_hook_ran"] = os.path.exists(hook_marker)
    if os.path.exists(hook_marker):
        os.remove(hook_marker)
    # Control: same flags without --safe-mode — the hook and CLAUDE.md load.
    run("safe_control", [f for f in READONLY if f != "--safe-mode"], "SAFE-PROBE: say hi.")
    results["safe_control_hook_ran"] = os.path.exists(hook_marker)
    # Cleaner control: no --restricted either, so settings/CLAUDE.md really can load.
    if os.path.exists(hook_marker):
        os.remove(hook_marker)
    run("plain_control", ["--tools", "Read,Glob,Grep", "--allowedTools", "Read,Glob,Grep", "--permission-mode", "default"],
        "SAFE-PROBE: say hi.")
    results["plain_control_hook_ran"] = os.path.exists(hook_marker)
    run("image", READONLY, "IMAGE-PROBE: describe the image.", image=PNG)
    run("system", ["--safe-mode"], "SYSTEM-PROBE: say hi.")
finally:
    srv.shutdown()
    with open(os.path.join(OUT, "result.json"), "w") as f:
        json.dump(results, f, indent=1)
    print(json.dumps(results, indent=1)[:4000])
