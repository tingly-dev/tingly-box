"""Active config + plugin registration tests."""

import json

import httpx
import pytest
import respx

import tingly
import tingly.config as cfg
from tingly.plugin import register as plugin_register

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
def test_register_binds_rule(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, BASE)
    monkeypatch.setenv(cfg.ENV_TOKEN, "admin")

    route = respx.post(f"{BASE}/api/v2/plugins").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": {
                "provider_uuid": "uuid-1", "model_id": "plugin/x",
                "scenario": "experiment", "rule_uuid": "rule-1",
                "ready": True, "note": "Plugin wired in.",
            },
        })
    )

    result = plugin_register.register(
        "x", "http://127.0.0.1:8765/v1", "plugin/x", scenario="experiment"
    )
    assert route.called
    assert result.provider_uuid == "uuid-1"
    assert result.rule_uuid == "rule-1"
    assert result.ready is True
    # anthropic is the SDK's default wire protocol for new plugins
    assert json.loads(route.calls.last.request.content)["api_style"] == "anthropic"


@respx.mock
def test_serve_registers_once(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, BASE)
    monkeypatch.setenv(cfg.ENV_TOKEN, "admin")
    route = respx.post(f"{BASE}/api/v2/plugins").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": {"provider_uuid": "pid", "model_id": "plugin/srv",
                     "scenario": "experiment", "ready": True},
        })
    )

    from tingly import Plugin

    plugin = Plugin(name="srv", scenario="experiment")

    @plugin.chat
    def handle(req):
        return "ok"

    port = plugin.serve(port=0, verbose=False, block=False)
    try:
        assert isinstance(port, int) and port > 0
        assert route.called  # registered exactly once on serve()

        # real plugin socket still serves (use urllib so respx doesn't intercept it)
        import urllib.request

        with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=5) as r:
            assert r.status == 200
    finally:
        plugin.stop()

    # stop() only tears down the HTTP server — nothing to deregister, the
    # provider stays configured in tb (same as any other provider).
    assert route.call_count == 1


def test_serve_register_false_skips(monkeypatch):
    from tingly import Plugin

    plugin = Plugin(name="noreg")

    @plugin.chat
    def handle(req):
        return "ok"

    port = plugin.serve(port=0, verbose=False, block=False, register=False)
    try:
        assert isinstance(port, int) and port > 0
    finally:
        plugin.stop()
