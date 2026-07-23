"""A tiny HTTP server for plugins (stdlib only), speaking two wire protocols.

Exposes exactly what tingly-box needs to treat the plugin as an upstream:

    POST /v1/messages            -> Anthropic message (+ SSE when stream=true) — primary
    POST /v1/chat/completions    -> OpenAI chat.completion (+ SSE)             — secondary
    GET  /v1/models              -> the plugin's model id
    GET  /health                 -> liveness

Anthropic is primary because that is tingly-box's native protocol; OpenAI
chat completions is kept as a secondary, equally-real path (not a shim) since
many callers still speak it. Both routes share the same normalized
:class:`ChatRequest` and the same handler — the author's code never sees the
wire-format difference, only the response shaping differs per route.

No framework dependency — uses ``http.server.ThreadingHTTPServer`` so a plugin
stays a single ``pip install tingly`` away. Streaming is real SSE: a handler
that returns an iterator is emitted as protocol-appropriate chunk/event frames.
"""

from __future__ import annotations

import json
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, Dict, Iterable, Iterator, Tuple, Union
from urllib.parse import urlsplit

from .types import ChatRequest

# A handler returns buffered text or an iterator of text deltas.
HandlerResult = Union[str, Iterable[str]]
Dispatch = Callable[[ChatRequest], HandlerResult]


