"""The few-lines layer: register a plain Python function as a tb provider.

    import tingly

    @tingly.image("qwen-image-2.1")
    def generate(prompt):
        return pipe(prompt).images[0]

    tingly.serve()

Everything here is built on the raw layer (`server.py`) and follows one
rule: **unpack, don't convert.** A function receives the one field that is
the point of the call — `messages` for Chat and Anthropic, `input` for
Responses, `prompt` (and `images` for edits) for images — plus the rest of the same request body as keyword
arguments, never a translated or normalized version of it. Its return value
goes through the raw layer's existing wrappers (`str` for text; `bytes` /
`.save()`-able images for images; a `dict` always passes through).

Text decorators are named for their wire protocol (`openai_chat`,
`openai_responses`, `anthropic_message`), image ones for the capability
(`image`, `image_edit`), with short aliases `chat` / `responses` /
`message` for the text ones; raw names are endpoints (`srv.chat`,
`srv.responses`, `srv.messages`, `srv.images`, `srv.image_edits`) — so one
word never means two contracts.

- Only the keyword arguments a function accepts are passed: `def f(prompt)`
  gets just `prompt`, `def f(prompt, size=None)` also gets `size`, and
  `def f(prompt, **kw)` gets the whole rest of the body.
- Each text decorator serves its own protocol only; nothing bridges them.
  Registered as a Chat-mode provider, tb translates Anthropic and Responses
  clients into Chat, so `openai_chat` alone covers most plugins.
- Several models can share one process: each is listed on `/v1/models`, and
  a request is routed by its `model` field. With exactly one function on an
  endpoint, that function answers whatever the name.

See `.design/python-sdk.md` ("Sugar — the few-lines path").
"""

from __future__ import annotations

import inspect
from typing import Any, Callable

from .server import Server

_server: Server | None = None
_registry: dict[str, dict[str, Callable[..., Any]]] = {}


def _rest(body: dict, *unpacked: str) -> dict:
    skip = {"model", *unpacked}
    return {k: v for k, v in body.items() if k not in skip}


# endpoint -> how to pull the positional arguments and the leftover keyword
# arguments out of that endpoint's raw body.
_UNPACK: dict[str, Callable[[dict], tuple[tuple, dict]]] = {
    "chat": lambda body: ((body.get("messages", []),), _rest(body, "messages")),
    "responses": lambda body: ((body.get("input", ""),), _rest(body, "input")),
    "messages": lambda body: ((body.get("messages", []),), _rest(body, "messages")),
    "images": lambda body: ((body.get("prompt", ""),), _rest(body, "prompt")),
    "image_edits": lambda body: ((body.get("prompt", ""), body.get("image", [])), _rest(body, "prompt", "image")),
}


def _call(fn: Callable[..., Any], args: tuple, rest: dict) -> Any:
    params = inspect.signature(fn).parameters.values()
    if any(p.kind is p.VAR_KEYWORD for p in params):
        return fn(*args, **rest)
    accepted = {p.name for p in params if p.kind in (p.POSITIONAL_OR_KEYWORD, p.KEYWORD_ONLY)}
    return fn(*args, **{k: v for k, v in rest.items() if k in accepted})


def _handle(endpoint: str, body: dict) -> Any:
    functions = _registry[endpoint]
    model = body.get("model", "")
    if model in functions:
        fn = functions[model]
    elif len(functions) == 1:
        fn = next(iter(functions.values()))
    else:
        raise LookupError(f"no function registered for model {model!r} (registered: {', '.join(functions)})")
    args, rest = _UNPACK[endpoint](body)
    return _call(fn, args, rest)


def _register(endpoint: str, model: str, fn: Callable[..., Any]) -> Callable[..., Any]:
    global _server
    if _server is None:
        _server = Server(model)
    elif model not in _server.models:
        _server.models.append(model)
    if endpoint not in _registry:
        _registry[endpoint] = {}
        # One raw handler per endpoint, dispatching by model name.
        getattr(_server, endpoint)(lambda body: _handle(endpoint, body))
    _registry[endpoint][model] = fn
    return fn


def openai_chat(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Serve `model` on `/v1/chat/completions`: `fn(messages, **rest)` →
    `str` (wrapped as a ChatCompletion) or a ChatCompletion `dict`."""
    return lambda fn: _register("chat", model, fn)


def openai_responses(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Serve `model` on `/v1/responses`: `fn(input, **rest)` with `input` as
    sent (a string or the item list) → `str` (wrapped as a Response) or a
    Response `dict`."""
    return lambda fn: _register("responses", model, fn)


def anthropic_message(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Serve `model` on `/v1/messages`: `fn(messages, **rest)` (`system`, if
    sent, is in `rest`) → `str` (wrapped as a Message) or a Message `dict`."""
    return lambda fn: _register("messages", model, fn)


# Short aliases: the same functions, for when the protocol needn't be spelled out.
chat = openai_chat
responses = openai_responses
message = anthropic_message


def image(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Serve `model` on `/v1/images/generations`: `fn(prompt, **rest)` →
    image(s) (`bytes`, `.save()`-able, or a list) or an `ImagesResponse` `dict`."""
    return lambda fn: _register("images", model, fn)


def image_edit(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Serve `model` on `/v1/images/edits`: `fn(prompt, images, **rest)` with
    `images` a `list[bytes]` (and `mask` in `rest` when sent) → same as `image`."""
    return lambda fn: _register("image_edits", model, fn)


def serve(host: str = "0.0.0.0", port: int = 8765) -> None:
    """Run everything registered with the decorators above."""
    if _server is None:
        raise RuntimeError(
            "nothing to serve — decorate a function with @tingly.openai_chat, @tingly.openai_responses, "
            "@tingly.anthropic_message, @tingly.image or @tingly.image_edit first"
        )
    _server.run(host=host, port=port)


def _reset() -> None:
    """Forget every registration (tests only)."""
    global _server
    _server = None
    _registry.clear()
