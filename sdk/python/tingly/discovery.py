"""Gateway discovery + SDK session minting.

``connect()`` calls into here to turn a resolved ``(base_url, admin_token)``
into a live :class:`Session`: it probes the gateway for liveness, then mints a
scenario-bound session via ``POST /api/v1/sdk/session``.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import httpx

from . import config as _config
from .errors import (
    AuthError,
    GatewayUnreachableError,
    ScenarioNotFoundError,
)


@dataclass
class Session:
    """The minted SDK session — everything the transports need to bind."""

    base_url: str  # scenario root, e.g. http://127.0.0.1:12580/tingly/experiment
    token: str  # model token used for LLM calls
    scenario: str
    transport: str  # "openai" | "anthropic" | "both"
    ready: bool
    services: int


def probe_version(base_url: str, timeout: float = 5.0) -> Optional[str]:
    """Return a liveness marker if the gateway is reachable, else ``None``.

    Uses the unauthenticated ``/api/v1/info/health`` endpoint (the version
    endpoint requires the admin token, so it cannot be used for discovery).
    Returns the string ``"ok"`` when healthy — callers only check for truthiness.
    """
    url = base_url.rstrip("/") + "/api/v1/info/health"
    try:
        resp = httpx.get(url, timeout=timeout)
    except httpx.HTTPError:
        return None
    if resp.status_code != 200:
        return None
    return "ok"


def create_session(
    base_url: str,
    admin_token: str,
    scenario: str,
    name: Optional[str] = None,
    timeout: float = 30.0,
) -> Session:
    """Mint an SDK session against ``POST /api/v1/sdk/session``."""
    url = base_url.rstrip("/") + "/api/v1/sdk/session"
    headers = {"Authorization": f"Bearer {admin_token}"} if admin_token else {}
    body = {"scenario": scenario, "name": name or ""}

    try:
        resp = httpx.post(url, json=body, headers=headers, timeout=timeout)
    except httpx.HTTPError as exc:
        raise GatewayUnreachableError(
            f"could not reach tingly-box at {base_url}: {exc}"
        ) from exc

    if resp.status_code == 401:
        raise AuthError(
            "tingly-box rejected the admin token. Set TINGLY_BOX_TOKEN or run "
            "`tingly doctor --link`."
        )

    payload = _safe_json(resp)
    if resp.status_code == 404:
        raise ScenarioNotFoundError(
            scenario, (payload or {}).get("valid_scenarios")
        )
    if resp.status_code != 200 or not payload or not payload.get("success"):
        raise GatewayUnreachableError(
            f"unexpected response from {url}: HTTP {resp.status_code} {resp.text[:200]}"
        )

    data = payload.get("data") or {}
    return Session(
        base_url=data["base_url"],
        token=data.get("token", ""),
        scenario=data.get("scenario", scenario),
        transport=data.get("transport", "both"),
        ready=bool(data.get("ready", False)),
        services=int(data.get("services", 0)),
    )


def discover_and_connect(
    scenario: str,
    base_url: Optional[str] = None,
    token: Optional[str] = None,
    name: Optional[str] = None,
    timeout: float = 30.0,
) -> Session:
    """Resolve config, verify reachability, and mint a session."""
    resolved = _config.resolve(base_url=base_url, token=token)

    if probe_version(resolved.base_url) is None:
        raise GatewayUnreachableError(
            f"no tingly-box gateway responding at {resolved.base_url} "
            f"(resolved via {resolved.source}). Is `tb` running? "
            f"Set TINGLY_BOX_URL or run `tingly doctor`."
        )

    return create_session(
        base_url=resolved.base_url,
        admin_token=resolved.token or "",
        scenario=scenario,
        name=name,
        timeout=timeout,
    )


def _safe_json(resp: httpx.Response) -> Optional[dict]:
    try:
        return resp.json()
    except ValueError:
        return None
