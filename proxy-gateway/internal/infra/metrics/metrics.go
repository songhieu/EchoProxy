package metrics

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Recorder fans out hot-path measurements into Prometheus metrics. The
// public methods are intentionally cheap (no map lookups) for the proxy
// hot path.
type Recorder struct {
	requestDuration *prometheus.HistogramVec
	dropped         prometheus.Counter
	truncated       *prometheus.CounterVec
	cacheHit        prometheus.Counter
	cacheMiss       prometheus.Counter
	droppedTotal    *uint64
}

func NewRecorder() *Recorder {
	var dropped uint64
	r := &Recorder{
		requestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "proxy_request_duration_seconds",
			Help:    "End-to-end proxy overhead measured at the gateway.",
			Buckets: []float64{0.001, 0.002, 0.005, 0.010, 0.015, 0.020, 0.050, 0.100, 0.500, 1.0},
		}, []string{"method", "status_class"}),
		dropped: promauto.NewCounter(prometheus.CounterOpts{
			Name: "proxy_dropped_events_total",
			Help: "Events dropped due to internal queue overflow.",
		}),
		truncated: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_body_truncated_total",
			Help: "Bodies truncated by the proxy capped writer.",
		}, []string{"side"}),
		cacheHit: promauto.NewCounter(prometheus.CounterOpts{
			Name: "proxy_apikey_cache_hit_total",
		}),
		cacheMiss: promauto.NewCounter(prometheus.CounterOpts{
			Name: "proxy_apikey_cache_miss_total",
		}),
		droppedTotal: &dropped,
	}
	return r
}

func (r *Recorder) ObserveLatency(method, statusClass string, d time.Duration) {
	r.requestDuration.WithLabelValues(method, statusClass).Observe(d.Seconds())
}

func (r *Recorder) IncDropped() {
	atomic.AddUint64(r.droppedTotal, 1)
	r.dropped.Inc()
}

func (r *Recorder) IncTruncated(side string) { r.truncated.WithLabelValues(side).Inc() }

func (r *Recorder) IncCacheHit()  { r.cacheHit.Inc() }
func (r *Recorder) IncCacheMiss() { r.cacheMiss.Inc() }
