package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/pkg/redact"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// Metrics is the minimal interface the use case needs. The real
// implementation lives in adapter/infra/metrics.
type Metrics interface {
	ObserveLatency(method string, statusClass string, d time.Duration)
	IncDropped()
	IncTruncated(side string)
	IncStream()
	IncStreamIdleTimeout()
	ObserveStreamChunks(n uint32)
}

// ProxyRequest forwards a request to the upstream described by X-Echo-Target,
// captures the request/response (bounded), and asynchronously enqueues an
// event for the Kafka pipeline. The hot path performs no blocking I/O beyond
// the upstream RoundTrip.
type ProxyRequest struct {
	transport         *http.Transport
	sink              domain.EventSink
	bufPool           *sync.Pool
	bodyCap           int
	metrics           Metrics
	defaultRedactor   *redact.Redactor
	streamIdleTimeout time.Duration
}

// NewProxyRequest constructs a ProxyRequest. streamIdleTimeout is the
// inactivity threshold for streaming responses: if no upstream bytes arrive
// for this long after headers, the watchdog cancels the connection and
// flags the event with stream_idle_timeout=true. Pass 0 to disable the
// watchdog (the request still ends when the client/upstream context does).
func NewProxyRequest(transport *http.Transport, sink domain.EventSink, bodyCap int, m Metrics, redactor *redact.Redactor, streamIdleTimeout time.Duration) *ProxyRequest {
	if redactor == nil {
		redactor = redact.New(redact.Rules{})
	}
	return &ProxyRequest{
		transport: transport,
		sink:      sink,
		bodyCap:   bodyCap,
		bufPool: &sync.Pool{
			New: func() any { return bytes.NewBuffer(make([]byte, 0, 8192)) },
		},
		metrics:           m,
		defaultRedactor:   redactor,
		streamIdleTimeout: streamIdleTimeout,
	}
}

