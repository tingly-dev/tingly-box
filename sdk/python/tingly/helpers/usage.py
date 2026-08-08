"""Usage view — concrete token / request numbers, never aliases.

Reads ``GET /api/v1/usage/stats``, aggregated by model and filtered to the
session's scenario, and returns the gateway's own ``AggregatedStat`` rows.

**Why not /api/v1/requests**, which an earlier version of this file used: that
endpoint returns ``{"total", "requests"}`` where each record is a
``ModelRequestSummary`` — a routing/latency view with no token fields at all.
The old code read ``payload["data"]`` (a key that endpoint never sends) and
summed ``rec["input_tokens"]`` (a field that record type does not have), all
inside a bare ``except`` that returned zeros. So it reported 0 tokens for every
session, always, and looked like it worked. Parsing into generated models is
what turned that from an invisible zero into a loud failure.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict, List, Optional

from .._api import ControlPlane
from .._generated.models import AggregatedStat, UsageStatsResponse
from ..errors import TinglyError


@dataclass
class UsageSummary:
    """Totals across the rows returned for this scenario."""

    requests: int = 0
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_write_tokens: int = 0
    by_model: Dict[str, int] = field(default_factory=dict)
    rows: List[AggregatedStat] = field(default_factory=list)

    @property
    def total_tokens(self) -> int:
        return self.input_tokens + self.output_tokens


class UsageView:
    def __init__(self, gateway_url: str, admin_token: str, scenario: str, timeout: float):
        self._api = ControlPlane(gateway_url, admin_token, timeout=timeout)
        self._scenario = scenario

    def this_session(self) -> UsageSummary:
        """Token and request totals for this session's scenario.

        Scoped to the *scenario*, not to the individual `connect(name=...)`
        caller: the usage store has no per-caller dimension, and the scenario
        is what an SDK session isolates anyway. Two experiments sharing a
        scenario share these numbers — use a profile (`experiment:p1`) to split
        them.
        """
        return self.by_model()

    def by_model(self) -> UsageSummary:
        """Per-model breakdown for this scenario."""
        return self._summarize(self._stats(group_by="model"))

    def by_provider(self) -> UsageSummary:
        """Per-provider breakdown for this scenario."""
        return self._summarize(self._stats(group_by="provider"))

    def raw(self, **params) -> UsageStatsResponse:
        """The full typed response, for any grouping/filtering the API supports.

        Escape hatch onto the real endpoint — see openapi.json for the whole
        parameter set (``start_time``, ``status``, ``sort_by``, …):

            tb.usage.raw(group_by="rule", sort_by="total_tokens", limit=5)
        """
        params.setdefault("scenario", self._scenario)
        return self._api.get("/api/v1/usage/stats", UsageStatsResponse, params=params)

    # -- internals -------------------------------------------------------

    def _stats(self, group_by: str) -> Optional[UsageStatsResponse]:
        try:
            return self.raw(group_by=group_by)
        except TinglyError:
            # A usage store that isn't configured yet returns 503. That is a
            # legitimate "nothing to report", unlike a shape mismatch, which
            # ControlPlane raises as SchemaMismatchError and we let through.
            return None

    @staticmethod
    def _summarize(stats: Optional[UsageStatsResponse]) -> UsageSummary:
        summary = UsageSummary()
        if stats is None:
            return summary
        summary.rows = list(stats.data)
        for row in stats.data:
            summary.requests += row.request_count
            summary.input_tokens += row.total_input_tokens
            summary.output_tokens += row.total_output_tokens
            summary.cache_read_tokens += row.cache_read_tokens
            summary.cache_write_tokens += row.cache_write_tokens
            summary.by_model[row.key] = summary.by_model.get(row.key, 0) + row.request_count
        return summary

    def close(self) -> None:
        self._api.close()
