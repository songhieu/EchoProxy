// Package capture is the outbound *capture-mode* helper: it wraps any
// http.RoundTripper so that every request and response is captured and shipped
// to ingest-api via sdk.Client. Your app makes the upstream call itself (no
// proxy hop) — use this when your runtime cannot reach proxy-gateway:8080.
//
// Usage:
//
//	client, _ := sdk.New(sdk.Config{
//	    APIKey:       os.Getenv("ECHOPROXY_API_KEY"),
//	    EndpointHTTP: os.Getenv("ECHOPROXY_ENDPOINT"),
//	})
//	defer client.Close(context.Background())
//
//	httpClient := &http.Client{
//	    Transport: capture.NewTransport(http.DefaultTransport, client),
//	}
//	res, err := httpClient.Get("https://api.example.com/users")
//
// Or wrap the default client process-wide:
//
//	http.DefaultClient.Transport = capture.NewTransport(nil, client)
//
// Latency breakdown is measured with httptrace so the dashboard's "upstream"
// and "TTFB" cells reflect the same numbers proxy-gateway would report. Body
// capture honors the Client's MaxBodyBytes; redaction is applied via the
// Client's configured Redactor. Failures are fail-open — capture never breaks
// the caller's request.
package capture

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	sdk "github.com/songhieu/EchoProxy/sdk-reference-go"
)

// Transport is an http.RoundTripper that captures every request/response and
// emits an outbound event via the wrapped Client. Drop it into any http.Client
// or any library that accepts an http.RoundTripper.
type Transport struct {
	// Base is the underlying RoundTripper used to make the upstream call.
	// Defaults to http.DefaultTransport when nil.
	Base http.RoundTripper

	client *sdk.Client
}

// NewTransport returns a Transport that captures via client. base may be nil
// to use http.DefaultTransport.
func NewTransport(base http.RoundTripper, client *sdk.Client) *Transport {
	return &Transport{Base: base, client: client}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// httptrace runs on a copy of the request to avoid disturbing callers
	// that read req.Context after the call.
	var (
		start         = time.Now()
		ttfbAt        time.Time
		gotFirstByte  bool
		upstreamStart = start
	)
	trace := &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) {
			upstreamStart = time.Now()
		},
		GotFirstResponseByte: func() {
			if !gotFirstByte {
				ttfbAt = time.Now()
				gotFirstByte = true
			}
		},
	}
	traced := req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	// Capture request body without breaking the caller — read, buffer, and
	// rewind via GetBody if available.
	reqBody, restored := snapshotReqBody(traced)
	if restored != nil {
		traced.Body = restored
	}

	res, err := base.RoundTrip(traced)
	totalLatency := time.Since(start)

	if err != nil || res == nil {
		// Still emit an event so failures show up in the dashboard.
		t.client.CaptureWithDirection(sdk.DirectionOutbound, sdk.CaptureInput{
			Method:     req.Method,
			Scheme:     req.URL.Scheme,
			Host:       req.URL.Host,
			Path:       req.URL.Path,
			Query:      req.URL.RawQuery,
			Status:     0,
			Latency:    totalLatency,
			ReqHeaders: req.Header,
			ReqBody:    reqBody,
		})
		return res, err
	}

	resBody, restoredRes := snapshotResBody(res)
	if restoredRes != nil {
		res.Body = restoredRes
	}

	var ttfb time.Duration
	if gotFirstByte {
		ttfb = ttfbAt.Sub(upstreamStart)
	}
	upstream := totalLatency
	if !upstreamStart.IsZero() {
		upstream = time.Since(upstreamStart)
	}

	t.client.CaptureWithDirection(sdk.DirectionOutbound, sdk.CaptureInput{
		Method:          req.Method,
		Scheme:          req.URL.Scheme,
		Host:            req.URL.Host,
		Path:            req.URL.Path,
		Query:           req.URL.RawQuery,
		Status:          res.StatusCode,
		Latency:         totalLatency,
		UpstreamLatency: upstream,
		UpstreamTTFB:    ttfb,
		ReqHeaders:      req.Header,
		ResHeaders:      res.Header,
		ReqBody:         reqBody,
		ResBody:         resBody,
	})
	return res, nil
}

// snapshotReqBody drains the request body so we can capture it, then returns
// a restored io.ReadCloser for the underlying RoundTripper to consume.
func snapshotReqBody(req *http.Request) ([]byte, io.ReadCloser) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	// Prefer GetBody when present — preserves the original stream cheaply.
	if req.GetBody != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, nil
		}
		fresh, gErr := req.GetBody()
		if gErr != nil {
			fresh = io.NopCloser(bytes.NewReader(b))
		}
		return b, fresh
	}
	b, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, nil
	}
	return b, io.NopCloser(bytes.NewReader(b))
}

// snapshotResBody drains the response body so we can capture it, then returns
// a fresh io.ReadCloser to hand back to the caller.
func snapshotResBody(res *http.Response) ([]byte, io.ReadCloser) {
	if res.Body == nil {
		return nil, nil
	}
	b, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		return nil, nil
	}
	return b, io.NopCloser(bytes.NewReader(b))
}
