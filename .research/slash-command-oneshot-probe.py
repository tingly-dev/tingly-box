"""One-shot (`claude -p PROMPT --resume ID`) counterpart of slash-command-probe.py.
Usage: python3 slash-command-oneshot-probe.py   (real CLI + scripted mock, isolated HOME under $TMPDIR)"""
import json, os, socket, subprocess, sys, tempfile, time
from pathlib import Path

HERE = Path(__file__).resolve().parent
MOCK = HERE.parent / "projects/showcase/e2e-demo/mock_anthropic.py"
tmpd = Path(tempfile.mkdtemp(prefix="oneshot-", dir=os.environ.get("TMPDIR")))
s = socket.socket(); s.bind(("127.0.0.1", 0)); port = s.getsockname()[1]; s.close()
mock = subprocess.Popen([sys.executable, str(MOCK), str(port), str(tmpd / "mock.log")])
time.sleep(1)
env = {**os.environ, "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}", "ANTHROPIC_API_KEY": "probe",
       "CLAUDE_CONFIG_DIR": str(tmpd / "config"), "HOME": str(tmpd), "NO_PROXY": "127.0.0.1",
       "DISABLE_TELEMETRY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
work = tmpd / "work"; work.mkdir()


def run(*args):
    r = subprocess.run(["claude", "-p", "--output-format", "stream-json", "--verbose", *args],
                       env=env, cwd=work, capture_output=True, text=True, timeout=60)
    return [json.loads(l) for l in r.stdout.splitlines() if l.startswith("{")]


def sm(evs):
    out = []
    for e in evs:
        t = e.get("type")
        if t == "system" and e.get("subtype") == "init": out.append(("init", e["session_id"][:8]))
        elif t == "system": out.append((e.get("subtype"), e.get("status"), e.get("compact_result")))
        elif t == "assistant": out.append(("assistant", str(e["message"]["content"][0].get("text"))[:80]))
        elif t == "result": out.append(("result", e.get("num_turns"), str(e.get("result"))[:80]))
        elif t == "user": out.append(("user", str(e["message"]["content"])[:60]))
        else: out.append((t,))
    return out


try:
    e1 = run("hello one"); sid = [e for e in e1 if e.get("type") == "result"][0]["session_id"]
    print("turn1", sm(e1)); print("turn2", sm(run("hello two", "--resume", sid)))
    for cmd in ("/cost", "/compact", "/clear"):
        print(f"one-shot {cmd} on resume:", sm(run(cmd, "--resume", sid)))
finally:
    mock.kill()
