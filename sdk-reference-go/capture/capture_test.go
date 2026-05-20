package capture_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/songhieu/EchoProxy/sdk-reference-go"
	"github.com/songhieu/EchoProxy/sdk-reference-go/capture"
)

// TestTransport_CapturesOutbound verifies that wrapping http.DefaultTransport
// with capture.Transport ships an outbound event to ingest-api containing the
// request method, host, status, and a non-zero latency breakdown.
func TestTransport_CapturesOutbound(t *testing.T) {
	t.Parallel()

	// Fake upstream the real http.Client calls. Sleeps briefly so the SDK's
	// httptrace records non-zero upstream + ttfb.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	// Fake ingest-api that captures the SDK's batch upload.
	var (
		mu     sync.Mutex
		events []map[string]any
	)
	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events:batch" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Events []map[string]any `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		events = append(events, body.Events...)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()

	client, err := sdk.New(sdk.Config{
		APIKey:        "sk_test_demo",
		EndpointHTTP:  ingest.URL,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     1,
	})
	if err != nil {
		t.Fatalf("sdk.New: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = client.Close(ctx)
	}()

	httpClient := &http.Client{Transport: capture.NewTransport(nil, client)}
	res, err := httpClient.Post(upstream.URL+"/x", "text/plain", strings.NewReader("ping"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resBody, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if string(resBody) != "hello" {
		t.Fatalf("body not preserved: %q", resBody)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status: %d", res.StatusCode)
	}

	// Wait for the SDK flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("no event was shipped to ingest")
	}
	ev := events[0]
	if ev["method"] != "POST" {
		t.Errorf("method=%v want POST", ev["method"])
	}
	if ev["direction"] != "outbound" {
		t.Errorf("direction=%v want outbound", ev["direction"])
	}
	if got, _ := ev["status"].(float64); got != 200 {
		t.Errorf("status=%v want 200", ev["status"])
	}
	if got, _ := ev["source"].(string); got != sdk.SourceName {
		t.Errorf("source=%q want %q", got, sdk.SourceName)
	}
	if got, _ := ev["upstream_latency_ms"].(float64); got <= 0 {
		t.Errorf("upstream_latency_ms=%v want >0", got)
	}
}
