package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_HTTPBatch(t *testing.T) {
	var got struct {
		Events []map[string]any `json:"events"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Echo-Key") != "sk_test" {
			t.Fatalf("missing/wrong api key header")
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":1}`))
	}))
	defer srv.Close()

	c, err := New(Config{
		APIKey:        "sk_test",
		EndpointHTTP:  srv.URL,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Capture(CaptureInput{
		Method: "GET", Host: "api.example.com", Path: "/v1/users", Status: 200, Latency: 5 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}

	if len(got.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Events))
	}
	if got.Events[0]["method"] != "GET" {
		t.Fatalf("event method mismatch: %v", got.Events[0]["method"])
	}
}

func TestClient_BodyCapTruncates(t *testing.T) {
	c, err := New(Config{APIKey: "k", EndpointHTTP: "http://localhost", MaxBodyBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	in := bytes.Repeat([]byte("a"), 10)
	out, trunc := c.cap(in)
	if len(out) != 4 || !trunc {
		t.Fatalf("cap failed: len=%d trunc=%v", len(out), trunc)
	}
}

func TestClient_HeaderMasking(t *testing.T) {
	c, err := New(Config{APIKey: "k", EndpointHTTP: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")
	h.Set("X-Custom", "ok")
	out := c.snapshotHeaders(h)
	if out["Authorization"] == "Bearer secret" {
		t.Fatalf("Authorization not masked: %q", out["Authorization"])
	}
	if out["X-Custom"] != "ok" {
		t.Fatalf("non-secret header should be intact, got %q", out["X-Custom"])
	}
}
