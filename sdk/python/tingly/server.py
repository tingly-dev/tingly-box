"""Be a provider tb can call — the one provider contract.

`Server` registers plain Python functions under a model name, one method
per endpoint, and serves them over HTTP so tb can call it like any other
self-hosted provider:

    srv = Server()

    @srv.openai_chat("my-model")          # POST /v1/chat/completions
    def reply(messages):
        return "..."

    srv.run(port=8765)

| method (alias)                  | endpoint                     | positional        |
|---------------------------------|------------------------------|-------------------|
| `openai_chat` (`chat`)          | `/v1/chat/completions`       | `messages`        |
| `openai_responses` (`responses`)| `/v1/responses`              | `input`           |
| `anthropic_message` (`message`) | `/v1/messages`               | `messages`        |
| `image`                         | `/v1/images/generations`     | `prompt`          |
| `image_edit`                    | `/v1/images/edits`           | `prompt`, `images`|

**Unpack, don't convert.** A function receives its endpoint's positional
field(s) exactly as the caller sent them, plus the rest of the same body as
keyword arguments — only the ones it declares (`model` included), or all of
them with `**kw`. Nothing is translated, normalized or wrapped in a type of
ours, and nothing bridges the three text protocols. A `str` reply is
wrapped into a minimal envelope of the endpoint's own protocol, image(s)
into `b64_json`, and a `dict` passes through as-is. With `"stream": true`,
the complete reply is replayed as that protocol's SSE in one chunk.

`tingly.openai_chat(...)` & co. and `tingly.serve()` (`default.py`) are
these same methods on a default `Server`. See `.design/python-sdk.md`.
"""

from __future__ import annotations

import base64
import email.policy
import inspect
import io
import json
import os
import time
import uuid
from email.parser import BytesParser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, TypeVar

from .client import Client, DEFAULT_SCENARIO

F = TypeVar("F", bound=Callable[..., Any])

# endpoint path -> the body fields handed to a function positionally, each
# with the value used when the caller left it out. Everything else in the
# body is offered as keyword arguments.
_POSITIONAL: dict[str, tuple[tuple[str, Any], ...]] = {
    "/chat/completions": (("messages", []),),
    "/responses": (("input", ""),),
    "/messages": (("messages", []),),
    "/images/generations": (("prompt", ""),),
    "/images/edits": (("prompt", ""), ("image", [])),
}


