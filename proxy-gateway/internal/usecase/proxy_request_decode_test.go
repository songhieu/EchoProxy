package usecase

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// TestExecute_GzippedJSON_ClientGetsDecompressibleBytes is the SDK-fidelity
// check the user asked about: a Python client that sends Accept-Encoding goes
// through the proxy and reads response.json(); requests' decompressor must
// see (a) the gzip header bytes intact, and (b) Content-Encoding: gzip on the
// response — otherwise the SDK would surface binary instead of JSON.
//
// We don't actually run Python here; we replicate what requests does: read
// the body bytes, look at headers, decompress gzip ourselves, and parse JSON.
func TestExecute_GzippedJSON_ClientGetsDecompressibleBytes(t *testing.T) {
	original := map[string]any{"hello": "world", "n": 42}
	originalJSON, _ := json.Marshal(original)

	// Upstream gzips the JSON before sending. This is what real APIs (OpenAI,
	// Anthropic, GitHub, …) do when the client advertises Accept-Encoding.
	var gzBody bytes.Buffer
	gz := gzip.NewWriter(&gzBody)
	gz.Write(originalJSON)
	gz.Close()
	gzBytes := gzBody.Bytes()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(gzBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(gzBytes)
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)
	sink := &captureSink{}
	uc := NewProxyRequest(&http.Transport{}, sink, 64*1024, &fakeMetrics{}, nil, 2*time.Second)

	// Mimic what the Python SDK adapter does: pass Accept-Encoding through
	// so Go's transport doesn't auto-decompress and the gzipped bytes flow
	// all the way to the client.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	uc.Execute(rec, req, &domain.APIKey{ID: 1, ProjectID: 1}, target)

	// 1. Status must be 200, not anything wrapped.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// 2. Content-Encoding must reach the client, otherwise requests won't
	//    know to decompress.
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected Content-Encoding=gzip to reach client, got %q (SDK would treat body as binary)", got)
	}
	// 3. The body bytes the client sees must be valid gzip that decompresses
	//    to the original JSON. If the proxy mangled the stream (e.g. flushed
	//    in a way that doubled or split a gzip header), this fails.
	clientBytes := rec.Body.Bytes()
	zr, err := gzip.NewReader(bytes.NewReader(clientBytes))
	if err != nil {
		t.Fatalf("client received non-gzip bytes: %v\nhead=%x", err, clientBytes[:min(32, len(clientBytes))])
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(plain, &roundTrip); err != nil {
		t.Fatalf("client-visible body is not JSON after decompression: %v\nplain=%q", err, plain)
	}
	if roundTrip["hello"] != "world" {
		t.Fatalf("round-trip JSON mismatch: %v", roundTrip)
	}

	// 4. Now the log side: the event payload's ResBody should be the
	//    DECODED JSON, not the gzipped bytes. This is the log-display fix.
	ev := sink.last()
	if string(ev.ResBody) != string(originalJSON) {
		t.Fatalf("captured ResBody should be decoded JSON for dashboard, got %q", ev.ResBody)
	}
	if ev.ResBodyTruncated {
		t.Fatalf("ResBodyTruncated should be false for a body within cap")
	}
}

func TestNormalizeCapturedBody_BinaryContentType(t *testing.T) {
	raw := []byte{0xde, 0xad, 0xbe, 0xef, 0xfa, 0xce}
	got, trunc := normalizeCapturedBody(raw, "", "application/grpc+proto", 1024, false)
	if trunc {
		t.Fatalf("truncated should be false")
	}
	gotStr := string(got)
	if !strings.HasPrefix(gotStr, "<binary application/grpc+proto, 6 bytes>") {
		t.Fatalf("expected placeholder, got %q", gotStr)
	}
}

func TestNormalizeCapturedBody_Identity(t *testing.T) {
	raw := []byte(`{"ok":true}`)
	got, trunc := normalizeCapturedBody(raw, "", "application/json", 1024, false)
	if trunc {
		t.Fatalf("truncated should remain false")
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("identity body should pass through, got %q", got)
	}
	// Independent slice — mutating the source must not bleed through.
	raw[0] = 'X'
	if got[0] == 'X' {
		t.Fatalf("normalized slice must not share memory with input")
	}
}

func TestNormalizeCapturedBody_GzipDecodes(t *testing.T) {
	var gzBody bytes.Buffer
	gz := gzip.NewWriter(&gzBody)
	gz.Write([]byte(`{"answer":42}`))
	gz.Close()

	got, trunc := normalizeCapturedBody(gzBody.Bytes(), "gzip", "application/json", 1024, false)
	if trunc {
		t.Fatalf("truncated should be false")
	}
	if string(got) != `{"answer":42}` {
		t.Fatalf("gzip body should be decoded, got %q", got)
	}
}

// A gzip stream whose header is missing entirely (think: capture started
// mid-stream, or a corrupt response body) is unrecoverable. The decoder
// fails immediately and we must surface a placeholder — otherwise the
// dashboard would render the raw compressed bytes as UTF-8 garbage.
func TestNormalizeCapturedBody_GzipHeaderUnrecoverable(t *testing.T) {
	// Random bytes claiming gzip encoding — no valid magic.
	corrupted := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}

	got, _ := normalizeCapturedBody(corrupted, "gzip", "application/json", 1024, true)
	if !strings.HasPrefix(string(got), "<gzip body, truncated capture,") {
		t.Fatalf("expected truncated-gzip placeholder, got %q", got)
	}
}

// Truncated gzip stream (valid header, missing trailer) is partially
// recoverable — the decoder emits the data it managed to decode and signals
// truncation. Surfacing the partial JSON is more useful than a placeholder.
func TestNormalizeCapturedBody_GzipTruncatedRecoversPartial(t *testing.T) {
	var gzBody bytes.Buffer
	gz := gzip.NewWriter(&gzBody)
	gz.Write(bytes.Repeat([]byte("x"), 1024))
	gz.Close()
	corrupted := gzBody.Bytes()[: gzBody.Len()-2]

	got, trunc := normalizeCapturedBody(corrupted, "gzip", "application/json", 4096, true)
	if !trunc {
		t.Fatalf("truncation should propagate")
	}
	if !strings.HasPrefix(string(got), "xxxx") {
		t.Fatalf("expected partial decoded data, got %q", got)
	}
}

func TestExecute_GrpcCaptureIsPlaceholder(t *testing.T) {
	// Synthetic gRPC-like binary payload from upstream.
	binaryBody := append([]byte{0x00}, bytes.Repeat([]byte{0xab}, 256)...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc+proto")
		w.WriteHeader(http.StatusOK)
		w.Write(binaryBody)
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)
	sink := &captureSink{}
	uc := NewProxyRequest(&http.Transport{}, sink, 64*1024, &fakeMetrics{}, nil, 2*time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	uc.Execute(rec, req, &domain.APIKey{ID: 1, ProjectID: 1}, target)

	// Client must still receive the exact binary bytes — gRPC clients depend
	// on them. Only the captured log copy is replaced.
	if !bytes.Equal(rec.Body.Bytes(), binaryBody) {
		t.Fatalf("client should receive verbatim gRPC bytes")
	}
	ev := sink.last()
	if !strings.HasPrefix(string(ev.ResBody), "<binary application/grpc+proto, ") {
		t.Fatalf("captured ResBody should be a binary placeholder, got %q", ev.ResBody)
	}
}
