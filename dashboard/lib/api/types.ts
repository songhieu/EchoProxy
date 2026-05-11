// Mirrors backend OpenAPI shapes. Update in lockstep with Go services.

export type Project = {
  id: number;
  owner_id: number;
  name: string;
  retention_days: number;
  created_at: string;
};

export type RedactRules = {
  header_denylist?: string[];
  json_field_denylist?: string[];
  disable_defaults?: boolean;
};

export type APIKey = {
  id: number;
  prefix: string;
  raw?: string; // only present on creation
  allowlist: string[];
  body_cap: number;
  rate_limit_rps: number;
  redact_rules: RedactRules;
  status: "active" | "revoked";
  description: string;
  created_at: string;
};

export type Direction = "inbound" | "outbound" | "";

export type LogEvent = {
  ts: string;
  event_id: string;
  project_id: number;
  api_key_id: number;
  source: string;
  direction?: Direction;
  method: string;
  host: string;
  path: string;
  query: string;
  status: number;
  latency_ms: number;
  upstream_latency_ms: number;
  upstream_ttfb_ms: number;
  req_size: number;
  res_size: number;
  req_headers?: Record<string, string>;
  res_headers?: Record<string, string>;
  req_body?: string;
  res_body?: string;
  req_truncated?: boolean;
  res_truncated?: boolean;
  client_ip: string;
  user_agent: string;
  trace_id: string;
  error?: string;
  is_stream?: boolean;
  stream_chunk_count?: number;
  stream_duration_ms?: number;
  stream_idle_timeout?: boolean;
};

export type MinuteMetric = {
  minute: string;
  method: string;
  status_class: string;
  requests: number;
  errors: number;
  latency_p50: number;
  latency_p95: number;
  latency_p99: number;
};

export type TimeBucket = {
  bucket: string;
  ok: number;
  err_4xx: number;
  err_5xx: number;
  p50: number;
  p95: number;
  p99: number;
  max: number;
  upstream_p50: number;
  upstream_p95: number;
  upstream_p99: number;
  overhead_p99: number;
};

export type DistBucket = { key: string; count: number };

export type EndpointStat = {
  method: string;
  path: string;
  requests: number;
  errors: number;
  error_rate: number;
  p50: number;
  p95: number;
  p99: number;
  avg_latency_ms: number;
};
