"""Defense-in-depth scrubber matching pkg/redact (Go) and sdk-laravel (PHP)."""

from __future__ import annotations

import json
import re
from typing import Any, Iterable

MASK = "[REDACTED]"

DEFAULT_HEADERS: tuple[str, ...] = (
    "authorization",
    "proxy-authorization",
    "cookie",
    "set-cookie",
    "x-api-key",
    "x-auth-token",
    "x-csrf-token",
    "x-xsrf-token",
    "x-session-token",
    "x-access-token",
    "x-echo-key",
    "x-forwarded-authorization",
    "www-authenticate",
)

DEFAULT_JSON_FIELDS: tuple[str, ...] = (
    "password", "passwd", "pwd",
    "secret", "client_secret",
    "token", "access_token", "refresh_token", "id_token", "session_token",
    "api_key", "apikey", "auth_token", "authorization",
    "private_key", "privatekey",
    "credit_card", "cardnumber", "card_number", "cvv", "cvc",
    "ssn",
)

_PATTERNS: tuple[re.Pattern[str], ...] = (
    re.compile(r"eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}"),
    re.compile(r"Bearer\s+[A-Za-z0-9._\-]{20,}", re.IGNORECASE),
    re.compile(r"AKIA[0-9A-Z]{16}"),
    re.compile(r"gh[pousr]_[A-Za-z0-9]{36,}"),
    re.compile(r"sk_live_[A-Za-z0-9]{20,}"),
    re.compile(r"sk_test_[A-Za-z0-9]{20,}"),
    re.compile(r"AIza[0-9A-Za-z_\-]{35}"),
    re.compile(r"xox[baprs]-[A-Za-z0-9\-]{10,}"),
)
_CC_PATTERN = re.compile(r"\b(?:\d[ -]*?){13,16}\b")


class Redactor:
    """Apply header + body + regex scrubbing rules."""

    def __init__(
        self,
        extra_headers: Iterable[str] = (),
        extra_json_fields: Iterable[str] = (),
        disable_defaults: bool = False,
    ):
        base_h = () if disable_defaults else DEFAULT_HEADERS
        base_j = () if disable_defaults else DEFAULT_JSON_FIELDS
        self._headers = {h.lower() for h in (*base_h, *extra_headers)}
        self._json = {f.lower() for f in (*base_j, *extra_json_fields)}
        self._mask_cards = not disable_defaults

    def headers(self, headers: dict[str, str]) -> dict[str, str]:
        return {k: (MASK if k.lower() in self._headers else v) for k, v in headers.items()}

    def body(self, body: bytes | str, content_type: str = "") -> bytes:
        if not body:
            return b"" if isinstance(body, bytes) else b""
        raw = body.encode("utf-8", errors="replace") if isinstance(body, str) else body
        if self._is_json(content_type, raw):
            try:
                obj = json.loads(raw.decode("utf-8", errors="replace"))
            except ValueError:
                obj = None
            if obj is not None:
                self._mask_json(obj)
                raw = json.dumps(obj, separators=(",", ":")).encode()
        return self._mask_patterns(raw)

    def _mask_json(self, value: Any) -> None:
        if isinstance(value, dict):
            for k, v in list(value.items()):
                if k.lower() in self._json:
                    value[k] = MASK
                else:
                    self._mask_json(v)
        elif isinstance(value, list):
            for item in value:
                self._mask_json(item)

    def _mask_patterns(self, body: bytes) -> bytes:
        text = body.decode("utf-8", errors="replace")
        for re_ in _PATTERNS:
            text = re_.sub(MASK, text)
        if self._mask_cards:
            text = _CC_PATTERN.sub(lambda m: MASK if _luhn(m.group(0)) else m.group(0), text)
        return text.encode("utf-8")

    @staticmethod
    def _is_json(content_type: str, body: bytes) -> bool:
        if "json" in content_type.lower():
            return True
        head = body.lstrip()[:1]
        return head in (b"{", b"[")


def _luhn(digits: str) -> bool:
    clean = digits.replace(" ", "").replace("-", "")
    n = len(clean)
    if n < 13 or n > 19 or not clean.isdigit():
        return False
    total = 0
    for i, ch in enumerate(reversed(clean)):
        d = int(ch)
        if i % 2:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return total % 10 == 0
