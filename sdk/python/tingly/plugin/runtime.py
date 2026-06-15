"""Dynamic (ephemeral) plugin registration with tingly-box.

A plugin registers a live instance, keeps it alive with a heartbeat, and
deregisters on shutdown — so it appears in tb only while it runs. Nothing is
persisted; if the plugin dies, tb's lease expires and routing falls back (tier
failover) to a real model.
"""

from __future__ import annotations

import threading
from dataclasses import dataclass
from typing import Optional

import httpx

from .. import config as _config
from ..errors import AuthError, GatewayUnreachableError


@dataclass
class Lease:
    gateway_url: str
    admin_token: str
    plugin_id: str
    lease_id: str
    model_id: str
    scenario: Optional[str]
    rule_uuid: Optional[str]
    ttl_seconds: int


def register(
    name: str,
    endpoint: str,
    model_id: str,
    *,
    scenario: Optional[str] = None,
    token: str = "",
    tier: int = 0,
    ttl_seconds: int = 30,
    gateway_url: Optional[str] = None,
    admin_token: Optional[str] = None,
    timeout: float = 30.0,
) -> Lease:
    """Register a live ephemeral plugin instance; returns a renewable lease."""
    resolved = _config.resolve(base_url=gateway_url, token=admin_token)
    headers = {"Authorization": f"Bearer {resolved.token or ''}"}
    url = resolved.base_url.rstrip("/") + "/api/v2/plugins/register"
    body = {
        "name": name, "endpoint": endpoint, "model_id": model_id,
        "scenario": scenario or "", "token": token, "tier": tier,
        "ttl_seconds": ttl_seconds,
    }
    try:
        resp = httpx.post(url, json=body, headers=headers, timeout=timeout)
    except httpx.HTTPError as exc:
        raise GatewayUnreachableError(f"could not reach tingly-box: {exc}") from exc
    if resp.status_code == 401:
        raise AuthError("tingly-box rejected the admin token during plugin register")
    payload = _json(resp) or {}
    if resp.status_code != 200 or not payload.get("success"):
        raise GatewayUnreachableError(
            f"plugin register failed: HTTP {resp.status_code} {resp.text[:200]}"
        )
    d = payload.get("data") or {}
    return Lease(
        gateway_url=resolved.base_url.rstrip("/"),
        admin_token=resolved.token or "",
        plugin_id=d.get("plugin_id", ""),
        lease_id=d.get("lease_id", ""),
        model_id=d.get("model_id", model_id),
        scenario=d.get("scenario") or None,
        rule_uuid=d.get("rule_uuid") or None,
        ttl_seconds=int(d.get("ttl_seconds", ttl_seconds)),
    )


def heartbeat(lease: Lease, timeout: float = 10.0) -> bool:
    """Extend the lease. Returns False if tb no longer knows it (re-register)."""
    url = lease.gateway_url + "/api/v2/plugins/heartbeat"
    headers = {"Authorization": f"Bearer {lease.admin_token}"}
    try:
        resp = httpx.post(
            url, json={"lease_id": lease.lease_id, "ttl_seconds": lease.ttl_seconds},
            headers=headers, timeout=timeout,
        )
    except httpx.HTTPError:
        return False
    return resp.status_code == 200


def deregister(lease: Lease, timeout: float = 10.0) -> None:
    """Remove the live instance now (best-effort)."""
    url = lease.gateway_url + "/api/v2/plugins/deregister"
    headers = {"Authorization": f"Bearer {lease.admin_token}"}
    try:
        httpx.post(url, json={"lease_id": lease.lease_id}, headers=headers, timeout=timeout)
    except httpx.HTTPError:
        pass


class Heartbeater:
    """Background thread that renews a lease until stopped."""

    def __init__(self, lease: Lease, interval: Optional[float] = None):
        self._lease = lease
        # Renew well within the TTL (default: a third of it, min 1s).
        self._interval = interval or max(1.0, lease.ttl_seconds / 3.0)
        self._stop = threading.Event()
        self._thread: Optional[threading.Thread] = None

    def start(self) -> "Heartbeater":
        self._thread = threading.Thread(target=self._loop, daemon=True)
        self._thread.start()
        return self

    def _loop(self) -> None:
        while not self._stop.wait(self._interval):
            heartbeat(self._lease)

    def stop(self) -> None:
        self._stop.set()
        if self._thread is not None:
            self._thread.join(timeout=2.0)


def _json(resp: httpx.Response):
    try:
        return resp.json()
    except ValueError:
        return None
