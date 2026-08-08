"""Gateway discovery + SDK session minting.

``connect()`` calls into here to turn a resolved ``(base_url, admin_token)``
into a live session: it probes the gateway for liveness, then mints a
scenario-bound session via ``POST /api/v1/sdk/session``.

Both responses are parsed into models generated from the gateway's own
``openapi.json``, so :data:`Session` *is* the backend's ``SDKSessionResponse``
rather than a hand-kept copy that can drift away from it.
"""

from __future__ import annotations

from typing import Optional

import httpx

from ._api import ControlPlane
from ._generated.models import HealthInfoResponse, SDKSessionResponse
from .errors import APIStatusError, ScenarioNotFoundError

# The minted session: base_url, token, scenario, transport, ready, services.
# Aliased rather than redeclared — the field list is the backend's to define.
Session = SDKSessionResponse

__all__ = ["Session", "probe_version", "create_session"]


def probe_version(base_url: str, timeout: float = 5.0) -> Optional[str]:
    """Return a liveness marker if the gateway is reachable, else ``None``.

    Uses the unauthenticated ``/api/v1/info/health``; the version endpoint
    requires the admin token, so it cannot be used for discovery. Returns the
    reported status string — callers only check truthiness.

    Unauthenticated and failure-tolerant by design, so it deliberately does not
    go through :class:`ControlPlane`, whose contract is to raise.
    """
    url = base_url.rstrip("/") + "/api/v1/info/health"
    try:
        resp = httpx.get(url, timeout=timeout)
    except httpx.HTTPError:
        return None
    if resp.status_code != 200:
        return None
    try:
        return HealthInfoResponse.model_validate(resp.json()).status or "ok"
    except ValueError:
        return None


def create_session(
    base_url: str,
    admin_token: str,
    scenario: str,
    name: Optional[str] = None,
    timeout: float = 30.0,
) -> Session:
    """Mint an SDK session against ``POST /api/v1/sdk/session``."""
    api = ControlPlane(base_url, admin_token, timeout=timeout)
    try:
        return api.post(
            "/api/v1/sdk/session",
            SDKSessionResponse,
            json={"scenario": scenario, "name": name or ""},
        )
    except APIStatusError as exc:
        # An unknown or non-bindable scenario is the one failure worth naming
        # precisely: the fix is "pick another scenario", and the gateway ships
        # the valid list in the 404 body, which APIStatusError already carries.
        if exc.status_code == 404:
            raise ScenarioNotFoundError(scenario, exc.body.get("valid_scenarios")) from exc
        raise
    finally:
        api.close()
