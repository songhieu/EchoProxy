// Package proxy is a drop-in replacement for net/http that routes every
// request through the echoproxy proxy. The function signatures mirror http.*
// so existing call-sites need only their import swapped.
//
// Configuration is env-driven and read once at first use:
//
//	ECHOPROXY_API_KEY    required, the raw sk_live_… value
//	ECHOPROXY_PROXY_URL  default http://localhost:8080
//
// Usage:
//
//	import sid "github.com/songhieu/EchoProxy/sdk-reference-go/proxy"
//
//	res, err := sid.Get("https://api.example.com/users")
//	res, err := sid.Post("https://api.example.com/users", "application/json", body)
package proxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// ErrAPIKeyMissing is returned when ECHOPROXY_API_KEY is not set in the
// environment and no override has been configured via Configure.
var ErrAPIKeyMissing = errors.New("echoproxy: ECHOPROXY_API_KEY env not set")

type config struct {
	apiKey   string
	proxyURL *url.URL
}

var (
	cfgOnce sync.Once
	cfg     config
	cfgErr  error
)

func loadEnv() {
	apiKey := os.Getenv("ECHOPROXY_API_KEY")
	raw := os.Getenv("ECHOPROXY_PROXY_URL")
	if raw == "" {
		raw = "http://localhost:8080"
	}
	u, err := url.Parse(raw)
	if err != nil {
		cfgErr = fmt.Errorf("echoproxy: invalid ECHOPROXY_PROXY_URL %q: %w", raw, err)
		return
	}
	cfg = config{apiKey: apiKey, proxyURL: u}
}

func ensure() error {
	cfgOnce.Do(loadEnv)
	if cfgErr != nil {
		return cfgErr
	}
	if cfg.apiKey == "" {
		return ErrAPIKeyMissing
	}
	return nil
}

// Configure overrides the env-derived defaults. Call before issuing any
// request when you need explicit credentials (e.g. tests, multi-tenant apps).
func Configure(apiKey, proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("echoproxy: invalid proxy url: %w", err)
	}
	cfgOnce.Do(func() {})
	cfg = config{apiKey: apiKey, proxyURL: u}
	cfgErr = nil
	return nil
}

// Transport implements http.RoundTripper. Drop it into an existing
// http.Client to retain custom timeouts, dialers, etc:
//
//	c := &http.Client{Timeout: 5*time.Second, Transport: &proxy.Transport{}}
type Transport struct {
	// Base wraps an existing RoundTripper. Defaults to http.DefaultTransport.
	Base http.RoundTripper
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	target := &url.URL{Scheme: r.URL.Scheme, Host: r.URL.Host}
	r.URL.Scheme = cfg.proxyURL.Scheme
	r.URL.Host = cfg.proxyURL.Host
	r.Host = cfg.proxyURL.Host
	r.Header.Set("X-Echo-Key", cfg.apiKey)
	r.Header.Set("X-Echo-Target", target.String())

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

// DefaultClient is the http.Client used by the package-level helpers below.
// Replace it (e.g. to add a custom timeout) before issuing any request.
var DefaultClient = &http.Client{Transport: &Transport{}}

// Client returns a copy of DefaultClient — useful when you want to tweak
// Timeout / Jar without mutating the package default.
func Client() *http.Client {
	cp := *DefaultClient
	return &cp
}

// ─── http.* mirror ───────────────────────────────────────────────────────────

// Get is the proxy-routed equivalent of http.Get.
func Get(url string) (*http.Response, error) { return DefaultClient.Get(url) }

// Head is the proxy-routed equivalent of http.Head.
func Head(url string) (*http.Response, error) { return DefaultClient.Head(url) }

// Post is the proxy-routed equivalent of http.Post.
func Post(url, contentType string, body io.Reader) (*http.Response, error) {
	return DefaultClient.Post(url, contentType, body)
}

// PostForm is the proxy-routed equivalent of http.PostForm.
func PostForm(url string, data map[string][]string) (*http.Response, error) {
	body := strings.NewReader(encodeForm(data))
	return Post(url, "application/x-www-form-urlencoded", body)
}

// Do mirrors (*http.Client).Do via the configured proxy.
func Do(req *http.Request) (*http.Response, error) { return DefaultClient.Do(req) }

func encodeForm(data map[string][]string) string {
	var sb strings.Builder
	first := true
	for k, vs := range data {
		for _, v := range vs {
			if !first {
				sb.WriteByte('&')
			}
			first = false
			sb.WriteString(url.QueryEscape(k))
			sb.WriteByte('=')
			sb.WriteString(url.QueryEscape(v))
		}
	}
	return sb.String()
}
