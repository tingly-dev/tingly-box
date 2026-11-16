"""Probe: how does a running `claude -p --input-format stream-json` process treat slash commands?

Usage: python3 slash-command-probe.py [CLAUDE_CLI] [what ...]
Runs the real CLI against the scripted local Anthropic mock (no key, no network) with an
isolated HOME / CLAUDE_CONFIG_DIR. Prints every event the CLI emits for each step, trimmed.
"""
import json, os, select, socket, subprocess, sys, tempfile, time
from pathlib import Path

CLAUDE = sys.argv[1] if len(sys.argv) > 1 else "claude"
HERE = Path(__file__).resolve().parent
MOCK = HERE.parent / "projects/showcase/e2e-demo/mock_anthropic.py"


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class Proc:
    def __init__(self, env, cwd, extra=()):
        self.p = subprocess.Popen(
            [CLAUDE, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
             "--permission-prompt-tool", "stdio", "--permission-mode", "default", *extra],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env, cwd=cwd)
        self.n = 0

    def write(self, obj):
        self.p.stdin.write(json.dumps(obj) + "\n")
        self.p.stdin.flush()

    def read(self, timeout):
        r, _, _ = select.select([self.p.stdout], [], [], timeout)
        if not r:
            return None
        line = self.p.stdout.readline()
        if not line:
            return {"type": "EOF"}
        if os.environ.get("RAW_OUT"):
            with open(os.environ["RAW_OUT"], "a") as f:
                f.write(line if line.endswith("\n") else line + "\n")
        try:
            return json.loads(line)
        except ValueError:
            return {"type": "nonjson", "raw": line[:200]}

    def drain(self, quiet=3, stop_on_result=True, limit=40):
        """Collect events until a result or `quiet` seconds of silence."""
        out, deadline = [], time.time() + limit
        while time.time() < deadline:
            ev = self.read(quiet)
            if ev is None:
                break
            out.append(ev)
            if ev.get("type") == "EOF":
                break
            if stop_on_result and ev.get("type") == "result":
                # take anything immediately following
                while True:
                    nxt = self.read(1)
                    if nxt is None:
                        break
                    out.append(nxt)
                break
        return out

    def control(self, request, quiet=15):
        self.n += 1
        rid = f"c{self.n}"
        self.write({"type": "control_request", "request_id": rid, "request": request})
        deadline = time.time() + quiet
        while time.time() < deadline:
            ev = self.read(1)
            if ev and ev.get("type") == "control_response" and ev["response"].get("request_id") == rid:
                return ev["response"]
        return {"error": "no answer"}

    def user(self, text):
        self.write({"type": "user", "message": {"role": "user", "content": text}})


def brief(ev, width=int(os.environ.get("W", "260"))):
    s = json.dumps(ev, ensure_ascii=False)
    return s if len(s) <= width else s[:width] + f"…(+{len(s)-width})"


def digest(ev):
    t = ev.get("type")
    if t in ("assistant", "user"):
        c = ev["message"]["content"]
        c = c if isinstance(c, str) else " ".join(b.get("text", b.get("type", "")) for b in c)
        extra = {k: ev[k] for k in ("isReplay", "isSynthetic") if k in ev}
        return f"{t} {extra or ''} {c[:int(os.environ.get('TXT', '300'))]!r}"
    if t == "result":
        return f"result subtype={ev.get('subtype')} is_error={ev.get('is_error')} num_turns={ev.get('num_turns')} result={str(ev.get('result'))[:200]!r}"
    if t == "system":
        keep = {k: v for k, v in ev.items() if k not in ("type", "session_id", "uuid", "tools", "slash_commands", "agents", "skills", "plugins", "mcp_servers", "cwd")}
        return "system " + brief(keep, 400)
    return brief(ev, 300)


def show(title, events):
    print(f"\n=== {title}")
    for ev in events:
        print("  <-", digest(ev) if os.environ.get("SUMMARY") else brief(ev))


def main():
    steps = sys.argv[2:] or ["all"]
    with tempfile.TemporaryDirectory(prefix="slash-probe-", dir=os.environ.get("TMPDIR")) as tmp:
        tmp = Path(tmp)
        (tmp / "home").mkdir()
        cwd = tmp / "work"
        cwd.mkdir()
        port = free_port()
        mock_path = MOCK
        if os.environ.get("RUN_MOCK"):  # model calls Bash when a user message says "@@RUN cmd@@"
            base = MOCK.read_text()
            a, b = "def " + "pick(req):", "class " + "H("
            pick = '''def pick(req):
    msgs, idx, cmd = req["messages"], -1, None
    for i, msg in enumerate(msgs):
        if msg["role"] != "user":
            continue
        for m in re.finditer(r"@@RUN (.+?)@@", text_of(msg["content"])):
            idx, cmd = i, m.group(1).strip()
    if cmd is None:
        return "t", [("text", "ok")]
    if any(tool_results(m) for m in msgs[idx + 1:] if m["role"] == "user"):
        return "t", [("text", "done")]
    return "t", [("tool", "Bash", {"command": cmd, "description": "probe"})]

'''
            mock_path = tmp / "mock.py"
            mock_path.write_text(base[:base.index(a)] + pick + base[base.index(b):])
        mock = subprocess.Popen([sys.executable, str(mock_path), str(port), str(tmp / "mock.log")])
        env = {**os.environ, "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}", "ANTHROPIC_API_KEY": "probe",
               "CLAUDE_CONFIG_DIR": str(tmp / "config"), "HOME": str(tmp / "home"), "NO_PROXY": "127.0.0.1",
               "no_proxy": "127.0.0.1", "DISABLE_TELEMETRY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
        proc = Proc(env, str(cwd), tuple(os.environ.get("CLI_ARGS", "").split()))
        try:
            time.sleep(2)
            exec(open(HERE / "slash-command-probe-steps.py").read(), {"proc": proc, "show": show, "brief": brief,
                                                                        "steps": steps, "tmp": tmp, "json": json, "time": time})
        finally:
            proc.p.kill()
            err = proc.p.stderr.read()[:1500]
            if err.strip():
                print("\n--- stderr:", err)
            mock.kill()


main()
