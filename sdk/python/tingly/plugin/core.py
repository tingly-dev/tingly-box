"""The ``Plugin`` class — write an AI server that tingly-box can route to.

A plugin is an OpenAI-compatible upstream: the author registers a single chat
handler, and ``serve()`` runs the HTTP server. tingly-box is then pointed at it
as a provider (``api_base = http://host:port``, model ``model_id``), so the
plugin composes with routing, fallback, guard rails, quota and logging like any
other model.

The handler may call **back into** tingly-box via ``plugin.llm`` (a Layer-1
:class:`~tingly.client.Client`) for its own LLM needs — the recursion shown in
the Layer 3 design graph.

    from tingly import Plugin

    plugin = Plugin(name="my-rag")

    @plugin.chat
    def handle(req):
        docs = retrieve(req.last_user_text())
        return plugin.llm.ask(f"Using {docs}, answer: {req.last_user_text()}")

    if __name__ == "__main__":
        plugin.serve()
"""

from __future__ import annotations

import threading
from typing import Callable, Optional

from .manifest import Manifest
from .server import Dispatch, HandlerResult, make_server
from .types import ChatRequest

ChatHandler = Callable[[ChatRequest], HandlerResult]


class Plugin:
    """An OpenAI-compatible AI server backed by the tingly-box gateway."""

    def __init__(
        self,
        name: str,
        *,
        model_id: Optional[str] = None,
        version: str = "0.1.0",
        description: str = "",
        api_key: str = "",
        scenario: str = "experiment",
    ):
        self.name = name
        self.model_id = model_id or f"plugin/{name}"
        self.version = version
        self.description = description
        self.api_key = api_key
        self.scenario = scenario

        self._handler: Optional[ChatHandler] = None
        self._llm = None  # lazy Layer-1 client
        self._httpd = None

    # -- authoring -------------------------------------------------------

    def chat(self, fn: ChatHandler) -> ChatHandler:
        """Register the plugin's chat handler. Decorator form.

        The handler receives a :class:`ChatRequest` and returns either a string
        (buffered) or an iterator of strings (streamed).
        """
        self._handler = fn
        return fn

    @property
    def llm(self):
        """A lazily-connected Layer-1 client for calling back into tingly-box.

        Lets a plugin reuse the gateway for its own model calls, so it never
        hard-codes a provider or key.
        """
        if self._llm is None:
            from ..client import connect

            self._llm = connect(scenario=self.scenario, name=f"plugin:{self.name}")
        return self._llm

    # -- dispatch --------------------------------------------------------

    def _dispatch(self, req: ChatRequest) -> HandlerResult:
        if self._handler is None:
            raise RuntimeError(
                f"plugin {self.name!r} has no chat handler; decorate one with @plugin.chat"
            )
        return self._handler(req)

    # -- manifest --------------------------------------------------------

    def manifest(self, entrypoint: str, port: int = 8765, transport: str = "openai") -> Manifest:
        """Build a :class:`Manifest` describing this plugin for tingly-box."""
        return Manifest(
            name=self.name,
            model_id=self.model_id,
            entrypoint=entrypoint,
            version=self.version,
            transport=transport,
            port=port,
            description=self.description,
        )

    # -- serving ---------------------------------------------------------

    def serve(
        self,
        host: str = "127.0.0.1",
        port: int = 8765,
        *,
        verbose: bool = True,
        block: bool = True,
    ) -> int:
        """Run the plugin's HTTP server.

        Returns the bound port (resolved even when ``port=0``). With
        ``block=False`` the server runs on a daemon thread and the call returns
        immediately — handy for tests and for embedding.
        """
        httpd, bound = make_server(
            self._dispatch,
            self.model_id,
            host=host,
            port=port,
            api_key=self.api_key,
            verbose=verbose,
        )
        self._httpd = httpd
        if verbose:
            print(
                f"[tingly] plugin {self.name!r} serving model {self.model_id!r} "
                f"on http://{host}:{bound}/v1  (register as an OpenAI provider in tb)"
            )
        if not block:
            t = threading.Thread(target=httpd.serve_forever, daemon=True)
            t.start()
            return bound
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
        finally:
            httpd.shutdown()
        return bound

    def stop(self) -> None:
        if self._httpd is not None:
            self._httpd.shutdown()
            self._httpd = None
