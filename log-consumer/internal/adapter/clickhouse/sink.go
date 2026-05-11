package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/songhieu/EchoProxy/log-consumer/internal/domain"
)

// Sink batch-inserts rows into the http_events table using the
// driver-native columnar API for throughput.
type Sink struct{ conn driver.Conn }

func NewSink(dsn string) (*Sink, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse parse dsn: %w", err)
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	return &Sink{conn: conn}, nil
}

func (s *Sink) Ping(ctx context.Context) error { return s.conn.Ping(ctx) }
func (s *Sink) Close() error                   { return s.conn.Close() }

func (s *Sink) Insert(ctx context.Context, rows []domain.Row) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO http_events (
		ts, event_id, project_id, api_key_id, source, direction, sdk_version,
		method, scheme, host, path, query, status, latency_ms, upstream_latency_ms, upstream_ttfb_ms,
		req_size, res_size,
		req_headers, res_headers, req_body, res_body, req_truncated, res_truncated,
		client_ip, user_agent, trace_id, attributes, error
	)`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, r := range rows {
		ts := time.UnixMilli(r.TimestampMs)
		direction := r.Direction
		if direction == "" && r.Source == "proxy" {
			direction = "outbound"
		}
		if err := batch.Append(
			ts, r.EventID, r.ProjectID, r.APIKeyID, r.Source, direction, r.SDKVersion,
			r.Method, r.Scheme, r.Host, r.Path, r.Query, r.Status, r.LatencyMs, r.UpstreamLatencyMs, r.UpstreamTtfbMs,
			r.ReqSize, r.ResSize,
			r.ReqHeaders, r.ResHeaders, r.ReqBody, r.ResBody, r.ReqTruncated, r.ResTruncated,
			r.ClientIP, r.UserAgent, r.TraceID, r.Attributes, r.Error,
		); err != nil {
			return fmt.Errorf("append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send batch: %w", err)
	}
	return nil
}
