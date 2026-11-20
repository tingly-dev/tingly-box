#!/usr/bin/env python3
"""Redact a `tb vmodel --capture` directory in place so it can be shared.

A capture is verbatim on purpose, so it holds whatever the client put on the
wire: credentials, device and session ids, local paths, and the client's own
long prompt/tool text. This rewrites the request side (raw/conn-N.in) with
these rules and fixes up what depends on lengths (Content-Length, index.jsonl
byte ranges), leaving header names, casing and order, and the body's JSON
structure and key order untouched:

  * Authorization / x-api-key / Proxy-Authorization / Cookie values keep only
    their scheme and token-kind prefix:  Bearer sk-ant-oat01-[redacted]
  * every UUID becomes 00000000-0000-4000-8000-0000000000NN, numbered by first
    appearance, so "same value" relationships (a prompt id in a header and in
    the billing header) still show;
  * 64-hex ids (device_id) become zeros; `cch=<5 hex>` becomes `cch=xxxxx`
    (it is a hash of the body, which no longer matches);
  * absolute paths under /home, /Users, /root, /dev/shm, /tmp become /work
    (cwd) or /home/user;
  * every JSON string longer than 120 characters is cut to its first 60
    characters plus "…[redacted N chars]": that is the client's static system
    prompt and tool descriptions, which bulk the capture up and are not ours.
    Two short wire facts are exempt: the `x-anthropic-billing-header: …` system
    block and `metadata.user_id` (their ids are already redacted above).

raw/conn-N.out (what the server answered) is left alone. meta.json gets
"redacted": true. The result is NOT a faithful capture any more (cch cannot be
verified from it); redact a copy, never the original.

    python3 redact.py <capture-dir>
"""
import json
import os
import re
import sys

SECRET_HEADERS = {"authorization", "x-api-key", "proxy-authorization", "cookie"}
UUID = re.compile(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")
HEX64 = re.compile(r"\b[0-9a-fA-F]{64}\b")
CCH = re.compile(r"cch=[0-9a-f]{5}")
PATH = re.compile(r"/(?:home|Users|root|dev/shm|tmp)/[^\s\"\\`']*")
LONG = 120
KEEP = 60


class Redactor:
    def __init__(self):
        self.uuids = {}

    def uuid(self, m):
        key = m.group(0).lower()
        if key not in self.uuids:
            self.uuids[key] = "00000000-0000-4000-8000-%012d" % (len(self.uuids) + 1)
        return self.uuids[key]

    def text(self, s):
        s = UUID.sub(self.uuid, s)
        s = HEX64.sub("0" * 64, s)
        s = CCH.sub("cch=xxxxx", s)
        s = PATH.sub(lambda m: "/work" if m.group(0).rstrip("/").endswith("work") else "/home/user", s)
        return s

    def secret(self, value):
        scheme, _, token = value.partition(" ")
        if not token:
            scheme, token = "", value
        kind = token[:13]  # sk-ant-oat01- / sk-ant-api03-
        return (scheme + " " if scheme else "") + kind + "[redacted]"

    def walk(self, node, key=None):
        if isinstance(node, dict):
            return {k: self.walk(v, k) for k, v in node.items()}
        if isinstance(node, list):
            return [self.walk(v) for v in node]
        if isinstance(node, str):
            node = self.text(node)
            if key == "user_id" or node.startswith("x-anthropic-billing-header:"):
                return node
            if len(node) > LONG:
                return "%s…[redacted %d chars]" % (node[:KEEP], len(node))
        return node

    def request(self, raw):
        head, sep, body = raw.partition(b"\r\n\r\n")
        lines = head.decode("latin-1").split("\r\n")
        out = [self.text(lines[0])]
        clen = None
        for i, line in enumerate(lines[1:], 1):
            name, colon, value = line.partition(":")
            if not colon:
                out.append(line)
                continue
            value = value.strip()
            if name.lower() in SECRET_HEADERS:
                value = self.secret(value)
            else:
                value = self.text(value)
            if name.lower() == "content-length":
                clen = len(out)
            out.append("%s: %s" % (name, value))
        try:
            doc = json.loads(body)
        except ValueError:
            body = self.text(body.decode("latin-1")).encode("latin-1")
        else:
            body = json.dumps(self.walk(doc), ensure_ascii=False, separators=(",", ":")).encode()
        if clen is not None:
            out[clen] = "%s: %d" % (out[clen].split(":")[0], len(body))
        return "\r\n".join(out).encode("latin-1") + sep + body


def main(directory):
    index_path = os.path.join(directory, "index.jsonl")
    entries = [json.loads(line) for line in open(index_path) if line.strip()]
    redactor = Redactor()
    by_conn = {}
    for e in entries:
        by_conn.setdefault(e["conn"], []).append(e)
    for conn, items in by_conn.items():
        path = os.path.join(directory, "raw", "conn-%04d.in" % conn)
        old = open(path, "rb").read()
        new = b""
        for e in sorted(items, key=lambda e: e["in"][0]):
            chunk = old[e["in"][0]:e["in"][1]]
            if not e.get("unparsed"):
                chunk = redactor.request(chunk)
            e["in"] = [len(new), len(new) + len(chunk)]
            new += chunk
        open(path, "wb").write(new)
    with open(index_path, "w") as f:
        for e in sorted(entries, key=lambda e: (e.get("seq") or 0, e["conn"])):
            f.write(json.dumps(e, separators=(",", ":")) + "\n")
    meta_path = os.path.join(directory, "meta.json")
    meta = json.load(open(meta_path))
    meta["redacted"] = True
    json.dump(meta, open(meta_path, "w"), indent=2)
    open(meta_path, "a").write("\n")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
