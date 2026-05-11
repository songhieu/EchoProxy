package usecase

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"echoproxy/pkg/event"
	"echoproxy/pkg/redact"
	"echoproxy/proxy-gateway/internal/domain"
)

// Metrics is the minimal interface the use case needs. The real
// implementation lives in adapter/infra/metrics.
type Metrics interface {
	ObserveLatency(method string, statusClass string, d time.Duration)
	IncDropped()
	IncTruncated(side string)
}

// ProxyRequest forwards a request to the upstream described by X-Echo-Target,
// captures the request/response (bounded), and asynchronously enqueues an
// event for the Kafka pipeline. The hot path performs no blocking I/O beyond
// the upstream RoundTrip.
type ProxyRequest struct {
	transport       *http.Transport
	sink            domain.EventSink
	bufPool         *sync.Pool
	bodyCap         int
	metrics         Metrics
	defaultRedactor *redact.Redactor
}

func NewProxyRequest(transport *http.Transport, sink domain.EventSink, bodyCap int, m Metrics, redactor *redact.Redactor) *ProxyRequest {
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
		metrics:         m,
		defaultRedactor: redactor,
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

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), bodyForUpstream)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		uc.emit(r, key, target, http.StatusBadRequest, start, time.Since(start), 0, 0, reqBuf, resBuf, cappedReq.truncated, false, nil, "", err.Error())
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Del("X-Echo-Key")
	outReq.Header.Del("X-Echo-Target")
	outReq.Host = target.Host

	// httptrace captures the first byte of the upstream response so we can
	// expose TTFB separately from total upstream latency.
	var (
		firstByteAt time.Time
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
		uc.emit(r, key, target, http.StatusBadGateway, start, time.Since(start), upstreamLatency, ttfb, reqBuf, resBuf, cappedReq.truncated, false, nil, "", err.Error())
		return
	}
	defer resp.Body.Close()

	resHeaders := flattenHeaders(resp.Header)
	resContentType := resp.Header.Get("Content-Type")
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	mw := io.MultiWriter(w, cappedRes)
	_, _ = io.Copy(mw, resp.Body)

	d := time.Since(start)
	uc.metrics.ObserveLatency(r.Method, statusClass(resp.StatusCode), d)
	if cappedReq.truncated {
		uc.metrics.IncTruncated("req")
	}
	if cappedRes.truncated {
		uc.metrics.IncTruncated("res")
	}

	uc.emit(r, key, target, resp.StatusCode, start, d, upstreamLatency, ttfb, reqBuf, resBuf, cappedReq.truncated, cappedRes.truncated, resHeaders, resContentType, "")
}

func (uc *ProxyRequest) emit(r *http.Request, key *domain.APIKey, target *url.URL, status int,
	arrival time.Time, total, upstream, ttfb time.Duration,
	reqBuf, resBuf *bytes.Buffer, reqTrunc, resTrunc bool, resHeaders map[string]string, resContentType, errStr string) {
	red := key.Redactor
	if red == nil {
		red = uc.defaultRedactor
	}
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
		ReqBody:          red.Body(append([]byte(nil), reqBuf.Bytes()...), r.Header.Get("Content-Type")),
		ResBody:          red.Body(append([]byte(nil), resBuf.Bytes()...), resContentType),
		ReqBodyTruncated: reqTrunc,
		ResBodyTruncated: resTrunc,
		ClientIP:         clientIP(r),
		UserAgent:        r.UserAgent(),
		TraceID:          r.Header.Get("traceparent"),
		Error:            errStr,
		Direction:        "outbound",
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
