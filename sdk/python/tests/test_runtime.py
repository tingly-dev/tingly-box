"""Active config + dynamic registration tests."""

import threading

import httpx
import pytest
import respx

import tingly
import tingly.config as cfg
from tingly.plugin import runtime

BASE = "http://tb.test:12580"


@pytest.fixture(autouse=True)
def _reset_override():
    cfg._OVERRIDE = None
    yield
    cfg._OVERRIDE = None


def test_configure_takes_precedence(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, "http://env:1")
    monkeypatch.setenv(cfg.ENV_TOKEN, "env-token")
    tingly.configure(url="http://configured:9", admin_token="cfg-token")
    r = cfg.resolve()
    assert r.base_url == "http://configured:9"
    assert r.token == "cfg-token"
    assert r.source == "configure"


def test_connection_token_by_env_reference(monkeypatch):
    monkeypatch.setenv("MY_TB_SECRET", "secret-123")
    conn = tingly.Connection(url="http://x", admin_token_env="MY_TB_SECRET")
    assert conn.token() == "secret-123"


@respx.mock
def test_runtime_register_heartbeat_deregister(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, BASE)
    monkeypatch.setenv(cfg.ENV_TOKEN, "admin")

    reg_route = respx.post(f"{BASE}/api/v2/plugins/register").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": {
                "plugin_id": "pid-1", "lease_id": "lease-1", "model_id": "plugin/x",
                "scenario": "experiment", "rule_uuid": "rule-1", "ttl_seconds": 30,
            },
        })
    )
    hb_route = respx.post(f"{BASE}/api/v2/plugins/heartbeat").mock(
        return_value=httpx.Response(200, json={"success": True})
    )
    dr_route = respx.post(f"{BASE}/api/v2/plugins/deregister").mock(
        return_value=httpx.Response(200, json={"success": True, "data": {"removed": True}})
    )

    lease = runtime.register("x", "http://127.0.0.1:8765/v1", "plugin/x", scenario="experiment")
    assert reg_route.called
    assert lease.lease_id == "lease-1"
    assert lease.rule_uuid == "rule-1"

    assert runtime.heartbeat(lease) is True
    assert hb_route.called

    runtime.deregister(lease)
    assert dr_route.called


@respx.mock
def test_serve_registers_and_deregisters(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, BASE)
    monkeypatch.setenv(cfg.ENV_TOKEN, "admin")
    respx.post(f"{BASE}/api/v2/plugins/register").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": {"plugin_id": "pid", "lease_id": "L", "model_id": "plugin/srv",
                     "scenario": "experiment", "ttl_seconds": 30},
        })
    )
    dr = respx.post(f"{BASE}/api/v2/plugins/deregister").mock(
        return_value=httpx.Response(200, json={"success": True})
    )

    from tingly import Plugin

    plugin = Plugin(name="srv", scenario="experiment")

    @plugin.chat
    def handle(req):
        return "ok"

    # ttl high so the heartbeat thread doesn't fire during the test
    port = plugin.serve(port=0, verbose=False, block=False, ttl_seconds=300)
    assert isinstance(port, int) and port > 0
    assert plugin._lease is not None and plugin._lease.lease_id == "L"

    # real plugin socket still serves (use urllib so respx doesn't intercept it)
    import urllib.request

    with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=5) as r:
        assert r.status == 200

    plugin.stop()
    assert dr.called  # deregistered on shutdown
    assert plugin._lease is None


def test_serve_register_false_skips(monkeypatch):
    from tingly import Plugin

    plugin = Plugin(name="noreg")

    @plugin.chat
    def handle(req):
        return "ok"

    port = plugin.serve(port=0, verbose=False, block=False, register=False)
    try:
        assert plugin._lease is None
    finally:
        plugin.stop()
