"""Scripted virtual models from Python: write an interaction, run it against a
real tingly-box, with nothing to set up by hand.

tb serves every `*.yaml` in `<config-dir>/vmodels/` as a model that walks a
*script* — one outcome per request: text, a tool call, an error status, a
stream cut off part-way (`.design/vmodel-script.md`). This module is the
Python way to write that file and the tb to serve it:

    from tingly import vmodel

    flow = (vmodel.Script("read-edit")
            .tool("Read", {"file_path": "/tmp/a.go"}, say="Let me look.")
            .error(529)
            .say("Done."))

    with vmodel.Testbed(flow) as tb:          # starts tb, writes the script
        reply = tb.messages(flow.model, "fix it")      # Anthropic wire
        ...                                            # or tb.anthropic_base, tb.token

`Script` only builds the schema tb already defines — it adds no behaviour, and
a script written here is byte-for-byte usable from a hand-written file.
`Testbed` is the other half: `Testbed(...)` launches a throwaway tb;
`Testbed.attach(...)` writes into the config dir of one that is already
running. Both need only the standard library.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

from .client import TinglyError

SCRIPT_DIR = "vmodels"
DEFAULT_BASE_URL = "http://localhost:12580"
_MIDSTREAM_MODES = ("close", "event", "eof")


class ScriptError(RuntimeError):
    """tb refused to load a script; the message is tb's own explanation."""


class Script:
    """A scripted model: an ordered program of per-request outcomes.

    Every step method returns the script, so a program reads top to bottom.
    Extra keyword options on any step: `usage` (a dict of `input`, `output`,
    `cache_read`, `cache_write`, `reasoning`), `stop_reason`, `repeat`.

    Args:
        id: the model name clients request. Letters, digits, `.`, `_`, `-`.
        on_exhaust: what happens after the last step — `"loop"` (default),
            `"clamp"` (repeat the last step) or `"fail"` (410 forever).
        description / default_content / delay: as in the YAML schema.
    """

    def __init__(self, id: str, *, on_exhaust: str | None = None, description: str | None = None,
                 default_content: str | None = None, delay: str | None = None):
        self.id = id
        self._head = {k: v for k, v in (
            ("id", id), ("description", description), ("default_content", default_content),
            ("delay", delay), ("on_exhaust", on_exhaust)) if v is not None}
        self._steps: list[int | dict] = []

    @property
    def model(self) -> str:
        """The model name to request."""
        return self.id

    def say(self, text: str, **opts) -> "Script":
        """A plain answer."""
        return self._add({"say": text}, opts)

    def tool(self, name: str, arguments: dict | None = None, *, say: str | None = None,
             tool_id: str | None = None, **opts) -> "Script":
        """Answer with one tool call, optionally after some text."""
        call = {"name": name, "arguments": arguments or {}}
        if tool_id:
            call["id"] = tool_id
        step = {"say": say} if say is not None else {}
        step["tool"] = call
        return self._add(step, opts)

    def error(self, status: int, *, message: str | None = None, error_type: str | None = None,
              **opts) -> "Script":
        """Fail the request with an HTTP error status (400-599)."""
        step: dict = {"status": status}
        if message is not None:
            step["error_message"] = message
        if error_type is not None:
            step["error_type"] = error_type
        return self._add(step, opts)

    def cut(self, mode: str = "close", *, after: int = 1, say: str | None = None, **opts) -> "Script":
        """A stream that dies part-way: `close` drops the connection, `event`
        sends an in-band error, `eof` just ends. `after` events are delivered
        first."""
        if mode not in _MIDSTREAM_MODES:
            raise ValueError(f"cut mode must be one of {_MIDSTREAM_MODES}, not {mode!r}")
        step: dict = {"midstream": {"mode": mode, "after_events": after}}
        if say is not None:
            step["say"] = say
        return self._add(step, opts)

    def _add(self, step: dict, opts: dict) -> "Script":
        step.update({k: v for k, v in opts.items() if v is not None})
        self._steps.append(step)
        return self

    def to_dict(self) -> dict:
        return {**self._head, "steps": list(self._steps)}

    def dumps(self) -> str:
        """The script as text. JSON, which is valid YAML — so the file is
        loaded by tb's YAML parser with no YAML dependency here."""
        return json.dumps(self.to_dict(), indent=2, ensure_ascii=False) + "\n"

    def write(self, config_dir: str) -> str:
        """Write `<config_dir>/vmodels/<id>.yaml`, atomically (tb never sees a
        half-written file), and return its path."""
        return _write_text(config_dir, self.id, self.dumps())

    def remove(self, config_dir: str) -> None:
        _unlink(_script_path(config_dir, self.id))


def _script_path(config_dir: str, name: str) -> str:
    return os.path.join(config_dir, SCRIPT_DIR, name + ".yaml")


def _unlink(path: str) -> None:
    try:
        os.remove(path)
    except FileNotFoundError:
        pass


