#!/usr/bin/env python3
"""Subprocess client driver for the tingly-box protocol matrix harness.

Speaks the JSON-over-stdin/stdout contract defined in
internal/protocoltest/client_subprocess.go: reads one request object from
stdin, sends it through the gateway using the REAL Python SDKs (anthropic,
openai), and writes one normalized response object to stdout.

The point of this driver is strict client-side validation: every response is
parsed by the SDKs' pydantic models. There is no raw-JSON fallback — if the
gateway emits something the real Python SDK rejects, that is the finding.

Gateway/API errors are reported in-band via the "error" field; a non-zero
exit code means the driver itself is broken.
"""

import json
import sys


def fail(msg):
    print(msg, file=sys.stderr)
    sys.exit(1)


def emit(resp):
    json.dump(resp, sys.stdout)
    sys.stdout.write("\n")


def tool_call(id_, name, arguments):
    return {"id": id_, "name": name, "arguments": arguments}


# ── Anthropic (v1 + beta) ────────────────────────────────────────────────────


def normalize_anthropic_message(msg):
    content = ""
    thinking = ""
    tool_calls = []
    for block in msg.content:
        btype = getattr(block, "type", None)
        if btype == "text":
            content += block.text
        elif btype == "thinking":
            thinking += block.thinking
        elif btype == "tool_use":
            tool_calls.append(
                tool_call(block.id, block.name, json.dumps(block.input or {}))
            )
    usage = None
    if getattr(msg, "usage", None) is not None:
        usage = {
            "input_tokens": msg.usage.input_tokens or 0,
            "output_tokens": msg.usage.output_tokens or 0,
        }
    return {
        "http_status": 200,
        "role": msg.role,
        "content": content,
        "model": str(msg.model),
        "finish_reason": msg.stop_reason or "",
        "thinking": thinking,
        "tool_calls": tool_calls,
        "usage": usage,
        "raw_body": msg.model_dump_json(),
    }


def run_anthropic(req, beta):
    import anthropic

    client = anthropic.Anthropic(
        base_url=req["base_url"] + "/tingly/anthropic",
        api_key=req["api_key"],
        max_retries=0,
        timeout=req.get("timeout_ms", 30000) / 1000.0,
    )
    svc = client.beta.messages if beta else client.messages
    kwargs = dict(
        model=req["model"],
        max_tokens=1024,
        messages=[{"role": "user", "content": req["prompt"]}],
    )
    if beta:
        # The gateway selects the Beta protocol via the ?beta=true query param.
        kwargs["extra_query"] = {"beta": "true"}

    try:
        if not req["stream"]:
            msg = svc.create(**kwargs)
            return normalize_anthropic_message(msg)

        count = 0
        try:
            with svc.stream(**kwargs) as stream:
                for _event in stream:
                    count += 1
                msg = stream.get_final_message()
            resp = normalize_anthropic_message(msg)
            resp["stream_event_count"] = count
            return resp
        except anthropic.APIStatusError:
            raise
        except Exception as e:
            # Mid-stream error event (truncated upstream): the SDK raised; the
            # turn was not completed. Report it in-band rather than crashing.
            return {"http_status": 200, "stream_event_count": count,
                    "raw_body": str(e),
                    "error": {"status": 0, "type": type(e).__name__, "message": str(e)}}
    except anthropic.APIStatusError as e:
        return api_error_response(e)


# ── OpenAI Chat Completions ──────────────────────────────────────────────────


def normalize_chat_completion(resp):
    choice = resp.choices[0]
    msg = choice.message
    tool_calls = []
    for tc in msg.tool_calls or []:
        tool_calls.append(tool_call(tc.id, tc.function.name, tc.function.arguments))
    usage = None
    if resp.usage is not None:
        usage = {
            "input_tokens": resp.usage.prompt_tokens or 0,
            "output_tokens": resp.usage.completion_tokens or 0,
        }
    return {
        "http_status": 200,
        "role": msg.role,
        "content": msg.content or "",
        "model": resp.model,
        "finish_reason": choice.finish_reason or "",
        "thinking": "",
        "tool_calls": tool_calls,
        "usage": usage,
        "raw_body": resp.model_dump_json(),
    }


