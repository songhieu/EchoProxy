// Package sdk is the reference Go SDK for echoproxy. It implements the
// contract documented in docs/sdk-spec.md and serves as the template every
// other-language SDK should mirror.
package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/pkg/redact"
)

const Version = "0.1.0"
const SourceName = "sdk-go"

// Config controls SDK behavior. Sensible defaults make the zero value usable
// (with APIKey + EndpointHTTP filled in).
type Config struct {
	APIKey        string
	EndpointHTTP  string // e.g. http://localhost:8081
	EndpointGRPC  string // e.g. localhost:8082 (optional, enables gRPC transport)
	BufferSize    int           // events buffered before drop
	FlushInterval time.Duration // worst-case flush latency
	BatchSize     int           // flush threshold by count
	MaxBodyBytes  int           // hard cap per body
	SampleRate    float64       // 0.0..1.0

	// Redaction. Defaults catch Authorization, Cookie, JWT, AWS keys, etc.
	RedactHeaders         []string // additional headers (case-insensitive) to mask
	RedactJSONFields      []string // additional JSON fields to mask
	DisableRedactDefaults bool     // skip the package defaults (advanced)

	// Route filtering for the inbound Middleware. If both empty, capture all.
	// Else: capture iff path matches CaptureRoutes (or empty list) AND does
	// NOT match IgnoreRoutes. Patterns are full-string Go regexp.
	//
	// The middleware also reads ECHOPROXY_CAPTURE_ROUTES / ECHOPROXY_IGNORE_ROUTES
	// from the environment as comma-separated regex lists when these slices
	// are unset.
	CaptureRoutes []string
	IgnoreRoutes  []string
}

// Client is goroutine-safe. Construct it once per process and reuse.
type Client struct {
	cfg     Config
	ch      chan *event.HttpEvent
	dropped uint64
	wg      sync.WaitGroup
	stop    chan struct{}

	httpClient *http.Client
	grpcClient event.EventIngestClient
	grpcConn   *grpc.ClientConn

	redactor *redact.Redactor
	capture  []*regexp.Regexp
	ignore   []*regexp.Regexp
}

// New constructs and starts a Client. Always call Close before exit.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("APIKey required")
	}
	if cfg.EndpointHTTP == "" && cfg.EndpointGRPC == "" {
		return nil, errors.New("endpoint required")
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10_000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 64 * 1024
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 1.0
	}
	r := redact.New(redact.Rules{
		HeaderDenylist:    cfg.RedactHeaders,
		JSONFieldDenylist: cfg.RedactJSONFields,
		DisableDefaults:   cfg.DisableRedactDefaults,
	})
	captureRoutes := cfg.CaptureRoutes
	if len(captureRoutes) == 0 {
		captureRoutes = splitEnv("ECHOPROXY_CAPTURE_ROUTES")
	}
	ignoreRoutes := cfg.IgnoreRoutes
	if len(ignoreRoutes) == 0 {
		ignoreRoutes = splitEnv("ECHOPROXY_IGNORE_ROUTES")
	}
	c := &Client{
		cfg:        cfg,
		ch:         make(chan *event.HttpEvent, cfg.BufferSize),
		stop:       make(chan struct{}),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		redactor:   r,
		capture:    compilePatterns(captureRoutes),
		ignore:     compilePatterns(ignoreRoutes),
	}
	if cfg.EndpointGRPC != "" {
		conn, err := grpc.NewClient(cfg.EndpointGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("grpc dial: %w", err)
		}
		c.grpcConn = conn
		c.grpcClient = event.NewEventIngestClient(conn)
	}
	c.wg.Add(1)
	go c.flushLoop()
	return c, nil
}

// Close drains the buffer and shuts down workers.
func (c *Client) Close(ctx context.Context) error {
	close(c.stop)
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if c.grpcConn != nil {
		_ = c.grpcConn.Close()
	}
	return nil
}

// Dropped returns the number of events dropped due to a full buffer.
func (c *Client) Dropped() uint64 { return atomic.LoadUint64(&c.dropped) }

// Capture builds an event from an http.Request/Response pair and enqueues it.
// It is fail-open: errors are swallowed and counted, never propagated to the
// caller's request path.
type CaptureInput struct {
	Method     string
	Host       string
	Scheme     string
	Path       string
	Query      string
	Status     int
	Latency    time.Duration
	ReqHeaders http.Header
	ResHeaders http.Header
	ReqBody    []byte
	ResBody    []byte
	ClientIP   string
	UserAgent  string
	TraceID    string
	Attributes map[string]string
}

