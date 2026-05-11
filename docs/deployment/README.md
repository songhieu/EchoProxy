# Deployment

EchoProxy is just containers + stateful infra (Kafka, ClickHouse, Postgres,
Redis). Below is the decision tree for picking a platform, then per-platform
guides.

## Pick your platform

| Scale | Platform | Why |
|-------|----------|-----|
| Dev / staging / < 5M req/day | [docker-compose](docker-compose.md) on a single VM | Cheapest, no orchestration. All-in-one with `make up`. |
| 5M – 1B req/day, you already run k8s | [Kubernetes (Helm)](kubernetes.md) + managed infra | Horizontal scale on the Go services; stateful infra hosted elsewhere |
| 5M – 1B req/day, no k8s yet | [docker-compose](docker-compose.md) on a beefy VM **or** [Fly.io](fly.md) | Skip k8s; managed-by-platform databases |
| > 1B req/day | [Kubernetes (Helm)](kubernetes.md) + ClickHouse cluster + Kafka cluster + body offload | At this scale you need shardable storage and proper isolation |

The split between **app** (proxy / ingest / consumer / auth / stats /
dashboard / cleanup) and **stateful infra** (Kafka / CH / PG / Redis) is the
key decision. The Helm chart only ships the app — infra is bring-your-own
(managed services or separate Bitnami charts) because:

1. Stateful operators (Strimzi, Altinity, CrunchyData...) have their own
   release cadence and operational expertise. Bundling = forking.
2. At any meaningful scale you'll want managed Kafka (Confluent / Aiven /
   MSK / Upstash), managed ClickHouse (ClickHouse Cloud / Aiven /
   Altinity.Cloud), managed Postgres (RDS / Cloud SQL / Neon / Supabase)
   anyway.

## Common across platforms

### Migrations

Postgres + ClickHouse `migrations/` files run automatically on a **first**
container start (via `docker-entrypoint-initdb.d`). After that, adding a
new migration requires running it manually — see each platform guide.

### Secrets to set

| Var | Where | Notes |
|-----|-------|-------|
| `JWT_SECRET` | auth-api + stats-api | ≥ 32 chars, random. Rotate forces re-login (token validation also re-checks user exists in DB, so stale tokens 401 cleanly). |
| `NEXTAUTH_SECRET` | dashboard | ≥ 32 chars, random. Used by NextAuth to sign session cookies. |
| `POSTGRES_DSN` | auth-api, stats-api, cleanup | Use `sslmode=require` in production. |
| `CLICKHOUSE_DSN` | log-consumer, stats-api, cleanup | TLS by default on managed services. |
| `KAFKA_BROKERS` | proxy-gateway, ingest-api, log-consumer | Comma-separated. With auth: `KAFKA_USERNAME`/`KAFKA_PASSWORD` (TODO docs). |

### Capacity sizing rule of thumb

For 1 billion req/day (~12k RPS sustained):

- proxy-gateway: 5–10 replicas (2 CPU, 1 GB each on Linux native)
- ingest-api: 2–4 replicas
- log-consumer: 4–8 replicas (must be ≤ Kafka partitions)
- Kafka: 3 brokers, **64–128 partitions** on `http_events` (default chart has 3 for dev)
- ClickHouse: 3-node cluster (sharded `Distributed` table on `project_id`)
- Postgres: 2 vCPU primary + 1 read replica
- Redis: 1 GB, single instance fine

Above 10B req/day you need body offload (separate S3 path) and per-project
quotas. See [retention.md](../retention.md) for the storage discussion.
