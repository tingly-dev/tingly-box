"""Plugin server tests — drive a real (ephemeral-port) plugin over HTTP.

These pin the OpenAI wire contract tingly-box relies on when it routes to a
plugin as an upstream: chat.completion shape, SSE streaming, /v1/models, auth.
"""

import json

import httpx
import pytest

from tingly import ChatRequest, Plugin


@pytest.fixture
def served():
    plugin = Plugin(name="t-plug", model_id="plugin/t-plug")

    @plugin.chat
    def handle(req: ChatRequest):
        if req.stream:
            return iter(["he", "llo ", req.last_user_text()])
        return f"echo: {req.last_user_text()}"

    port = plugin.serve(port=0, verbose=False, block=False)
    base = f"http://127.0.0.1:{port}"
    yield base
    plugin.stop()


def test_health(served):
    r = httpx.get(f"{served}/health", timeout=5)
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_models_lists_model_id(served):
    r = httpx.get(f"{served}/v1/models", timeout=5)
    assert r.status_code == 200
    ids = [m["id"] for m in r.json()["data"]]
    assert "plugin/t-plug" in ids


def test_chat_completion_shape(served):
    r = httpx.post(
        f"{served}/v1/chat/completions",
        json={"model": "plugin/t-plug", "messages": [{"role": "user", "content": "hi"}]},
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
            "model": "plugin/t-plug",
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


def test_auth_enforced_when_key_set():
    plugin = Plugin(name="auth-plug", api_key="secret")

    @plugin.chat
    def handle(req):
        return "ok"

    port = plugin.serve(port=0, verbose=False, block=False)
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
    finally:
        plugin.stop()
