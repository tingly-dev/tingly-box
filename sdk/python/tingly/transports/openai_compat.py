"""Build an ``openai.OpenAI`` client bound to a tingly-box scenario.

The OpenAI Python SDK appends ``/chat/completions`` (etc.) to its ``base_url``
and expects the version segment to be part of it, so we target the scenario
root + ``/v1``: e.g. ``http://127.0.0.1:12580/tingly/experiment/v1`` →
``…/tingly/experiment/v1/chat/completions``, which matches the gateway's
``/tingly/:scenario/v1`` route group.
"""

from __future__ import annotations

from typing import Any


def build_openai(base_url: str, token: str, timeout: float) -> Any:
    try:
        import openai
    except ImportError as exc:  # pragma: no cover - import guard
        raise ImportError(
            "The OpenAI transport requires the `openai` package. "
            "Reinstall tingly (it ships with openai)."
        ) from exc

    return openai.OpenAI(
        base_url=base_url.rstrip("/") + "/v1",
        api_key=token or "tingly-box",
        timeout=timeout,
    )


def build_async_openai(base_url: str, token: str, timeout: float) -> Any:
    try:
        import openai
    except ImportError as exc:  # pragma: no cover - import guard
        raise ImportError(
            "The OpenAI transport requires the `openai` package. "
            "Reinstall tingly (it ships with openai)."
        ) from exc

    return openai.AsyncOpenAI(
        base_url=base_url.rstrip("/") + "/v1",
        api_key=token or "tingly-box",
        timeout=timeout,
    )
