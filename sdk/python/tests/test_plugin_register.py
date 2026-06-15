"""register_with_tb hits the one-step /api/v2/plugins endpoint (respx mocked)."""

import httpx
import respx

from tingly.plugin.register import register_with_tb

BASE = "http://tb.test:12580"


@respx.mock
def test_register_binds_rule(monkeypatch):
    monkeypatch.setenv("TINGLY_BOX_URL", BASE)
    monkeypatch.setenv("TINGLY_BOX_TOKEN", "admin")

    route = respx.post(f"{BASE}/api/v2/plugins").mock(
        return_value=httpx.Response(
            200,
            json={
                "success": True,
                "data": {
                    "provider_uuid": "uuid-1",
                    "model_id": "plugin/my-rag",
                    "scenario": "experiment",
                    "rule_uuid": "rule-1",
                    "ready": True,
                    "note": "Plugin wired in.",
                },
            },
        )
    )

    result = register_with_tb(
        "my-rag",
        "http://127.0.0.1:8765/v1",
        "plugin/my-rag",
        scenario="experiment",
    )

    assert route.called
    sent = route.calls.last.request
    assert sent.headers["Authorization"] == "Bearer admin"
    assert result.provider_uuid == "uuid-1"
    assert result.rule_uuid == "rule-1"
    assert result.ready is True
    assert result.scenario == "experiment"


@respx.mock
def test_register_provider_only(monkeypatch):
    monkeypatch.setenv("TINGLY_BOX_URL", BASE)
    monkeypatch.setenv("TINGLY_BOX_TOKEN", "admin")

    respx.post(f"{BASE}/api/v2/plugins").mock(
        return_value=httpx.Response(
            200,
            json={
                "success": True,
                "data": {
                    "provider_uuid": "uuid-2",
                    "model_id": "plugin/solo",
                    "ready": False,
                    "note": "Provider created.",
                },
            },
        )
    )

    result = register_with_tb("solo", "http://127.0.0.1:9000/v1", "plugin/solo")
    assert result.ready is False
    assert result.rule_uuid is None
