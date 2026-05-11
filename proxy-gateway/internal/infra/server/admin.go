package server

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AdminConfigView is the JSON payload returned by /admin/config so the
// dashboard can display the effective timeouts without needing access to
// the host's environment. Only safe-to-show fields are included.
type AdminConfigView struct {
	UpstreamTimeoutSeconds   int  `json:"upstream_timeout_seconds"`
	StreamIdleTimeoutSeconds int  `json:"stream_idle_timeout_seconds"`
	BodyCapBytes             int  `json:"body_cap_bytes"`
	AllowPrivateTargets      bool `json:"allow_private_targets"`
}

// NewAdminMux builds the admin handler exposing /metrics, /healthz, /readyz,
// /admin/config, and pprof endpoints. NEVER bind this to a public address.
func NewAdminMux(ready func() bool, cfgView func() AdminConfigView) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	mux.HandleFunc("/admin/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfgView())
	})
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}
