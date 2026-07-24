"""Usage view — concrete token / request numbers, never aliases.

v0.1 reads the gateway's request history (``/api/v1/requests``) filtered to this
SDK session's name. A dedicated per-session usage endpoint is a backend
follow-up; until then this gives real, inspectable numbers.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict

import httpx


@dataclass
class UsageSummary:
    requests: int = 0
    input_tokens: int = 0
    output_tokens: int = 0
    by_model: Dict[str, int] = field(default_factory=dict)

    @property
    def total_tokens(self) -> int:
        return self.input_tokens + self.output_tokens


class UsageView:
    def __init__(self, gateway_url: str, admin_token: str, name: str, timeout: float):
        self._gateway_url = gateway_url.rstrip("/")
        self._admin_token = admin_token
        self._name = name
        self._timeout = timeout

    def this_session(self) -> UsageSummary:
        """Return token / request totals for this session's named caller."""
        url = f"{self._gateway_url}/api/v1/requests"
        headers = {"Authorization": f"Bearer {self._admin_token}"}
        try:
            resp = httpx.get(url, headers=headers, timeout=self._timeout)
            resp.raise_for_status()
            payload = resp.json()
        except (httpx.HTTPError, ValueError):
            return UsageSummary()

        records = payload.get("data") or payload.get("records") or []
        summary = UsageSummary()
        for rec in records:
            # Best-effort filter by the SDK caller name when the field exists.
            source = rec.get("source") or rec.get("name") or ""
            if self._name and self._name not in str(source):
                continue
            summary.requests += 1
            summary.input_tokens += int(rec.get("input_tokens", 0) or 0)
            summary.output_tokens += int(rec.get("output_tokens", 0) or 0)
            model = rec.get("model", "unknown")
            summary.by_model[model] = summary.by_model.get(model, 0) + 1
        return summary
