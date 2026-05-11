"""Drop-in proxy client for ``requests``.

Function signatures mirror ``requests.get/post/...`` so existing code only
needs the import swapped::

    import echoproxy.proxy as r
    r.get("https://api.example.com/v1/users")

Configuration is environment-driven::

    ECHOPROXY_API_KEY     required, the raw sk_live_... value
    ECHOPROXY_PROXY_URL   default http://localhost:8080
"""

from __future__ import annotations

import os
from typing import Any, Optional
from urllib.parse import urlparse

import requests
from requests.adapters import HTTPAdapter


class _SidAdapter(HTTPAdapter):
    """HTTPAdapter that rewrites the request URL to point at the proxy and
    adds the X-Echo-Key / X-Echo-Target headers. Mounted on both http:// and
    https:// schemes so any URL the caller passes is intercepted."""

    def __init__(self, proxy_url: str, api_key: str, **kw: Any) -> None:
        super().__init__(**kw)
        self.proxy_url = proxy_url.rstrip("/")
        self.api_key = api_key

    def send(self, request, **kw):  # type: ignore[override]
        u = urlparse(request.url)
        target = f"{u.scheme}://{u.netloc}"
        request.headers["X-Echo-Key"] = self.api_key
        request.headers["X-Echo-Target"] = target
        request.url = f"{self.proxy_url}{u.path or '/'}"
        if u.query:
            request.url += f"?{u.query}"
        return super().send(request, **kw)


def session(api_key: Optional[str] = None, proxy_url: Optional[str] = None) -> requests.Session:
    """Return a ``requests.Session`` that routes every call through the proxy.
    Pass explicit values or rely on environment defaults."""
    api_key = api_key or os.environ.get("ECHOPROXY_API_KEY", "")
    if not api_key:
        raise RuntimeError("echoproxy.proxy: ECHOPROXY_API_KEY env not set")
    proxy_url = proxy_url or os.environ.get("ECHOPROXY_PROXY_URL", "http://localhost:8080")
    s = requests.Session()
    adapter = _SidAdapter(proxy_url, api_key)
    s.mount("https://", adapter)
    s.mount("http://", adapter)
    return s


_default: Optional[requests.Session] = None


def _client() -> requests.Session:
    global _default
    if _default is None:
        _default = session()
    return _default


def configure(api_key: str, proxy_url: Optional[str] = None) -> None:
    """Override env-derived defaults. Useful in tests or multi-tenant apps."""
    global _default
    _default = session(api_key, proxy_url)


# ─── requests.* mirror ──────────────────────────────────────────────────────

def request(method: str, url: str, **kw: Any) -> requests.Response:
    return _client().request(method, url, **kw)


def get(url: str, **kw: Any) -> requests.Response:
    return _client().get(url, **kw)


def options(url: str, **kw: Any) -> requests.Response:
    return _client().options(url, **kw)


def head(url: str, **kw: Any) -> requests.Response:
    return _client().head(url, **kw)


def post(url: str, data: Any = None, json: Any = None, **kw: Any) -> requests.Response:
    return _client().post(url, data=data, json=json, **kw)


def put(url: str, data: Any = None, **kw: Any) -> requests.Response:
    return _client().put(url, data=data, **kw)


def patch(url: str, data: Any = None, **kw: Any) -> requests.Response:
    return _client().patch(url, data=data, **kw)


def delete(url: str, **kw: Any) -> requests.Response:
    return _client().delete(url, **kw)
