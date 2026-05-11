# Runbook

## First-time setup

```bash
git clone <repo>
cd echoproxy
make up           # bring up everything
make migrate      # apply Postgres + ClickHouse migrations
```

Open http://localhost:3000, sign up, create a project, create an API key, copy the raw key.

## Smoke test

```bash
SID_KEY="sk_live_xxx"   # the raw key from the dashboard

# Round-trip a request through the proxy
curl -i \
  -H "X-Echo-Key: $SID_KEY" \
  -H "X-Echo-Target: https://httpbin.org" \
  http://localhost:8080/get

# Verify a row landed in ClickHouse (within ~2s)
docker compose exec clickhouse clickhouse-client \
  --query "SELECT count() FROM echoproxy.http_events WHERE source='proxy'"
```

The dashboard's Logs tab should show the event. Analytics tab populates after a few minutes' worth of traffic (the materialized view aggregates per minute).

## Send events from the SDK

```go
import sdk "echoproxy/sdk-reference-go"

c, _ := sdk.New(sdk.Config{
    APIKey:       os.Getenv("SID_KEY"),
    EndpointHTTP: "http://localhost:8081",
})
defer c.Close(context.Background())

http.Handle("/", c.Middleware(myApp))
```

## Common operations

### Tail logs

```bash
make logs                   # all services
docker compose logs -f proxy-gateway
```

### Check proxy metrics

```bash
curl -s http://localhost:6060/metrics | grep proxy_request_duration
```

### Profile the proxy

```bash
curl -s "http://localhost:6060/debug/pprof/profile?seconds=30" > cpu.pprof
go tool pprof -http=:8090 cpu.pprof
```

### Run benchmark

```bash
make bench-proxy   # k6 against local proxy + mock upstream
```

Expected: p99 < 20ms at 5000 RPS, 0 dropped events.

### Apply a new migration

Add a numbered SQL file to `migrations/postgres/` or `migrations/clickhouse/`, then:

```bash
make migrate-postgres    # or migrate-clickhouse
```

### Regenerate proto

```bash
make proto-gen
```

Required after editing `api/event.proto`. CI fails if generated code is out of date.

## Troubleshooting

### `proxy_dropped_events_total` keeps climbing

- Increase `EVENT_CHAN_SIZE` env on proxy-gateway.
- Add Kafka partitions / brokers; current default is 3 partitions.
- Check `kafka:9092` reachability from proxy-gateway.

### p99 latency > 20ms

- Capture pprof profile (see above) and look for I/O in the hot path. Anything talking to Postgres/Redis/Kafka synchronously is a bug.
- Verify `MaxIdleConnsPerHost` is being saturated (i.e. that connection reuse is working). DNS lookups every request kill latency too.
- Confirm body cap isn't being hit massively (`proxy_body_truncated_total{side="req"}`).

### log-consumer not inserting

- Check it can reach ClickHouse: `docker compose logs log-consumer`.
- Verify the topic has data: `docker compose exec kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic http_events --max-messages 1`.
- Look for ClickHouse errors in `system.query_log`.

### Dashboard 401s

- The JWT_SECRET in `auth-api` and `stats-api` must match.
- NextAuth `NEXTAUTH_SECRET` must be set in `dashboard`.

### API key not recognized by proxy

- The cache refresh interval is 10s; new keys take up to that long to propagate.
- Force a refresh by restarting `proxy-gateway`, or shorten `APIKEY_REFRESH_SECONDS`.

## Production checklist (when ready)

- [ ] Switch `JWT_SECRET` and `NEXTAUTH_SECRET` to long random values.
- [ ] Enable TLS termination in front of proxy-gateway and the dashboard.
- [ ] Postgres + ClickHouse with managed backups.
- [ ] Kafka with `acks=all` and at least 3 brokers.
- [ ] Add per-API-key rate limiting (Redis token bucket).
- [ ] Restrict `proxy-gateway` admin port (6060) to the internal network.
- [ ] Set up Grafana alerts on `proxy_request_duration_seconds:p99 > 0.018` and `proxy_dropped_events_total > 0`.