class _Handler(BaseHTTPRequestHandler):
    # set by make_server via closure attributes on the server instance
    server_version = "tingly-plugin/0.1"

    def log_message(self, fmt, *args):  # quiet by default; plugin owns logging
        if getattr(self.server, "verbose", False):
            super().log_message(fmt, *args)

    # -- routing ---------------------------------------------------------

    def _route_path(self) -> str:
        """The request path with any query string (e.g. tb's ``?beta=true``
        on Anthropic calls) and trailing slash stripped."""
        return urlsplit(self.path).path.rstrip("/")

    def do_GET(self):
        path = self._route_path()
        if path == "/health":
            return self._json(200, {"status": "ok"})
        if path in ("/v1/models", "/models"):
            return self._models()
        return self._json(404, {"error": {"message": "not found", "type": "not_found"}})

    def do_POST(self):
        path = self._route_path()
        if path in ("/v1/messages", "/messages"):
            return self._handle_chat(anthropic=True)
        if path in ("/v1/chat/completions", "/chat/completions"):
            return self._handle_chat(anthropic=False)
        return self._json(404, {"error": {"message": "not found", "type": "not_found"}})

    def _handle_chat(self, anthropic: bool):
        if not self._authorized():
            if anthropic:
                return self._json(
                    401, {"type": "error", "error": {"type": "authentication_error", "message": "invalid token"}}
                )
            return self._json(
                401, {"error": {"message": "invalid token", "type": "auth_error"}}
            )
        body = self._read_json()
        if body is None:
            if anthropic:
                return self._json(
                    400, {"type": "error", "error": {"type": "invalid_request_error", "message": "invalid JSON body"}}
                )
            return self._json(
                400, {"error": {"message": "invalid JSON body", "type": "invalid_request_error"}}
            )

        req = (
            ChatRequest.from_anthropic_body(body) if anthropic else ChatRequest.from_openai_body(body)
        )
        try:
            result = self.server.dispatch(req)  # type: ignore[attr-defined]
        except Exception as exc:  # noqa: BLE001 - surface as upstream 500
            if anthropic:
                return self._json(
                    500, {"type": "error", "error": {"type": "api_error", "message": f"plugin handler error: {exc}"}}
                )
            return self._json(
                500,
                {"error": {"message": f"plugin handler error: {exc}", "type": "api_error"}},
            )

        model = req.model or self.server.model_id  # type: ignore[attr-defined]
        if anthropic:
            return self._stream_anthropic(result, model) if req.stream else self._complete_anthropic(result, model)
        return self._stream(result, model) if req.stream else self._complete(result, model)

    # -- responses -------------------------------------------------------

    def _models(self):
        model_id = self.server.model_id  # type: ignore[attr-defined]
        self._json(
            200,
            {
                "object": "list",
                "data": [
                    {
                        "id": model_id,
                        "object": "model",
                        "created": int(time.time()),
                        "owned_by": "tingly-plugin",
                    }
                ],
            },
        )

    def _complete(self, result: HandlerResult, model: str):
        text = result if isinstance(result, str) else "".join(result)
        self._json(
            200,
            {
                "id": _chat_id(),
                "object": "chat.completion",
                "created": int(time.time()),
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": text},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
            },
        )

    def _stream(self, result: HandlerResult, model: str):
        cid = _chat_id()
        created = int(time.time())
        # Close-delimited SSE: no Content-Length, connection closes at end so the
        # client's stream iterator terminates after the [DONE] frame.
        self.close_connection = True
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()

        def frame(delta: Dict[str, Any], finish: Any = None) -> bytes:
            payload = {
                "id": cid,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [{"index": 0, "delta": delta, "finish_reason": finish}],
            }
            return f"data: {json.dumps(payload)}\n\n".encode("utf-8")

        # role preamble, then content deltas, then terminal frame + [DONE]
        self.wfile.write(frame({"role": "assistant"}))
        chunks: Iterator[str] = iter([result]) if isinstance(result, str) else iter(result)
        for piece in chunks:
            if piece:
                self.wfile.write(frame({"content": piece}))
                self.wfile.flush()
        self.wfile.write(frame({}, finish="stop"))
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()

    def _complete_anthropic(self, result: HandlerResult, model: str):
        text = result if isinstance(result, str) else "".join(result)
        self._json(
            200,
            {
                "id": _msg_id(),
                "type": "message",
                "role": "assistant",
                "model": model,
                "content": [{"type": "text", "text": text}],
                "stop_reason": "end_turn",
                "stop_sequence": None,
                "usage": {"input_tokens": 0, "output_tokens": 0},
            },
        )

    def _stream_anthropic(self, result: HandlerResult, model: str):
        mid = _msg_id()
        self.close_connection = True
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()

        def event(name: str, payload: Dict[str, Any]) -> bytes:
            return f"event: {name}\ndata: {json.dumps(payload)}\n\n".encode("utf-8")

        self.wfile.write(event("message_start", {
            "type": "message_start",
            "message": {
                "id": mid, "type": "message", "role": "assistant", "model": model,
                "content": [], "stop_reason": None, "stop_sequence": None,
                "usage": {"input_tokens": 0, "output_tokens": 0},
            },
        }))
        self.wfile.write(event("content_block_start", {
            "type": "content_block_start", "index": 0,
            "content_block": {"type": "text", "text": ""},
        }))
        chunks: Iterator[str] = iter([result]) if isinstance(result, str) else iter(result)
        for piece in chunks:
            if piece:
                self.wfile.write(event("content_block_delta", {
                    "type": "content_block_delta", "index": 0,
                    "delta": {"type": "text_delta", "text": piece},
                }))
                self.wfile.flush()
        self.wfile.write(event("content_block_stop", {"type": "content_block_stop", "index": 0}))
        self.wfile.write(event("message_delta", {
            "type": "message_delta",
            "delta": {"stop_reason": "end_turn", "stop_sequence": None},
            "usage": {"output_tokens": 0},
        }))
        self.wfile.write(event("message_stop", {"type": "message_stop"}))
        self.wfile.flush()

    # -- helpers ---------------------------------------------------------

    def _authorized(self) -> bool:
        expected = self.server.api_key  # type: ignore[attr-defined]
        if not expected:
            return True
        header = self.headers.get("Authorization", "")
        token = header[7:] if header.startswith("Bearer ") else header
        return token.strip() == expected

    def _read_json(self):
        try:
            length = int(self.headers.get("Content-Length", 0))
            raw = self.rfile.read(length) if length else b""
            return json.loads(raw or b"{}")
        except (ValueError, OSError):
            return None

    def _json(self, status: int, payload: Dict[str, Any]):
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def _chat_id() -> str:
    return "chatcmpl-" + uuid.uuid4().hex[:24]


def _msg_id() -> str:
    return "msg_" + uuid.uuid4().hex[:24]


def make_server(
    dispatch: Dispatch,
    model_id: str,
    host: str,
    port: int,
    api_key: str = "",
    verbose: bool = False,
) -> Tuple[ThreadingHTTPServer, int]:
    """Build (but do not start) the plugin's HTTP server.

    Returns the server and the resolved port (useful when ``port=0`` asks the
    OS for an ephemeral one — handy in tests).
    """
    httpd = ThreadingHTTPServer((host, port), _Handler)
    # stash config on the server instance for the handler to read
    httpd.dispatch = dispatch  # type: ignore[attr-defined]
    httpd.model_id = model_id  # type: ignore[attr-defined]
    httpd.api_key = api_key  # type: ignore[attr-defined]
    httpd.verbose = verbose  # type: ignore[attr-defined]
    return httpd, httpd.server_address[1]
