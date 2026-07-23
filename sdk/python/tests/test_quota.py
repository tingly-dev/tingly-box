"""QuotaView tests (gateway mocked with respx) — pin the exact response
shapes tb's provider-quota endpoints return (list/refresh wrap {meta,data};
get/refresh-one are bare ProviderUsage; batch wraps {data: {uuid: usage}}),
plus the headroom_percent / remaining_percent heuristics used for routing.
"""

import httpx
import respx

from tingly.helpers.quota import ProviderQuota, QuotaView, UsageWindow

BASE = "http://tb.test:12580"


def _view() -> QuotaView:
    return QuotaView(BASE, "admin", 5.0)


def _window(**overrides):
    base = {"key": "session", "type": "session", "used": 10, "limit": 100, "used_percent": 10}
    base.update(overrides)
    return base


@respx.mock
def test_list_unwraps_data_array():
    respx.get(f"{BASE}/api/v1/provider-quota").mock(
        return_value=httpx.Response(200, json={
            "meta": {"total": 1, "updated_at": "2026-01-01T00:00:00Z"},
            "data": [{
                "provider_uuid": "p1", "provider_name": "Anthropic", "provider_type": "anthropic",
                "windows": [_window()],
            }],
        })
    )
    result = _view().list()
    assert len(result) == 1
    assert result[0].provider_uuid == "p1"
    assert result[0].windows[0].used_percent == 10


@respx.mock
def test_get_is_bare_provider_usage_no_envelope():
    respx.get(f"{BASE}/api/v1/provider-quota/p1").mock(
        return_value=httpx.Response(200, json={
            "provider_uuid": "p1", "provider_name": "Anthropic", "provider_type": "anthropic",
            "windows": [_window(used_percent=42)],
        })
    )
    result = _view().get("p1")
    assert result.provider_uuid == "p1"
    assert result.windows[0].used_percent == 42


@respx.mock
def test_batch_unwraps_uuid_keyed_map():
    respx.post(f"{BASE}/api/v1/provider-quota/batch").mock(
        return_value=httpx.Response(200, json={
            "data": {
                "p1": {"provider_uuid": "p1", "provider_name": "A", "provider_type": "anthropic", "windows": []},
                "p2": {"provider_uuid": "p2", "provider_name": "B", "provider_type": "openai", "windows": []},
            }
        })
    )
    result = _view().batch(["p1", "p2"])
    assert set(result) == {"p1", "p2"}
    assert result["p2"].provider_name == "B"


@respx.mock
def test_refresh_one_returns_bare_provider_usage():
    route = respx.post(f"{BASE}/api/v1/provider-quota/p1/refresh").mock(
        return_value=httpx.Response(200, json={
            "provider_uuid": "p1", "provider_name": "A", "provider_type": "anthropic", "windows": [],
        })
    )
    result = _view().refresh("p1")
    assert route.called
    assert result.provider_uuid == "p1"


@respx.mock
def test_refresh_all_hits_refresh_endpoint_and_returns_none():
    route = respx.post(f"{BASE}/api/v1/provider-quota/refresh").mock(
        return_value=httpx.Response(200, json={"meta": {"total": 0, "updated_at": "x"}, "data": []})
    )
    assert _view().refresh() is None
    assert route.called


# -- headroom heuristics --------------------------------------------------

def test_window_remaining_percent():
    assert UsageWindow._from_json(_window(used_percent=30)).remaining_percent == 70.0


def test_window_remaining_percent_none_when_unlimited():
    # tb's convention: limit<=0 means "unlimited" — there's no percent of an
    # unbounded quantity, so this must not be treated as "0% remaining".
    assert UsageWindow._from_json(_window(limit=0)).remaining_percent is None


def test_provider_headroom_is_the_most_constrained_window():
    quota = ProviderQuota(
        provider_uuid="p1", provider_name="A", provider_type="anthropic",
        windows=[
            UsageWindow._from_json(_window(key="session", used_percent=10)),   # 90% remaining
            UsageWindow._from_json(_window(key="daily", used_percent=80)),     # 20% remaining
        ],
    )
    assert quota.headroom_percent == 20.0


def test_provider_headroom_defaults_to_100_with_no_bounded_windows():
    quota = ProviderQuota(provider_uuid="p1", provider_name="A", provider_type="anthropic", windows=[])
    assert quota.headroom_percent == 100.0
