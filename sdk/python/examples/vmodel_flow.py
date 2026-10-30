#!/usr/bin/env python3
"""A complete scripted interaction, end to end, with nothing to set up.

This starts a real tingly-box, gives it a *script* — "call Read, call Edit,
get overloaded once, then answer" — and plays an agent against it: send a
prompt, run whatever tool the model asks for, send the result back, and retry
when the upstream says it is overloaded.

Run (needs a tb binary: `go build -o tb ./cli/tingly-box`, or `task demo:vmodel`):
    TINGLY_TB_BIN=./tb python examples/vmodel_flow.py

Already have tb running? Attach instead — only the script file is added:
    python examples/vmodel_flow.py --attach          # config dir: ~/.tingly-box
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import TinglyError, vmodel  # noqa: E402

FLOW = (vmodel.Script("demo-read-edit")
        .tool("Read", {"file_path": "/tmp/a.go"}, say="Let me look at the file first.")
        .tool("Edit", {"file_path": "/tmp/a.go", "old_string": "foo", "new_string": "bar"})
        .error(529)                                  # the upstream hiccups once
        .say("Done: foo is now bar."))


def run_tool(name: str, args: dict) -> str:
    """Stand-in for the agent's tools."""
    return f"(ran {name} {args})"


def agent(session: vmodel.Session, prompt: str) -> str:
    messages = [{"role": "user", "content": prompt}]
    for turn in range(1, 10):
        try:
            reply = session.messages(FLOW, messages)
        except TinglyError as exc:
            print(f"  turn {turn}: HTTP {exc.status} — retrying")
            continue
        text = "".join(b["text"] for b in reply["content"] if b["type"] == "text")
        calls = [b for b in reply["content"] if b["type"] == "tool_use"]
        print(f"  turn {turn}: {reply['stop_reason']:<9} {text!r} {[c['name'] for c in calls]}")
        if not calls:
            return text
        messages.append({"role": "assistant", "content": reply["content"]})
        messages.append({"role": "user", "content": [
            {"type": "tool_result", "tool_use_id": c["id"], "content": run_tool(c["name"], c["input"])}
            for c in calls]})
    raise RuntimeError("the agent never finished")


if __name__ == "__main__":
    testbed = vmodel.Testbed.attach(FLOW) if "--attach" in sys.argv else vmodel.Testbed(FLOW)
    with testbed as tb:
        print(f"tb at {tb.base_url}; model {FLOW.model!r} (Anthropic base_url: {tb.anthropic_base})")
        # A session is this agent's own run of the script: nothing else calling
        # the model (another test, a second agent) can consume its steps.
        print("answer:", agent(tb.session(), "rename foo to bar in a.go"))
