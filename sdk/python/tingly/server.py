"""Be a provider tb can call.

`Server` speaks just enough of tb's supported protocols to be registered as
a self-hosted provider — OpenAI (`GET /v1/models`, `POST /v1/chat/completions`)
via `@srv.chat`, and/or Anthropic (`POST /v1/messages`) via `@srv.messages`.
No streaming. Both endpoints also answer without the `/v1` prefix
(`/chat/completions`, `/messages`) — the one bit of path leniency this
prototype bothers with, since which shape a caller's configured base URL
expects is exactly the kind of detail not worth troubleshooting by hand.

This is a prototype, not a protocol-translation layer: the two decorators
are independent, and there is **no bridging between them**. `@srv.chat`
gets an OpenAI-shaped `ChatRequest`; `@srv.messages` gets the raw parsed
Anthropic request body, unmodified — content blocks, `system`, tool defs
and all — because building a shared abstraction that normalizes across
wire protocols is exactly the kind of forward-looking design this v1
explicitly isn't attempting yet. A handler that wants to serve both
protocols registers both decorators and writes native code for each; the
framework does not do it on the handler's behalf. Every `/v1/messages`
(or `/messages`) request is handled as if beta unconditionally — no
`?beta=true` / `anthropic-version` branching — the same simplification
tb's own vmodel virtual server already makes at its HTTP boundary.

Each handler answers by returning either a plain string (wrapped into a
minimal one-choice envelope in that handler's own protocol — a local
convenience, not a cross-protocol conversion) or a dict that is already a
complete response in that same protocol's shape (e.g. straight from
`srv.tb.chat(...)` for `@srv.chat`, since `.tb` always speaks OpenAI).
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

ChatHandler = Callable[[ChatRequest], "str | dict"]
MessagesHandler = Callable[[dict], "str | dict"]


class Server:
    """A single-model HTTP server, OpenAI and/or Anthropic protocol.

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
        self._chat_handler: ChatHandler | None = None
        self._messages_handler: MessagesHandler | None = None
        self._httpd: ThreadingHTTPServer | None = None

        base_url = tb_base_url or os.environ.get("TINGLY_BASE_URL")
        token = tb_token or os.environ.get("TINGLY_TOKEN")
        self.tb = Client(base_url, token, scenario=tb_scenario) if base_url else None

    def chat(self, fn: ChatHandler) -> ChatHandler:
        """Decorator registering the OpenAI-protocol handler
        (`POST /v1/chat/completions`)."""
        self._chat_handler = fn
        return fn

    def messages(self, fn: MessagesHandler) -> MessagesHandler:
        """Decorator registering the Anthropic-protocol handler
        (`POST /v1/messages`). Receives the raw parsed request body as-is —
        no normalization — and returns either a dict already in Anthropic
        Messages shape, or a plain string to wrap minimally."""
        self._messages_handler = fn
        return fn

    def run(self, host: str = "0.0.0.0", port: int = 8765):
        if self._chat_handler is None and self._messages_handler is None:
            raise RuntimeError("no handler registered — use @srv.chat and/or @srv.messages before srv.run()")

        self._httpd = ThreadingHTTPServer((host, port), _make_request_handler(self))
        bound_port = self._httpd.server_address[1]
        print(f"tingly.Server '{self.name}' listening on http://{host}:{bound_port}")
        try:
            self._httpd.serve_forever()
        except KeyboardInterrupt:
            self._httpd.shutdown()


def _strip_v1(path: str) -> str:
    """Accept a request whether or not the caller's base URL already
    included `/v1` — some clients configure it either way."""
    path = path.rstrip("/") or "/"
    if path == "/v1":
        return "/"
    if path.startswith("/v1/"):
        return path[3:]
    return path


def _make_request_handler(srv: Server):
    class RequestHandler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):  # quieter default logging
            pass

        def do_GET(self):
            if _strip_v1(self.path) == "/models":
                self._json(200, {
                    "object": "list",
                    "data": [{"id": srv.name, "object": "model", "owned_by": "tingly-sdk"}],
                })
            else:
                self._json(404, {"error": "not found"})

        def do_POST(self):
            path = _strip_v1(self.path)
            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length) or b"{}")

            if path == "/chat/completions":
                self._handle_chat(body)
            elif path == "/messages":
                self._handle_messages(body)
            else:
                self._json(404, {"error": "not found"})

        def _handle_chat(self, body: dict):
            if srv._chat_handler is None:
                self._json(404, {"error": "no @srv.chat handler registered"})
                return
            req = ChatRequest.from_body(body)
            try:
                result = srv._chat_handler(req)
            except Exception as exc:  # surfaced to the caller, not a 500 traceback
                self._json(500, {"error": str(exc)})
                return
            self._json(200, result if isinstance(result, dict) else _wrap_text_openai(req.model, result))

        def _handle_messages(self, body: dict):
            if srv._messages_handler is None:
                self._json(404, {"error": "no @srv.messages handler registered"})
                return
            try:
                result = srv._messages_handler(body)
            except Exception as exc:
                self._json(500, {"error": str(exc)})
                return
            self._json(200, result if isinstance(result, dict) else _wrap_text_anthropic(body.get("model", ""), result))

        def _json(self, status: int, payload: dict):
            data = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

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
