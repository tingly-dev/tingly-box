#!/usr/bin/env python3
"""Generate or edit images through a Tingly-Box (or any OpenAI-compatible) endpoint.

Stdlib only. Subcommands:

    image.py models
    image.py generate --prompt "..." [--model M] [--size 1024x1024] [--n 1] [--out DIR]
    image.py edit --prompt "..." --image in.png [--image ref.png] [--mask m.png] [--out DIR]

Endpoint / token resolution (first hit wins):

    1. --base-url / --token
    2. TINGLY_IMAGE_BASE_URL / TINGLY_IMAGE_TOKEN
    3. The agent's own Tingly-Box connection: ANTHROPIC_BASE_URL (+ ANTHROPIC_AUTH_TOKEN
       or ANTHROPIC_API_KEY) or OPENAI_BASE_URL (+ OPENAI_API_KEY), when the URL points at
       a Tingly-Box scenario (/tingly/<scenario>). The scenario is swapped for `imagegen`,
       because agent scenarios such as claude_code do not serve /images/*.

Nothing else is guessed. When the endpoint, token or model is missing or rejected the
script exits with status 2 and a line starting with NEED_INPUT: — the caller should ask
the user for the value and re-run with the matching flag.
"""

import argparse
import base64
import json
import mimetypes
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

EXIT_NEED_INPUT = 2
IMAGE_SCENARIO = "imagegen"
TINGLY_SCENARIO_RE = re.compile(r"/tingly/[^/]+")


class NeedInput(Exception):
    """A value only the user can supply is missing or wrong."""


def need_input(what, hint):
    raise NeedInput(f"NEED_INPUT: {what} — {hint}")


# ---------------------------------------------------------------- resolution


def normalize_base(url):
    """Explicit URL: a bare Tingly-Box host gets /tingly/imagegen; anything with a
    path is used as-is (an OpenAI-compatible base such as https://api.openai.com/v1)."""
    url = url.strip().rstrip("/")
    parsed = urllib.parse.urlparse(url)
    if not parsed.scheme or not parsed.netloc:
        need_input("base URL", f"'{url}' is not a valid http(s) URL; ask the user for the endpoint")
    if parsed.path in ("", "/"):
        return f"{url}/tingly/{IMAGE_SCENARIO}"
    return url


def from_agent_env():
    """Reuse the agent's Tingly-Box connection. Only Tingly-Box URLs qualify: a direct
    Anthropic endpoint has no image API, so borrowing it would only fail later."""
    pairs = (
        ("ANTHROPIC_BASE_URL", ("ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY")),
        ("OPENAI_BASE_URL", ("OPENAI_API_KEY",)),
    )
    for url_var, token_vars in pairs:
        url = os.environ.get(url_var, "").strip().rstrip("/")
        if not url or not TINGLY_SCENARIO_RE.search(url):
            continue
        base = TINGLY_SCENARIO_RE.sub(f"/tingly/{IMAGE_SCENARIO}", url, count=1)
        base = re.sub(r"/v1$", "", base)
        token = next((os.environ[v] for v in token_vars if os.environ.get(v)), "")
        return base, token, url_var
    return None


def resolve(args):
    base = args.base_url or os.environ.get("TINGLY_IMAGE_BASE_URL", "")
    token = args.token or os.environ.get("TINGLY_IMAGE_TOKEN", "")
    source = "flag/TINGLY_IMAGE_BASE_URL"
    if base:
        base = normalize_base(base)
    else:
        derived = from_agent_env()
        if derived is None:
            need_input(
                "base URL",
                "no Tingly-Box endpoint found in --base-url, TINGLY_IMAGE_BASE_URL, "
                "ANTHROPIC_BASE_URL or OPENAI_BASE_URL; ask the user for it "
                "(e.g. http://localhost:12580) and pass --base-url",
            )
        base, env_token, source = derived
        token = token or env_token
    if not token:
        need_input("token", f"no token for {base}; ask the user for their Tingly-Box model token and pass --token")
    return base, token, source


# ---------------------------------------------------------------- http


def request(method, url, token, body=None, timeout=300):
    data = None
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read() or b"{}")
    except urllib.error.HTTPError as e:
        detail = error_message(e.read())
        if e.code in (401, 403):
            need_input("token", f"{url} rejected the token ({e.code}: {detail}); ask the user for a valid one")
        if e.code == 404:
            need_input("base URL", f"{url} not found; ask the user to confirm the endpoint")
        if "scenario" in detail and ("invalid" in detail or "does not support" in detail):
            need_input("base URL", f"{detail}; the URL must point at an image-capable scenario such as /tingly/{IMAGE_SCENARIO}")
        if "model" in detail.lower() and e.code in (400, 422):
            need_input("model", f"{detail}; run `models` and ask the user which one to use")
        sys.exit(f"error: HTTP {e.code} from {url}: {detail}")
    except urllib.error.URLError as e:
        need_input("base URL", f"cannot reach {url} ({e.reason}); ask the user whether Tingly-Box is running and the URL is right")


def error_message(raw):
    try:
        payload = json.loads(raw)
    except ValueError:
        return raw.decode(errors="replace").strip()[:500]
    err = payload.get("error", payload)
    if isinstance(err, dict):
        return str(err.get("message") or err)
    return str(err)


def list_models(base, token):
    payload = request("GET", f"{base}/v1/models", token, timeout=30)
    return [m.get("id") for m in payload.get("data", []) if m.get("id")]


