import json

from echoproxy.redact import MASK, Redactor


def test_headers_default_denylist():
    r = Redactor()
    out = r.headers({"Authorization": "Bearer x", "Cookie": "sid=1", "User-Agent": "ua"})
    assert out["Authorization"] == MASK
    assert out["Cookie"] == MASK
    assert out["User-Agent"] == "ua"


def test_json_field_masking():
    r = Redactor()
    raw = json.dumps({"user": "a", "password": "p", "nested": {"api_key": "k"}}).encode()
    out = r.body(raw, "application/json")
    obj = json.loads(out)
    assert obj["password"] == MASK
    assert obj["nested"]["api_key"] == MASK
    assert obj["user"] == "a"


def test_jwt_pattern():
    # Realistic-shaped JWT: each segment ≥10 chars, matching the regex used
    # in pkg/redact (Go) and Echoproxy\Sdk\Redact\Redactor (PHP).
    body = (
        b"token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
        b".eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4ifQ"
        b".SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c ok"
    )
    out = Redactor().body(body, "text/plain").decode()
    assert "eyJhbGciOi" not in out
    assert MASK in out


def test_credit_card_luhn_only():
    valid = b"card 4111-1111-1111-1111 ok"
    invalid = b"id 1234-5678-9012-3456 ok"
    r = Redactor()
    assert b"4111-1111-1111-1111" not in r.body(valid, "text/plain")
    assert b"1234-5678-9012-3456" in r.body(invalid, "text/plain")


def test_disable_defaults():
    r = Redactor(extra_headers=["X-Only"], disable_defaults=True)
    out = r.headers({"Authorization": "x", "X-Only": "y"})
    assert out["Authorization"] == "x"
    assert out["X-Only"] == MASK
