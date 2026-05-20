"""Smoke example for the Python SDK *capture mode*.

Uses httpx with the echoproxy event hooks: the app makes the upstream call
itself, the SDK ships the event to ingest-api. The orchestrator runs this with
a unique tag in the path and verifies the event landed in ClickHouse with
source = sdk-python.
"""

from __future__ import annotations

import os
import sys

import httpx

import echoproxy
from echoproxy import httpx_hook


def main() -> int:
    tag = os.environ.get("ECHOPROXY_TAG", "py-default")
    target = os.environ.get("ECHOPROXY_EXAMPLE_TARGET", "http://upstream-mock:9000")

    client = echoproxy.Client()
    http = httpx.Client(event_hooks=httpx_hook.hooks(client))

    url = f"{target}/api/users/sdkbench-py-capture-{tag}"
    r = http.get(url, timeout=5)
    print(f"py sdk (capture): {url} -> {r.status_code} ({len(r.content)} bytes)")

    client.close()
    return 0 if r.status_code == 200 else 1


if __name__ == "__main__":
    sys.exit(main())
