-- ClickHouse schema for HTTP event log + materialized views for analytics.

CREATE DATABASE IF NOT EXISTS echoproxy;

USE echoproxy;

CREATE TABLE IF NOT EXISTS http_events (
    ts             DateTime64(3, 'UTC') CODEC(DoubleDelta, LZ4),
    event_id       String,
    project_id     UInt64,
    api_key_id     UInt64,
    source         LowCardinality(String),
    sdk_version    LowCardinality(String),

    method         LowCardinality(String),
    scheme         LowCardinality(String),
    host           LowCardinality(String),
    path           String,
    query          String,
    status         UInt16,
    latency_ms     UInt32,
    req_size       UInt32,
    res_size       UInt32,

    req_headers    Map(LowCardinality(String), String),
    res_headers    Map(LowCardinality(String), String),
    req_body       String CODEC(ZSTD(3)),
    res_body       String CODEC(ZSTD(3)),
    req_truncated  UInt8,
    res_truncated  UInt8,

    client_ip      String,
    user_agent     String,
    trace_id       String,
    attributes     Map(LowCardinality(String), String),
    error          String,

    INDEX idx_path  path     TYPE tokenbf_v1(8192, 3, 0) GRANULARITY 4,
    INDEX idx_trace trace_id TYPE bloom_filter GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (project_id, api_key_id, ts)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Per-minute aggregates for the analytics dashboard.
CREATE TABLE IF NOT EXISTS http_events_minute (
    minute        DateTime,
    project_id    UInt64,
    api_key_id    UInt64,
    method        LowCardinality(String),
    status_class  LowCardinality(String),
    requests      AggregateFunction(count, UInt64),
    latency       AggregateFunction(quantilesTDigest(0.5, 0.95, 0.99), UInt32),
    errors        AggregateFunction(countIf, UInt64, UInt8)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(minute)
ORDER BY (project_id, api_key_id, minute, method, status_class);

CREATE MATERIALIZED VIEW IF NOT EXISTS http_events_minute_mv
TO http_events_minute AS
SELECT
    toStartOfMinute(ts)            AS minute,
    project_id,
    api_key_id,
    method,
    concat(toString(intDiv(status, 100)), 'xx') AS status_class,
    countState()                   AS requests,
    quantilesTDigestState(0.5, 0.95, 0.99)(latency_ms) AS latency,
    countIfState(status >= 400)    AS errors
FROM http_events
GROUP BY minute, project_id, api_key_id, method, status_class;
