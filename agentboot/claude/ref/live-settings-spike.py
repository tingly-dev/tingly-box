"""Does a permission rule pushed with apply_flag_settings take effect in a running process?

Usage: python3 live-settings-spike.py [CLAUDE_CLI]
Runs the real Claude Code CLI against a scripted local Anthropic server (no key,
no network), in a temporary config directory. A user message "@@RUN CMD@@"
makes the script call the Bash tool with CMD.

Checks, all on ONE process:
  1. the --settings file is read once at start (editing it later changes nothing);
  2. a Bash call with no rule prompts (can_use_tool);
  3. apply_flag_settings adds an allow rule for that exact command; the same call
     then runs with no prompt, and a different command still prompts;
  4. apply_flag_settings with an empty permissions object clears the live rules.
Exit status 0 means every check passed.
"""
import json, os, re, select, socket, subprocess, sys, tempfile, time
from pathlib import Path

CLAUDE = sys.argv[1] if len(sys.argv) > 1 else "claude"
HERE = Path(__file__).resolve().parent
MOCK_BASE = (HERE.parents[2] / "projects/showcase/e2e-demo/mock_anthropic.py").read_text()
PICK_START, PICK_END = "def " + "pick(req):", "class " + "H("

# The CLI may append messages of its own after ours, so the script looks back
# for the latest marker and checks whether a tool result has come in since.
PICK = '''def pick(req):
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
MOCK = MOCK_BASE[:MOCK_BASE.index(PICK_START)] + PICK + MOCK_BASE[MOCK_BASE.index(PICK_END):]


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class Proc:
    def __init__(self, env, settings, cwd):
        self.p = subprocess.Popen(
            [CLAUDE, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
             "--permission-prompt-tool", "stdio", "--permission-mode", "default", "--settings", settings],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, env=env, cwd=cwd)
        self.n = 0

    def write(self, obj):
        self.p.stdin.write(json.dumps(obj) + "\n")
        self.p.stdin.flush()

    def read(self, timeout):
        r, _, _ = select.select([self.p.stdout], [], [], timeout)
        if not r:
            return None
        line = self.p.stdout.readline()
        try:
            return json.loads(line) if line else None
        except ValueError:
            return {}

    def control(self, request):
        self.n += 1
        rid = f"c{self.n}"
        self.write({"type": "control_request", "request_id": rid, "request": request})
        deadline = time.time() + 20
        while time.time() < deadline:
            ev = self.read(1)
            if ev and ev.get("type") == "control_response" and ev["response"].get("request_id") == rid:
                return ev["response"]
        raise SystemExit(f"no answer to {request}")

    def turn(self, command):
        """Runs one scripted turn, denying any permission prompt; returns whether the CLI asked."""
        self.write({"type": "user", "message": {"role": "user", "content": f"@@RUN {command}@@"}})
        asked = False
        deadline = time.time() + 40
        while time.time() < deadline:
            ev = self.read(2)
            if not ev:
                continue
            if os.environ.get("SPIKE_DEBUG"):
                print("  <-", json.dumps(ev)[:200])
            if ev.get("type") == "control_request" and ev["request"].get("subtype") == "can_use_tool":
                asked = True
                self.write({"type": "control_response", "response": {
                    "subtype": "success", "request_id": ev["request_id"],
                    "response": {"behavior": "deny", "message": "probe denies"}}})
            if ev.get("type") == "result":
                return asked
        raise SystemExit("turn did not finish")


def main():
    fails = []

    def check(name, ok):
        print(("PASS " if ok else "FAIL ") + name)
        if not ok:
            fails.append(name)

    with tempfile.TemporaryDirectory(prefix="live-settings-") as tmp:
        tmp = Path(tmp)
        (tmp / "mock.py").write_text(MOCK)
        (tmp / "home").mkdir()
        cwd = tmp / "work"
        cwd.mkdir()
        settings = tmp / "settings.json"
        settings.write_text(json.dumps({"permissions": {"allow": ["Bash(echo from-file)"], "ask": ["AskUserQuestion"]}}))
        port = free_port()
        mock = subprocess.Popen([sys.executable, str(tmp / "mock.py"), str(port), str(tmp / "mock.log")])
        env = {**os.environ, "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}", "ANTHROPIC_API_KEY": "probe",
               "CLAUDE_CONFIG_DIR": str(tmp / "config"), "HOME": str(tmp / "home"), "NO_PROXY": "127.0.0.1", "no_proxy": "127.0.0.1",
               "DISABLE_TELEMETRY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
        proc = Proc(env, str(settings), str(cwd))
        try:
            time.sleep(2)
            perm = lambda k: proc.control({"subtype": "get_settings"})["response"]["effective"].get("permissions", {}).get(k)
            allow = lambda: perm("allow")

            check("file rule is loaded at start", allow() == ["Bash(echo from-file)"])
            settings.write_text(json.dumps({"permissions": {"allow": ["Bash(echo from-file)", "Bash(edited on disk)"]}}))
            time.sleep(3)
            check("editing the --settings file does not reach the running process", allow() == ["Bash(echo from-file)"])

            check("a Bash call with no rule prompts", proc.turn("touch a.txt") is True)
            check("a denied call did not run", not (cwd / "a.txt").exists())

            r = proc.control({"subtype": "apply_flag_settings", "settings": {"permissions": {"allow": ["Bash(touch a.txt)"]}}})
            check("apply_flag_settings is accepted", r.get("subtype") == "success")
            check("the live rule is added to the file's rules", allow() == ["Bash(echo from-file)", "Bash(touch a.txt)"])
            check("start-up rules of other kinds survive a push that does not mention them", perm("ask") == ["AskUserQuestion"])

            check("the allowed command now runs with no prompt", proc.turn("touch a.txt") is False)
            check("and it ran", (cwd / "a.txt").exists())
            check("a different command still prompts", proc.turn("touch b.txt") is True)

            proc.control({"subtype": "apply_flag_settings", "settings": {"permissions": {}}})
            check("an empty permissions object clears the live rules", allow() == ["Bash(echo from-file)"] and perm("ask") == ["AskUserQuestion"])
            check("the cleared rule prompts again", proc.turn("touch a.txt") is True)
        finally:
            proc.p.kill()
            mock.kill()
    print("ok" if not fails else "FAILED: " + "; ".join(fails))
    return 1 if fails else 0


sys.exit(main())
