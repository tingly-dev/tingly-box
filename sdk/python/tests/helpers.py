"""Shared test helpers (not a test module: unittest only collects test*.py)."""

import json
import time
import urllib.request
import uuid


def wait_until_serving(srv, timeout: float = 2.0):
    deadline = time.time() + timeout
    while srv._httpd is None:
        if time.time() > deadline:
            raise TimeoutError("server did not start")
        time.sleep(0.01)


def get_json(url: str) -> dict:
    with urllib.request.urlopen(url) as resp:
        return json.loads(resp.read())


def post_json(url: str, body: dict) -> dict:
    request = urllib.request.Request(
        url, data=json.dumps(body).encode(), method="POST", headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(request) as resp:
        return json.loads(resp.read())


def post_multipart(url: str, fields: list[tuple[str, str | bytes]]) -> dict:
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
        url, data=data, method="POST", headers={"Content-Type": f"multipart/form-data; boundary={boundary}"}
    )
    with urllib.request.urlopen(request) as resp:
        return json.loads(resp.read())
