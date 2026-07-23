"""``tingly`` CLI — a single diagnostic command.

`tingly doctor` traverses the *real* code path a user's program takes
(discovery → session → a live LLM round-trip) and prints what worked and what
didn't, so a green doctor is a guarantee that user code will run.

`tingly doctor --link` writes ``~/.tingly-box/sdk.json`` so future runs need no
env vars.
"""

from __future__ import annotations

import argparse
import getpass
import json
import sys
from typing import Optional

from . import config as _config
from . import discovery as _discovery
from . import scenarios as _scenarios

OK = "OK"
FAIL = "FAIL"
WARN = "WARN"


def _row(label: str, detail: str, status: str) -> None:
    print(f"{label:<14}{detail:<40}{status}")


def doctor(scenario: str, link: bool) -> int:
    if link:
        _do_link()

    resolved = _config.resolve()

    # 1. gateway reachable
    alive = _discovery.probe_version(resolved.base_url)
    if alive is None:
        _row("gateway", resolved.base_url, FAIL)
        print(
            f"\nNo tingly-box gateway responding at {resolved.base_url} "
            f"(resolved via {resolved.source}).\n"
            "Start tb, set TINGLY_BOX_URL, or run `tingly doctor --link`."
        )
        return 1
    _row("gateway", f"{resolved.base_url}  (reachable)", OK)
    _row("token", f"{resolved.source}", OK if resolved.token else WARN)

    # 2. mint a session (real path)
    try:
        session = _discovery.create_session(
            base_url=resolved.base_url,
            admin_token=resolved.token or "",
            scenario=scenario,
            name="tingly-doctor",
        )
    except Exception as exc:  # noqa: BLE001 - report any failure verbatim
        _row("session", scenario, FAIL)
        print(f"\n{type(exc).__name__}: {exc}")
        return 1

    scen_detail = f"{session.scenario} ({session.transport}, {session.services} svc)"
    _row("scenario", scen_detail, OK if session.ready else WARN)
    if not session.ready:
        print(
            f"\nScenario {session.scenario!r} has no active rule with a service. "
            "Bind a rule to it in the tingly-box UI before sending requests."
        )

    # 3. live round-trip (only if ready)
    if session.ready:
        _live_check(session)

    return 0


def _live_check(session: "_discovery.Session") -> None:
    from .client import Client

    client = Client(
        session=session,
        gateway_url="",
        admin_token="",
        name="tingly-doctor",
        timeout=30.0,
    )
    try:
        text = client.ask("Reply with the single word: pong", model="auto")
        ok = isinstance(text, str) and len(text) > 0
        transport = "messages" if _scenarios.supports_anthropic(session.transport) else "chat.completions"
        _row("llm test", transport, OK if ok else FAIL)
    except Exception as exc:  # noqa: BLE001
        _row("llm test", "round-trip", FAIL)
        print(f"\n{type(exc).__name__}: {exc}")
    finally:
        client.close()


def _do_link() -> None:
    """Prompt for the admin token and persist a link file."""
    path = _config.sdk_link_path()
    base_url = input(f"Gateway URL [{_config.resolve().base_url}]: ").strip()
    if not base_url:
        base_url = _config.resolve().base_url
    token = getpass.getpass("Admin token (TINGLY_BOX_TOKEN): ").strip()

    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump({"base_url": base_url, "token": token}, fh, indent=2)
    try:
        path.chmod(0o600)
    except OSError:
        pass
    print(f"Wrote {path}")


def _plugin_init(name: str) -> int:
    """Scaffold a minimal plugin: a starter module + tingly.toml."""
    from pathlib import Path

    from .plugin.manifest import Manifest

    safe = name.replace("-", "_")
    module = f"{safe}_plugin.py"
    Path(module).write_text(_PLUGIN_TEMPLATE.format(name=name), encoding="utf-8")
    Manifest(
        name=name,
        model_id=f"plugin/{name}",
        entrypoint=f"{safe}_plugin:plugin",
        description=f"{name} plugin",
    ).write(Path.cwd())
    print(f"Created {module} and tingly.toml.")
    print(f"Run it with:  python {module}   (or: tingly plugin run {module})")
    return 0


def _plugin_run(target: str) -> int:
    """Import a plugin (``module:attr`` or a .py path) and serve it."""
    import importlib
    import importlib.util
    from pathlib import Path

    if target.endswith(".py") or "/" in target:
        path = Path(target)
        spec = importlib.util.spec_from_file_location(path.stem, path)
        mod = importlib.util.module_from_spec(spec)  # type: ignore[arg-type]
        spec.loader.exec_module(mod)  # type: ignore[union-attr]
        plugin = getattr(mod, "plugin", None)
    else:
        mod_name, _, attr = target.partition(":")
        mod = importlib.import_module(mod_name)
        plugin = getattr(mod, attr or "plugin", None)

    if plugin is None:
        print(f"No `plugin` found in {target!r}")
        return 1
    plugin.serve()
    return 0


def main(argv: Optional[list] = None) -> int:
    parser = argparse.ArgumentParser(prog="tingly", description="tingly-box Python SDK")
    sub = parser.add_subparsers(dest="command")

    p_doctor = sub.add_parser("doctor", help="diagnose the SDK ↔ gateway connection")
    p_doctor.add_argument(
        "--scenario", default=_scenarios.EXPERIMENT, help="scenario to test"
    )
    p_doctor.add_argument(
        "--link", action="store_true", help="prompt for and save gateway URL + token"
    )

    p_plugin = sub.add_parser("plugin", help="author / run a plugin")
    psub = p_plugin.add_subparsers(dest="plugin_command")
    p_init = psub.add_parser("init", help="scaffold a starter plugin")
    p_init.add_argument("name", help="plugin name, e.g. my-rag")
    # `run` serves the plugin AND registers it with tb (idempotent upsert on
    # start), so there is no separate one-shot register command.
    p_run = psub.add_parser("run", help="serve a plugin and register it with tb")
    p_run.add_argument("target", help="e.g. my_rag_plugin:plugin or my_rag_plugin.py")

    args = parser.parse_args(argv)
    if args.command == "doctor":
        return doctor(args.scenario, args.link)
    if args.command == "plugin":
        if args.plugin_command == "init":
            return _plugin_init(args.name)
        if args.plugin_command == "run":
            return _plugin_run(args.target)
        p_plugin.print_help()
        return 0

    parser.print_help()
    return 0


_PLUGIN_TEMPLATE = '''"""A tingly-box plugin: an OpenAI-compatible AI server backed by the gateway."""

from tingly import Plugin

plugin = Plugin(name="{name}")


@plugin.chat
def handle(req):
    question = req.last_user_text()
    # Your logic here. Call back into tingly-box for LLM work via plugin.llm:
    return plugin.llm.ask(f"Answer concisely: {{question}}", model="auto")


if __name__ == "__main__":
    plugin.serve()
'''


if __name__ == "__main__":
    sys.exit(main())
