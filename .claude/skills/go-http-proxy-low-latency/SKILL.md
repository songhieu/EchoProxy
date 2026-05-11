---
name: go-http-proxy-low-latency
description: Patterns and rules to keep proxy-gateway under p99 < 20ms — async logging, body capture via TeeReader, sync.Pool, franz-go tuning, pprof, benchmark gate. Apply when creating/modifying code in proxy-gateway/internal/usecase or adapter/http; adding middleware/features to the proxy; tuning the Kafka producer; profiling latency; capturing new fields; or reviewing PRs that touch the hot path.
---

# Low-Latency HTTP Proxy in Go

`proxy-gateway` has a hard requirement: **p99 overhead < 20ms** (excluding upstream latency). This skill is the compass: every change in the hot path must be validated against it before merging.

## 1. What is the hot path?

The hot path = from the moment we receive the `*http.Request` to the moment we've finished `Write`-ing the response back to the client. Any blocking I/O in this window can blow the budget.

NOT allowed in the hot path:
- ❌ Postgres queries (even via a connection pool).
- ❌ Redis calls.
- ❌ Synchronous Kafka `Produce`.
- ❌ DNS lookups that aren't cached.
- ❌ Full-event JSON marshaling.
- ❌ Logging to disk (zerolog → file).
- ❌ Acquiring a globally contended mutex.
- ❌ `time.Sleep` (even 1ms).

Allowed:
- ✅ In-memory map / sync.Map / ristretto Get.
- ✅ Copying bytes into a buffer from a sync.Pool.
- ✅ Atomic counter increments.
- ✅ Pushing into a buffered channel (non-blocking via `select` with `default`).
- ✅ Forwarding HTTP through a pre-warmed `http.Transport` (only the real network cost).

## 2. Async logging — required pattern

```go
// usecase/proxy_request.go
type ProxyRequest struct {
    transport *http.Transport
    eventCh   chan *event.HttpEvent  // buffered, e.g. 100k
    bufPool   *sync.Pool             // pool of *bytes.Buffer
    bodyCap   int                    // 64KB default
    metrics   *metrics.Recorder
}

func (uc *ProxyRequest) Execute(w http.ResponseWriter, r *http.Request, key *domain.APIKey, target string) {
    start := time.Now()
    reqBuf := uc.bufPool.Get().(*bytes.Buffer)
    resBuf := uc.bufPool.Get().(*bytes.Buffer)
    defer func() {
        reqBuf.Reset(); resBuf.Reset()
        uc.bufPool.Put(reqBuf); uc.bufPool.Put(resBuf)
    }()

    // Capture request body via TeeReader (bounded)
    cappedReq := &cappedWriter{buf: reqBuf, max: uc.bodyCap}
    r.Body = io.NopCloser(io.TeeReader(r.Body, cappedReq))

    // Build outbound request
    outReq, _ := http.NewRequestWithContext(r.Context(), r.Method, target+r.URL.Path, r.Body)
    copyHeaders(outReq.Header, r.Header)

    resp, err := uc.transport.RoundTrip(outReq)
    if err != nil {
        http.Error(w, "bad gateway", http.StatusBadGateway)
        uc.enqueue(buildErrorEvent(key, r, err, time.Since(start), reqBuf, cappedReq.truncated))
        return
    }
    defer resp.Body.Close()

    // Capture response body via TeeReader to client + buffer
    cappedRes := &cappedWriter{buf: resBuf, max: uc.bodyCap}
    copyHeaders(w.Header(), resp.Header)
    w.WriteHeader(resp.StatusCode)
    io.Copy(io.MultiWriter(w, cappedRes), resp.Body)

    // After response written → enqueue (non-blocking)
    uc.enqueue(buildEvent(key, r, resp, time.Since(start), reqBuf, resBuf, cappedReq.truncated, cappedRes.truncated))
}

func (uc *ProxyRequest) enqueue(ev *event.HttpEvent) {
    select {
    case uc.eventCh <- ev:
    default:
        uc.metrics.DroppedEvents.Inc()  // chan full → drop
    }
}
```

**Rule:** `enqueue` MUST be a `select` with `default`. Absolutely no blocking. If the channel is full → drop and increment the counter.

## 3. Bounded body capture

```go
type cappedWriter struct {
    buf       *bytes.Buffer
    max       int
    truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
    if c.buf.Len() >= c.max {
        c.truncated = true
        return len(p), nil  // pretend we wrote, discard
    }
    remaining := c.max - c.buf.Len()
    if len(p) <= remaining {
        return c.buf.Write(p)
    }
    n, _ := c.buf.Write(p[:remaining])
    c.truncated = true
    return n + len(p) - remaining, nil
}
```

