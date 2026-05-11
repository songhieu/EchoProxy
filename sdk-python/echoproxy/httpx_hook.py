"""httpx event hooks: capture every outbound HTTP call.

Usage:
    import httpx, echoproxy
    client = echoproxy.Client()
    httpx_client = httpx.Client(event_hooks=echoproxy.httpx_hook.hooks(client))
"""

from __future__ import annotations

import time
from typing import Any, Callable

import httpx

from .client import Client


def hooks(client: Client) -> dict[str, list[Callable[..., Any]]]:
    state: dict[str, Any] = {}

    def on_request(request: httpx.Request) -> None:
        state[id(request)] = {"start": time.perf_counter(), "body": request.content}

    def on_response(response: httpx.Response) -> None:
        ctx = state.pop(id(response.request), None)
        if ctx is None:
            return
        latency_ms = int((time.perf_counter() - ctx["start"]) * 1000)
        try:
            response.read()
        except httpx.StreamError:
            pass
        client.capture({
            "method":      response.request.method,
            "scheme":      response.request.url.scheme,
            "host":        response.request.url.host,
            "path":        response.request.url.path,
            "query":       str(response.request.url.query.decode() if isinstance(response.request.url.query, bytes) else response.request.url.query),
            "status":      response.status_code,
            "latency_ms":  latency_ms,
            "req_size":    len(ctx["body"] or b""),
            "res_size":    len(response.content),
            "req_headers": dict(response.request.headers),
            "res_headers": dict(response.headers),
            "req_body":    ctx["body"] or b"",
            "res_body":    response.content,
            "direction":   "outbound",
        })

    return {"request": [on_request], "response": [on_response]}