func (uc *ProxyRequest) Execute(w http.ResponseWriter, r *http.Request, key *domain.APIKey, target *url.URL) {
	start := time.Now()
	cap := uc.bodyCap
	if key.BodyCap > 0 {
		cap = key.BodyCap
	}

	reqBuf := uc.bufPool.Get().(*bytes.Buffer)
	resBuf := uc.bufPool.Get().(*bytes.Buffer)
	defer func() {
		reqBuf.Reset()
		resBuf.Reset()
		uc.bufPool.Put(reqBuf)
		uc.bufPool.Put(resBuf)
	}()

	cappedReq := &cappedWriter{buf: reqBuf, max: cap}
	cappedRes := &cappedWriter{buf: resBuf, max: cap}

	// Tee the inbound body into the bounded buffer.
	var bodyForUpstream io.Reader
	if r.Body != nil {
		bodyForUpstream = io.TeeReader(r.Body, cappedReq)
	}

	outURL := *target
	outURL.Path = singleJoiningSlash(target.Path, r.URL.Path)
	outURL.RawQuery = r.URL.RawQuery

	// Derive a cancellable context so the idle-timeout watchdog can abort
	// the upstream connection if a stream stalls. The cancel also runs at
	// the end of the handler to guarantee no goroutine leak.
	upstreamCtx, cancelUpstream := context.WithCancel(r.Context())
	defer cancelUpstream()

	outReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, outURL.String(), bodyForUpstream)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		uc.emit(r, key, target, http.StatusBadRequest, start, time.Since(start), 0, 0, reqBuf, resBuf, cappedReq.truncated, false, nil, "", "", err.Error(), streamInfo{})
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Del("X-Echo-Key")
	outReq.Header.Del("X-Echo-Target")
	outReq.Host = target.Host

	// httptrace captures the first byte of the upstream response so we can
	// expose TTFB separately from total upstream latency.
	var (
		firstByteAt  time.Time
		ttfbCaptured bool
	)
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			if !ttfbCaptured {
				firstByteAt = time.Now()
				ttfbCaptured = true
			}
		},
	}
	outReq = outReq.WithContext(httptrace.WithClientTrace(outReq.Context(), trace))

	upstreamStart := time.Now()
	resp, err := uc.transport.RoundTrip(outReq)
	upstreamLatency := time.Since(upstreamStart)
	var ttfb time.Duration
	if ttfbCaptured {
		ttfb = firstByteAt.Sub(upstreamStart)
	}
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		uc.emit(r, key, target, http.StatusBadGateway, start, time.Since(start), upstreamLatency, ttfb, reqBuf, resBuf, cappedReq.truncated, false, nil, "", "", err.Error(), streamInfo{})
		return
	}
	defer resp.Body.Close()

	resHeaders := flattenHeaders(resp.Header)
	resContentType := resp.Header.Get("Content-Type")
	resContentEncoding := resp.Header.Get("Content-Encoding")
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Stream detection must happen *after* the response headers arrive but
	// before we start copying the body — that is where we decide whether to
	// flush every chunk and arm the idle watchdog.
	stream := streamInfo{isStream: isStreaming(resp)}
	var copyStart time.Time
	if stream.isStream {
		copyStart = time.Now()
		uc.metrics.IncStream()
		flusher, _ := w.(http.Flusher)
		stream = uc.streamCopy(upstreamCtx, cancelUpstream, w, flusher, resp.Body, cappedRes)
		stream.isStream = true
		stream.durationMs = uint32(time.Since(copyStart).Milliseconds())
		uc.metrics.ObserveStreamChunks(stream.chunkCount)
		if stream.idleTimeout {
			uc.metrics.IncStreamIdleTimeout()
		}
	} else {
		mw := io.MultiWriter(w, cappedRes)
		_, _ = io.Copy(mw, resp.Body)
	}

	d := time.Since(start)
	uc.metrics.ObserveLatency(r.Method, statusClass(resp.StatusCode), d)
	if cappedReq.truncated {
		uc.metrics.IncTruncated("req")
	}
	if cappedRes.truncated {
		uc.metrics.IncTruncated("res")
	}

	uc.emit(r, key, target, resp.StatusCode, start, d, upstreamLatency, ttfb, reqBuf, resBuf, cappedReq.truncated, cappedRes.truncated, resHeaders, resContentType, resContentEncoding, "", stream)
}

// streamInfo carries observations gathered during a streaming copy.
type streamInfo struct {
	isStream    bool
	chunkCount  uint32
	durationMs  uint32
	idleTimeout bool
}

// isStreaming returns true when the response should be treated as a stream:
// SSE, gRPC, chunked transfer encoding, or a 200 with unknown length. These
// responses need per-chunk flush so the client sees data in real time.
func isStreaming(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "text/event-stream"):
		return true
	case strings.HasPrefix(ct, "application/grpc"):
		return true
	}
	for _, te := range resp.TransferEncoding {
		if strings.EqualFold(te, "chunked") {
			return true
		}
	}
	// Unknown length on a 200 is a strong signal of a stream (e.g. SSE
	// proxies that forget Content-Type, or NDJSON endpoints).
	if resp.ContentLength < 0 && resp.StatusCode == http.StatusOK {
		return true
	}
	return false
}

