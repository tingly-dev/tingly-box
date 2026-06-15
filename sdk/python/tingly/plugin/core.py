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
        self._clients: dict = {}  # scenario -> lazily-connected client
        self._httpd = None
        self._lease = None  # runtime.Lease when dynamically registered
        self._heartbeater = None

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
        """A lazily-connected client for calling back into tingly-box.

        This is the plugin's default calling context (``self.scenario``). The
        plugin reuses the gateway for its own model calls, so it never hard-codes
        a provider or key — and ``ask(model=...)`` can target *any* model tb
        routes. To drive a *different* rule-set, use :meth:`use`.
        """
        return self.use(self.scenario)

    def use(self, scenario: str):
        """Return a client bound to a specific scenario (rule-set) in tb.

        A plugin composes the box: it can hold clients to several scenarios and
        pick a model on each, so "the plugin can use any other rule / model
        configured in tb" is one call:

            self.use("claude_code").ask("…", model="claude-sonnet-4-6")
            self.use("experiment").ask("…", model="auto")
        """
        client = self._clients.get(scenario)
        if client is None:
            from ..client import connect

            client = connect(scenario=scenario, name=f"plugin:{self.name}")
            self._clients[scenario] = client
        return client

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
        register: bool = True,
        advertise_host: Optional[str] = None,
        ttl_seconds: int = 30,
        tb: Optional[Any] = None,
    ) -> int:
        """Run the plugin's HTTP server and (by default) register it with tb.

        Dynamic registration is ephemeral: the plugin appears in tb only while it
        runs — a background heartbeat keeps the lease, and it deregisters on
        shutdown. ``tb`` may be a :class:`tingly.config.Connection` to point at a
        specific gateway / inject credentials (containers / CI / remote).

        Returns the bound port (resolved even when ``port=0``). ``block=False``
        runs the server on a daemon thread and returns immediately.
        """
        if tb is not None:
            from ..config import configure

            configure(url=tb.url, admin_token=tb.admin_token, admin_token_env=tb.admin_token_env)

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
                f"on http://{host}:{bound}/v1"
            )

        if register:
            self._register(advertise_host or host, bound, ttl_seconds, verbose)

        if not block:
            t = threading.Thread(target=httpd.serve_forever, daemon=True)
            t.start()
            return bound
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
        finally:
            self.stop()
        return bound

    def _register(self, host: str, port: int, ttl_seconds: int, verbose: bool) -> None:
        from . import runtime

        endpoint = f"http://{host}:{port}/v1"
        try:
            lease = runtime.register(
                self.name, endpoint, self.model_id,
                scenario=self.scenario, token=self.api_key, ttl_seconds=ttl_seconds,
            )
        except Exception as exc:  # noqa: BLE001 - registration is best-effort
            if verbose:
                print(f"[tingly] plugin registration skipped: {exc}")
            return
        self._lease = lease
        self._heartbeater = runtime.Heartbeater(lease).start()
        if verbose:
            print(
                f"[tingly] registered '{self.name}' as model {lease.model_id!r}"
                + (f" under scenario {lease.scenario!r}" if lease.scenario else "")
                + f" (lease ttl={lease.ttl_seconds}s)"
            )

    def stop(self) -> None:
        if self._heartbeater is not None:
            self._heartbeater.stop()
            self._heartbeater = None
        if self._lease is not None:
            from . import runtime

            runtime.deregister(self._lease)
            self._lease = None
        if self._httpd is not None:
            self._httpd.shutdown()
            self._httpd = None
