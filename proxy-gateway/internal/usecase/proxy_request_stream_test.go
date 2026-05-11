package usecase

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// fakeMetrics records counter increments so tests can assert side effects
// without depending on Prometheus internals.
type fakeMetrics struct {
	streamCount         atomic.Int64
	streamIdleTimeouts  atomic.Int64
	observedStreamChunk atomic.Int64
}

func (f *fakeMetrics) ObserveLatency(string, string, time.Duration) {}
func (f *fakeMetrics) IncDropped()                                  {}
func (f *fakeMetrics) IncTruncated(string)                          {}
func (f *fakeMetrics) IncStream()                                   { f.streamCount.Add(1) }
func (f *fakeMetrics) IncStreamIdleTimeout()                        { f.streamIdleTimeouts.Add(1) }
func (f *fakeMetrics) ObserveStreamChunks(n uint32)                 { f.observedStreamChunk.Store(int64(n)) }

// captureSink stores the last enqueued event so tests can inspect the
// stream fields written to the payload.
type captureSink struct {
	mu  sync.Mutex
	got domain.EventPayload
}

func (s *captureSink) Enqueue(_ context.Context, ev domain.EventPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = ev
}

func (s *captureSink) last() domain.EventPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got
}

// flushSpy wraps an http.ResponseWriter and counts Flush() calls. It's how
// we prove the proxy is flushing per chunk instead of buffering until EOF.
type flushSpy struct {
	http.ResponseWriter
	flushes atomic.Int64
	// writes records the byte length of each Write that happened between
	// Flush calls so tests can assert chunk ordering.
	writeCh chan int
}

func (f *flushSpy) Flush() {
	f.flushes.Add(1)
	if flusher, ok := f.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (f *flushSpy) Write(p []byte) (int, error) {
	n, err := f.ResponseWriter.Write(p)
	if f.writeCh != nil {
		select {
		case f.writeCh <- n:
		default:
		}
	}
	return n, err
}

// TestStreamCopy_FlushesEachChunk asserts that for SSE responses the proxy
// flushes after every upstream chunk so the client sees data in real time.
// Without flush, Go buffers up to ~4KB and SSE clients stall.
func TestStreamCopy_FlushesEachChunk(t *testing.T) {
	// Upstream emits 5 SSE events with a 20ms gap. If the proxy buffers
	// instead of flushing per write, the client receives them together at
	// the end.
	const events = 5
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for i := 0; i < events; i++ {
			fmt.Fprintf(w, "data: event-%d\n\n", i)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)
	uc := NewProxyRequest(
		&http.Transport{},
		&captureSink{},
		1024,
		&fakeMetrics{},
		nil,
		2*time.Second, // idle timeout — well above the 20ms cadence
	)

	// Drive Execute through an httptest recorder wrapped in flushSpy so we
	// can count flushes and confirm chunks arrive one at a time.
	rec := httptest.NewRecorder()
	spy := &flushSpy{ResponseWriter: rec, writeCh: make(chan int, events*2)}
	req := httptest.NewRequest("GET", "/", nil)
	key := &domain.APIKey{ID: 1, ProjectID: 1}
	uc.Execute(spy, req, key, target)

	if spy.flushes.Load() < events {
		t.Fatalf("expected at least %d flushes, got %d (proxy is buffering instead of streaming)", events, spy.flushes.Load())
	}
	body := rec.Body.String()
	for i := 0; i < events; i++ {
		if !strings.Contains(body, fmt.Sprintf("data: event-%d", i)) {
			t.Fatalf("missing event %d in body", i)
		}
	}
}

// TestStreamCopy_EmitsStreamMetadata verifies the captured EventPayload
// carries is_stream + chunk_count + duration so the dashboard can render
// them. The flush count is an upper bound for chunk_count (Go's net/http
// can coalesce or split reads, but each Write counts as one chunk).
func TestStreamCopy_EmitsStreamMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	sink := &captureSink{}
	metrics := &fakeMetrics{}
	target, _ := url.Parse(upstream.URL)
	uc := NewProxyRequest(&http.Transport{}, sink, 1024, metrics, nil, 2*time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	uc.Execute(rec, req, &domain.APIKey{ID: 1, ProjectID: 1}, target)

	ev := sink.last()
	if !ev.IsStream {
		t.Fatalf("is_stream should be true for SSE response")
	}
	if ev.StreamChunkCount == 0 {
		t.Fatalf("stream_chunk_count should be > 0, got %d", ev.StreamChunkCount)
	}
	if ev.StreamIdleTimeout {
		t.Fatalf("idle timeout should not fire when upstream sends regularly")
	}
	if metrics.streamCount.Load() != 1 {
		t.Fatalf("expected stream counter to increment exactly once, got %d", metrics.streamCount.Load())
	}
	if metrics.streamIdleTimeouts.Load() != 0 {
		t.Fatalf("idle timeout counter should not fire on healthy stream")
	}
}

// TestStreamCopy_IdleTimeoutFires drives a stream that opens normally then
// stops sending bytes. The watchdog should cancel the upstream, mark the
// event with stream_idle_timeout=true, and bump the Prometheus counter.
func TestStreamCopy_IdleTimeoutFires(t *testing.T) {
	// Upstream writes one chunk, then sleeps far past the idle threshold.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: hello\n\n")
		flusher.Flush()
		// Block until the client side cancels.
		<-r.Context().Done()
	}))
	defer upstream.Close()

	sink := &captureSink{}
	metrics := &fakeMetrics{}
	target, _ := url.Parse(upstream.URL)
	uc := NewProxyRequest(
		&http.Transport{},
		sink,
		1024,
		metrics,
		nil,
		400*time.Millisecond, // small idle window so the test runs fast
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	done := make(chan struct{})
	go func() {
		uc.Execute(rec, req, &domain.APIKey{ID: 1, ProjectID: 1}, target)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Execute did not return within 3s — watchdog likely did not fire")
	}

	ev := sink.last()
	if !ev.IsStream {
		t.Fatalf("event should be marked as stream")
	}
	if !ev.StreamIdleTimeout {
		t.Fatalf("stream_idle_timeout should be true when watchdog fires")
	}
	if metrics.streamIdleTimeouts.Load() != 1 {
		t.Fatalf("expected idle-timeout counter to be 1, got %d", metrics.streamIdleTimeouts.Load())
	}
}

// TestExecute_NonStreamSkipsFlush ensures the fast path (regular responses
// with a Content-Length) does NOT route through the stream-copy code path,
// so we keep the hot-path overhead at its current numbers.
func TestExecute_NonStreamSkipsFlush(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"ok":true}`
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))
	defer upstream.Close()

	sink := &captureSink{}
	metrics := &fakeMetrics{}
	target, _ := url.Parse(upstream.URL)
	uc := NewProxyRequest(&http.Transport{}, sink, 1024, metrics, nil, 2*time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	uc.Execute(rec, req, &domain.APIKey{ID: 1, ProjectID: 1}, target)

	ev := sink.last()
	if ev.IsStream {
		t.Fatalf("plain JSON response with Content-Length should NOT be flagged as stream")
	}
	if metrics.streamCount.Load() != 0 {
		t.Fatalf("stream counter should not fire on non-stream response")
	}
}
