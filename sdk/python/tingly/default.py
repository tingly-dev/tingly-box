"""The module-level API: `tingly.openai_chat(...)` & co. and `tingly.serve()`
are the same methods on a default `Server`, created on first use.

    import tingly

    @tingly.image("qwen-image-2.1")
    def generate(prompt):
        return pipe(prompt).images[0]

    tingly.serve()

The contract — what a function receives and returns — lives in `server.py`
only. Build your own `Server` for what the default one doesn't have: `.tb`,
several servers in one process, or isolation in tests.
"""

from __future__ import annotations

from typing import Any, Callable

from .server import Server

_server: Server | None = None


def _default() -> Server:
    global _server
    if _server is None:
        _server = Server()
    return _server


def openai_chat(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """`Server.openai_chat` on the default server."""
    return _default().openai_chat(model)


def openai_responses(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """`Server.openai_responses` on the default server."""
    return _default().openai_responses(model)


def anthropic_message(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """`Server.anthropic_message` on the default server."""
    return _default().anthropic_message(model)


def image(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """`Server.image` on the default server."""
    return _default().image(model)


def image_edit(model: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """`Server.image_edit` on the default server."""
    return _default().image_edit(model)


# Short aliases, as on Server.
chat = openai_chat
responses = openai_responses
message = anthropic_message


def serve(host: str = "0.0.0.0", port: int = 8765) -> None:
    """`Server.run` on the default server."""
    _default().run(host=host, port=port)


def _reset() -> None:
    """Forget the default server and everything registered on it (tests only)."""
    global _server
    _server = None
