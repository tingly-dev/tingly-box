#!/usr/bin/env python3
"""Mask what is private in a `tb vmodel --capture` directory, in place.

A capture is verbatim on purpose, so next to the wire facts it holds whatever
the client embedded about its environment. For Claude Code that is the working
directory and home path, and the `<system-reminder>` blocks of its first user
message: the skills list, agent types, every CLAUDE.md found up the directory
tree, git/attribution settings. This masks exactly that and nothing else, so
the example still shows the client's own system prompt, tool descriptions,
headers and body as sent:

  * absolute paths under /home/<user> and /Users/<user> keep their depth but
    lose the user and everything below it:  /home/user/[REDACTED]
  * a `<system-reminder>` text block is kept only if it is one of the known
    harmless kinds (environment, model name, token budget, date); any other one
    keeps its first line and its closing tag, its content becomes [REDACTED]:

        <system-reminder>\\nThe following skills are available for use with the Skill tool:\\n[REDACTED]\\n</system-reminder>

  * Authorization / x-api-key / Proxy-Authorization / Cookie values keep their
    scheme and token kind:  Bearer sk-ant-oat01-[REDACTED]

Header names, casing and order and the body's JSON structure and key order are
untouched; Content-Length and the index.jsonl byte ranges are recomputed.
raw/conn-N.out (what the server answered) and timestamps are left as they were.
meta.json gets "redacted": true. The result is NOT a faithful capture any more:
the body changed, so a `cch=` hash in it no longer verifies. Redact a copy,
never the original, and still read the result before sharing it.

    python3 redact.py <capture-dir>
"""
import json
import os
import re
import sys

MARK = "[REDACTED]"
SECRET_HEADERS = {"authorization", "x-api-key", "proxy-authorization", "cookie"}
# /home/alice/project/x  ->  /home/user/[REDACTED]
HOME_PATH = re.compile(r"/(home|Users)/[^\s\"\\`'<>:;,)]+")
# Claude Code's per-project state dirs encode the cwd with dashes: -home-alice-project
DASH_PATH = re.compile(r"-(home|Users)-[A-Za-z0-9_.\-]+")
# Reminder kinds that carry nothing private; matched against the text after the tag.
HARMLESS = (
    "# Environment\n",
    "You are powered by the model named",
    "<total_tokens>",
    "Today's date is",
)
REMINDER = "<system-reminder>"
REMINDER_END = "</system-reminder>"


class Redactor:
    def path(self, s):
        s = HOME_PATH.sub(lambda m: "/%s/user/%s" % (m.group(1), MARK), s)
        return DASH_PATH.sub(lambda m: "-%s-user-%s" % (m.group(1), MARK), s)

    def secret(self, value):
        scheme, _, token = value.partition(" ")
        if not token:
            scheme, token = "", value
        kind = token[:13]  # sk-ant-oat01- / sk-ant-api03-
        return (scheme + " " if scheme else "") + kind + MARK

    def reminder(self, text):
        if not (text.startswith(REMINDER) and text.rstrip().endswith(REMINDER_END)):
            return text
        inner = text[len(REMINDER):].lstrip("\n")
        if inner.startswith(HARMLESS):
            return text
        first = inner.split("\n", 1)[0]
        return "%s\n%s\n%s\n%s" % (REMINDER, first, MARK, REMINDER_END)

    def walk(self, node):
        if isinstance(node, dict):
            if node.get("type") == "text" and isinstance(node.get("text"), str):
                node = dict(node, text=self.reminder(node["text"]))
            return {k: self.walk(v) for k, v in node.items()}
        if isinstance(node, list):
            return [self.walk(v) for v in node]
        if isinstance(node, str):
            return self.path(node)
        return node

    def request(self, raw):
        head, sep, body = raw.partition(b"\r\n\r\n")
        lines = head.decode("latin-1").split("\r\n")
        out = [self.path(lines[0])]
        clen = None
        for line in lines[1:]:
            name, colon, value = line.partition(":")
            if not colon:
                out.append(line)
                continue
            value = value.strip()
            value = self.secret(value) if name.lower() in SECRET_HEADERS else self.path(value)
            if name.lower() == "content-length":
                clen = len(out)
            out.append("%s: %s" % (name, value))
        try:
            doc = json.loads(body)
        except ValueError:
            body = self.path(body.decode("latin-1")).encode("latin-1")
        else:
            compact = json.dumps(doc, ensure_ascii=False, separators=(",", ":")).encode()
            if compact != body:
                sys.stderr.write("warning: body is not byte-identical after re-serialising "
                                 "(formatting differs from the original)\n")
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
