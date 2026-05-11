package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/songhieu/EchoProxy/stats-api/internal/domain"
)

type Repo struct{ conn driver.Conn }

func New(dsn string) (*Repo, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}
	return &Repo{conn: conn}, nil
}

func (r *Repo) Ping(ctx context.Context) error { return r.conn.Ping(ctx) }
func (r *Repo) Close() error                   { return r.conn.Close() }

func (r *Repo) ListLogs(ctx context.Context, f domain.LogsFilter) ([]domain.LogEvent, error) {
	var (
		args  []any
		where []string
	)
	where = append(where, "project_id = ?")
	args = append(args, f.ProjectID)
	if f.APIKeyID > 0 {
		where = append(where, "api_key_id = ?")
		args = append(args, f.APIKeyID)
	}
	if !f.From.IsZero() {
		where = append(where, "ts >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where = append(where, "ts < ?")
		args = append(args, f.To)
	}
	if f.Method != "" {
		where = append(where, "method = ?")
		args = append(args, f.Method)
	}
	if f.Status > 0 {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.PathLike != "" {
		where = append(where, "positionCaseInsensitive(path, ?) > 0")
		args = append(args, f.PathLike)
	}
	if f.Direction != "" {
		where = append(where, "direction = ?")
		args = append(args, f.Direction)
	}

	q := fmt.Sprintf(`
		SELECT ts, event_id, project_id, api_key_id, source, direction, method, host, path, query, status,
		       latency_ms, upstream_latency_ms, upstream_ttfb_ms,
		       req_size, res_size, client_ip, user_agent, trace_id, error
		FROM http_events
		WHERE %s
		ORDER BY ts DESC
		LIMIT ? OFFSET ?
	`, strings.Join(where, " AND "))
	args = append(args, f.Limit, f.Offset)

	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LogEvent
	for rows.Next() {
		var e domain.LogEvent
		if err := rows.Scan(
			&e.Timestamp, &e.EventID, &e.ProjectID, &e.APIKeyID, &e.Source, &e.Direction,
			&e.Method, &e.Host, &e.Path, &e.Query, &e.Status,
			&e.LatencyMs, &e.UpstreamLatencyMs, &e.UpstreamTtfbMs,
			&e.ReqSize, &e.ResSize, &e.ClientIP, &e.UserAgent, &e.TraceID, &e.Error,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) GetLog(ctx context.Context, projectID uint64, eventID string) (*domain.LogEvent, error) {
	const q = `
		SELECT ts, event_id, project_id, api_key_id, source, direction, method, host, path, query, status,
		       latency_ms, upstream_latency_ms, upstream_ttfb_ms,
		       req_size, res_size, req_headers, res_headers, req_body, res_body,
		       req_truncated, res_truncated, client_ip, user_agent, trace_id, error
		FROM http_events WHERE project_id = ? AND event_id = ? LIMIT 1
	`
	var (
		e            domain.LogEvent
		reqHeaders   map[string]string
		resHeaders   map[string]string
		reqTrunc     uint8
		resTrunc     uint8
	)
	row := r.conn.QueryRow(ctx, q, projectID, eventID)
	if err := row.Scan(
		&e.Timestamp, &e.EventID, &e.ProjectID, &e.APIKeyID, &e.Source, &e.Direction,
		&e.Method, &e.Host, &e.Path, &e.Query, &e.Status,
		&e.LatencyMs, &e.UpstreamLatencyMs, &e.UpstreamTtfbMs,
		&e.ReqSize, &e.ResSize, &reqHeaders, &resHeaders,
		&e.ReqBody, &e.ResBody, &reqTrunc, &resTrunc,
		&e.ClientIP, &e.UserAgent, &e.TraceID, &e.Error,
	); err != nil {
		return nil, err
	}
	e.ReqHeaders = reqHeaders
	e.ResHeaders = resHeaders
	e.ReqTruncated = reqTrunc == 1
	e.ResTruncated = resTrunc == 1
	return &e, nil
}

func (r *Repo) MinuteMetrics(ctx context.Context, projectID, apiKeyID uint64, from, to time.Time) ([]domain.MinuteMetric, error) {
	args := []any{projectID, from, to}
	keyClause := ""
	if apiKeyID > 0 {
		keyClause = "AND api_key_id = ?"
		args = append(args, apiKeyID)
	}
	q := fmt.Sprintf(`
		SELECT minute, method, status_class,
		       countMerge(requests) AS reqs,
		       countIfMerge(errors) AS errs,
		       toFloat64(quantilesTDigestMerge(0.5)(latency)[1]) AS p50,
		       toFloat64(quantilesTDigestMerge(0.95)(latency)[1]) AS p95,
		       toFloat64(quantilesTDigestMerge(0.99)(latency)[1]) AS p99
		FROM http_events_minute
		WHERE project_id = ? AND minute >= ? AND minute < ? %s
		GROUP BY minute, method, status_class
		ORDER BY minute
	`, keyClause)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MinuteMetric
	for rows.Next() {
		var m domain.MinuteMetric
		if err := rows.Scan(&m.Minute, &m.Method, &m.StatusClass, &m.Requests, &m.Errors, &m.LatencyP50, &m.LatencyP95, &m.LatencyP99); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repo) TopPaths(ctx context.Context, projectID uint64, from, to time.Time, limit int) ([]domain.TopPath, error) {
	const q = `
		SELECT path, method, count() AS c, avg(latency_ms) AS lat
		FROM http_events
		WHERE project_id = ? AND ts >= ? AND ts < ?
		GROUP BY path, method
		ORDER BY c DESC
		LIMIT ?
	`
	rows, err := r.conn.Query(ctx, q, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TopPath
	for rows.Next() {
		var p domain.TopPath
		if err := rows.Scan(&p.Path, &p.Method, &p.Count, &p.AvgLatency); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── Analytics ──────────────────────────────────────────────────────────────

func (r *Repo) TimeSeries(ctx context.Context, f domain.AnalyticsFilter) ([]domain.TimeBucket, error) {
	args, where := analyticsWhere(f)
	bucket := f.BucketSec
	if bucket == 0 {
		bucket = 60
	}
	q := fmt.Sprintf(`
		SELECT toStartOfInterval(ts, INTERVAL %d SECOND) AS bucket,
		       countIf(status >= 200 AND status < 400) AS ok,
		       countIf(status >= 400 AND status < 500) AS err_4xx,
		       countIf(status >= 500) AS err_5xx,
		       toFloat64(quantile(0.5)(latency_ms))  AS p50,
		       toFloat64(quantile(0.95)(latency_ms)) AS p95,
		       toFloat64(quantile(0.99)(latency_ms)) AS p99,
		       max(latency_ms) AS p100,
		       toFloat64(quantile(0.5)(upstream_latency_ms))  AS up_p50,
		       toFloat64(quantile(0.95)(upstream_latency_ms)) AS up_p95,
		       toFloat64(quantile(0.99)(upstream_latency_ms)) AS up_p99,
		       toFloat64(quantile(0.99)(if(latency_ms >= upstream_latency_ms, latency_ms - upstream_latency_ms, 0))) AS overhead_p99
		FROM http_events
		WHERE %s
		GROUP BY bucket
		ORDER BY bucket
	`, bucket, where)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TimeBucket
	for rows.Next() {
		var b domain.TimeBucket
		if err := rows.Scan(&b.Bucket, &b.OK, &b.Err4xx, &b.Err5xx, &b.P50, &b.P95, &b.P99, &b.Max,
			&b.UpstreamP50, &b.UpstreamP95, &b.UpstreamP99, &b.OverheadP99); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) StatusDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	args, where := analyticsWhere(f)
	q := fmt.Sprintf(`
		SELECT concat(toString(intDiv(status, 100)), 'xx') AS bucket, count() AS c
		FROM http_events WHERE %s GROUP BY bucket ORDER BY bucket
	`, where)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bucket
	for rows.Next() {
		var b domain.Bucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) MethodDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	args, where := analyticsWhere(f)
	q := fmt.Sprintf(`
		SELECT method AS bucket, count() AS c
		FROM http_events WHERE %s GROUP BY method ORDER BY c DESC LIMIT 10
	`, where)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bucket
	for rows.Next() {
		var b domain.Bucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) HostDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	args, where := analyticsWhere(f)
	q := fmt.Sprintf(`
		SELECT host AS bucket, count() AS c
		FROM http_events WHERE %s GROUP BY host ORDER BY c DESC LIMIT 20
	`, where)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bucket
	for rows.Next() {
		var b domain.Bucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) EndpointStats(ctx context.Context, f domain.AnalyticsFilter, limit int) ([]domain.EndpointStat, error) {
	args, where := analyticsWhere(f)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT method, path,
		       count() AS reqs,
		       countIf(status >= 400) AS errs,
		       toFloat64(countIf(status >= 400)) / count() AS err_rate,
		       toFloat64(quantile(0.5)(latency_ms)) AS p50,
		       toFloat64(quantile(0.95)(latency_ms)) AS p95,
		       toFloat64(quantile(0.99)(latency_ms)) AS p99,
		       toFloat64(avg(latency_ms)) AS avg_lat
		FROM http_events WHERE %s
		GROUP BY method, path
		ORDER BY reqs DESC
		LIMIT ?
	`, where)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EndpointStat
	for rows.Next() {
		var s domain.EndpointStat
		if err := rows.Scan(&s.Method, &s.Path, &s.Requests, &s.Errors, &s.ErrorRate, &s.P50, &s.P95, &s.P99, &s.AvgLatency); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func analyticsWhere(f domain.AnalyticsFilter) ([]any, string) {
	args := []any{f.ProjectID, f.From, f.To}
	clauses := []string{"project_id = ?", "ts >= ?", "ts < ?"}
	if f.APIKeyID > 0 {
		clauses = append(clauses, "api_key_id = ?")
		args = append(args, f.APIKeyID)
	}
	if f.Method != "" {
		clauses = append(clauses, "method = ?")
		args = append(args, f.Method)
	}
	if f.Host != "" {
		clauses = append(clauses, "host = ?")
		args = append(args, f.Host)
	}
	if f.PathLike != "" {
		clauses = append(clauses, "positionCaseInsensitive(path, ?) > 0")
		args = append(args, f.PathLike)
	}
	if f.Direction != "" {
		clauses = append(clauses, "direction = ?")
		args = append(args, f.Direction)
	}
	return args, strings.Join(clauses, " AND ")
}

// Compile-time check we expose the same shape as the interface.
var _ = errors.New
