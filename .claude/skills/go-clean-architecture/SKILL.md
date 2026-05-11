---
name: go-clean-architecture
description: Layout, dependency rule, DI, error handling and testing pattern for every Go service in echoproxy. Apply when creating a new Go service (proxy-gateway, ingest-api, log-consumer, auth-api, stats-api, or any new analyzer/processor); adding a new use case; adding a new adapter (Postgres → MySQL, Kafka → NATS); or reviewing PRs that touch layer boundaries.
---

# Go Clean Architecture for echoproxy

Goal: every Go service in the repo follows the **same layout** and **strict dependency rule**, so the team can add new SDKs/services without breaking the architecture.

## 1. Dependency rule (ABSOLUTE)

```
domain  ← usecase  ← adapter  ← infra/cmd
```

- `domain` does not import any project package.
- `usecase` imports only `domain`.
- `adapter` imports `domain` + `usecase` (to implement interfaces and call use cases).
- `infra` and `cmd` import everything to wire it up.

**Never** import in the reverse direction. If a use case needs data from an adapter, define the interface in `domain`, have the adapter implement it, and inject from `cmd`.

## 2. Standard layout

```
<service>/
├── cmd/<name>/main.go              ← wire DI, start server
├── internal/
│   ├── domain/                     ← entities + repo interfaces (zero deps)
│   │   ├── apikey.go
│   │   ├── event.go
│   │   └── errors.go
│   ├── usecase/                    ← business rules
│   │   ├── validate_apikey.go
│   │   └── proxy_request.go
│   ├── adapter/
│   │   ├── http/                   ← delivery: handlers, middleware, router
│   │   ├── grpc/                   ← delivery: gRPC handlers (if any)
│   │   ├── kafka/                  ← repo: Kafka producer/consumer
│   │   ├── clickhouse/             ← repo: ClickHouse query/insert
│   │   ├── postgres/               ← repo: Postgres
│   │   └── cache/                  ← repo: ristretto/redis
│   └── infra/                      ← config, logger, metrics, tracing
├── api/                            ← OpenAPI / proto specs for the service
└── go.mod
```

`pkg/` at the monorepo root holds shared code (e.g. `pkg/event` for the protobuf schema). Don't create `pkg/` per-service unless code is genuinely exported beyond the module.

## 3. Domain layer

- Contains only: entity structs, value objects, repository interfaces, sentinel errors.
- Does NOT contain: complex business logic (that's the use case), context-dependent validation (that's the use case).
- Methods on entities are fine when the logic belongs to the entity itself (e.g. `event.IsTruncated()`).

```go
// internal/domain/apikey.go
package domain

import "errors"

var ErrAPIKeyNotFound = errors.New("api key not found")
var ErrAPIKeyRevoked  = errors.New("api key revoked")

type APIKey struct {
    ID        uint64
    ProjectID uint64
    Hash      string
    Prefix    string
    Allowlist []string
    Status    APIKeyStatus
}

type APIKeyStatus int

const (
    APIKeyActive APIKeyStatus = iota
    APIKeyRevoked
)

func (k *APIKey) AllowsHost(host string) bool {
    for _, h := range k.Allowlist {
        if h == host {
            return true
        }
    }
    return false
}

// Repository interface — adapter implements
type APIKeyRepository interface {
    GetByHash(ctx context.Context, hash string) (*APIKey, error)
    List(ctx context.Context, projectID uint64) ([]*APIKey, error)
}
```

## 4. Usecase layer

- Each use case = 1 file = 1 struct with one `Execute` method (or a clearly named verb).
- Constructor accepts **interfaces** from `domain`, not concrete types.
- Return errors using `domain` sentinels; wrap with context using `fmt.Errorf("...: %w", err)`.

```go
// internal/usecase/validate_apikey.go
package usecase

import (
    "context"
    "fmt"
    "echoproxy/proxy-gateway/internal/domain"
)

type ValidateAPIKey struct {
    repo  domain.APIKeyRepository
    cache domain.APIKeyCache
}

func NewValidateAPIKey(repo domain.APIKeyRepository, cache domain.APIKeyCache) *ValidateAPIKey {
    return &ValidateAPIKey{repo: repo, cache: cache}
}

func (uc *ValidateAPIKey) Execute(ctx context.Context, rawKey, targetHost string) (*domain.APIKey, error) {
    hash := hashKey(rawKey)
    key, err := uc.cache.Get(ctx, hash)
    if err != nil {
        key, err = uc.repo.GetByHash(ctx, hash)
        if err != nil {
            return nil, fmt.Errorf("validate apikey: %w", err)
        }
        uc.cache.Set(ctx, hash, key)
    }
    if key.Status == domain.APIKeyRevoked {
        return nil, domain.ErrAPIKeyRevoked
    }
    if !key.AllowsHost(targetHost) {
        return nil, domain.ErrTargetNotAllowed
    }
    return key, nil
}
```

## 5. Adapter layer

### HTTP handler

