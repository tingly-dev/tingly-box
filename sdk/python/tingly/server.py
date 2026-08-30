"""Be a provider tb can call.

`Server` speaks just enough OpenAI-compatible protocol
(`GET /v1/models`, `POST /v1/chat/completions`, no streaming) to be
registered in tb as a self-hosted provider — the same mechanism as Ollama
or vLLM (Connect AI -> Self-hosted -> Custom endpoint, no key required).

The handler you register does whatever it wants (pure relay, fan-out,
lookups) and answers by returning either a plain string, or a dict that is
already an OpenAI-shaped chat completion (e.g. straight from
`srv.tb.chat(...)`).
"""

from __future__ import annotations

import json
import os
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Callable

from .client import Client, DEFAULT_SCENARIO
from .types import ChatRequest

Handler = Callable[[ChatRequest], "str | dict"]


class Server:
    """A single-model OpenAI-compatible HTTP server.

    Args:
        name: the model id this server advertises via `GET /v1/models`.
        tb_base_url: address of the tb instance to call back into, via
            `.tb`. Falls back to the `TINGLY_BASE_URL` env var. Optional —
            a pure standalone provider never needs `.tb`.
        tb_token: gateway token for `.tb`. Falls back to `TINGLY_TOKEN`.
        tb_scenario: default scenario `.tb.chat()` targets.
    """

    def __init__(
        self,
        name: str,
        tb_base_url: str | None = None,
        tb_token: str | None = None,
        tb_scenario: str = DEFAULT_SCENARIO,
    ):
        self.name = name
        self._handler: Handler | None = None
        self._httpd: ThreadingHTTPServer | None = None

        base_url = tb_base_url or os.environ.get("TINGLY_BASE_URL")
        token = tb_token or os.environ.get("TINGLY_TOKEN")
        self.tb = Client(base_url, token, scenario=tb_scenario) if base_url else None

    def chat(self, fn: Handler) -> Handler:
        """Decorator registering the handler called for every chat request."""
        self._handler = fn
        return fn

    def run(self, host: str = "0.0.0.0", port: int = 8765):
        if self._handler is None:
            raise RuntimeError("no handler registered — use @srv.chat before srv.run()")

        self._httpd = ThreadingHTTPServer((host, port), _make_request_handler(self))
        bound_port = self._httpd.server_address[1]
        print(f"tingly.Server '{self.name}' listening on http://{host}:{bound_port}/v1")
        try:
            self._httpd.serve_forever()
        except KeyboardInterrupt:
            self._httpd.shutdown()


def _make_request_handler(srv: Server):
    class RequestHandler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):  # quieter default logging
            pass

        def do_GET(self):
            if self.path.rstrip("/") == "/v1/models":
                self._json(200, {
                    "object": "list",
                    "data": [{"id": srv.name, "object": "model", "owned_by": "tingly-sdk"}],
                })
            else:
                self._json(404, {"error": "not found"})

        def do_POST(self):
            if self.path.rstrip("/") != "/v1/chat/completions":
                self._json(404, {"error": "not found"})
                return

            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length) or b"{}")
            req = ChatRequest.from_body(body)

            try:
                result = srv._handler(req)
            except Exception as exc:  # surfaced to the caller, not a 500 traceback
                self._json(500, {"error": str(exc)})
                return

            self._json(200, result if isinstance(result, dict) else _wrap_text(req.model, result))

        def _json(self, status: int, payload: dict):
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return RequestHandler


def _wrap_text(model: str, text: str) -> dict:
    """Wrap a plain string handler reply into a minimal one-choice
    OpenAI ChatCompletion envelope."""
    return {
        "id": f"chatcmpl-{uuid.uuid4().hex[:24]}",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": model,
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": text},
            "finish_reason": "stop",
        }],
    }
