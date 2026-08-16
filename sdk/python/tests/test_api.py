"""Tests for the typed control-plane layer (gateway mocked with respx).

The point of this layer is that a wrong field name becomes a loud error rather
than a plausible zero, so most of these assert on failure behaviour.
"""

import httpx
import pytest
import respx

from tingly._api import ControlPlane
from tingly._generated.models import HealthInfoResponse, UsageStatsResponse
from tingly.errors import APIStatusError, AuthError, GatewayUnreachableError, SchemaMismatchError

BASE = "http://tb.test:12580"


def _api(token: str = "admin") -> ControlPlane:
    return ControlPlane(BASE, token, timeout=5.0)


@respx.mock
def test_parses_into_the_generated_model():
    respx.get(f"{BASE}/api/v1/info/health").mock(
        return_value=httpx.Response(
            200, json={"health": True, "status": "healthy", "service": "tingly-box"}
        )
    )
    with _api() as api:
        got = api.get("/api/v1/info/health", HealthInfoResponse)
    assert isinstance(got, HealthInfoResponse)
    assert got.status == "healthy"


@respx.mock
def test_sends_the_admin_token():
    route = respx.get(f"{BASE}/api/v1/info/health").mock(
        return_value=httpx.Response(
            200, json={"health": True, "status": "healthy", "service": "tingly-box"}
        )
    )
    with _api("sekrit") as api:
        api.get("/api/v1/info/health", HealthInfoResponse)
    assert route.calls.last.request.headers["Authorization"] == "Bearer sekrit"


@respx.mock
def test_shape_mismatch_raises_instead_of_returning_a_default():
    """The regression this whole layer exists for.

    The previous hand-written usage view read a key the endpoint never sends
    and summed fields the record type does not have, inside a bare except that
    returned zeros — so it reported 0 tokens forever and looked fine. A body
    that doesn't match the generated model must fail loudly.
    """
    respx.get(f"{BASE}/api/v1/usage/stats").mock(
        return_value=httpx.Response(200, json={"unexpected": "shape"})
    )
    with _api() as api:
        with pytest.raises(SchemaMismatchError) as ei:
            api.get("/api/v1/usage/stats", UsageStatsResponse)
    # The message must name the fix, not just the failure.
    assert "task gen:py" in str(ei.value)


@respx.mock
def test_error_status_carries_the_decoded_body():
    """A 404 body's `valid_scenarios` must survive to the caller.

    discovery.create_session reads it off the raised error rather than issuing
    a second request just to see the hint.
    """
    respx.post(f"{BASE}/api/v1/sdk/session").mock(
        return_value=httpx.Response(
            404, json={"error": "unknown scenario", "valid_scenarios": ["experiment"]}
        )
    )
    with _api() as api:
        with pytest.raises(APIStatusError) as ei:
            api.post("/api/v1/sdk/session", HealthInfoResponse, json={})
    assert ei.value.status_code == 404
    assert ei.value.body["valid_scenarios"] == ["experiment"]


@respx.mock
def test_401_is_an_auth_error_with_an_actionable_message():
    respx.get(f"{BASE}/api/v1/info/health").mock(return_value=httpx.Response(401))
    with _api("bad") as api:
        with pytest.raises(AuthError) as ei:
            api.get("/api/v1/info/health", HealthInfoResponse)
    assert "TINGLY_BOX_TOKEN" in str(ei.value)


@respx.mock
def test_connection_failure_is_gateway_unreachable():
    respx.get(f"{BASE}/api/v1/info/health").mock(side_effect=httpx.ConnectError("refused"))
    with _api() as api:
        with pytest.raises(GatewayUnreachableError):
            api.get("/api/v1/info/health", HealthInfoResponse)


@respx.mock
def test_none_query_params_are_dropped():
    """Otherwise httpx serializes them as the literal string "None"."""
    route = respx.get(f"{BASE}/api/v1/usage/stats").mock(
        return_value=httpx.Response(
            200,
            json={
                "data": [],
                "meta": {
                    "start_time": "2026-01-01T00:00:00Z",
                    "end_time": "2026-01-02T00:00:00Z",
                    "group_by": "model",
                    "total_count": 0,
                },
            },
        )
    )
    with _api() as api:
        api.get(
            "/api/v1/usage/stats",
            UsageStatsResponse,
            params={"group_by": "model", "provider": None},
        )
    assert "provider" not in route.calls.last.request.url.params
    assert route.calls.last.request.url.params["group_by"] == "model"


def test_missing_generated_models_explains_how_to_build_them(monkeypatch):
    """A checkout without `task gen:py` must say what to run.

    The models are gitignored, so this is the first thing a new contributor
    hits. Without the guard in tingly/_generated/__init__.py it surfaces as a
    bare "No module named tingly._generated.models", which names the symptom
    and not the fix.
    """
    import importlib
    import importlib.util
    import sys

    real_find_spec = importlib.util.find_spec

    def hide_models(name, *args, **kwargs):
        if name == "tingly._generated.models":
            return None
        return real_find_spec(name, *args, **kwargs)

    monkeypatch.setattr(importlib.util, "find_spec", hide_models)
    # Force a fresh import of the package so its module-level guard re-runs.
    for mod in [m for m in sys.modules if m.startswith("tingly._generated")]:
        monkeypatch.delitem(sys.modules, mod)

    with pytest.raises(ModuleNotFoundError) as ei:
        importlib.import_module("tingly._generated")

    msg = str(ei.value)
    assert "task gen:py" in msg
    assert "openapi.json" in msg
