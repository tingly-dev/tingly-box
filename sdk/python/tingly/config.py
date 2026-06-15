"""Configuration resolution for the tingly SDK.

Resolves ``(base_url, token)`` for connecting to a local tingly-box gateway,
following a fixed precedence so behaviour is predictable across dev and hosted
contexts:

    1. Explicit arguments to ``connect()``.
    2. Environment: ``TINGLY_BOX_URL`` / ``TINGLY_BOX_TOKEN``.
    3. The SDK link file: ``~/.tingly-box/sdk.json`` (written by
       ``tingly doctor --link`` or the tb UI "Connect SDK" panel).
    4. The tb config file: ``~/.tingly-box/config.json`` (admin ``UserToken``)
       combined with a localhost probe.

The ``token`` resolved here is the *admin* token used to provision an SDK
session via ``POST /api/v1/sdk/session``; the session itself hands back the
*model* token used for the actual LLM calls.

This module performs no network I/O; the probe lives in ``discovery``.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

DEFAULT_PORT = 12580
DEFAULT_HOST = "127.0.0.1"

ENV_URL = "TINGLY_BOX_URL"
ENV_TOKEN = "TINGLY_BOX_TOKEN"


def config_dir() -> Path:
    """Return the tingly-box config directory (``~/.tingly-box`` by default)."""
    override = os.environ.get("TINGLY_BOX_HOME")
    if override:
        return Path(override)
    return Path.home() / ".tingly-box"


def sdk_link_path() -> Path:
    return config_dir() / "sdk.json"


def tb_config_path() -> Path:
    return config_dir() / "config.json"


@dataclass
class Resolved:
    """A resolved gateway target plus where it came from (for diagnostics)."""

    base_url: Optional[str]
    token: Optional[str]
    source: str  # "args" | "env" | "sdk.json" | "config.json" | "probe-default"


def _read_json(path: Path) -> Optional[dict]:
    try:
        with path.open("r", encoding="utf-8") as fh:
            return json.load(fh)
    except (FileNotFoundError, ValueError, OSError):
        return None


def resolve(
    base_url: Optional[str] = None,
    token: Optional[str] = None,
) -> Resolved:
    """Resolve ``(base_url, token)`` by precedence, without touching the network.

    A missing piece is left as ``None`` for the caller (``discovery``) to fill —
    e.g. an explicit ``base_url`` with no token still falls through to pick up a
    token from env / files.
    """
    src = "args"
    if base_url is None:
        env_url = os.environ.get(ENV_URL)
        if env_url:
            base_url, src = env_url, "env"
    if token is None:
        env_token = os.environ.get(ENV_TOKEN)
        if env_token:
            token = env_token
            if src == "args":
                src = "env"

    if base_url is None or token is None:
        link = _read_json(sdk_link_path())
        if link:
            if base_url is None and link.get("base_url"):
                base_url, src = link["base_url"], "sdk.json"
            if token is None and link.get("token"):
                token = link["token"]
                if src == "args":
                    src = "sdk.json"

    if token is None:
        cfg = _read_json(tb_config_path())
        if cfg:
            # tb stores UserToken (admin) and ModelToken (LLM API key). We need
            # the admin token to provision an SDK session.
            token = cfg.get("UserToken") or cfg.get("user_token")
            if token and src == "args":
                src = "config.json"

    if base_url is None:
        base_url = f"http://{DEFAULT_HOST}:{DEFAULT_PORT}"
        if src == "args":
            src = "probe-default"

    return Resolved(base_url=base_url, token=token, source=src)
