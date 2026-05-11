package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransport_RewritesURLAndAddsHeaders(t *testing.T) {
	var (
		gotKey, gotTarget, gotPath string
	)
	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Echo-Key")
		gotTarget = r.Header.Get("X-Echo-Target")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer mockProxy.Close()

	if err := Configure("sk_test_abc", mockProxy.URL); err != nil {
		t.Fatal(err)
	}

	res, err := Get("https://api.example.com/v1/users?x=1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if gotKey != "sk_test_abc" {
		t.Fatalf("X-Echo-Key not set, got %q", gotKey)
	}
	if gotTarget != "https://api.example.com" {
		t.Fatalf("X-Echo-Target wrong: %q", gotTarget)
	}
	if gotPath != "/v1/users" {
		t.Fatalf("path lost: %q", gotPath)
	}
}

func TestTransport_PassesBody(t *testing.T) {
	var got string
	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockProxy.Close()
	_ = Configure("sk_test_xyz", mockProxy.URL)

	_, err := Post("http://api.example.com/", "application/json", strings.NewReader(`{"k":"v"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"k":"v"}` {
		t.Fatalf("body lost: %q", got)
	}
}

func TestEnsure_RequiresAPIKey(t *testing.T) {
	cfg = config{}
	cfgErr = nil
	if err := ensure(); err == nil {
		t.Fatal("expected ErrAPIKeyMissing when nothing configured")
	}
}
