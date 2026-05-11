package usecase

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

func TestParseTarget_RejectsLoopback(t *testing.T) {
	if _, err := ParseTarget("http://127.0.0.1/"); err == nil {
		t.Fatalf("expected error for loopback")
	}
	if _, err := ParseTarget("http://localhost/"); err == nil {
		t.Fatalf("expected error for localhost")
	}
}

func TestParseTarget_DefaultsHTTPS(t *testing.T) {
	u, err := ParseTarget("api.example.com")
	if err != nil {
		// DNS may fail in sandbox; only assert when it succeeds.
		t.Skipf("dns lookup failed: %v", err)
	}
	if u.Scheme != "https" {
		t.Fatalf("expected https default, got %q", u.Scheme)
	}
}

func TestCappedWriter_Truncates(t *testing.T) {
	cw := &cappedWriter{buf: &bytes.Buffer{}, max: 4}
	n, _ := cw.Write([]byte("abcdef"))
	if n != 6 {
		t.Fatalf("write should report total bytes consumed, got %d", n)
	}
	if cw.buf.Len() != 4 {
		t.Fatalf("buffer should hold only 4 bytes, got %d", cw.buf.Len())
	}
	if !cw.truncated {
		t.Fatalf("should be marked truncated")
	}
}

func TestAllowsHost_EmptyAllowAll(t *testing.T) {
	k := &domain.APIKey{Allowlist: nil}
	if !k.AllowsHost("anything.example.com") {
		t.Fatalf("empty allowlist should allow all")
	}
}

func TestAllowsHost_CaseInsensitive(t *testing.T) {
	k := &domain.APIKey{Allowlist: []string{"Api.Example.com"}}
	if !k.AllowsHost("api.example.COM") {
		t.Fatalf("hostname comparison should be case insensitive")
	}
	if k.AllowsHost("evil.example.com") {
		t.Fatalf("non-allowlisted host should be rejected")
	}
}

// Compile-time check: usecase compiles with the URL package used in adapters.
var _ = (&url.URL{}).String
