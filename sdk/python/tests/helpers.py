"""Shared test helpers (not a test module: unittest only collects test*.py)."""

import json
import threading
import time
import urllib.request
import uuid


def wait_until_serving(srv, timeout: float = 2.0):
    deadline = time.time() + timeout
    while srv._httpd is None:
        if time.time() > deadline:
            raise TimeoutError("server did not start")
        time.sleep(0.01)


def serve_sugar_in_background() -> str:
    """Run tingly.serve() for whatever is registered; return its base URL."""
    import tingly
    from tingly import sugar

    threading.Thread(target=tingly.serve, kwargs={"host": "127.0.0.1", "port": 0}, daemon=True).start()
    wait_until_serving(sugar._server)
    return f"http://127.0.0.1:{sugar._server._httpd.server_address[1]}"


def stop_sugar():
    from tingly import sugar

    sugar._server._httpd.shutdown()
    sugar._reset()


def get_json(url: str, headers: dict | None = None) -> dict:
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers or {})) as resp:
        return json.loads(resp.read())


def post_json(url: str, body: dict, headers: dict | None = None) -> dict:
    request = urllib.request.Request(
        url, data=json.dumps(body).encode(), method="POST",
        headers={"Content-Type": "application/json", **(headers or {})},
    )
    with urllib.request.urlopen(request) as resp:
        return json.loads(resp.read())


def post_multipart(url: str, fields: list[tuple[str, str | bytes]], headers: dict | None = None) -> dict:
    """POST multipart/form-data the way openai-go does for /images/edits: str
    values as plain fields, bytes values as file parts."""
    boundary = uuid.uuid4().hex
    parts = []
    for name, value in fields:
        if isinstance(value, bytes):
            header = (
                f'Content-Disposition: form-data; name="{name}"; filename="{name}.png"\r\n'
                "Content-Type: image/png\r\n\r\n"
            )
            parts.append(f"--{boundary}\r\n{header}".encode() + value + b"\r\n")
        else:
            header = f'Content-Disposition: form-data; name="{name}"\r\n\r\n'
            parts.append(f"--{boundary}\r\n{header}{value}\r\n".encode())
    data = b"".join(parts) + f"--{boundary}--\r\n".encode()
    request = urllib.request.Request(
        url, data=data, method="POST",
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}", **(headers or {})},
    )
    with urllib.request.urlopen(request) as resp:
        return json.loads(resp.read())


def post_sse(url: str, body: dict, headers: dict | None = None) -> tuple[str, list[tuple[str | None, object]]]:
    """POST a JSON body and read back a text/event-stream reply as
    (content type, [(event name or None, parsed data)]). A `data:` line that
    isn't JSON (OpenAI's `[DONE]`) stays a string."""
    request = urllib.request.Request(
        url, data=json.dumps(body).encode(), method="POST",
        headers={"Content-Type": "application/json", **(headers or {})},
    )
    with urllib.request.urlopen(request) as resp:
        content_type, raw = resp.headers.get("Content-Type", ""), resp.read().decode()
    events: list[tuple[str | None, object]] = []
    for block in raw.split("\n\n"):
        if not block.strip():
            continue
        event, data = None, ""
        for line in block.split("\n"):
            field, _, value = line.partition(":")
            value = value[1:] if value.startswith(" ") else value  # per the SSE spec, one optional space
            if field == "event":
                event = value
            elif field == "data":
                data = value
        try:
            events.append((event, json.loads(data)))
        except ValueError:
            events.append((event, data))
    return content_type, events
