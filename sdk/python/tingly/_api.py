"""Typed access to tingly-box's control plane.

Every response here is parsed into a model generated from the gateway's own
``openapi.json`` (see ``tingly/_generated/``), so the SDK never guesses at a
field name. That matters more than it sounds: before this layer existed the
usage view read ``payload["data"]`` from an endpoint that returns
``{"total", "requests"}``, and summed ``input_tokens`` off a record type that
has no such field — so it silently reported zeros forever. A wrong key is now
a validation error at the boundary instead of a plausible-looking zero.

Scope: this is the **control plane** (``/api/v1``, ``/api/v2``) only. LLM calls
go through the vendored ``openai`` / ``anthropic`` SDKs against
``/tingly/<scenario>``, which openapi.json does not describe — those are
already generated, by their own vendors, and stay that way.
"""

from __future__ import annotations

from typing import Any, Mapping, Optional, Type, TypeVar

import httpx
from pydantic import BaseModel, ValidationError

from .errors import (
    APIStatusError,
    AuthError,
    GatewayUnreachableError,
    SchemaMismatchError,
)

M = TypeVar("M", bound=BaseModel)


class ControlPlane:
    """A thin, typed httpx wrapper bound to one gateway + admin token.

    Holds a single ``httpx.Client`` so connections are pooled across calls
    rather than reopened per request, which is what the previous
    module-level ``httpx.get`` calls did.
    """

    def __init__(self, base_url: str, admin_token: str, timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self._admin_token = admin_token
        headers = {"Authorization": f"Bearer {admin_token}"} if admin_token else {}
        self._http = httpx.Client(base_url=self.base_url, headers=headers, timeout=timeout)

    # -- core ------------------------------------------------------------

    def request(
        self,
        method: str,
        path: str,
        model: Type[M],
        *,
        params: Optional[Mapping[str, Any]] = None,
        json: Any = None,
    ) -> M:
        """Call ``path`` and parse the body into ``model``.

        ``model`` is any class from :mod:`tingly._generated.models`, so all 195
        of the gateway's operations are reachable with full typing, not just
        the handful this SDK wraps by name:

            from tingly._generated.models import ProvidersResponse
            tb.api.request("GET", "/api/v2/providers", ProvidersResponse)
        """
        try:
            resp = self._http.request(
                method, path, params=self._clean_params(params), json=json
            )
        except httpx.HTTPError as exc:
            raise GatewayUnreachableError(
                f"could not reach tingly-box at {self.base_url}: {exc}"
            ) from exc

        if resp.status_code == 401:
            raise AuthError(
                "tingly-box rejected the admin token. Set TINGLY_BOX_TOKEN or run "
                "`tingly doctor --link`."
            )
        if resp.status_code >= 400:
            raise APIStatusError(method, path, resp.status_code, _safe_json(resp), resp.text)

        try:
            return model.model_validate(resp.json())
        except ValueError as exc:  # ValidationError and JSONDecodeError both
            detail = exc.errors() if isinstance(exc, ValidationError) else str(exc)
            raise SchemaMismatchError(
                f"{method} {path} returned a body that does not match {model.__name__}. "
                f"The gateway may be newer than these generated models — re-run "
                f"`task gen:py`. Detail: {detail}"
            ) from exc

    def get(self, path: str, model: Type[M], **kw: Any) -> M:
        return self.request("GET", path, model, **kw)

    def post(self, path: str, model: Type[M], **kw: Any) -> M:
        return self.request("POST", path, model, **kw)

    # -- helpers ---------------------------------------------------------

    @staticmethod
    def _clean_params(params: Optional[Mapping[str, Any]]) -> Optional[dict]:
        """Drop None-valued query params so they aren't sent as "None"."""
        if not params:
            return None
        return {k: v for k, v in params.items() if v is not None}

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> "ControlPlane":
        return self

    def __exit__(self, *exc: Any) -> None:
        self.close()


def _safe_json(resp: httpx.Response) -> Optional[dict]:
    """Decode a JSON body, returning None instead of raising on garbage."""
    try:
        body = resp.json()
    except ValueError:
        return None
    return body if isinstance(body, dict) else None
