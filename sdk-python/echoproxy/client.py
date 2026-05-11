"""Buffered, thread-safe ingest client."""

from __future__ import annotations

import base64
import os
import re
import secrets
import threading
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Any

import httpx

from .redact import Redactor

VERSION = "0.1.0"
SOURCE = "sdk-python"
DIRECTION_INBOUND = "inbound"
DIRECTION_OUTBOUND = "outbound"


def _env_routes(name: str) -> list[str]:
    raw = os.environ.get(name, "")
    return [p.strip() for p in raw.split(",") if p.strip()]


@dataclass
class Config:
    api_key: str = field(default_factory=lambda: os.environ.get("ECHOPROXY_API_KEY", ""))
    endpoint: str = field(default_factory=lambda: os.environ.get("ECHOPROXY_ENDPOINT", "http://localhost:8081"))
    buffer_size: int = 10_000
    batch_size: int = 500
    flush_interval: float = 2.0
    max_body_bytes: int = 64 * 1024
    sample_rate: float = 1.0
    redactor: Redactor | None = None

    # Route filter for inbound middleware. Empty include = allow all;
    # ignore wins over include. Falls back to env if both unset.
    capture_routes: list[str] = field(default_factory=lambda: _env_routes("ECHOPROXY_CAPTURE_ROUTES"))
    ignore_routes: list[str] = field(default_factory=lambda: _env_routes("ECHOPROXY_IGNORE_ROUTES"))


class Client:
    """Captures events to an in-memory ring buffer, flushed by a background thread."""

    def __init__(self, config: Config | None = None):
        self.cfg = config or Config()
        self._buf: deque[dict[str, Any]] = deque(maxlen=self.cfg.buffer_size)
        self._lock = threading.Lock()
        self._stop = threading.Event()
        self._dropped = 0
        self._redactor = self.cfg.redactor or Redactor()
        self._http = httpx.Client(timeout=5.0)
        self._capture_re = [re.compile(p) for p in (self.cfg.capture_routes or [])]
        self._ignore_re = [re.compile(p) for p in (self.cfg.ignore_routes or [])]

        if self.cfg.api_key:
            self._thread = threading.Thread(target=self._flush_loop, daemon=True)
            self._thread.start()
        else:
            self._thread = None

    def capture(self, event: dict[str, Any]) -> None:
        if not self.cfg.api_key:
            return
        if self.cfg.sample_rate < 1.0 and secrets.randbelow(1_000_000) >= int(self.cfg.sample_rate * 1_000_000):
            return
        normalized = self._normalize(event)
        with self._lock:
            if len(self._buf) >= self.cfg.buffer_size:
                self._buf.popleft()
                self._dropped += 1
            self._buf.append(normalized)
            should_flush = len(self._buf) >= self.cfg.batch_size
        if should_flush:
            self.flush()

    def route_allowed(self, path: str) -> bool:
        """Apply CaptureRoutes / IgnoreRoutes filters. Empty include = all."""
        for r in self._ignore_re:
            if r.search(path):
                return False
        if not self._capture_re:
            return True
        return any(r.search(path) for r in self._capture_re)

    def flush(self) -> None:
        with self._lock:
            if not self._buf:
                return
            batch = list(self._buf)
            self._buf.clear()
        try:
            self._http.post(
                f"{self.cfg.endpoint.rstrip('/')}/v1/events:batch",
                headers={"X-Echo-Key": self.cfg.api_key, "Content-Type": "application/json"},
                json={"events": batch},
            )
        except (httpx.HTTPError, ValueError):
            # Fail open: drop the batch on transport errors.
            with self._lock:
                self._dropped += len(batch)

    def close(self) -> None:
        self._stop.set()
        self.flush()
        self._http.close()

    @property
    def dropped(self) -> int:
        return self._dropped

    def _flush_loop(self) -> None:
        while not self._stop.wait(self.cfg.flush_interval):
            self.flush()

    def _normalize(self, event: dict[str, Any]) -> dict[str, Any]:
        event.setdefault("source", SOURCE)
        event.setdefault("sdk_version", VERSION)
        event.setdefault("event_id", secrets.token_hex(16))
        event.setdefault("timestamp_ns", int(time.time_ns()))

        req_headers = event.get("req_headers") or {}
        res_headers = event.get("res_headers") or {}
        event["req_headers"] = self._redactor.headers(dict(req_headers))
        event["res_headers"] = self._redactor.headers(dict(res_headers))

        req_ct = event["req_headers"].get("Content-Type", "")
        res_ct = event["res_headers"].get("Content-Type", "")
        event["req_body"] = self._cap_redact(event.get("req_body") or b"", req_ct)
        event["res_body"] = self._cap_redact(event.get("res_body") or b"", res_ct)
        return event

    def _cap_redact(self, body: bytes | str, content_type: str) -> str:
        raw = body.encode() if isinstance(body, str) else body
        if not raw:
            return ""
        if len(raw) > self.cfg.max_body_bytes:
            raw = raw[: self.cfg.max_body_bytes]
        scrubbed = self._redactor.body(raw, content_type)
        return base64.b64encode(scrubbed).decode("ascii")
