package domain

import (
	"context"
	"time"
)

type LogEvent struct {
	Timestamp     time.Time         `json:"ts"`
	EventID       string            `json:"event_id"`
	ProjectID     uint64            `json:"project_id"`
	APIKeyID      uint64            `json:"api_key_id"`
	Source        string            `json:"source"`
	Direction     string            `json:"direction"`
	Method        string            `json:"method"`
	Host          string            `json:"host"`
	Path          string            `json:"path"`
	Query         string            `json:"query"`
	Status            uint16            `json:"status"`
	LatencyMs         uint32            `json:"latency_ms"`
	UpstreamLatencyMs uint32            `json:"upstream_latency_ms"`
	UpstreamTtfbMs    uint32            `json:"upstream_ttfb_ms"`
	ReqSize           uint32            `json:"req_size"`
	ResSize           uint32            `json:"res_size"`
	ReqHeaders    map[string]string `json:"req_headers"`
	ResHeaders    map[string]string `json:"res_headers"`
	ReqBody       string            `json:"req_body,omitempty"`
	ResBody       string            `json:"res_body,omitempty"`
	ReqTruncated  bool              `json:"req_truncated"`
	ResTruncated  bool              `json:"res_truncated"`
	ClientIP      string            `json:"client_ip"`
	UserAgent     string            `json:"user_agent"`
	TraceID       string            `json:"trace_id"`
	Error         string            `json:"error,omitempty"`

	IsStream          bool   `json:"is_stream"`
	StreamChunkCount  uint32 `json:"stream_chunk_count"`
	StreamDurationMs  uint32 `json:"stream_duration_ms"`
	StreamIdleTimeout bool   `json:"stream_idle_timeout"`
}

type LogsFilter struct {
	ProjectID uint64
	APIKeyID  uint64 // 0 = all keys
	From      time.Time
	To        time.Time
	Method    string
	Status    uint16
	PathLike  string
	Direction string // "inbound" | "outbound" | "" (any)
	IsStream  *bool  // nil = any; true = streams only; false = non-streams
	Limit     int
	Offset    int
}

type MinuteMetric struct {
	Minute       time.Time `json:"minute"`
	Method       string    `json:"method"`
	StatusClass  string    `json:"status_class"`
	Requests     uint64    `json:"requests"`
	Errors       uint64    `json:"errors"`
	LatencyP50   float64   `json:"latency_p50"`
	LatencyP95   float64   `json:"latency_p95"`
	LatencyP99   float64   `json:"latency_p99"`
}

type Repository interface {
	ListLogs(ctx context.Context, f LogsFilter) ([]LogEvent, error)
	GetLog(ctx context.Context, projectID uint64, eventID string) (*LogEvent, error)
	MinuteMetrics(ctx context.Context, projectID, apiKeyID uint64, from, to time.Time) ([]MinuteMetric, error)
	TopPaths(ctx context.Context, projectID uint64, from, to time.Time, limit int) ([]TopPath, error)

	TimeSeries(ctx context.Context, f AnalyticsFilter) ([]TimeBucket, error)
	StatusDistribution(ctx context.Context, f AnalyticsFilter) ([]Bucket, error)
	MethodDistribution(ctx context.Context, f AnalyticsFilter) ([]Bucket, error)
	HostDistribution(ctx context.Context, f AnalyticsFilter) ([]Bucket, error)
	EndpointStats(ctx context.Context, f AnalyticsFilter, limit int) ([]EndpointStat, error)
}

type TopPath struct {
	Path       string  `json:"path"`
	Method     string  `json:"method"`
	Count      uint64  `json:"count"`
	AvgLatency float64 `json:"avg_latency_ms"`
}

// TimeBucket is a single point in a latency + volume time series. The
// frontend renders separate charts off the same series.
type TimeBucket struct {
	Bucket       time.Time `json:"bucket"`
	OK           uint64    `json:"ok"`     // 2xx + 3xx
	Err4xx       uint64    `json:"err_4xx"`
	Err5xx       uint64    `json:"err_5xx"`
	P50          float64   `json:"p50"`
	P95          float64   `json:"p95"`
	P99          float64   `json:"p99"`
	Max          uint32    `json:"max"`
	UpstreamP50  float64   `json:"upstream_p50"`
	UpstreamP95  float64   `json:"upstream_p95"`
	UpstreamP99  float64   `json:"upstream_p99"`
	OverheadP99  float64   `json:"overhead_p99"`
}

// Bucket summarizes one row of a count-by-key distribution.
type Bucket struct {
	Key   string `json:"key"`
	Count uint64 `json:"count"`
}

// EndpointStat is the per-(method,path) aggregate used by the endpoint table.
type EndpointStat struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Requests   uint64  `json:"requests"`
	Errors     uint64  `json:"errors"`
	ErrorRate  float64 `json:"error_rate"`
	P50        float64 `json:"p50"`
	P95        float64 `json:"p95"`
	P99        float64 `json:"p99"`
	AvgLatency float64 `json:"avg_latency_ms"`
}

// AnalyticsFilter narrows the query window for analytics endpoints.
type AnalyticsFilter struct {
	ProjectID uint64
	APIKeyID  uint64 // 0 = all keys for the project
	Method    string // "" = all methods
	Host      string // exact host match; "" = all
	PathLike  string // case-insensitive path substring; "" = all
	Direction string // "inbound" | "outbound" | "" (any)
	From      time.Time
	To        time.Time
	BucketSec uint32 // for time series
}