def _declared_id(text: str) -> str | None:
    """The `id:` a hand-written script declares, or None (tb then uses the file
    name). Reads a JSON document or a top-level YAML `id:` line — enough to
    know the model name without a YAML parser."""
    try:
        doc = json.loads(text)
        return doc.get("id") if isinstance(doc, dict) else None
    except ValueError:
        match = re.search(r"(?m)^id:\s*['\"]?([A-Za-z0-9._-]+)", text)
        return match.group(1) if match else None


def _write_text(config_dir: str, name: str, text: str) -> str:
    path = _script_path(config_dir, name)
    directory = os.path.dirname(path)
    os.makedirs(directory, exist_ok=True)
    tmp = os.path.join(directory, f".{name}.tmp")  # dot-prefixed: tb ignores it
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(text)
    os.replace(tmp, path)
    return path


class Testbed:
    """A tb with your scripts loaded, ready to call.

    `Testbed(script, ...)` starts a throwaway tb (own config dir, free port) and
    stops it on exit. `Testbed.attach(...)` instead writes into a tb that is
    already running and leaves it alone on exit — the day-to-day mode when tb
    is open anyway.

    Attributes: `base_url`, `token` (the model token), `anthropic_base` (give
    it to an Anthropic SDK as `base_url`), `openai_base` (OpenAI SDK
    `base_url`), `config_dir`, `log_path` (spawned tb's output).

    Args:
        *scripts: `Script` objects, or paths to hand-written `.yaml` scripts.
        tb_bin: the tb binary. Default: `$TINGLY_TB_BIN`, else `tingly-box` or
            `tb` on PATH.
        keep: leave the config dir behind (for inspecting tb's log).
        startup_timeout: seconds to wait for tb to come up.
    """

    def __init__(self, *scripts: "Script | str", tb_bin: str | None = None, keep: bool = False,
                 startup_timeout: float = 60.0):
        self._initial = scripts
        self._tb_bin = tb_bin
        self._keep = keep
        self._startup_timeout = startup_timeout
        self._written: list[str] = []  # script files this Testbed wrote; removed on exit when attached
        self._proc: subprocess.Popen | None = None
        self._log = None
        self._owns_config_dir = False
        self._attached = False
        self.config_dir = ""
        self.base_url = ""
        self.token = ""
        self.log_path: str | None = None

    @classmethod
    def attach(cls, *scripts: "Script | str", config_dir: str | None = None,
               base_url: str | None = None) -> "Testbed":
        """Use a tb that is already running. `config_dir` defaults to
        `$TINGLY_CONFIG_DIR` then `~/.tingly-box`; `base_url` to
        `$TINGLY_BASE_URL` then `http://localhost:12580`. Scripts you add are
        removed again on exit."""
        tb = cls(*scripts)
        tb._attached = True
        tb.config_dir = os.path.expanduser(
            config_dir or os.environ.get("TINGLY_CONFIG_DIR") or "~/.tingly-box")
        tb.base_url = (base_url or os.environ.get("TINGLY_BASE_URL") or DEFAULT_BASE_URL).rstrip("/")
        return tb

    # -- lifecycle -----------------------------------------------------------

    def __enter__(self) -> "Testbed":
        self.start()
        return self

    def __exit__(self, *exc) -> None:
        self.stop()

    def start(self) -> "Testbed":
        try:
            if self._attached:
                self.token = _read_config(self.config_dir).get("model_token", "")
                if not self.token:
                    raise RuntimeError(f"no model_token in {self.config_dir}/config.json — is tb set up there?")
            else:
                self._spawn()
            for script in self._initial:
                self.add(script)
        except BaseException:
            self.stop()
            raise
        return self

    def stop(self) -> None:
        if self._attached:
            for path in self._written:
                _unlink(path)
        self._written.clear()
        if self._proc is not None:
            self._proc.terminate()
            try:
                self._proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self._proc.kill()
            self._proc = None
        if self._log is not None:
            self._log.close()
            self._log = None
        if self._owns_config_dir and not self._keep:
            shutil.rmtree(self.config_dir, ignore_errors=True)
            self._owns_config_dir = False

    def _spawn(self) -> None:
        binary = self._tb_bin or os.environ.get("TINGLY_TB_BIN") or shutil.which("tingly-box") or shutil.which("tb")
        if not binary:
            raise RuntimeError(
                "cannot find the tb binary: set TINGLY_TB_BIN, pass tb_bin=..., or put `tingly-box` on PATH "
                "(`go build -o tb ./cli/tingly-box` from the tingly-box repo builds one)")
        self.config_dir = tempfile.mkdtemp(prefix="tingly-testbed-")
        self._owns_config_dir = True
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        self.base_url = f"http://127.0.0.1:{port}"
        self.log_path = os.path.join(self.config_dir, "tb-stdout.log")
        self._log = open(self.log_path, "wb")
        self._proc = subprocess.Popen(
            [binary, "--config-dir", self.config_dir, "start", "--no-daemon",
             "--port", str(port), "--host", "127.0.0.1", "--browser=false"],
            stdout=self._log, stderr=subprocess.STDOUT)
        deadline = time.time() + self._startup_timeout
        while True:
            if self._proc.poll() is not None:
                raise RuntimeError(f"tb exited with {self._proc.returncode}; see {self.log_path}")
            if time.time() > deadline:
                raise TimeoutError(f"tb did not come up within {self._startup_timeout:.0f}s; see {self.log_path}")
            try:
                token = _read_config(self.config_dir).get("model_token")
                with socket.create_connection(("127.0.0.1", port), timeout=1):
                    pass
                if token:
                    self.token = token
                    return
            except OSError:
                pass
            time.sleep(0.2)

    # -- scripts ---------------------------------------------------------------

    def add(self, script: "Script | str") -> str:
        """Load a script (a `Script`, or the path of a `.yaml` file) into tb and
        return its model name (a file's `id:`, else its file name). Raises
        `ScriptError` with tb's own message if tb rejects it. Adding the same id
        again replaces it and restarts its program. Attached to a running tb it
        never overwrites a script file it did not write itself."""
        if isinstance(script, Script):
            name, text, model = script.id, script.dumps(), script.id
        else:
            with open(script, encoding="utf-8") as f:
                text = f.read()
            name = os.path.splitext(os.path.basename(script))[0]
            model = _declared_id(text) or name
        path = _script_path(self.config_dir, name)
        if self._attached and os.path.exists(path) and path not in self._written:
            raise ScriptError(f"{path} already exists and was not written by this Testbed; use another id")
        if path not in self._written and model in self._models():
            # tb keeps a built-in (or another file's) model and ignores the
            # script, so a listing alone could not tell the two apart later.
            raise ScriptError(f"{model!r} is already served by this tb (a built-in model or another script); use another id")
        _write_text(self.config_dir, name, text)
        if path not in self._written:
            self._written.append(path)
        self._await_loaded(model)
        return model

    def remove(self, script: "Script | str") -> None:
        """Take a script back out of tb."""
        name = script.id if isinstance(script, Script) else os.path.splitext(os.path.basename(script))[0]
        path = _script_path(self.config_dir, name)
        _unlink(path)
        if path in self._written:
            self._written.remove(path)

    def _await_loaded(self, model: str) -> None:
        # tb re-reads the directory on every vmodel request, so one listing is
        # enough to know whether the file loaded; a miss is explained by the
        # 404 text of a probe request.
        if model in self._models():
            return
        try:
            self._call("/virtual/openai/v1/chat/completions", {"model": model, "messages": []})
        except TinglyError as exc:
            raise ScriptError(f"script {model!r} was not loaded: {_error_message(exc)}") from exc
        raise ScriptError(f"script {model!r} was not loaded")

    def _models(self) -> set[str]:
        out = self._call("/virtual/openai/v1/models", None)
        return {m["id"] for m in out.get("data", [])}

    # -- calling ---------------------------------------------------------------

    @property
    def anthropic_base(self) -> str:
        return f"{self.base_url}/virtual/anthropic"

    @property
    def openai_base(self) -> str:
        return f"{self.base_url}/virtual/openai/v1"

    def messages(self, model: str, content: "str | list", **body) -> dict:
        """One Anthropic `/v1/messages` request (non-streaming); `content` is a
        user text or a full `messages` list. Returns the parsed response, or
        raises `TinglyError` carrying the HTTP status for an error step."""
        messages = [{"role": "user", "content": content}] if isinstance(content, str) else content
        return self._call("/virtual/anthropic/v1/messages",
                          {"model": model, "max_tokens": 1024, "messages": messages, **body})

    def chat(self, model: str, content: "str | list", **body) -> dict:
        """One OpenAI `/chat/completions` request, as `messages()`."""
        messages = [{"role": "user", "content": content}] if isinstance(content, str) else content
        return self._call("/virtual/openai/v1/chat/completions", {"model": model, "messages": messages, **body})

    def _call(self, path: str, body: dict | None) -> dict:
        request = urllib.request.Request(
            self.base_url + path, method="POST" if body is not None else "GET",
            data=json.dumps(body).encode() if body is not None else None,
            headers={"Content-Type": "application/json", "Authorization": f"Bearer {self.token}",
                     "x-api-key": self.token, "anthropic-version": "2023-06-01"})
        try:
            with urllib.request.urlopen(request, timeout=60) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as exc:
            raise TinglyError(exc.code, exc.read().decode("utf-8", "replace")) from exc


def _read_config(config_dir: str) -> dict:
    try:
        with open(os.path.join(config_dir, "config.json"), encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def _error_message(exc: TinglyError) -> str:
    try:
        err = json.loads(exc.body).get("error", {})
        return err.get("message") or exc.body
    except ValueError:
        return exc.body
