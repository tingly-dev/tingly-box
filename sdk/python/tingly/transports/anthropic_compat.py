"""Build an ``anthropic.Anthropic`` client bound to a tingly-box scenario.

The Anthropic Python SDK appends ``/v1/messages`` to its ``base_url``, so we
target the scenario root *without* a version segment: e.g.
``http://127.0.0.1:12580/tingly/experiment`` → ``…/tingly/experiment/v1/messages``,
which matches the gateway's ``/tingly/:scenario/v1`` route group.
"""

from __future__ import annotations

from typing import Any


def build_anthropic(base_url: str, token: str, timeout: float) -> Any:
    try:
        import anthropic
    except ImportError as exc:  # pragma: no cover - import guard
        raise ImportError(
            "The Anthropic transport requires the `anthropic` package. "
            "Install it with `pip install tingly[anthropic]`."
        ) from exc

    return anthropic.Anthropic(
        base_url=base_url.rstrip("/"),
        api_key=token or "tingly-box",
        timeout=timeout,
    )


def build_async_anthropic(base_url: str, token: str, timeout: float) -> Any:
    try:
        import anthropic
    except ImportError as exc:  # pragma: no cover - import guard
        raise ImportError(
            "The Anthropic transport requires the `anthropic` package. "
            "Install it with `pip install tingly[anthropic]`."
        ) from exc

    return anthropic.AsyncAnthropic(
        base_url=base_url.rstrip("/"),
        api_key=token or "tingly-box",
        timeout=timeout,
    )
