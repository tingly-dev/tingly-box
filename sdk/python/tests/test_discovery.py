"""Discovery + session minting tests (gateway mocked with respx)."""

import httpx
import pytest
import respx

import tingly.discovery as disco
from tingly.errors import AuthError, GatewayUnreachableError, ScenarioNotFoundError

BASE = "http://tb.test:12580"


def _version_route():
    return respx.get(f"{BASE}/api/v1/info/version").mock(
        return_value=httpx.Response(200, json={"version": "1.2.3"})
    )


@respx.mock
def test_probe_version_ok():
    _version_route()
    assert disco.probe_version(BASE) == "1.2.3"


@respx.mock
def test_probe_version_down():
    respx.get(f"{BASE}/api/v1/info/version").mock(
        return_value=httpx.Response(503)
    )
    assert disco.probe_version(BASE) is None


@respx.mock
def test_create_session_ok():
    respx.post(f"{BASE}/api/v1/sdk/session").mock(
        return_value=httpx.Response(
            200,
            json={
                "success": True,
                "data": {
                    "base_url": f"{BASE}/tingly/experiment",
                    "token": "model-tok",
                    "scenario": "experiment",
                    "transport": "both",
                    "ready": True,
                    "services": 2,
                },
            },
        )
    )
    s = disco.create_session(BASE, "admin", "experiment", name="exp")
    assert s.base_url == f"{BASE}/tingly/experiment"
    assert s.token == "model-tok"
    assert s.transport == "both"
    assert s.ready is True
    assert s.services == 2


@respx.mock
def test_create_session_auth_error():
    respx.post(f"{BASE}/api/v1/sdk/session").mock(
        return_value=httpx.Response(401, json={"error": "nope"})
    )
    with pytest.raises(AuthError):
        disco.create_session(BASE, "bad", "experiment")


@respx.mock
def test_create_session_scenario_not_found():
    respx.post(f"{BASE}/api/v1/sdk/session").mock(
        return_value=httpx.Response(
            404,
            json={"success": False, "valid_scenarios": ["experiment", "openai"]},
        )
    )
    with pytest.raises(ScenarioNotFoundError) as ei:
        disco.create_session(BASE, "admin", "bogus")
    assert "experiment" in ei.value.valid_scenarios


@respx.mock
def test_create_session_unreachable():
    respx.post(f"{BASE}/api/v1/sdk/session").mock(
        side_effect=httpx.ConnectError("refused")
    )
    with pytest.raises(GatewayUnreachableError):
        disco.create_session(BASE, "admin", "experiment")