def pick_model(args, base, token):
    model = args.model or os.environ.get("TINGLY_IMAGE_MODEL", "")
    if model:
        return model
    models = list_models(base, token)
    if len(models) == 1:
        return models[0]
    if not models:
        need_input("model", f"{base} exposes no models; ask the user to add an image rule in Tingly-Box or pass --model")
    need_input("model", "several models available: " + ", ".join(models) + "; ask the user which one and pass --model")


# ---------------------------------------------------------------- images


def data_url(path):
    if not os.path.isfile(path):
        sys.exit(f"error: image not found: {path}")
    mime = mimetypes.guess_type(path)[0] or "image/png"
    with open(path, "rb") as f:
        return f"data:{mime};base64,{base64.b64encode(f.read()).decode()}"


def slug(text):
    s = re.sub(r"[^a-zA-Z0-9]+", "-", text).strip("-").lower()
    return s[:40] or "image"


def rel_path(path):
    """Path relative to the working directory the agent ran the script from;
    falls back to absolute when there is none (e.g. another drive on Windows)."""
    try:
        return os.path.relpath(path)
    except ValueError:
        return os.path.abspath(path)


def save_images(payload, out_dir, prompt, ext):
    os.makedirs(out_dir, exist_ok=True)
    stamp = time.strftime("%Y%m%d-%H%M%S")
    saved = []
    for i, item in enumerate(payload.get("data") or []):
        path = os.path.join(out_dir, f"{stamp}-{slug(prompt)}-{i + 1}.{ext}")
        if item.get("b64_json"):
            blob = base64.b64decode(item["b64_json"])
        elif item.get("url"):
            with urllib.request.urlopen(item["url"], timeout=120) as resp:
                blob = resp.read()
        else:
            continue
        if not blob:
            sys.exit(f"error: image {i + 1} in the response is empty")
        with open(path, "wb") as f:
            f.write(blob)
        if not os.path.isfile(path) or os.path.getsize(path) == 0:
            sys.exit(f"error: failed to write {path}")
        entry = {"path": rel_path(path), "abs_path": os.path.abspath(path), "bytes": len(blob)}
        if item.get("revised_prompt"):
            entry["revised_prompt"] = item["revised_prompt"]
        saved.append(entry)
    if not saved:
        sys.exit("error: response contained no images: " + json.dumps(payload)[:500])
    return saved


def common_body(args, model):
    body = {"model": model, "prompt": args.prompt}
    for key in ("size", "quality", "background", "output_format"):
        value = getattr(args, key)
        if value:
            body[key] = value
    if args.n:
        body["n"] = args.n
    return body


def cmd_models(args):
    base, token, source = resolve(args)
    print(json.dumps({"endpoint": base, "source": source, "models": list_models(base, token)}, indent=2))


def cmd_generate(args):
    base, token, source = resolve(args)
    model = pick_model(args, base, token)
    payload = request("POST", f"{base}/v1/images/generations", token, common_body(args, model), args.timeout)
    report(base, source, model, save_images(payload, args.out, args.prompt, args.output_format or "png"))


def cmd_edit(args):
    base, token, source = resolve(args)
    model = pick_model(args, base, token)
    body = common_body(args, model)
    images = [data_url(p) for p in args.image]
    body["image"] = images[0] if len(images) == 1 else images
    if args.mask:
        body["mask"] = data_url(args.mask)
    payload = request("POST", f"{base}/v1/images/edits", token, body, args.timeout)
    report(base, source, model, save_images(payload, args.out, args.prompt, args.output_format or "png"))


def report(base, source, model, saved):
    out = {"endpoint": base, "source": source, "model": model, "cwd": os.getcwd(), "images": saved}
    print(json.dumps(out, indent=2))


# ---------------------------------------------------------------- cli


def main():
    parser = argparse.ArgumentParser(description="Generate or edit images via Tingly-Box.")
    parser.add_argument("--base-url", help="endpoint, e.g. http://localhost:12580 or http://host:12580/tingly/imagegen")
    parser.add_argument("--token", help="Tingly-Box model token (or provider API key)")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("models", help="list models the endpoint serves")

    def image_args(p):
        p.add_argument("--prompt", required=True)
        p.add_argument("--model")
        p.add_argument("--size", help="e.g. 1024x1024, 1536x1024, auto")
        p.add_argument("--quality", help="low | medium | high | auto")
        p.add_argument("--background", help="transparent | opaque | auto")
        p.add_argument("--output-format", dest="output_format", help="png | jpeg | webp")
        p.add_argument("--n", type=int)
        p.add_argument("--out", default="generated-images", help="output directory (default ./generated-images)")
        p.add_argument("--timeout", type=int, default=300)

    image_args(sub.add_parser("generate", help="text to image"))
    edit = sub.add_parser("edit", help="edit existing image(s)")
    image_args(edit)
    edit.add_argument("--image", action="append", required=True, help="input image path; repeat for references")
    edit.add_argument("--mask", help="PNG mask; transparent pixels mark the area to change")

    args = parser.parse_args()
    try:
        {"models": cmd_models, "generate": cmd_generate, "edit": cmd_edit}[args.cmd](args)
    except NeedInput as e:
        print(str(e), file=sys.stderr)
        sys.exit(EXIT_NEED_INPUT)


if __name__ == "__main__":
    main()
