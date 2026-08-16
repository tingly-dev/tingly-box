"""``tingly`` CLI — a single diagnostic command.

`tingly doctor` traverses the *real* code path a user's program takes
(discovery → session → a live LLM round-trip) and prints what worked and what
didn't, so a green doctor is a guarantee that user code will run.

`tingly doctor --link` writes ``~/.tingly-box/sdk.json`` so future runs need no
env vars.

There is deliberately no `tingly serve` / scaffold command: a
:class:`tingly.Server` is a plain Python process you start yourself, and it is
added to tingly-box through the ordinary Connect AI → Self-hosted flow — the
same as Ollama. Nothing about it needs a CLI.
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

    _print_provider_hint()
    return 0


def _print_provider_hint() -> None:
    """Print the literal values for wiring a Python Server into tb.

    doctor verifies the *consume* half. The *provide* half needs no probing —
    it is a provider like any other — but the values are printed here so the
    next action sits in the same field of view as the check that precedes it.
    """
    from .server.core import DEFAULT_HOST, DEFAULT_PORT

    print(
        "\nServing your own model with tingly.Server? Add it in tingly-box →\n"
        "Connect AI → Self-hosted → Python Server (tingly). Defaults:\n"
        f"  Base URL (Anthropic) : http://{DEFAULT_HOST}:{DEFAULT_PORT}\n"
        f"  Base URL (OpenAI)    : http://{DEFAULT_HOST}:{DEFAULT_PORT}/v1\n"
        "  API key              : (none required)\n"
        "  Model                : whatever you passed as Server(name=...)\n"
        "Your running server prints the same block with its actual values."
    )


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

    args = parser.parse_args(argv)
    if args.command == "doctor":
        return doctor(args.scenario, args.link)

    parser.print_help()
    return 0


if __name__ == "__main__":
    sys.exit(main())
