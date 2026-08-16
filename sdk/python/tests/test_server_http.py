"""Server tests — drive a real (ephemeral-port) tingly Server over HTTP.

These pin the two wire contracts tingly-box relies on when it routes to a
the process as an upstream — Anthropic /v1/messages and OpenAI
/v1/chat/completions (secondary) — plus buffered/streaming shape, /v1/models,
and auth. Both protocols share one handler; only response shaping differs.
"""

import json

import httpx
import pytest

from tingly import ChatRequest, Server


@pytest.fixture
def served():
    srv = Server(name="t-plug")

    @srv.chat
    def handle(req: ChatRequest):
        if req.stream:
            return iter(["he", "llo ", req.last_user_text()])
        return f"echo: {req.last_user_text()}"

    port = srv.run(port=0, verbose=False, block=False)
    base = f"http://127.0.0.1:{port}"
    yield base
    srv.stop()


def test_health(served):
    r = httpx.get(f"{served}/health", timeout=5)
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_models_lists_model_id(served):
    r = httpx.get(f"{served}/v1/models", timeout=5)
    assert r.status_code == 200
    ids = [m["id"] for m in r.json()["data"]]
    assert "t-plug" in ids


def test_chat_completion_shape(served):
    r = httpx.post(
        f"{served}/v1/chat/completions",
        json={"model": "t-plug", "messages": [{"role": "user", "content": "hi"}]},
        timeout=5,
    )
    assert r.status_code == 200
    body = r.json()
    assert body["object"] == "chat.completion"
    assert body["choices"][0]["message"]["content"] == "echo: hi"
    assert body["choices"][0]["finish_reason"] == "stop"


def test_chat_completion_streaming(served):
    with httpx.stream(
        "POST",
        f"{served}/v1/chat/completions",
        json={
            "model": "t-plug",
            "messages": [{"role": "user", "content": "world"}],
            "stream": True,
        },
        timeout=5,
    ) as r:
        assert r.status_code == 200
        deltas = []
        saw_done = False
        for line in r.iter_lines():
            if not line.startswith("data: "):
                continue
            data = line[len("data: "):]
            if data == "[DONE]":
                saw_done = True
                continue
            chunk = json.loads(data)
            assert chunk["object"] == "chat.completion.chunk"
            delta = chunk["choices"][0]["delta"]
            if delta.get("content"):
                deltas.append(delta["content"])
    assert "".join(deltas) == "hello world"
    assert saw_done


def test_anthropic_message_shape(served):
    r = httpx.post(
        f"{served}/v1/messages",
        json={
            "model": "t-plug",
            "max_tokens": 256,
            "messages": [{"role": "user", "content": "hi"}],
        },
        timeout=5,
    )
    assert r.status_code == 200
    body = r.json()
    assert body["type"] == "message"
    assert body["role"] == "assistant"
    assert body["content"] == [{"type": "text", "text": "echo: hi"}]
    assert body["stop_reason"] == "end_turn"


def test_anthropic_message_streaming(served):
    with httpx.stream(
        "POST",
        f"{served}/v1/messages",
        json={
            "model": "t-plug",
            "max_tokens": 256,
            "messages": [{"role": "user", "content": "world"}],
            "stream": True,
        },
        timeout=5,
    ) as r:
        assert r.status_code == 200
        events = []
        deltas = []
        for line in r.iter_lines():
            if line.startswith("event: "):
                events.append(line[len("event: "):])
            elif line.startswith("data: "):
                payload = json.loads(line[len("data: "):])
                if payload.get("type") == "content_block_delta":
                    deltas.append(payload["delta"]["text"])
    assert events == [
        "message_start", "content_block_start", "content_block_delta",
        "content_block_delta", "content_block_delta", "content_block_stop",
        "message_delta", "message_stop",
    ]
    assert "".join(deltas) == "hello world"


def test_anthropic_messages_route_ignores_query_string(served):
    """tb's real Anthropic client appends ?beta=true to /v1/messages; the
    the server must route on the path, not the raw request-target."""
    r = httpx.post(
        f"{served}/v1/messages?beta=true",
        json={"model": "t-plug", "max_tokens": 32, "messages": [{"role": "user", "content": "hi"}]},
        timeout=5,
    )
    assert r.status_code == 200
    assert r.json()["content"][0]["text"] == "echo: hi"


def test_anthropic_system_field_reaches_handler():
    srv = Server(name="sys-plug")

    @srv.chat
    def handle(req: ChatRequest):
        return f"system={req.system_text()!r} user={req.last_user_text()!r}"

    port = srv.run(port=0, verbose=False, block=False)
    base = f"http://127.0.0.1:{port}"
    try:
        r = httpx.post(
            f"{base}/v1/messages",
            json={
                "model": "x",
                "max_tokens": 64,
                "system": "be terse",
                "messages": [{"role": "user", "content": "hi"}],
            },
            timeout=5,
        )
        assert r.status_code == 200
        text = r.json()["content"][0]["text"]
        assert text == "system='be terse' user='hi'"
    finally:
        srv.stop()


def test_auth_enforced_when_key_set():
    srv = Server(name="auth-plug", api_key="secret")

    @srv.chat
    def handle(req):
        return "ok"

    port = srv.run(port=0, verbose=False, block=False)
    base = f"http://127.0.0.1:{port}"
    try:
        bad = httpx.post(
            f"{base}/v1/chat/completions",
            json={"model": "x", "messages": []},
            timeout=5,
        )
        assert bad.status_code == 401

        good = httpx.post(
            f"{base}/v1/chat/completions",
            headers={"Authorization": "Bearer secret"},
            json={"model": "x", "messages": [{"role": "user", "content": "hi"}]},
            timeout=5,
        )
        assert good.status_code == 200

        bad_anthropic = httpx.post(
            f"{base}/v1/messages",
            json={"model": "x", "max_tokens": 8, "messages": []},
            timeout=5,
        )
        assert bad_anthropic.status_code == 401
        assert bad_anthropic.json()["type"] == "error"
    finally:
        srv.stop()


def test_model_id_defaults_to_name_without_prefix():
    """The provider is already the namespace, so the model id carries no prefix
    — the value in /v1/models is exactly what the user types into the rule."""
    assert Server(name="rag-demo").model_id == "rag-demo"
    assert Server(name="rag-demo", model_id="custom/id").model_id == "custom/id"


def test_connect_hint_prints_the_values_tb_asks_for():
    """The startup banner is the artifact for the next action (Connect AI), so
    it must carry literal, pasteable values — not a pointer to the docs."""
    hint = Server(name="rag-demo").connect_hint("127.0.0.1", 8765)
    assert "http://127.0.0.1:8765" in hint       # Anthropic base URL
    assert "http://127.0.0.1:8765/v1" in hint    # OpenAI base URL
    assert "rag-demo" in hint
    assert "none required" in hint               # no-key providers


def test_default_port_matches_the_tb_provider_template():
    """internal/data/providers.json pre-fills localhost:8765 for the
    `tingly-python` self-hosted template; a Server that bound anywhere else by
    default would make that pre-filled value wrong."""
    from tingly.server.core import DEFAULT_HOST, DEFAULT_PORT

    assert (DEFAULT_HOST, DEFAULT_PORT) == ("127.0.0.1", 8765)