class Server:
    """A self-hosted provider: functions registered per endpoint and model,
    served over HTTP.

    Args:
        tb_base_url: address of the tb instance to call back into, via
            `.tb`. Falls back to the `TINGLY_BASE_URL` env var. Optional —
            a pure standalone provider never needs `.tb`.
        tb_token: gateway token for `.tb`. Falls back to `TINGLY_TOKEN`.
        tb_scenario: default scenario `.tb.chat()` targets.
    """

    def __init__(
        self,
        tb_base_url: str | None = None,
        tb_token: str | None = None,
        tb_scenario: str = DEFAULT_SCENARIO,
    ):
        self._functions: dict[str, dict[str, Callable[..., Any]]] = {}
        self._httpd: ThreadingHTTPServer | None = None

        base_url = tb_base_url or os.environ.get("TINGLY_BASE_URL")
        token = tb_token or os.environ.get("TINGLY_TOKEN")
        self.tb = Client(base_url, token, scenario=tb_scenario) if base_url else None

    @property
    def models(self) -> list[str]:
        """Every registered model name, in registration order — exactly what
        `GET /v1/models` lists."""
        return list(dict.fromkeys(model for functions in self._functions.values() for model in functions))

    def _register(self, path: str, model: str) -> Callable[[F], F]:
        def decorator(fn: F) -> F:
            self._functions.setdefault(path, {})[model] = fn
            return fn
        return decorator

    def openai_chat(self, model: str) -> Callable[[F], F]:
        """Serve `model` on `/v1/chat/completions`: `fn(messages, **rest)` →
        `str` (wrapped as a ChatCompletion) or a ChatCompletion `dict`."""
        return self._register("/chat/completions", model)

    def openai_responses(self, model: str) -> Callable[[F], F]:
        """Serve `model` on `/v1/responses`: `fn(input, **rest)` with `input`
        as sent (a string or the item list) → `str` (wrapped as a Response)
        or a Response `dict`."""
        return self._register("/responses", model)

    def anthropic_message(self, model: str) -> Callable[[F], F]:
        """Serve `model` on `/v1/messages`: `fn(messages, **rest)` (`system`,
        if sent, is in `rest`) → `str` (wrapped as a Message) or a Message
        `dict`. Always treated as beta-shaped."""
        return self._register("/messages", model)

    def image(self, model: str) -> Callable[[F], F]:
        """Serve `model` on `/v1/images/generations`: `fn(prompt, **rest)` →
        image(s) (`bytes`, `.save()`-able, or a list) or an `ImagesResponse`
        `dict`."""
        return self._register("/images/generations", model)

    def image_edit(self, model: str) -> Callable[[F], F]:
        """Serve `model` on `/v1/images/edits`: `fn(prompt, images, **rest)`
        with `images` a `list[bytes]` (sent as `image` or `image[]`) and
        `mask`, if sent, as `bytes` in `rest`; other form fields arrive as
        strings. Returns the same as `image`."""
        return self._register("/images/edits", model)

    # Short aliases: the same methods, for when the protocol needn't be spelled out.
    chat = openai_chat
    responses = openai_responses
    message = anthropic_message

    def _call(self, path: str, body: dict) -> Any:
        """Pick the function for this request's model and call it with the
        body unpacked."""
        functions = self._functions[path]
        model = body.get("model", "")
        if model in functions:
            fn = functions[model]
        elif len(functions) == 1:
            fn = next(iter(functions.values()))
        else:
            raise LookupError(f"no function registered for model {model!r} (registered: {', '.join(functions)})")
        positional = _POSITIONAL[path]
        args = tuple(body.get(name, default) for name, default in positional)
        unpacked = {name for name, _ in positional}
        rest = {k: v for k, v in body.items() if k not in unpacked}
        return _call_with_declared(fn, args, rest)

    def run(self, host: str = "0.0.0.0", port: int = 8765):
        if not self._functions:
            raise RuntimeError(
                "nothing registered — decorate a function with openai_chat(model), openai_responses(model), "
                "anthropic_message(model), image(model) or image_edit(model) first"
            )

        self._httpd = ThreadingHTTPServer((host, port), _make_request_handler(self))
        bound_port = self._httpd.server_address[1]
        print(f"tingly.Server listening on http://{host}:{bound_port} — models: {', '.join(self.models)}")
        try:
            self._httpd.serve_forever()
        except KeyboardInterrupt:
            self._httpd.shutdown()


def _call_with_declared(fn: Callable[..., Any], args: tuple, rest: dict) -> Any:
    """Call `fn(*args, ...)` passing only the keyword arguments it declares,
    or all of `rest` if it takes `**kw`."""
    params = inspect.signature(fn).parameters.values()
    if any(p.kind is p.VAR_KEYWORD for p in params):
        return fn(*args, **rest)
    accepted = {p.name for p in params if p.kind in (p.POSITIONAL_OR_KEYWORD, p.KEYWORD_ONLY)}
    return fn(*args, **{k: v for k, v in rest.items() if k in accepted})


def _strip_v1(path: str) -> str:
    """Accept a request whether or not the caller's base URL already
    included `/v1` — some clients configure it either way."""
    path = path.split("?", 1)[0].rstrip("/") or "/"  # e.g. /v1/messages?beta=true
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
                    "data": [{"id": m, "object": "model", "owned_by": "tingly-sdk"} for m in srv.models],
                })
            else:
                self._json(404, {"error": "not found"})

        def do_POST(self):
            path = _strip_v1(self.path)
            length = int(self.headers.get("Content-Length", 0))
            data = self.rfile.read(length)
            if path not in _ENDPOINTS:
                self._json(404, {"error": "not found"})
                return
            try:
                if path == "/images/edits":
                    body = _parse_multipart(self.headers.get("Content-Type", ""), data)
                else:
                    body = json.loads(data or b"{}")
            except ValueError as exc:
                self._json(400, {"error": f"invalid request body: {exc}"})
                return
            if path not in srv._functions:
                self._json(404, {"error": f"no function registered for {path}"})
                return

            wrap, stream = _ENDPOINTS[path]
            try:
                result = srv._call(path, body)
                payload = result if isinstance(result, dict) else wrap(body.get("model", ""), result)
                # Built in full before the first byte goes out, so a function
                # error is always a plain 500, never a half-written stream.
                events = stream(payload) if stream is not None and body.get("stream") is True else None
            except Exception as exc:  # surfaced to the caller, not a 500 traceback
                self._json(500, {"error": str(exc)})
                return
            if events is None:
                self._json(200, payload)
            else:
                self._sse(events)

        def _sse(self, events: "SSEEvents"):
            """Write (event name or None, data) pairs as text/event-stream.
            The handler speaks HTTP/1.0, so closing the connection ends it."""
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            for event, data in events:
                line = data if isinstance(data, str) else json.dumps(data)
                self.wfile.write((f"event: {event}\n" if event else "").encode() + f"data: {line}\n\n".encode())
            self.wfile.flush()

        def _json(self, status: int, payload: dict):
            data = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    return RequestHandler