// Direction values stamped onto outgoing events.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// Capture is shorthand for CaptureWithDirection("", in) — leaves the field
// empty (server defaults outbound for proxy events).
func (c *Client) Capture(in CaptureInput) { c.CaptureWithDirection("", in) }

// CaptureWithDirection records an event tagged with the given direction.
// Use DirectionInbound from server middleware, DirectionOutbound from
// client wrappers.
func (c *Client) CaptureWithDirection(direction string, in CaptureInput) {
	if !c.shouldSample() {
		return
	}
	ev := &event.HttpEvent{
		EventId:     event.NewEventID(),
		TimestampNs: event.NowNanos(),
		Source:      SourceName,
		SdkVersion:  Version,
		Direction:   direction,
		Method:      in.Method,
		Scheme:      in.Scheme,
		Host:        in.Host,
		Path:        in.Path,
		Query:       in.Query,
		Status:      uint32(in.Status),
		LatencyMs:   uint32(in.Latency.Milliseconds()),
		ReqSize:     uint32(len(in.ReqBody)),
		ResSize:     uint32(len(in.ResBody)),
		ReqHeaders:  c.snapshotHeaders(in.ReqHeaders),
		ResHeaders:  c.snapshotHeaders(in.ResHeaders),
		ClientIp:    in.ClientIP,
		UserAgent:   in.UserAgent,
		TraceId:     in.TraceID,
		Attributes:  in.Attributes,
	}
	reqBody, reqTrunc := c.cap(in.ReqBody)
	resBody, resTrunc := c.cap(in.ResBody)
	ev.ReqBody = c.redactor.Body(reqBody, contentType(in.ReqHeaders))
	ev.ResBody = c.redactor.Body(resBody, contentType(in.ResHeaders))
	ev.ReqBodyTruncated = reqTrunc
	ev.ResBodyTruncated = resTrunc
	select {
	case c.ch <- ev:
	default:
		atomic.AddUint64(&c.dropped, 1)
	}
}

// Middleware is an HTTP server middleware that captures every inbound
// request matching the configured route filters and ships the event with
// direction="inbound".
func (c *Client) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.routeAllowed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		reqBody, _ := io.ReadAll(io.LimitReader(r.Body, int64(c.cfg.MaxBodyBytes)+1))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(reqBody))

		rec := &recorder{ResponseWriter: w, status: 200, body: bytes.NewBuffer(nil), maxBody: c.cfg.MaxBodyBytes}
		next.ServeHTTP(rec, r)

		c.CaptureWithDirection(DirectionInbound, CaptureInput{
			Method:     r.Method,
			Host:       r.Host,
			Scheme:     scheme(r),
			Path:       r.URL.Path,
			Query:      r.URL.RawQuery,
			Status:     rec.status,
			Latency:    time.Since(start),
			ReqHeaders: r.Header,
			ResHeaders: rec.Header(),
			ReqBody:    reqBody,
			ResBody:    rec.body.Bytes(),
			ClientIP:   r.RemoteAddr,
			UserAgent:  r.UserAgent(),
			TraceID:    r.Header.Get("traceparent"),
		})
	})
}

// ─── internals ──────────────────────────────────────────────────────────────
func (c *Client) flushLoop() {
	defer c.wg.Done()
	t := time.NewTicker(c.cfg.FlushInterval)
	defer t.Stop()
	batch := make([]*event.HttpEvent, 0, c.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.send(ctx, batch); err != nil {
			// Fail-open: count dropped, do not retry beyond send-level retry.
			atomic.AddUint64(&c.dropped, uint64(len(batch)))
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-c.stop:
			drainAll(c.ch, &batch)
			flush()
			return
		case <-t.C:
			flush()
		case ev := <-c.ch:
			batch = append(batch, ev)
			if len(batch) >= c.cfg.BatchSize {
				flush()
			}
		}
	}
}

func drainAll(ch chan *event.HttpEvent, batch *[]*event.HttpEvent) {
	for {
		select {
		case ev := <-ch:
			*batch = append(*batch, ev)
		default:
			return
		}
	}
}

func (c *Client) send(ctx context.Context, batch []*event.HttpEvent) error {
	if c.grpcClient != nil {
		md := metadata.New(map[string]string{"x-echo-key": c.cfg.APIKey})
		ctx = metadata.NewOutgoingContext(ctx, md)
		_, err := c.grpcClient.Ingest(ctx, &event.IngestRequest{Events: batch})
		return err
	}
	return c.sendHTTP(ctx, batch)
}

