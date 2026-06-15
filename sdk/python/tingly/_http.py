"""Small shared HTTP helpers for the tingly SDK."""

from __future__ import annotations

from typing import Optional

import httpx


def safe_json(resp: httpx.Response) -> Optional[dict]:
    """Parse a JSON body, returning None instead of raising on malformed input."""
    try:
        return resp.json()
    except ValueError:
        return None
