"""Smoke example for the Python SDK proxy mode.

Routes one GET through the EchoProxy proxy via echoproxy.proxy.session() and
prints the response status. The orchestrator runs this with a unique tag
in the path and verifies the event landed in ClickHouse.
"""

from __future__ import annotations

import os
import sys

import echoproxy.proxy as sid


def main() -> int:
    tag = os.environ.get("ECHOPROXY_TAG", "py-default")
    target = os.environ.get("ECHOPROXY_EXAMPLE_TARGET", "http://upstream-mock:9000")

    url = f"{target}/api/users/sdkbench-py-{tag}"
    s = sid.session()
    r = s.get(url, timeout=5)
    print(f"py sdk: {url} -> {r.status_code} ({len(r.content)} bytes)")
    return 0 if r.status_code == 200 else 1


if __name__ == "__main__":
    sys.exit(main())