- Default 64KB. Override per-API-key via the config DB (loaded into the cache alongside the API key).
- DO NOT decompress the body in the proxy (gzip/br) — pass it through; the consumer or query layer can decompress if needed.

## 4. sync.Pool for buffers

```go
var bufPool = &sync.Pool{
    New: func() any { return bytes.NewBuffer(make([]byte, 0, 8192)) },
}
```

- Get → Reset before Put (already done in the defer in §2).
- DO NOT hold a buffer reference across a channel (race with Pool reuse). Copy bytes into a fresh slice when building the event:

```go
func buildEvent(...) *event.HttpEvent {
    return &event.HttpEvent{
        ReqBody: append([]byte(nil), reqBuf.Bytes()...),  // copy, since buf returns to pool
        ResBody: append([]byte(nil), resBuf.Bytes()...),
        // ...
    }
}
```

## 5. Worker pool → Kafka

```go
// adapter/kafka/producer.go
func (p *Producer) Run(ctx context.Context, eventCh <-chan *event.HttpEvent) {
    for i := 0; i < p.workers; i++ {  // 8-16 workers
        go p.worker(ctx, eventCh)
    }
}

func (p *Producer) worker(ctx context.Context, ch <-chan *event.HttpEvent) {
    for {
        select {
        case <-ctx.Done():
            return
        case ev := <-ch:
            data, _ := proto.Marshal(ev)
            p.client.Produce(ctx, &kgo.Record{
                Topic: "http_events",
                Key:   keyFromAPIKey(ev.ApiKey),
                Value: data,
            }, nil)  // async callback nil → fire-and-forget
        }
    }
}
```

`franz-go` tuning:
```go
client, _ := kgo.NewClient(
    kgo.SeedBrokers(brokers...),
    kgo.ProducerLinger(5*time.Millisecond),
    kgo.ProducerBatchCompression(kgo.ZstdCompression()),
    kgo.RequiredAcks(kgo.LeaderAck()),
    kgo.MaxBufferedRecords(1_000_000),
    kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
)
```

`acks=leader` is enough for log data (no need for `all`). Zstd compression saves 60-70% bandwidth versus raw.

## 6. Upstream Transport

```go
transport := &http.Transport{
    MaxIdleConns:          1000,
    MaxIdleConnsPerHost:   100,
    IdleConnTimeout:       90 * time.Second,
    DisableCompression:    false,        // proxy passthrough
    ForceAttemptHTTP2:     true,
    DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
    TLSHandshakeTimeout:   3 * time.Second,
    ResponseHeaderTimeout: 10 * time.Second,
    ExpectContinueTimeout: 1 * time.Second,
}
```

