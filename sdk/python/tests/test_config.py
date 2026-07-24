"""Config resolution precedence tests (no network)."""

import json

import tingly.config as cfg


def test_args_win(monkeypatch):
    monkeypatch.setenv(cfg.ENV_URL, "http://env:1/")
    monkeypatch.setenv(cfg.ENV_TOKEN, "env-token")
    r = cfg.resolve(base_url="http://arg:2", token="arg-token")
    assert r.base_url == "http://arg:2"
    assert r.token == "arg-token"
    assert r.source == "args"


def test_env_fallback(monkeypatch, tmp_path):
    monkeypatch.setenv("TINGLY_BOX_HOME", str(tmp_path))
    monkeypatch.setenv(cfg.ENV_URL, "http://env:1")
    monkeypatch.setenv(cfg.ENV_TOKEN, "env-token")
    r = cfg.resolve()
    assert r.base_url == "http://env:1"
    assert r.token == "env-token"
    assert r.source == "env"


def test_sdk_link_file(monkeypatch, tmp_path):
    monkeypatch.delenv(cfg.ENV_URL, raising=False)
    monkeypatch.delenv(cfg.ENV_TOKEN, raising=False)
    monkeypatch.setenv("TINGLY_BOX_HOME", str(tmp_path))
    (tmp_path / "sdk.json").write_text(
        json.dumps({"base_url": "http://link:3", "token": "link-token"})
    )
    r = cfg.resolve()
    assert r.base_url == "http://link:3"
    assert r.token == "link-token"
    assert r.source == "sdk.json"


def test_config_json_admin_token(monkeypatch, tmp_path):
    monkeypatch.delenv(cfg.ENV_URL, raising=False)
    monkeypatch.delenv(cfg.ENV_TOKEN, raising=False)
    monkeypatch.setenv("TINGLY_BOX_HOME", str(tmp_path))
    (tmp_path / "config.json").write_text(json.dumps({"UserToken": "admin-xyz"}))
    r = cfg.resolve()
    assert r.token == "admin-xyz"
    # default localhost base when nothing else set
    assert r.base_url.startswith("http://127.0.0.1:")


def test_default_localhost(monkeypatch, tmp_path):
    monkeypatch.delenv(cfg.ENV_URL, raising=False)
    monkeypatch.delenv(cfg.ENV_TOKEN, raising=False)
    monkeypatch.setenv("TINGLY_BOX_HOME", str(tmp_path))
    r = cfg.resolve()
    assert r.base_url == f"http://{cfg.DEFAULT_HOST}:{cfg.DEFAULT_PORT}"
    assert r.token is None