def run_openai_chat(req, client):
    import openai

    kwargs = dict(
        model=req["model"],
        messages=[{"role": "user", "content": req["prompt"]}],
    )
    try:
        if not req["stream"]:
            return normalize_chat_completion(client.chat.completions.create(**kwargs))

        count = 0
        role = ""
        content = ""
        finish = ""
        model = ""
        # tool calls accumulate by index, like the Go ChatCompletionAccumulator
        tools = {}
        for chunk in client.chat.completions.create(stream=True, **kwargs):
            count += 1
            model = chunk.model or model
            if not chunk.choices:
                continue
            choice = chunk.choices[0]
            delta = choice.delta
            if delta is not None:
                role = delta.role or role
                content += delta.content or ""
                for tc in delta.tool_calls or []:
                    slot = tools.setdefault(tc.index, {"id": "", "name": "", "arguments": ""})
                    if tc.id:
                        slot["id"] = tc.id
                    if tc.function is not None:
                        if tc.function.name:
                            slot["name"] = tc.function.name
                        slot["arguments"] += tc.function.arguments or ""
            finish = choice.finish_reason or finish
        tool_calls = [
            tool_call(t["id"], t["name"], t["arguments"])
            for _, t in sorted(tools.items())
        ]
        return {
            "http_status": 200,
            "role": role,
            "content": content,
            "model": model,
            "finish_reason": finish,
            "thinking": "",
            "tool_calls": tool_calls,
            "usage": None,
            "stream_event_count": count,
            "raw_body": content,
        }
    except openai.APIStatusError as e:
        return api_error_response(e)


# ── OpenAI Responses ─────────────────────────────────────────────────────────


def normalize_response(resp):
    content = ""
    tool_calls = []
    for item in resp.output or []:
        itype = getattr(item, "type", None)
        if itype == "message":
            for part in item.content or []:
                if getattr(part, "type", None) == "output_text":
                    content += part.text
        elif itype == "function_call":
            tool_calls.append(
                tool_call(item.call_id or item.id or "", item.name, item.arguments or "")
            )
    usage = None
    if getattr(resp, "usage", None) is not None:
        usage = {
            "input_tokens": resp.usage.input_tokens or 0,
            "output_tokens": resp.usage.output_tokens or 0,
        }
    return {
        "http_status": 200,
        "role": "assistant",
        "content": content,
        "model": str(resp.model),
        "finish_reason": resp.status or "",
        "thinking": "",
        "tool_calls": tool_calls,
        "usage": usage,
        "raw_body": resp.model_dump_json(),
    }


def run_openai_responses(req, client):
    import openai

    kwargs = dict(model=req["model"], input=req["prompt"])
    try:
        if not req["stream"]:
            return normalize_response(client.responses.create(**kwargs))

        count = 0
        final = None
        delta_text = ""
        for event in client.responses.create(stream=True, **kwargs):
            count += 1
            etype = getattr(event, "type", "")
            if etype in ("response.completed", "response.incomplete"):
                final = event.response
            elif etype == "response.output_text.delta":
                delta_text += event.delta or ""
        if final is not None:
            resp = normalize_response(final)
        else:
            # Stream was cut before a terminal event (midstream-close):
            # report what was accumulated from deltas.
            resp = {
                "http_status": 200,
                "role": "assistant",
                "content": delta_text,
                "model": "",
                "finish_reason": "",
                "thinking": "",
                "tool_calls": [],
                "usage": None,
                "raw_body": delta_text,
            }
        resp["stream_event_count"] = count
        return resp
    except openai.APIStatusError as e:
        return api_error_response(e)


# ── shared ───────────────────────────────────────────────────────────────────


def api_error_response(e):
    """Map an SDK APIStatusError to the in-band error envelope."""
    try:
        raw = e.response.text
    except Exception:
        raw = str(e)
    return {
        "http_status": e.status_code,
        "raw_body": raw,
        "error": {
            "status": e.status_code,
            "type": type(e).__name__,
            "message": str(e),
        },
    }


def main():
    req = json.load(sys.stdin)
    if req.get("version") != 1:
        fail(f"unsupported driver protocol version: {req.get('version')}")

    source = req["source"]
    if source == "anthropic_v1":
        emit(run_anthropic(req, beta=False))
    elif source == "anthropic_beta":
        emit(run_anthropic(req, beta=True))
    elif source in ("openai_chat", "openai_responses"):
        import openai

        client = openai.OpenAI(
            base_url=req["base_url"] + "/tingly/openai/v1",
            api_key=req["api_key"],
            max_retries=0,
            timeout=req.get("timeout_ms", 30000) / 1000.0,
        )
        if source == "openai_chat":
            emit(run_openai_chat(req, client))
        else:
            emit(run_openai_responses(req, client))
    else:
        fail(f"unsupported source protocol: {source}")


if __name__ == "__main__":
    main()
