#!/usr/bin/env python3
"""An image provider in a few lines, via the sugar layer.

The "model" here is fake — it renders a solid-colour PNG (colour picked from
the prompt) with nothing but the stdlib, so this runs anywhere. To serve a
real model, replace the body of `generate`, e.g. with a diffusers pipeline:

    pipe = DiffusionPipeline.from_pretrained("Qwen/Qwen-Image").to("cuda")
    ...
    return pipe(prompt).images[0]        # a PIL image — returned as-is

Register it with tb: Connect AI -> Self-hosted -> Custom endpoint, OpenAI,
http://localhost:8765/v1, no key; then point an `imagegen` rule at that
provider and the model `fake-image`.

Run:
    python image.py
"""

import hashlib
import os
import struct
import sys
import zlib

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import tingly  # noqa: E402


@tingly.image("fake-image")
def generate(prompt, size="256x256"):
    width, height = (int(v) for v in size.split("x"))
    return solid_png(width, height, hashlib.sha256(prompt.encode()).digest()[:3])


def solid_png(width: int, height: int, rgb: bytes) -> bytes:
    """A width x height PNG filled with one RGB colour."""
    def chunk(kind: bytes, data: bytes) -> bytes:
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    rows = (b"\x00" + rgb * width) * height  # filter byte 0 per row, then RGB pixels
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(rows))
        + chunk(b"IEND", b"")
    )


if __name__ == "__main__":
    tingly.serve(port=int(os.environ.get("PORT", 8765)))