// streamCopy reads the upstream body chunk-by-chunk, writes each chunk to
// the client + the bounded capture buffer, and flushes immediately. It
// also arms an idle-timeout watchdog: if no upstream bytes arrive for
// uc.streamIdleTimeout, it cancels the upstream context — which closes
// resp.Body and breaks the Read loop with a non-nil error.
func (uc *ProxyRequest) streamCopy(ctx context.Context, cancel context.CancelFunc, w io.Writer, flusher http.Flusher, body io.Reader, captured *cappedWriter) streamInfo {
	var info streamInfo
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	// Watchdog goroutine. Tick at idle/4 so we react within a quarter of
	// the budget after the upstream stalls. A 0 idle timeout disables it.
	if uc.streamIdleTimeout > 0 {
		idleNs := uc.streamIdleTimeout.Nanoseconds()
		tickEvery := uc.streamIdleTimeout / 4
		if tickEvery < 250*time.Millisecond {
			tickEvery = 250 * time.Millisecond
		}
		done := make(chan struct{})
		defer close(done)
		go func() {
			t := time.NewTicker(tickEvery)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case now := <-t.C:
					if now.UnixNano()-lastActivity.Load() >= idleNs {
						info.idleTimeout = true
						cancel()
						return
					}
				}
			}
		}()
	}

	// 32 KiB matches io.Copy's default — large enough to amortize syscall
	// overhead, small enough not to bloat the per-request footprint.
	buf := make([]byte, 32*1024)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			lastActivity.Store(time.Now().UnixNano())
			info.chunkCount++
			if _, werr := w.Write(buf[:n]); werr != nil {
				return info
			}
			_, _ = captured.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return info
		}
	}
}

func (uc *ProxyRequest) emit(r *http.Request, key *domain.APIKey, target *url.URL, status int,
	arrival time.Time, total, upstream, ttfb time.Duration,
	reqBuf, resBuf *bytes.Buffer, reqTrunc, resTrunc bool, resHeaders map[string]string, resContentType, resContentEncoding, errStr string,
	stream streamInfo) {
	red := key.Redactor
	if red == nil {
		red = uc.defaultRedactor
	}

	// On-the-wire bytes have already been forwarded to the client by the time
	// we get here. The captures below are persisted ONLY for dashboard
	// display, so we decode common compression and replace opaque binary
	// payloads with a short placeholder. The client's response is untouched.
	cap := uc.bodyCap
	if key.BodyCap > 0 {
		cap = key.BodyCap
	}
	reqBody, reqTruncOut := normalizeCapturedBody(reqBuf.Bytes(), r.Header.Get("Content-Encoding"), r.Header.Get("Content-Type"), cap, reqTrunc)
	resBody, resTruncOut := normalizeCapturedBody(resBuf.Bytes(), resContentEncoding, resContentType, cap, resTrunc)

	payload := domain.EventPayload{
		EventID:           event.NewEventID(),
		TimestampNs:       arrival.UnixNano(),
		Source:            event.SourceProxy,
		APIKeyID:          key.ID,
		ProjectID:         key.ProjectID,
		Method:            r.Method,
		Scheme:            target.Scheme,
		Host:              target.Host,
		Path:              r.URL.Path,
		Query:             r.URL.RawQuery,
		Status:            uint32(status),
		LatencyMs:         uint32(total.Milliseconds()),
		UpstreamLatencyMs: uint32(upstream.Milliseconds()),
		UpstreamTtfbMs:    uint32(ttfb.Milliseconds()),
		ReqSize:           uint32(reqBuf.Len()),
		ResSize:           uint32(resBuf.Len()),
		ReqHeaders:       red.Headers(headerSnapshot(r.Header, []string{"X-Echo-Key"})),
		ResHeaders:       red.Headers(resHeaders),
		ReqBody:          red.Body(reqBody, r.Header.Get("Content-Type")),
		ResBody:          red.Body(resBody, resContentType),
		ReqBodyTruncated: reqTruncOut,
		ResBodyTruncated: resTruncOut,
		ClientIP:         clientIP(r),
		UserAgent:        r.UserAgent(),
		TraceID:          r.Header.Get("traceparent"),
		Error:            errStr,
		Direction:        "outbound",

		IsStream:          stream.isStream,
		StreamChunkCount:  stream.chunkCount,
		StreamDurationMs:  stream.durationMs,
		StreamIdleTimeout: stream.idleTimeout,
	}
	// Sink is responsible for non-blocking enqueue + drop counting.
	uc.sink.Enqueue(r.Context(), payload)
}