Avoid the default `httputil.ReverseProxy` if you need granular tweaks — a hand-written handler is leaner (skips middleware you don't need).

## 7. SSRF & target validation

`X-Echo-Target` parsing:

```go
func parseTarget(raw string) (*url.URL, error) {
    if !strings.Contains(raw, "://") {
        raw = "https://" + raw  // default https
    }
    u, err := url.Parse(raw)
    if err != nil { return nil, err }
    if u.Scheme != "http" && u.Scheme != "https" {
        return nil, errors.New("invalid scheme")
    }
    if isPrivateOrLoopback(u.Hostname()) {
        return nil, errors.New("private address blocked")
    }
    return u, nil
}
```

Resolve DNS early to detect private IPs (`isPrivateOrLoopback` after `net.LookupIP`). Cache the DNS result (~30s TTL) so we don't lookup on every request.

## 8. Pre-warming on startup

```go
func main() {
    // ... wire deps

    // Pre-warm Kafka connection
    if err := producer.Ping(ctx); err != nil { log.Fatal(err) }

    // Pre-warm Postgres pool
    db.Ping(ctx)

    // Hot-reload API keys into cache before accepting traffic
    if err := apiKeyLoader.Initial(ctx); err != nil { log.Fatal(err) }

    // Now accept traffic
    srv.ListenAndServe()
}
```

Liveness/readiness:
- `/healthz` (liveness): always 200 if the process is alive.
- `/readyz` (readiness): 503 until pre-warm is done.

## 9. Profiling & metrics

```go
import _ "net/http/pprof"

go http.ListenAndServe(":6060", nil)  // admin port, NEVER expose publicly
```

Required Prometheus metrics:

```go
var (
    requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "proxy_request_duration_seconds",
        Buckets: []float64{0.001, 0.002, 0.005, 0.010, 0.015, 0.020, 0.050, 0.100, 0.500, 1.0},
    }, []string{"method", "status_class"})
    droppedEvents       = promauto.NewCounter(prometheus.CounterOpts{Name: "proxy_dropped_events_total"})
    bodyTruncated       = promauto.NewCounterVec(prometheus.CounterOpts{Name: "proxy_body_truncated_total"}, []string{"side"})
    kafkaProduceErrors  = promauto.NewCounter(prometheus.CounterOpts{Name: "proxy_kafka_produce_errors_total"})
    apiKeyCacheHit      = promauto.NewCounter(prometheus.CounterOpts{Name: "proxy_apikey_cache_hit_total"})
    apiKeyCacheMiss     = promauto.NewCounter(prometheus.CounterOpts{Name: "proxy_apikey_cache_miss_total"})
)
```

The 0.020s bucket exists because that's the budget. Use it for alerts and debugging.

## 10. Benchmark gate

`make bench-proxy` runs k6 or vegeta. Pass criteria: **p99 < 20ms at 5000 RPS**, mock upstream at 1ms.

```js
// bench/k6.js
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    main: { executor: 'constant-arrival-rate', rate: 5000, timeUnit: '1s', duration: '60s', preAllocatedVUs: 200 },
  },
  thresholds: {
    'http_req_duration{expected_response:true}': ['p(99)<20', 'p(95)<10'],
    'http_req_failed': ['rate<0.001'],
  },
};

export default function () {
  const res = http.post('http://localhost:8080/', JSON.stringify({foo: 'bar'}), {
    headers: {
      'X-Echo-Key': __ENV.SID_KEY,
      'X-Echo-Target': 'http://upstream-mock:9000/echo',
      'Content-Type': 'application/json',
    },
  });
  check(res, { 'status 200': r => r.status === 200 });
}
```

Mock upstream: one endpoint that returns after ~1ms.

CI gate: PRs that touch `proxy-gateway/internal/{usecase,adapter}/...` must pass `make bench-proxy` on CI hardware. If it passes locally but fails CI → trace resources or network setup.

## 11. When the profile shows it's slow — playbook

1. Capture a profile: `curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.pprof`.
2. `go tool pprof -http=:8090 cpu.pprof` → flamegraph.
3. Common culprits:
   - GC pressure → check `runtime.MemStats`, increase pool size, or avoid allocation in the hot path.
   - Mutex contention → `mutex` profile, switch to a sharded lock or atomic.
   - Channel block → increase buffer size or add workers.
   - DNS lookup → enable a resolver cache.
   - TLS handshake → enable connection reuse, check `MaxIdleConnsPerHost`.

## 12. Adding a new captured field — checklist

- [ ] Can the new field be captured WITHOUT extra I/O?
- [ ] If extra bytes need reading → still within the cap?
- [ ] Proto edited following the schema rules (see `echoproxy-event-schema` skill)?
- [ ] Re-benched p99 after the addition? Must stay < 20ms.
- [ ] New metric (if any) added?

## 13. Anti-patterns

- ❌ Pushing events into Kafka synchronously inside the handler.
- ❌ Marshaling proto in the handler instead of the worker.
- ❌ Reading the full body into memory without a cap.
- ❌ Decompressing/transforming bodies inside the proxy.
- ❌ goroutine-per-request for logging (alloc + GC).
- ❌ Blocking channel send `eventCh <- ev` without `default`.
- ❌ Global lock for the cache. Use ristretto or a shard.
- ❌ Creating a fresh `http.Client` per request. One global instance, shared Transport.
- ❌ Ignoring context cancellation. Forward `r.Context()` into the outbound request.
- ❌ Logging the full body via zerolog/zap. That's the Kafka pipeline's job.

## 14. File pointers

- Hot-path entry: `proxy-gateway/internal/adapter/http/proxy_handler.go`
- Use case: `proxy-gateway/internal/usecase/proxy_request.go`
- Kafka producer: `proxy-gateway/internal/adapter/kafka/producer.go`
- API key cache: `proxy-gateway/internal/adapter/cache/apikey_cache.go`
- Benchmark: `proxy-gateway/bench/k6.js`
- Mock upstream: `proxy-gateway/bench/mock-upstream/`
- Metrics: `proxy-gateway/internal/infra/metrics/metrics.go`
