"""A tiny OpenAI-compatible HTTP server for plugins (stdlib only).

Exposes exactly what tingly-box needs to treat the plugin as an upstream
OpenAI provider:

    POST /v1/chat/completions   -> chat.completion (+ SSE when stream=true)
    GET  /v1/models             -> the plugin's model id
    GET  /health                -> liveness

No framework dependency — uses ``http.server.ThreadingHTTPServer`` so a plugin
stays a single ``pip install tingly`` away. Streaming is real SSE: a handler
that returns an iterator is emitted as ``chat.completion.chunk`` frames.
"""

from __future__ import annotations

import json
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, Dict, Iterable, Iterator, Tuple, Union

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

    def do_GET(self):
        if self.path.rstrip("/") == "/health":
            return self._json(200, {"status": "ok"})
        if self.path.rstrip("/") in ("/v1/models", "/models"):
            return self._models()
        return self._json(404, {"error": {"message": "not found", "type": "not_found"}})

    def do_POST(self):
        if self.path.rstrip("/") not in ("/v1/chat/completions", "/chat/completions"):
            return self._json(
                404, {"error": {"message": "not found", "type": "not_found"}}
            )
        if not self._authorized():
            return self._json(
                401, {"error": {"message": "invalid token", "type": "auth_error"}}
            )
        body = self._read_json()
        if body is None:
            return self._json(
                400, {"error": {"message": "invalid JSON body", "type": "invalid_request_error"}}
            )

        req = ChatRequest.from_openai_body(body)
        try:
            result = self.server.dispatch(req)  # type: ignore[attr-defined]
        except Exception as exc:  # noqa: BLE001 - surface as upstream 500
            return self._json(
                500,
                {"error": {"message": f"plugin handler error: {exc}", "type": "api_error"}},
            )

        model = req.model or self.server.model_id  # type: ignore[attr-defined]
        if req.stream:
            return self._stream(result, model)
        return self._complete(result, model)

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
