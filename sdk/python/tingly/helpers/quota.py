"""Quota view — per-provider usage/limit windows, and a live refresh.

tb tracks quota per provider as one or more named **windows** (session /
daily / weekly / monthly / balance / model / ...), each with its own
``used`` / ``limit`` / ``used_percent`` — a provider is rarely a single
number. ``list()`` / ``get()`` / ``batch()`` read tb's cache (tb itself
lazily re-fetches from the upstream account when a provider's cached snapshot
has expired, ~20 min TTL by default); ``refresh()`` forces a **live**
re-fetch right now. Prefer the cache for routing decisions made on every
request — LiteLLM's own usage-based-routing docs warn that hitting a live
usage source on every single request adds real per-request latency; reserve
``refresh()`` for when you specifically need a number fresher than the cache.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

import httpx


@dataclass
class UsageWindow:
    """One quota window (e.g. "session", "daily", "monthly TPM")."""

    key: str
    type: str
    used: float
    limit: float
    used_percent: float = 0.0
    unit: str = ""
    label: str = ""
    resets_at: Optional[str] = None
    allowed: Optional[bool] = None
    limit_reached: Optional[bool] = None

    @property
    def remaining_percent(self) -> Optional[float]:
        """0-100 remaining, or ``None`` when ``limit<=0`` (tb's convention
        for "unlimited" — there is no percentage to be remaining *of*)."""
        if self.limit <= 0:
            return None
        return max(0.0, 100.0 - self.used_percent)

    @classmethod
    def _from_json(cls, d: Dict[str, Any]) -> "UsageWindow":
        return cls(
            key=d.get("key", ""),
            type=d.get("type", ""),
            used=d.get("used", 0) or 0,
            limit=d.get("limit", 0) or 0,
            used_percent=d.get("used_percent", 0) or 0,
            unit=d.get("unit", ""),
            label=d.get("label", ""),
            resets_at=d.get("resets_at"),
            allowed=d.get("allowed"),
            limit_reached=d.get("limit_reached"),
        )


@dataclass
class ProviderQuota:
    """A provider's quota snapshot — as cached by tb, or freshly fetched."""

    provider_uuid: str
    provider_name: str
    provider_type: str
    windows: List[UsageWindow] = field(default_factory=list)
    last_error: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)

    @property
    def headroom_percent(self) -> float:
        """The most CONSTRAINED window's remaining percent — i.e. whichever
        limit this provider will hit first. ``100.0`` when no window carries
        a real limit (nothing to be constrained by).

        This is deliberately a single naive number for making a routing pick
        between candidates at a glance (see ``examples/router_plugin.py``);
        session/daily/cost windows are not fungible, so anything more
        precise than "which one is worse off right now" should read
        ``.windows`` directly instead of trusting this alone.
        """
        percents = [w.remaining_percent for w in self.windows if w.remaining_percent is not None]
        return min(percents) if percents else 100.0

    @classmethod
    def _from_json(cls, d: Dict[str, Any]) -> "ProviderQuota":
        return cls(
            provider_uuid=d.get("provider_uuid", ""),
            provider_name=d.get("provider_name", ""),
            provider_type=d.get("provider_type", ""),
            windows=[UsageWindow._from_json(w) for w in d.get("windows") or []],
            last_error=d.get("last_error", ""),
            raw=d,
        )


class QuotaView:
    def __init__(self, gateway_url: str, admin_token: str, timeout: float):
        self._gateway_url = gateway_url.rstrip("/")
        self._admin_token = admin_token
        self._timeout = timeout

    def _headers(self) -> Dict[str, str]:
        return {"Authorization": f"Bearer {self._admin_token}"}

    def list(self) -> List[ProviderQuota]:
        """Every provider's cached quota."""
        resp = httpx.get(
            f"{self._gateway_url}/api/v1/provider-quota",
            headers=self._headers(), timeout=self._timeout,
        )
        resp.raise_for_status()
        data = resp.json().get("data") or []
        return [ProviderQuota._from_json(d) for d in data]

    def get(self, provider_uuid: str) -> ProviderQuota:
        """One provider's cached quota (tb transparently refetches if the
        cached snapshot has expired)."""
        resp = httpx.get(
            f"{self._gateway_url}/api/v1/provider-quota/{provider_uuid}",
            headers=self._headers(), timeout=self._timeout,
        )
        resp.raise_for_status()
        return ProviderQuota._from_json(resp.json())

    def batch(self, provider_uuids: List[str]) -> Dict[str, ProviderQuota]:
        """Cached quota for a specific set of providers in one round trip —
        the shape a router picking between N candidates actually wants."""
        resp = httpx.post(
            f"{self._gateway_url}/api/v1/provider-quota/batch",
            headers=self._headers(), json={"provider_uuids": provider_uuids},
            timeout=self._timeout,
        )
        resp.raise_for_status()
        data = resp.json().get("data") or {}
        return {uuid: ProviderQuota._from_json(d) for uuid, d in data.items()}

    def refresh(self, provider_uuid: Optional[str] = None) -> Optional[ProviderQuota]:
        """Force a LIVE re-fetch from the upstream account, bypassing tb's
        cache entirely. Omit ``provider_uuid`` to refresh every enabled
        provider (returns ``None`` in that case — use :meth:`list` to read
        the results back)."""
        if provider_uuid:
            resp = httpx.post(
                f"{self._gateway_url}/api/v1/provider-quota/{provider_uuid}/refresh",
                headers=self._headers(), timeout=self._timeout,
            )
            resp.raise_for_status()
            return ProviderQuota._from_json(resp.json())
        resp = httpx.post(
            f"{self._gateway_url}/api/v1/provider-quota/refresh",
            headers=self._headers(), timeout=self._timeout,
        )
        resp.raise_for_status()
        return None
