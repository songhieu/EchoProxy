# Data retention

EchoProxy follows the tiered-retention pattern that observability platforms
have converged on (Datadog "live / indexed / archived", Sentry "default 30d
+ project override", Honeycomb 60d events, Cloudflare Logpush 7d on
platform). Cheap aggregates outlive expensive raw rows; bodies are the most
expensive bit and shrink first.

## Tiers in this repo

| Store | What | Default | Hard cap | Mechanism | Migration |
|-------|------|---------|----------|-----------|-----------|
| ClickHouse `http_events` | raw events, full headers + bodies | **30 days** per project | **90 days** | per-project: nightly DELETE mutation by `cleanup`. Cap: table TTL `INTERVAL 90 DAY` | 001 + 005 |
| ClickHouse `http_events_minute` | per-minute aggregates (count / latency percentiles / errors) | **180 days** | 180 days | `TTL toDateTime(minute) + INTERVAL 180 DAY` | 004 |
| Postgres `body_access_log` | who opened which body, for compliance audits | **90 days** | — | nightly cleanup job (`cleanup/cmd/cleanup`) | — |
| Kafka topic `http_events` | in-flight buffer between proxy/SDK and consumer | **24 hours** | — | broker setting `KAFKA_LOG_RETENTION_HOURS` | docker-compose env |

## Why each number

- **30 days raw** mirrors Sentry's free / Sentry Cloud default. Long enough to
  debug last month's incident, short enough that body storage doesn't blow up
  ([1B req/day × ~10 KB compressed × 30d = ~300 TB] is *already* the cap on a
  big stack — anything longer needs body offload, not more disk).
- **180 days aggregates** matches Honeycomb / Datadog metric retention.
  Aggregates compress ~100x vs raw so 6 months is cheap.
- **90 days audit log** is the floor most SOC2 / HIPAA programs accept for
  body-access trails. Below that, auditors complain.
- **24h Kafka** is just enough to replay if log-consumer is down a day.
  Longer is wasteful — events go to ClickHouse, replay from Kafka is the
  emergency case.

## Per-project override

Each project has `projects.retention_days` (Postgres column, 1..90 CHECK
constraint, default 30). Owners can change it from
`/projects/<id>/settings` in the dashboard, or directly via the auth-api:

```bash
curl -X PATCH -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"retention_days": 7}' \
     http://localhost:8083/v1/projects/1
```

The cleanup binary reads this column on each run and issues:

```sql
ALTER TABLE echoproxy.http_events
DELETE WHERE project_id = <id> AND ts < now() - INTERVAL <days> DAY
```

— an async ClickHouse mutation. CH applies it during the next merge cycle.
This is the pattern Sentry / Datadog / Honeycomb use to give per-tenant
retention without rewriting the whole table.

Projects at the 90-day hard cap are skipped (CH's table-level TTL already
covers them — no point issuing a redundant mutation).

## ClickHouse TTL mechanics

`TTL <expr>` on a MergeTree table runs as part of background merges. Rows
past the TTL get deleted **on merge**, not instantly. To force a sweep:

```sql
OPTIMIZE TABLE echoproxy.http_events FINAL;
```

Don't do this on the hot path; it rewrites entire parts. Schedule it during
low traffic if you need immediate purge (compliance request, GDPR delete).

## Forced delete for a single project

```sql
ALTER TABLE echoproxy.http_events DELETE WHERE project_id = ?;
```

This is a *mutation*, which is async + eventually consistent. Check
`system.mutations` for status:

```sql
SELECT * FROM system.mutations WHERE table = 'http_events' AND NOT is_done;
```

For GDPR-style "delete one user's traffic in <30 days" requests you typically
want both: the mutation above + waiting on the natural TTL sweep.

## Scheduling

The `cleanup` binary supports two modes via the `INTERVAL` env:

| Mode | When `INTERVAL` is… | Use case |
|------|---------------------|----------|
| **one-shot** (run once, exit) | unset | k8s CronJob, systemd timer, plain crontab — scheduler fires each tick |
| **loop** (run forever, sleep between sweeps) | set, e.g. `24h` | docker-compose, ECS service — no external scheduler |

### docker-compose (dev/staging) — already wired

The `cleanup` service in `docker-compose.yml` runs in loop mode with
`INTERVAL=24h`. Comes up with `make up`, runs immediately on start, sleeps
24h between sweeps, restarts on failure.

```bash
make up                                     # cleanup container included
docker logs echoproxy-cleanup-1 -f           # watch sweeps
docker compose restart cleanup              # force a fresh sweep
```

### Kubernetes (production) — manifest ready

`deploy/k8s/cleanup-cronjob.yaml` defines a CronJob at `17 3 * * *` UTC
(03:17 daily, off-peak). One-shot per fire; `concurrencyPolicy: Forbid`
prevents overlap; `startingDeadlineSeconds: 600` avoids the "thundering
herd" if a fire is delayed.

```bash
kubectl apply -f deploy/k8s/cleanup-cronjob.yaml

# Trigger manually (e.g. after a GDPR delete request)
kubectl create job --from=cronjob/echoproxy-cleanup \
    echoproxy-cleanup-manual-$(date +%s)
```

Update the `image:` field to point at your registry build of the cleanup
binary (`deploy/Dockerfile.go SERVICE=cleanup BIN=cleanup`).

### systemd timer (single-VM deploy)

```ini
# /etc/systemd/system/echoproxy-cleanup.timer
[Unit]
Description=EchoProxy nightly cleanup
[Timer]
OnCalendar=*-*-* 03:17:00
Persistent=true
[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/echoproxy-cleanup.service
[Unit]
Description=EchoProxy nightly cleanup
[Service]
Type=oneshot
Environment=POSTGRES_DSN=postgres://...
Environment=CLICKHOUSE_DSN=clickhouse://...
ExecStart=/usr/local/bin/echoproxy-cleanup
```

### Plain crontab (smallest deploy)

```cron
17 3 * * * POSTGRES_DSN=... CLICKHOUSE_DSN=... /usr/local/bin/echoproxy-cleanup >> /var/log/echoproxy-cleanup.log 2>&1
```

### Manual one-off

```bash
make cleanup                 # run once against the local stack
```

## How to schedule the ClickHouse OPTIMIZE (optional)

Background merges already enforce TTL. You only need to schedule OPTIMIZE if
you want immediate purge (compliance) or if merge backlog grows. Same
schedulers as above — pick a low-traffic window:

```bash
clickhouse-client --query "OPTIMIZE TABLE echoproxy.http_events FINAL"
```

## Verifying

```sql
-- TTL is set
SELECT name, engine_full FROM system.tables
WHERE database='echoproxy' AND name IN ('http_events', 'http_events_minute');

-- Oldest row per table
SELECT 'raw' AS t, min(ts) FROM echoproxy.http_events
UNION ALL
SELECT 'min', min(minute) FROM echoproxy.http_events_minute;
```

```sql
-- Postgres: oldest audit row
SELECT min(accessed_at) FROM body_access_log;
```