// EmitAuthFailure enqueues an audit event when a request is rejected before
// it reaches the upstream — invalid/revoked/disallowed key, rate-limited,
// or bad target. Without this, the dashboard has no visibility into "why
// isn't my key working" and admins can't see brute-force attempts.
//
// key may be nil (unknown key). target may be nil (header missing/invalid).
// status is the HTTP status code returned to the caller (401/403/400/429).
// errCode is a short machine tag like "invalid_api_key" / "rate_limited".
//
// The event is recorded with no body capture (the request never reached the
// hot path) and minimal context. It uses the same async sink as success
// events, so it inherits drop-on-overflow protection — a flood of failures
// will degrade gracefully instead of taking down the proxy.
func (uc *ProxyRequest) EmitAuthFailure(r *http.Request, target *url.URL, key *domain.APIKey, status int, errCode string, arrival time.Time) {
	var (
		projectID uint64
		apiKeyID  uint64
		scheme    = ""
		host      = ""
	)
	if key != nil {
		projectID = key.ProjectID
		apiKeyID = key.ID
	}
	if target != nil {
		scheme = target.Scheme
		host = target.Host
	}
	payload := domain.EventPayload{
		EventID:     event.NewEventID(),
		TimestampNs: arrival.UnixNano(),
		Source:      event.SourceProxy,
		APIKeyID:    apiKeyID,
		ProjectID:   projectID,
		Method:      r.Method,
		Scheme:      scheme,
		Host:        host,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		Status:      uint32(status),
		LatencyMs:   uint32(time.Since(arrival).Milliseconds()),
		ReqHeaders:  uc.defaultRedactor.Headers(headerSnapshot(r.Header, []string{"X-Echo-Key"})),
		ClientIP:    clientIP(r),
		UserAgent:   r.UserAgent(),
		TraceID:     r.Header.Get("traceparent"),
		Error:       errCode,
		Direction:   "outbound",
	}
	uc.sink.Enqueue(r.Context(), payload)
}

// cappedWriter implements io.Writer with a hard byte cap.
type cappedWriter struct {
	buf       *bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.buf.Len() >= c.max {
		c.truncated = true
		return len(p), nil
	}
	remaining := c.max - c.buf.Len()
	if len(p) <= remaining {
		return c.buf.Write(p)
	}
	n, _ := c.buf.Write(p[:remaining])
	c.truncated = true
	return n + (len(p) - remaining), nil
}

// AllowPrivateTargets controls whether ParseTarget permits private / loopback
// upstream addresses. Defaults to false (production). Set to true for local
// docker development where upstream services live on a private network.
var AllowPrivateTargets = false

// ParseTarget validates the X-Echo-Target header. Defaults to https when no
// scheme is provided. Rejects loopback / private addresses to mitigate SSRF
// unless AllowPrivateTargets is set.
func ParseTarget(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, domain.ErrTargetMissing
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, domain.ErrTargetInvalid
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, domain.ErrTargetInvalid
	}
	if !AllowPrivateTargets && isPrivateOrLoopback(u.Hostname()) {
		return nil, domain.ErrTargetUnsafe
	}
	return u, nil
}

func isPrivateOrLoopback(host string) bool {
	if host == "" {
		return true
	}
	// Hostnames starting with . are obviously bogus.
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// If we cannot resolve, let the upstream call fail; it's not our job
		// to block based on resolver outage.
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func flattenHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vs := range h {
		out[k] = strings.Join(vs, ",")
	}
	return out
}

func headerSnapshot(h http.Header, drop []string) map[string]string {
	out := make(map[string]string, len(h))
	dropSet := make(map[string]struct{}, len(drop))
	for _, d := range drop {
		dropSet[strings.ToLower(d)] = struct{}{}
	}
	for k, vs := range h {
		if _, skip := dropSet[strings.ToLower(k)]; skip {
			continue
		}
		out[k] = strings.Join(vs, ",")
	}
	return out
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func statusClass(s int) string {
	if s == 0 {
		return "0xx"
	}
	return fmt.Sprintf("%dxx", s/100)
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

// Compile-time guard so callers can swap implementations without import cycles.
var _ error = errors.New("")
