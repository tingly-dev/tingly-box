"""Exercise the real Claude Code coordinator against a scripted local model.

Usage: python3 coordinator_session_probe.py CLAUDE_CLI TINGLY_PROJECT
The probe uses a temporary data directory and never needs an upstream API key.
"""
import json
import os
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def request(url, method="GET", body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=5) as response:
        return json.load(response)


def main():
    claude, app = map(os.path.abspath, sys.argv[1:3])
    with tempfile.TemporaryDirectory(prefix="projects-coordinator-probe-") as temp:
        temp = Path(temp)
        mock_source = (Path(__file__).parent / "mock_anthropic.py").read_text()
        start, end = mock_source.index("def pick(req):"), mock_source.index("class H(")
        mock_source = mock_source[:start] + '''def pick(req):
    tools = [t.get("name", "") for t in req.get("tools") or []]
    LOG.write("tools=" + json.dumps(tools) + "\\n"); LOG.flush()
    msgs = req["messages"]
    last = msgs[-1]
    if tool_results(last):
        results = tool_results(last)
        if any(result.get("is_error") for result in results) or not any("Project memory" in text_of(result.get("content", "")) for result in results):
            return "coordinator", [("text", "SHARED FILE READ FAILED")]
        return "coordinator", [("text", "First turn completed.")]
    text = text_of(last["content"])
    if "second probe" in text:
        prior = any("First turn completed." in text_of(m.get("content", "")) for m in msgs)
        return "coordinator", [("text", "Second turn remembers first." if prior else "HISTORY MISSING")]
    name = next((name for name in tools if name.endswith("__list_threads")), None)
    read = next((name for name in tools if name.endswith("__read_shared_file")), None)
    if not name or not read:
        return "coordinator", [("text", "PROJECT TOOLS MISSING")]
    return "coordinator", [("tool", name, {}), ("tool", read, {"path": "memory/MEMORY.md"})]

''' + mock_source[end:]
        mock = temp / "mock.py"
        mock.write_text(mock_source)
        model_port, server_port = port(), port()
        log = temp / "mock.log"
        env = os.environ.copy()
        env.update({"CLAUDE_CLI_PATH": claude, "CLAUDE_CONFIG_DIR": str(temp / "claude-config"), "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost"})
        with (temp / "server.log").open("w+") as server_log:
            model = subprocess.Popen([sys.executable, str(mock), str(model_port), str(log)])
            server = subprocess.Popen([app, "--port", str(server_port), "--data-dir", str(temp / "data")], env=env, stdout=server_log, stderr=subprocess.STDOUT)
            base = f"http://127.0.0.1:{server_port}/api/v1"
            try:
                for _ in range(100):
                    try:
                        request(base + "/health")
                        break
                    except Exception:
                        time.sleep(.1)
                request(base + "/settings", "PUT", {"api_style": "anthropic", "base_url": f"http://127.0.0.1:{model_port}", "api_key": "test-key", "default_model": "claude-sonnet-4-6"})
                project = request(base + "/projects", "POST", {"name": "Probe"})
                thread = project["main_thread_id"]
                for prompt, expected in [("first probe", "First turn completed."), ("second probe", "Second turn remembers first.")]:
                    request(base + f"/threads/{thread}/messages", "POST", {"content": prompt})
                    for _ in range(300):
                        events = request(base + f"/threads/{thread}/events?limit=200")["events"]
                        if any(expected in event.get("content", "") for event in events):
                            break
                        errors = [e.get("content") for e in events if e.get("kind") == "error"]
                        if errors:
                            raise RuntimeError(str(errors))
                        time.sleep(.1)
                    else:
                        raise RuntimeError(f"missing {expected}: {events}")
                print("real Claude Code: MCP tool call and resumed conversation passed")
            except Exception:
                server_log.flush()
                print("server log:", (temp / "server.log").read_text()[-6000:], file=sys.stderr)
                if log.exists():
                    print("model log:", log.read_text()[-6000:], file=sys.stderr)
                raise
            finally:
                server.terminate()
                model.terminate()
                server.wait(timeout=10)
                model.wait(timeout=10)


if __name__ == "__main__":
    main()
