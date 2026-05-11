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
	requestDuration  *prometheus.HistogramVec
	dropped          prometheus.Counter
	truncated        *prometheus.CounterVec
	cacheHit         prometheus.Counter
	cacheMiss        prometheus.Counter
	streamRequests   prometheus.Counter
	streamIdleTimeo  prometheus.Counter
	streamChunkCount prometheus.Histogram
	droppedTotal     *uint64
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
		streamRequests: promauto.NewCounter(prometheus.CounterOpts{
			Name: "proxy_stream_requests_total",
			Help: "Upstream responses detected as a stream (SSE / gRPC / chunked).",
		}),
		streamIdleTimeo: promauto.NewCounter(prometheus.CounterOpts{
			Name: "proxy_stream_idle_timeouts_total",
			Help: "Streams terminated by the idle-timeout watchdog.",
		}),
		streamChunkCount: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "proxy_stream_chunk_count",
			Help:    "Chunks flushed per streaming response.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 10000},
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

func (r *Recorder) IncStream()                 { r.streamRequests.Inc() }
func (r *Recorder) IncStreamIdleTimeout()      { r.streamIdleTimeo.Inc() }
func (r *Recorder) ObserveStreamChunks(n uint32) { r.streamChunkCount.Observe(float64(n)) }
