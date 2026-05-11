"""WSGI middleware. Wraps Flask, Django, Pyramid — anything WSGI-compliant."""

from __future__ import annotations

import io
import time
from typing import Any, Callable, Iterable

from .client import Client


class CaptureMiddleware:
    """Capture every inbound request and the response the app returned."""

    def __init__(self, app: Callable, client: Client):
        self.app = app
        self.client = client

    def __call__(self, environ: dict[str, Any], start_response: Callable) -> Iterable[bytes]:
        path = environ.get("PATH_INFO", "")
        if not self.client.route_allowed(path):
            yield from self.app(environ, start_response)
            return

        start = time.perf_counter()

        # Buffer the request body so the downstream app can still read it.
        body = environ.get("wsgi.input")
        try:
            length = int(environ.get("CONTENT_LENGTH") or 0)
        except (TypeError, ValueError):
            length = 0
        req_body = body.read(length) if body and length > 0 else b""
        environ["wsgi.input"] = io.BytesIO(req_body)

        captured: dict[str, Any] = {"status": 0, "headers": []}

        def _start(status: str, headers: list[tuple[str, str]], exc_info=None):  # type: ignore[override]
            captured["status"] = int(status.split(" ", 1)[0])
            captured["headers"] = headers
            return start_response(status, headers, exc_info)

        chunks: list[bytes] = []
        for chunk in self.app(environ, _start):
            chunks.append(chunk)
            yield chunk

        latency_ms = int((time.perf_counter() - start) * 1000)
        res_body = b"".join(chunks)

        self.client.capture({
            "method":      environ.get("REQUEST_METHOD", ""),
            "scheme":      environ.get("wsgi.url_scheme", "http"),
            "host":        environ.get("HTTP_HOST", ""),
            "path":        environ.get("PATH_INFO", ""),
            "query":       environ.get("QUERY_STRING", ""),
            "status":      captured["status"],
            "latency_ms":  latency_ms,
            "req_size":    len(req_body),
            "res_size":    len(res_body),
            "req_headers": _headers_from_env(environ),
            "res_headers": dict(captured["headers"]),
            "req_body":    req_body,
            "res_body":    res_body,
            "client_ip":   environ.get("REMOTE_ADDR", ""),
            "user_agent":  environ.get("HTTP_USER_AGENT", ""),
            "trace_id":    environ.get("HTTP_TRACEPARENT", ""),
            "direction":   "inbound",
        })


def _headers_from_env(environ: dict[str, Any]) -> dict[str, str]:
    out: dict[str, str] = {}
    for k, v in environ.items():
        if k.startswith("HTTP_"):
            out[k[5:].replace("_", "-").title()] = v
    if "CONTENT_TYPE" in environ:
        out["Content-Type"] = environ["CONTENT_TYPE"]
    if "CONTENT_LENGTH" in environ:
        out["Content-Length"] = environ["CONTENT_LENGTH"]
    return out
