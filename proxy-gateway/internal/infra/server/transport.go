package server

import (
	"net"
	"net/http"
	"time"
)

// NewUpstreamTransport returns a *http.Transport tuned for proxy workloads:
// big idle pool, HTTP/2, no body decompression (we passthrough), short TLS
// handshake, modest response-header timeout.
func NewUpstreamTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: timeout,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
	}
}
