"""Be a provider tb can call.

`Server` speaks just enough of both tb-supported protocols — OpenAI
(`GET /v1/models`, `POST /v1/chat/completions`) and Anthropic
(`POST /v1/messages`) — no streaming — to be registered in tb as a
self-hosted **dual** provider: one process, both URLs, the same mechanism
Connect AI's "Dual endpoint" card uses for any other provider (no key
required).

Every request, on either endpoint, is reduced to one `ChatRequest` and
handed to the single registered handler — a handler is written once and
never branches on which wire protocol the caller used. `/v1/messages`
requests are always handled as if beta (ignoring `anthropic-version` /
`?beta=true` entirely), the same simplification tb's own vmodel virtual
server already makes at its HTTP boundary.

The handler answers by returning either a plain string, or a dict that is
already an OpenAI-shaped chat completion (e.g. straight from
`srv.tb.chat(...)` — `.tb` always speaks OpenAI to tb, regardless of which
protocol the inbound request arrived on). A dict answering an
`/v1/messages` request is reduced back to text and re-wrapped as an
Anthropic message — v1 handlers work with text, not content blocks, on
either side.
"""

from __future__ import annotations

import json
import os
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Callable

from .client import Client, DEFAULT_SCENARIO, text_of
from .types import ChatRequest

Handler = Callable[[ChatRequest], "str | dict"]


class Server:
    """A single-model dual-protocol (OpenAI + Anthropic) HTTP server.

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
            path = self.path.rstrip("/")
            if path == "/v1/chat/completions":
                wire = "openai"
            elif path == "/v1/messages":
                wire = "anthropic"
            else:
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

            if wire == "openai":
                payload = result if isinstance(result, dict) else _wrap_text_openai(req.model, result)
            else:
                text = result if isinstance(result, str) else text_of(result)
                payload = _wrap_text_anthropic(req.model, text)
            self._json(200, payload)

        def _json(self, status: int, payload: dict):
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return RequestHandler


def _wrap_text_openai(model: str, text: str) -> dict:
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


def _wrap_text_anthropic(model: str, text: str) -> dict:
    """Wrap a plain string handler reply into a minimal Anthropic Messages
    envelope. Treated as beta-shaped unconditionally (see module docstring)."""
    return {
        "id": f"msg_{uuid.uuid4().hex[:24]}",
        "type": "message",
        "role": "assistant",
        "model": model,
        "content": [{"type": "text", "text": text}],
        "stop_reason": "end_turn",
        "stop_sequence": None,
        "usage": {"input_tokens": 0, "output_tokens": len(text.split())},
    }
