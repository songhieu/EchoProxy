# Architecture

## Components

| Component        | Lang     | Role                                                         |
|------------------|----------|--------------------------------------------------------------|
| `proxy-gateway`  | Go       | Reverse proxy with `X-Echo-Target` dynamic routing; async log |
| `ingest-api`     | Go       | HTTP+gRPC ingestion endpoint for SDKs                        |
| `log-consumer`   | Go       | Drain Kafka → batch insert ClickHouse                        |
| `auth-api`       | Go       | Users / projects / API keys; issues JWT for the dashboard    |
| `stats-api`      | Go       | Read-side analytics (logs + metrics + top paths)             |
| `sdk-reference-go` | Go     | Reference SDK; template for other languages                  |
| `dashboard`      | Next.js  | Self-serve UI                                                |

## Data flow

1. **Proxy mode** — client sends `Method /path` with `X-Echo-Key` + `X-Echo-Target` headers. proxy-gateway validates the API key from the in-mem cache, parses & SSRF-checks the target, captures bodies via `io.TeeReader` (capped), forwards to upstream, returns the response, and asynchronously enqueues an event into a buffered channel. A worker pool drains the channel into Kafka via `franz-go`.
2. **SDK mode** — the SDK captures requests inside the host app, batches them in-process, and POSTs `/v1/events:batch` (or streams over gRPC) to ingest-api with `X-Echo-Key`. ingest-api validates, stamps `project_id`/`api_key_id` (authoritatively), and produces to the same Kafka topic.
3. **log-consumer** consumes the topic, batches per (1s, 1000 rows), and `INSERT`s into the `http_events` ClickHouse table.
4. **stats-api** reads `http_events` and the materialized aggregate `http_events_minute` for the dashboard. Hot queries are cached in Redis with a 30s TTL.
5. **dashboard** signs in via NextAuth → `auth-api` → JWT in an httpOnly cookie. Every backend call uses that JWT.

## Single source of truth

`api/event.proto` defines the event schema. Both proxy-gateway and ingest-api emit the same `HttpEvent`. Adding a new SDK = pointing at ingest-api with the right schema. Schema rules: see `.claude/skills/echoproxy-event-schema/SKILL.md`.

## Latency budget (proxy)

p99 overhead < 20ms (excluding upstream). Achieved by:

- API key validation from in-mem cache (`ristretto`); background loader from Postgres.
- Body capture via `io.TeeReader` writing into a `sync.Pool`-backed `*bytes.Buffer`, hard-capped (default 64KB).
- Drop-on-overflow event channel (`select` + `default`); a worker pool drains it into a `franz-go` async producer (`linger.ms=5`, `acks=leader`, zstd compression).
- Pre-warmed `http.Transport` (HTTP/2, big idle pool).
- Histogram bucket at 0.020s; admin port exposes `/metrics` + pprof.

Discipline rules: `.claude/skills/go-http-proxy-low-latency/SKILL.md`.

## Storage

- **Postgres** — operational data (users, projects, api_keys). Single source of truth for authorization.
- **Kafka** — event log buffer. Topic `http_events`, partitioned by `api_key_id`, `acks=leader`, zstd compressed.
- **ClickHouse** — analytics store. `http_events` (raw, 30-day TTL) + `http_events_minute` materialized aggregate for dashboard charts.
- **Redis** — query cache for stats-api hot paths.

## Auth model

- Dashboard users authenticate against auth-api with email+password (bcrypt).
- auth-api issues HS256 JWT (24h TTL); dashboard stores it in a NextAuth httpOnly cookie.
- stats-api validates the JWT independently using the shared `JWT_SECRET`.
- API keys are SHA-256 hashed at rest; the raw key is shown to the user only at creation.
- Each API key has an `allowlist` of target hosts — empty = allow-all (dev only).
- proxy-gateway rejects loopback / private IPs as upstream targets to mitigate SSRF.

## Failure modes

| Failure                        | Effect                                                | Recovery                                |
|--------------------------------|-------------------------------------------------------|-----------------------------------------|
| Kafka unreachable              | proxy drops events; SDK retries then drops            | Reconnect on next produce; metrics show |
| ClickHouse unreachable         | log-consumer retries the batch; offsets uncommitted   | Topic backlog grows; consume on recovery|
| Postgres unreachable           | API-key cache stays warm; no new keys until reconnect | Loader retries every 10s                |
| Body cap exceeded              | Truncate + flag `req_body_truncated`                  | Per-key override available              |
| API key revoked                | Cache TTL or refresh interval; ≤ 10s window           | Tighten via Postgres LISTEN/NOTIFY      |

## Roadmap (post-MVP)

- Live tail page (SSE bridge in stats-api consuming a Kafka tail consumer-group).
- HMAC request signing on ingest endpoints.
- Per-key rate limiting via Redis token bucket.
- Multi-tenant row-level security in ClickHouse.
- Laravel + Python SDK ports.
- k8s manifests / Helm chart.
