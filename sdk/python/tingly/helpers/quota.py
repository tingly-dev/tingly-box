"""Quota view — how much headroom each provider has left.

Reads ``GET /api/v1/provider-quota``, which returns one ``ProviderUsage`` per
provider that has a quota fetcher (Codex, Claude Code, Copilot, Cursor, …).

The interesting question is never "what does this provider report" — every
upstream reports in its own currency, some in percentages, some in request
counts, some in dollars — but **"which of my accounts can still take work"**.
:func:`headroom_percent` answers that, mirroring the reduction tb itself does
in Go (``ai/quota/semantic.go``): take the *tightest countable window*, because
that is the one that runs out first.

"Countable" is doing real work there. A window that is unknown, unlimited, or
has no cap is not usage of 0% — it is an absence of information, and treating
it as 0% would make an account with no data look like the emptiest one and win
every comparison. So those are skipped, and a provider with no countable window
at all reports ``None`` rather than a number.
"""

from __future__ import annotations

from typing import List, Optional, Tuple

from .._api import ControlPlane
from .._generated.models import ListQuotaResponse, ProviderUsage, UsageWindow
from ..errors import TinglyError


def countable(window: UsageWindow) -> bool:
    """Whether this window carries a figure comparable across providers.

    Mirrors ``UsageWindow.Countable`` in Go. The ``limit > 0`` check is the
    backstop for a fetcher that reports spend without a cap and forgets the
    ``unlimited`` flag — it would otherwise contribute a fabricated 0%.
    """
    return (
        window is not None
        and not window.unknown
        and not window.unlimited
        and window.limit > 0
    )


def tightest_window(usage: ProviderUsage) -> Optional[UsageWindow]:
    """The binding window — the one that runs out first.

    Mirrors ``ProviderUsage.Tightest``: highest used-percent wins; ties go to
    the shorter window, since that is the more urgent one.
    """
    best: Optional[UsageWindow] = None
    for window in usage.windows or []:
        if not countable(window):
            continue
        if best is None or _tighter(window, best):
            best = window
    return best


def used_percent(usage: ProviderUsage) -> Optional[float]:
    """The provider's usage as a single 0-100 figure, or None if unknown.

    None is deliberately distinct from 0.0: "we have no data" is not "nothing
    has been used".
    """
    window = tightest_window(usage)
    return _percent(window) if window is not None else None


def headroom_percent(usage: ProviderUsage) -> Optional[float]:
    """How much of the binding window is left, 0-100, or None if unknown."""
    pct = used_percent(usage)
    return None if pct is None else 100.0 - pct


def _percent(window: UsageWindow) -> float:
    """used_percent, falling back to used/limit for windows that lack it."""
    if window.used_percent:
        return window.used_percent
    return (window.used / window.limit * 100.0) if window.limit else 0.0


def _tighter(a: UsageWindow, b: UsageWindow) -> bool:
    pa, pb = _percent(a), _percent(b)
    if pa != pb:
        return pa > pb
    # Equally used: prefer the shorter window. One with no known duration sorts
    # last — it cannot be the more urgent if we can't say when it resets.
    return _period_rank(a) < _period_rank(b)


def _period_rank(window: UsageWindow) -> float:
    return window.window_minutes if window.window_minutes else float("inf")


class QuotaView:
    def __init__(self, gateway_url: str, admin_token: str, timeout: float):
        self._api = ControlPlane(gateway_url, admin_token, timeout=timeout)

    def all(self) -> List[ProviderUsage]:
        """Cached quota for every provider that reports it."""
        try:
            return self._api.get("/api/v1/provider-quota", ListQuotaResponse).data
        except TinglyError:
            # 503 when quota tracking isn't enabled on this box — a normal
            # state, not something an experiment should have to handle.
            return []

    def of_type(self, provider_type: str) -> List[ProviderUsage]:
        """Quota for one kind of upstream, e.g. ``"codex"`` or ``"anthropic"``.

        ``provider_type`` is tb's own classification (``ai/quota`` infers it
        from the OAuth issuer, falling back to the API base domain), so several
        accounts of the same kind all share one value — which is exactly the
        set you want to compare against each other.
        """
        return [u for u in self.all() if u.provider_type == provider_type]

    def usable(
        self, provider_type: str, min_headroom: float = 5.0
    ) -> List[Tuple[ProviderUsage, float]]:
        """Accounts of ``provider_type`` with more than ``min_headroom``% left.

        Sorted most-headroom first, so ``[0]`` is the one to send work to.
        Providers whose usage is unknown are excluded rather than assumed
        empty — see the module docstring.
        """
        scored = []
        for usage in self.of_type(provider_type):
            left = headroom_percent(usage)
            if left is not None and left > min_headroom:
                scored.append((usage, left))
        scored.sort(key=lambda pair: pair[1], reverse=True)
        return scored

    def close(self) -> None:
        self._api.close()