func (c *Client) sendHTTP(ctx context.Context, batch []*event.HttpEvent) error {
	type wireEvent struct {
		EventID          string            `json:"event_id"`
		TimestampNs      int64             `json:"timestamp_ns"`
		Source           string            `json:"source"`
		SDKVersion       string            `json:"sdk_version"`
		Method           string            `json:"method"`
		Scheme           string            `json:"scheme"`
		Host             string            `json:"host"`
		Path             string            `json:"path"`
		Query            string            `json:"query"`
		Status           uint32            `json:"status"`
		LatencyMs        uint32            `json:"latency_ms"`
		ReqSize          uint32            `json:"req_size"`
		ResSize          uint32            `json:"res_size"`
		ReqHeaders       map[string]string `json:"req_headers"`
		ResHeaders       map[string]string `json:"res_headers"`
		ReqBody          []byte            `json:"req_body"`
		ResBody          []byte            `json:"res_body"`
		ReqBodyTruncated bool              `json:"req_body_truncated"`
		ResBodyTruncated bool              `json:"res_body_truncated"`
		ClientIP         string            `json:"client_ip"`
		UserAgent        string            `json:"user_agent"`
		TraceID          string            `json:"trace_id"`
		Attributes       map[string]string `json:"attributes"`
	}
	wire := struct {
		Events []wireEvent `json:"events"`
	}{Events: make([]wireEvent, 0, len(batch))}
	for _, e := range batch {
		wire.Events = append(wire.Events, wireEvent{
			EventID: e.EventId, TimestampNs: e.TimestampNs, Source: e.Source, SDKVersion: e.SdkVersion,
			Method: e.Method, Scheme: e.Scheme, Host: e.Host, Path: e.Path, Query: e.Query,
			Status: e.Status, LatencyMs: e.LatencyMs, ReqSize: e.ReqSize, ResSize: e.ResSize,
			ReqHeaders: e.ReqHeaders, ResHeaders: e.ResHeaders, ReqBody: e.ReqBody, ResBody: e.ResBody,
			ReqBodyTruncated: e.ReqBodyTruncated, ResBodyTruncated: e.ResBodyTruncated,
			ClientIP: e.ClientIp, UserAgent: e.UserAgent, TraceID: e.TraceId, Attributes: e.Attributes,
		})
	}
	body, _ := json.Marshal(wire)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.EndpointHTTP+"/v1/events:batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Echo-Key", c.cfg.APIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("ingest http %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) cap(b []byte) ([]byte, bool) {
	if len(b) <= c.cfg.MaxBodyBytes {
		out := make([]byte, len(b))
		copy(out, b)
		return out, false
	}
	out := make([]byte, c.cfg.MaxBodyBytes)
	copy(out, b)
	return out, true
}

func (c *Client) snapshotHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	flat := make(map[string]string, len(h))
	for k, vs := range h {
		flat[k] = joinComma(vs)
	}
	return c.redactor.Headers(flat)
}

// routeAllowed implements the env-driven include/exclude logic described on
// Config.CaptureRoutes. Empty include = allow all; ignore wins over include.
func (c *Client) routeAllowed(path string) bool {
	for _, re := range c.ignore {
		if re.MatchString(path) {
			return false
		}
	}
	if len(c.capture) == 0 {
		return true
	}
	for _, re := range c.capture {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func splitEnv(name string) []string {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	parts := []string{}
	for _, p := range bytesSplit(v, ',') {
		t := bytesTrim(p)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return parts
}

func bytesSplit(s string, sep byte) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func bytesTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func compilePatterns(raw []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(raw))
	for _, r := range raw {
		re, err := regexp.Compile(r)
		if err == nil {
			out = append(out, re)
		}
	}
	return out
}

func (c *Client) shouldSample() bool {
	if c.cfg.SampleRate >= 1.0 {
		return true
	}
	if c.cfg.SampleRate <= 0 {
		return false
	}
	// Cheap, OK for sampling — not security-relevant.
	return float64(time.Now().UnixNano()%1_000_000)/1_000_000.0 < c.cfg.SampleRate
}

// recorder lets the middleware capture status + a bounded copy of the body.
type recorder struct {
	http.ResponseWriter
	status  int
	body    *bytes.Buffer
	maxBody int
	written int
}

func (r *recorder) WriteHeader(code int)       { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *recorder) Write(p []byte) (int, error) {
	if r.body.Len() < r.maxBody {
		room := r.maxBody - r.body.Len()
		if room > len(p) {
			room = len(p)
		}
		_, _ = r.body.Write(p[:room])
	}
	n, err := r.ResponseWriter.Write(p)
	r.written += n
	return n, err
}

func contentType(h http.Header) string {
	if h == nil {
		return ""
	}
	return h.Get("Content-Type")
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func joinComma(s []string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for i := 1; i < len(s); i++ {
		out += "," + s[i]
	}
	return out
}