def _wrap_text_openai_chat(model: str, text: str) -> dict:
    """Wrap a plain string reply into a minimal one-choice
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


def _wrap_text_openai_responses(model: str, text: str) -> dict:
    """Wrap a plain string reply into a minimal OpenAI Responses
    envelope — one completed message output item, no tool calls."""
    return {
        "id": f"resp_{uuid.uuid4().hex[:24]}",
        "object": "response",
        "created_at": int(time.time()),
        "model": model,
        "status": "completed",
        "output": [{
            "id": f"msg_{uuid.uuid4().hex[:24]}",
            "type": "message",
            "role": "assistant",
            "status": "completed",
            "content": [{"type": "output_text", "text": text, "annotations": []}],
        }],
        "parallel_tool_calls": False,
        "tool_choice": "auto",
        "tools": [],
    }


def _wrap_text_anthropic(model: str, text: str) -> dict:
    """Wrap a plain string reply into a minimal Anthropic Messages
    envelope. Treated as beta-shaped unconditionally."""
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


def _wrap_images(model: str, result: Any) -> dict:
    """Wrap returned image(s) into a minimal OpenAI `ImagesResponse`:
    always `b64_json` (the only kind tb persists), never `url`."""
    items = result if isinstance(result, (list, tuple)) else [result]
    return {
        "created": int(time.time()),
        "data": [{"b64_json": base64.b64encode(_image_bytes(item)).decode("ascii")} for item in items],
    }


def _image_bytes(image: Any) -> bytes:
    if isinstance(image, (bytes, bytearray, memoryview)):
        return bytes(image)
    if hasattr(image, "save"):  # PIL.Image and look-alikes, without importing PIL
        buf = io.BytesIO()
        image.save(buf, format="PNG")
        return buf.getvalue()
    raise TypeError(
        f"can't turn {type(image).__name__} into an image — return bytes, "
        "an object with .save(fp, format), a list of those, or a dict"
    )


def _parse_multipart(content_type: str, data: bytes) -> dict:
    """Decode a multipart/form-data body into a dict — the multipart
    counterpart of `json.loads`, nothing more. Text fields stay strings;
    file fields become bytes; `image` and `image[]` (one field on the wire,
    sent in two encodings depending on count) both collect into
    `body["image"]` as a list."""
    if not content_type.startswith("multipart/form-data"):
        raise ValueError("expected multipart/form-data")
    message = BytesParser(policy=email.policy.HTTP).parsebytes(
        b"Content-Type: " + content_type.encode("latin-1") + b"\r\n\r\n" + data
    )
    form: dict = {}
    for part in message.iter_parts():
        name = part.get_param("name", header="content-disposition")
        if not name:
            continue
        payload = part.get_payload(decode=True)  # bytes for any non-multipart part
        if not isinstance(payload, bytes):
            payload = b""
        if name in ("image", "image[]"):
            form.setdefault("image", []).append(payload)
        elif part.get_filename() is None:
            form[name] = payload.decode(part.get_content_charset() or "utf-8")
        else:
            form[name] = payload
    return form


# --- Streaming: the complete reply, replayed as one chunk -------------------
#
# Each function takes a complete, non-streamed reply in its protocol and
# returns the SSE events that stream the same reply with all of its content in
# a single chunk. Same protocol in and out — only "whole" becomes "streamed".

SSEEvents = list[tuple[str | None, Any]]  # (event name or None, data: a dict, or "[DONE]")


def _stream_openai_chat(payload: dict) -> SSEEvents:
    base = {"id": payload.get("id", f"chatcmpl-{uuid.uuid4().hex[:24]}"), "object": "chat.completion.chunk",
            "created": payload.get("created", int(time.time())), "model": payload.get("model", "")}
    choices = []
    for i, choice in enumerate(payload.get("choices", [])):
        delta = dict(choice.get("message") or {})
        if delta.get("tool_calls"):
            delta["tool_calls"] = [{"index": n, **call} for n, call in enumerate(delta["tool_calls"])]
        choices.append({"index": choice.get("index", i), "delta": delta,
                        "finish_reason": choice.get("finish_reason", "stop")})
    events: SSEEvents = [(None, {**base, "choices": choices})]
    if payload.get("usage"):
        events.append((None, {**base, "choices": [], "usage": payload["usage"]}))
    events.append((None, "[DONE]"))
    return events


def _stream_anthropic(payload: dict) -> SSEEvents:
    usage = payload.get("usage") or {}
    start = {**payload, "content": [], "stop_reason": None, "stop_sequence": None,
             "usage": {**usage, "output_tokens": 0}}
    events: SSEEvents = [("message_start", {"type": "message_start", "message": start})]
    for i, block in enumerate(payload.get("content", [])):
        kind = block.get("type")
        if kind == "text":
            opening, delta = {**block, "text": ""}, {"type": "text_delta", "text": block.get("text", "")}
        elif kind == "tool_use":
            opening, delta = {**block, "input": {}}, {"type": "input_json_delta",
                                                     "partial_json": json.dumps(block.get("input", {}))}
        else:  # any other block type goes out whole in its start event
            opening, delta = block, None
        events.append(("content_block_start", {"type": "content_block_start", "index": i, "content_block": opening}))
        if delta is not None:
            events.append(("content_block_delta", {"type": "content_block_delta", "index": i, "delta": delta}))
        events.append(("content_block_stop", {"type": "content_block_stop", "index": i}))
    events.append(("message_delta", {
        "type": "message_delta",
        "delta": {"stop_reason": payload.get("stop_reason", "end_turn"), "stop_sequence": payload.get("stop_sequence")},
        "usage": {"output_tokens": usage.get("output_tokens", 0)},
    }))
    events.append(("message_stop", {"type": "message_stop"}))
    return events


def _stream_openai_responses(payload: dict) -> SSEEvents:
    events: SSEEvents = []

    def emit(kind: str, **fields):
        events.append((kind, {"type": kind, "sequence_number": len(events), **fields}))

    emit("response.created", response={**payload, "status": "in_progress", "output": []})
    for oi, item in enumerate(payload.get("output", [])):
        is_message = item.get("type") == "message"
        emit("response.output_item.added", output_index=oi,
             item={**item, "status": "in_progress", "content": []} if is_message else item)
        if is_message:
            for ci, part in enumerate(item.get("content", [])):
                ids = {"item_id": item.get("id", ""), "output_index": oi, "content_index": ci}
                if part.get("type") == "output_text":
                    emit("response.content_part.added", **ids, part={**part, "text": ""})
                    emit("response.output_text.delta", **ids, delta=part.get("text", ""), logprobs=[])
                    emit("response.output_text.done", **ids, text=part.get("text", ""), logprobs=[])
                else:
                    emit("response.content_part.added", **ids, part=part)
                emit("response.content_part.done", **ids, part=part)
        emit("response.output_item.done", output_index=oi, item=item)
    emit("response.completed", response={**payload, "status": payload.get("status") or "completed"})
    return events


# endpoint path -> (how a non-dict reply is wrapped, how a reply is streamed or None)
_ENDPOINTS: dict[str, tuple[Callable[[str, Any], dict], Callable[[dict], SSEEvents] | None]] = {
    "/chat/completions": (_wrap_text_openai_chat, _stream_openai_chat),
    "/responses": (_wrap_text_openai_responses, _stream_openai_responses),
    "/messages": (_wrap_text_anthropic, _stream_anthropic),
    "/images/generations": (_wrap_images, None),
    "/images/edits": (_wrap_images, None),
}
