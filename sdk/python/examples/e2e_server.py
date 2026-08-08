"""E2E fixture: a routing provider that dispatches across two tb rules.

Demonstrates the full loop with no network and no API keys:

  client → tb (rule `router`) → THIS server
         → classify → srv.use("openai").ask(model=<one of two rules>)
         → tb (rule fast-model | strong-model → vmodel provider)
         → that vmodel's answer → back to the client

The two target rules are backed by *different* virtual models, so the response
text itself proves which branch was taken — `e2e_run.sh` asserts that a short
prompt and a long one come back from different upstreams.

Note what is absent: this file performs no registration of any kind. tb learns
about it the same way it learns about Ollama — someone created a provider
pointing at its base URL. `e2e_run.sh` does that with the ordinary
POST /api/v2/providers call, which is exactly what the Connect AI dialog does.
"""

from tingly import ChatRequest, Server

TARGET_SCENARIO = "openai"
LONG_REQUEST_CHARS = 200

srv = Server(name="router", scenario=TARGET_SCENARIO)


def classify(req: ChatRequest) -> str:
    return "strong" if len(req.last_user_text()) >= LONG_REQUEST_CHARS else "fast"


@srv.chat
def handle(req: ChatRequest) -> str:
    bucket = classify(req)
    # Dispatch to a different rule per bucket. Both are ordinary tb rules, so
    # each hop gets guard rails, quota, logging and failover of its own.
    target = "strong-model" if bucket == "strong" else "fast-model"
    answer = srv.use(TARGET_SCENARIO).ask(req.last_user_text(), model=target)
    return f"[routed:{bucket}->{target}] {answer}"


if __name__ == "__main__":
    srv.run(port=8765)
