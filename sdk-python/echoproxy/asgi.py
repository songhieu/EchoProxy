"""ASGI middleware. Works with FastAPI, Starlette, Django ASGI."""

from __future__ import annotations

import time
from typing import Any, Awaitable, Callable

from .client import Client

ASGIApp = Callable[[dict[str, Any], Callable, Callable], Awaitable[None]]


class CaptureMiddleware:
    """ASGI middleware: tees request/response through to capture both bodies."""

    def __init__(self, app: ASGIApp, client: Client):
        self.app = app
        self.client = client

    async def __call__(self, scope: dict[str, Any], receive: Callable, send: Callable) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        if not self.client.route_allowed(scope.get("path", "")):
            await self.app(scope, receive, send)
            return

        start = time.perf_counter()
        req_body = bytearray()
        res_body = bytearray()
        status_holder: dict[str, Any] = {"status": 0, "headers": []}

        async def _receive() -> dict[str, Any]:
            message = await receive()
            if message["type"] == "http.request":
                req_body.extend(message.get("body", b""))
            return message

        async def _send(message: dict[str, Any]) -> None:
            if message["type"] == "http.response.start":
                status_holder["status"] = message["status"]
                status_holder["headers"] = [
                    (k.decode("latin-1"), v.decode("latin-1")) for k, v in message.get("headers", [])
                ]
            elif message["type"] == "http.response.body":
                res_body.extend(message.get("body", b""))
            await send(message)

        await self.app(scope, _receive, _send)

        latency_ms = int((time.perf_counter() - start) * 1000)
        client_ip = ""
        if scope.get("client"):
            client_ip = scope["client"][0]
        host = ""
        for k, v in scope.get("headers", []):
            if k == b"host":
                host = v.decode("latin-1")
                break

        req_headers = {k.decode("latin-1").title(): v.decode("latin-1") for k, v in scope.get("headers", [])}

        self.client.capture({
            "method":      scope.get("method", ""),
            "scheme":      scope.get("scheme", "http"),
            "host":        host,
            "path":        scope.get("path", ""),
            "query":       scope.get("query_string", b"").decode("latin-1"),
            "status":      status_holder["status"],
            "latency_ms":  latency_ms,
            "req_size":    len(req_body),
            "res_size":    len(res_body),
            "req_headers": req_headers,
            "res_headers": dict(status_holder["headers"]),
            "req_body":    bytes(req_body),
            "res_body":    bytes(res_body),
            "client_ip":   client_ip,
            "user_agent":  req_headers.get("User-Agent", ""),
            "trace_id":    req_headers.get("Traceparent", ""),
            "direction":   "inbound",
        })
