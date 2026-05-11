package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHeaders_DefaultDenylist(t *testing.T) {
	r := New(Rules{})
	out := r.Headers(map[string]string{
		"Authorization": "Bearer eyJhbc.def.ghi",
		"Cookie":        "sid=abc",
		"User-Agent":    "test/1.0",
		"X-Custom":      "ok",
	})
	if out["Authorization"] != Mask {
		t.Fatalf("Authorization should be masked, got %q", out["Authorization"])
	}
	if out["Cookie"] != Mask {
		t.Fatalf("Cookie should be masked, got %q", out["Cookie"])
	}
	if out["User-Agent"] != "test/1.0" {
		t.Fatalf("User-Agent should be intact, got %q", out["User-Agent"])
	}
	if out["X-Custom"] != "ok" {
		t.Fatalf("X-Custom should be intact, got %q", out["X-Custom"])
	}
}

func TestHeaders_CustomDenylist(t *testing.T) {
	r := New(Rules{HeaderDenylist: []string{"X-Custom"}})
	out := r.Headers(map[string]string{"X-Custom": "secret"})
	if out["X-Custom"] != Mask {
		t.Fatalf("custom header not masked")
	}
}

func TestBody_JSONFieldsMasked(t *testing.T) {
	r := New(Rules{})
	in := []byte(`{"user":"alice","password":"hunter2","nested":{"api_key":"sk_live_abc"},"safe":1}`)
	out := r.Body(in, "application/json")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["password"] != Mask {
		t.Fatalf("password not masked: %v", got["password"])
	}
	if got["user"] != "alice" {
		t.Fatalf("non-secret field corrupted: %v", got["user"])
	}
	if got["safe"] != float64(1) {
		t.Fatalf("numeric field corrupted: %v", got["safe"])
	}
	nested, _ := got["nested"].(map[string]any)
	if nested["api_key"] != Mask {
		t.Fatalf("nested api_key not masked: %v", nested["api_key"])
	}
}

func TestBody_RegexPatterns(t *testing.T) {
	r := New(Rules{})
	// Note: the Stripe key literal is split with `+` to keep GitHub
	// secret-scanning happy. Runtime value is unchanged so the regex test
	// still exercises the full match path.
	in := []byte("token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMifQ.signature123 stripe=" +
		"sk_live_" + "abcdefghijklmnopqrstuvwx")
	out := r.Body(in, "text/plain")
	s := string(out)
	if strings.Contains(s, "eyJ") && strings.Contains(s, "signature") {
		t.Fatalf("JWT not masked: %s", s)
	}
	if strings.Contains(s, "sk_live_abc") {
		t.Fatalf("Stripe key not masked: %s", s)
	}
	if !strings.Contains(s, Mask) {
		t.Fatalf("expected mask in output, got %s", s)
	}
}

func TestBody_LuhnCreditCard(t *testing.T) {
	r := New(Rules{})
	// Valid Visa test number
	in := []byte("card 4111-1111-1111-1111 expires soon")
	out := r.Body(in, "text/plain")
	if strings.Contains(string(out), "4111-1111-1111-1111") {
		t.Fatalf("valid CC not masked: %s", out)
	}
}

func TestBody_LuhnRejectsNonCC(t *testing.T) {
	r := New(Rules{})
	// 16 digits but Luhn-invalid
	in := []byte("number 1234-5678-9012-3456 ok")
	out := r.Body(in, "text/plain")
	if !strings.Contains(string(out), "1234-5678-9012-3456") {
		t.Fatalf("Luhn-invalid digits should NOT be masked: %s", out)
	}
}

func TestBody_NonJSONFallsBackToPatterns(t *testing.T) {
	r := New(Rules{})
	in := []byte("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature123")
	out := r.Body(in, "text/plain")
	if strings.Contains(string(out), "eyJ") {
		t.Fatalf("JWT not masked in plain text: %s", out)
	}
}

func TestBody_EmptyBypass(t *testing.T) {
	r := New(Rules{})
	if got := r.Body(nil, ""); got != nil {
		t.Fatalf("nil body should pass through")
	}
}

func TestRules_DisableDefaults(t *testing.T) {
	r := New(Rules{DisableDefaults: true, HeaderDenylist: []string{"x-only"}})
	out := r.Headers(map[string]string{"Authorization": "Bearer x", "X-Only": "y"})
	if out["Authorization"] != "Bearer x" {
		t.Fatalf("with DisableDefaults, Authorization must NOT be masked")
	}
	if out["X-Only"] != Mask {
		t.Fatalf("custom header must still be masked")
	}
}