```go
// internal/adapter/http/proxy_handler.go
type ProxyHandler struct {
    validateUC *usecase.ValidateAPIKey
    proxyUC    *usecase.ProxyRequest
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    target := r.Header.Get("X-Echo-Target")
    rawKey := r.Header.Get("X-Echo-Key")
    targetHost := parseHost(target)

    key, err := h.validateUC.Execute(r.Context(), rawKey, targetHost)
    if err != nil {
        writeError(w, err)
        return
    }
    h.proxyUC.Execute(w, r, key, target)
}
```

### Repository impl

Each adapter is one struct that implements an interface from `domain`. Keep lifetime in the constructor (connection pool, prepared statements...).

```go
// internal/adapter/postgres/apikey_repo.go
type APIKeyRepo struct{ db *pgxpool.Pool }

func NewAPIKeyRepo(db *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{db} }

func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
    var k domain.APIKey
    err := r.db.QueryRow(ctx, `SELECT id, project_id, hash, prefix, allowlist, status FROM api_keys WHERE hash=$1`, hash).
        Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &k.Allowlist, &k.Status)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, domain.ErrAPIKeyNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("postgres apikey: %w", err)
    }
    return &k, nil
}
```

## 6. Wiring in cmd/main.go

Plain constructor injection. No `wire`/`fx` for MVP.

```go
func main() {
    cfg := infra.LoadConfig()
    logger := infra.NewLogger(cfg)
    db := infra.MustConnectPostgres(cfg.PostgresDSN)
    cache := cache.NewRistretto()
    producer := kafka.NewProducer(cfg.KafkaBrokers)

    apiKeyRepo := postgres.NewAPIKeyRepo(db)
    validateUC := usecase.NewValidateAPIKey(apiKeyRepo, cache)
    proxyUC := usecase.NewProxyRequest(producer, cfg.BodyCap)

    handler := httpadapter.NewProxyHandler(validateUC, proxyUC)
    srv := infra.NewServer(cfg, handler)
    srv.Run(context.Background())
}
```

## 7. Error handling

- **Sentinel errors** in `domain/errors.go`. Adapters return sentinels when mapping infra errors.
- **Wrap** with `fmt.Errorf("layer: action: %w", err)` to preserve the chain.
- HTTP/gRPC delivery layer uses `errors.Is` / `errors.As` to map to status codes.

```go
func writeError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, domain.ErrAPIKeyNotFound), errors.Is(err, domain.ErrAPIKeyRevoked):
        http.Error(w, "unauthorized", http.StatusUnauthorized)
    case errors.Is(err, domain.ErrTargetNotAllowed):
        http.Error(w, "target not allowed", http.StatusForbidden)
    default:
        http.Error(w, "internal", http.StatusInternalServerError)
    }
}
```

## 8. Testing strategy

- **`domain` + `usecase`**: unit tests with fake repos (in-mem map). No DB/Kafka.
- **`adapter`**: integration tests with `testcontainers-go` (postgres, kafka, clickhouse). Place in `_integration_test.go` files with build tag `//go:build integration`.
- **E2E**: in `test/e2e/` using docker-compose, call over real HTTP/gRPC, assert final data in ClickHouse.

```go
// usecase test with fake repo
func TestValidateAPIKey_AllowsHost(t *testing.T) {
    repo := &fakeRepo{keys: map[string]*domain.APIKey{
        "hash1": {ID: 1, Allowlist: []string{"api.example.com"}, Status: domain.APIKeyActive},
    }}
    uc := usecase.NewValidateAPIKey(repo, &noopCache{})
    key, err := uc.Execute(context.Background(), "rawkey", "api.example.com")
    require.NoError(t, err)
    require.Equal(t, uint64(1), key.ID)
}
```

## 9. Anti-patterns (DO NOT do)

- ❌ Importing `adapter` from `usecase` or `domain`. Breaks the dependency rule.
- ❌ Putting SQL queries inside `usecase`. SQL belongs in adapters.
- ❌ HTTP request/response structs in `domain`. That's a delivery concern.
- ❌ Modifying a `domain` entity to add a field for one specific query. Create a DTO at the adapter/use-case boundary instead.
- ❌ `interface{}` or `any` at boundaries. Define concrete types.
- ❌ A "manager" / "helper" / "util" struct holding N unrelated methods. One use case = one struct.
- ❌ Initializing the Postgres connection pool inside a handler. Dependencies must be injected from `main`.
- ❌ Creating packages named `services/`, `models/`, `controllers/` (that's MVC, not Clean Architecture).

## 10. When extending (adding a new SDK / service)

1. Copy the layout from an existing service (e.g. `proxy-gateway/`) into `<new-service>/`.
2. Define new entities + interfaces in `domain/`.
3. Implement use cases; write tests with fake repos first.
4. Implement adapters (HTTP, repos) last.
5. Wire it up in `cmd/<new-service>/main.go`.
6. Add the module to `go.work` at the repo root.
7. Add the service to `docker-compose.yml` with a healthcheck.

Remember: **do not** invent a new layout. If the current layout doesn't fit, discuss it, update this skill, and refactor the existing services in lockstep.
