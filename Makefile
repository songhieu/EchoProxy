SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO_MODULES := pkg/event pkg/redact pkg/ratelimit proxy-gateway ingest-api log-consumer auth-api stats-api sdk-reference-go cleanup

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Proto ────────────────────────────────────────────────────────────────────
.PHONY: proto-gen
proto-gen: ## Regenerate Go bindings from api/event.proto
	protoc \
		--go_out=. --go_opt=module=github.com/songhieu/EchoProxy/pkg/event --go_opt=Mapi/event.proto=. \
		--go-grpc_out=. --go-grpc_opt=module=github.com/songhieu/EchoProxy/pkg/event --go-grpc_opt=Mapi/event.proto=. \
		-I api api/event.proto
	mv event.pb.go pkg/event/event.pb.go 2>/dev/null || true
	mv event_grpc.pb.go pkg/event/event_grpc.pb.go 2>/dev/null || true

# ─── Build / Test ─────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build all Go services
	@for m in $(GO_MODULES); do echo "== $$m =="; (cd $$m && go build ./...) || exit 1; done

.PHONY: tidy
tidy: ## go mod tidy in every module
	@for m in $(GO_MODULES); do echo "== $$m =="; (cd $$m && go mod tidy) || exit 1; done

.PHONY: test
test: ## Run unit tests in every module
	@for m in $(GO_MODULES); do echo "== $$m =="; (cd $$m && go test ./...) || exit 1; done

.PHONY: vet
vet: ## go vet every module
	@for m in $(GO_MODULES); do echo "== $$m =="; (cd $$m && go vet ./...) || exit 1; done

# ─── Docker / Infra ───────────────────────────────────────────────────────────
.PHONY: up
up: ## Start full local stack (kafka, clickhouse, postgres, redis, services, dashboard)
	docker compose up -d --build

.PHONY: up-infra
up-infra: ## Start only infra (kafka, clickhouse, postgres, redis)
	docker compose up -d kafka clickhouse postgres redis

.PHONY: down
down: ## Stop and remove containers
	docker compose down

.PHONY: down-volumes
down-volumes: ## Stop containers and remove volumes (DESTRUCTIVE)
	docker compose down -v

.PHONY: logs
logs: ## Tail logs from all services
	docker compose logs -f --tail=100

# ─── Migrations ───────────────────────────────────────────────────────────────
# Note: on `make up` from a clean state, postgres + clickhouse auto-apply
# everything under migrations/ via docker-entrypoint-initdb.d. These targets
# are for adding NEW migrations after the stack is already running (the
# init dir only runs on first container start).
#
# Uses `docker exec` against the container name so it works with both the
# `docker compose` plugin and the legacy `docker-compose` binary.

PG_CT := $(shell docker ps --filter 'label=com.docker.compose.service=postgres' --filter 'status=running' --format '{{.Names}}' | head -n1)
CH_CT := $(shell docker ps --filter 'label=com.docker.compose.service=clickhouse' --filter 'status=running' --format '{{.Names}}' | head -n1)

.PHONY: migrate-postgres
migrate-postgres: ## Apply all Postgres migrations
	@test -n "$(PG_CT)" || (echo "no running postgres container; run \`make up\` first" && exit 1)
	@for f in migrations/postgres/*.sql; do echo "applying $$f"; docker exec -i $(PG_CT) psql -U echoproxy -d echoproxy -v ON_ERROR_STOP=1 < $$f; done

.PHONY: migrate-clickhouse
migrate-clickhouse: ## Apply all ClickHouse migrations
	@test -n "$(CH_CT)" || (echo "no running clickhouse container; run \`make up\` first" && exit 1)
	@for f in migrations/clickhouse/*.sql; do echo "applying $$f"; docker exec -i $(CH_CT) clickhouse-client --multiquery < $$f; done

.PHONY: migrate
migrate: migrate-postgres migrate-clickhouse ## Run all migrations

# ─── Bench ────────────────────────────────────────────────────────────────────
.PHONY: bench-proxy
bench-proxy: ## Run k6 benchmark on proxy-gateway (target: p99 < 20ms @ 5000 RPS)
	docker run --rm -i --network host \
		-e ECHO_KEY=$${ECHO_KEY:-sk_test_demo} \
		grafana/k6 run - < proxy-gateway/bench/k6.js

.PHONY: bench-stress
bench-stress: ## Ramp proxy-gateway RPS until p99 breaks 20ms SLO (find capacity)
	bash proxy-gateway/bench/run-stress.sh

# ─── Dashboard ────────────────────────────────────────────────────────────────
.PHONY: dashboard-dev
dashboard-dev: ## Run Next.js dashboard in dev mode
	cd dashboard && pnpm dev

.PHONY: dashboard-build
dashboard-build: ## Build Next.js dashboard
	cd dashboard && pnpm build

# ─── Convenience ──────────────────────────────────────────────────────────────
.PHONY: seed
seed: ## Seed a demo project + API key (requires up + migrate)
	bash scripts/seed.sh

# ─── Cleanup ──────────────────────────────────────────────────────────────────
.PHONY: cleanup
cleanup: ## Run retention sweep against Postgres (see docs/retention.md for cron setup)
	POSTGRES_DSN=$${POSTGRES_DSN:-postgres://echoproxy:echoproxy@localhost:5433/echoproxy?sslmode=disable} \
	AUDIT_RETENTION_DAYS=$${AUDIT_RETENTION_DAYS:-90} \
	cd cleanup && go run ./cmd/cleanup

# ─── Images (parity with the GitHub Actions release workflow) ─────────────────
REGISTRY ?= ghcr.io/songhieu
TAG      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
PLATFORMS ?= linux/amd64,linux/arm64
GO_SERVICES := proxy-gateway:proxy ingest-api:ingest log-consumer:consumer auth-api:auth stats-api:stats cleanup:cleanup

.PHONY: build-images
build-images: ## Build every image locally (no push). Set TAG=v1.2.3 to stamp a release.
	@for s in $(GO_SERVICES); do \
	  svc=$${s%:*}; bin=$${s#*:}; \
	  echo "building $$svc -> $(REGISTRY)/echoproxy-$$svc:$(TAG)"; \
	  docker build -f deploy/Dockerfile.go --build-arg SERVICE=$$svc --build-arg BIN=$$bin \
	    -t $(REGISTRY)/echoproxy-$$svc:$(TAG) -t $(REGISTRY)/echoproxy-$$svc:latest . || exit 1; \
	done
	docker build -t $(REGISTRY)/echoproxy-dashboard:$(TAG) -t $(REGISTRY)/echoproxy-dashboard:latest ./dashboard

.PHONY: push-images
push-images: ## Push the images you just built (requires `docker login ghcr.io`)
	@for s in $(GO_SERVICES) dashboard:dashboard; do \
	  svc=$${s%:*}; \
	  echo "pushing $(REGISTRY)/echoproxy-$$svc:$(TAG)"; \
	  docker push $(REGISTRY)/echoproxy-$$svc:$(TAG) || exit 1; \
	  docker push $(REGISTRY)/echoproxy-$$svc:latest || exit 1; \
	done

.PHONY: buildx-push
buildx-push: ## Build multi-arch + push in one shot via buildx (matches the GHA release job)
	@for s in $(GO_SERVICES); do \
	  svc=$${s%:*}; bin=$${s#*:}; \
	  echo "buildx $(PLATFORMS) -> $(REGISTRY)/echoproxy-$$svc:$(TAG)"; \
	  docker buildx build --platform $(PLATFORMS) -f deploy/Dockerfile.go \
	    --build-arg SERVICE=$$svc --build-arg BIN=$$bin \
	    -t $(REGISTRY)/echoproxy-$$svc:$(TAG) -t $(REGISTRY)/echoproxy-$$svc:latest \
	    --push . || exit 1; \
	done
	docker buildx build --platform $(PLATFORMS) \
	  -t $(REGISTRY)/echoproxy-dashboard:$(TAG) -t $(REGISTRY)/echoproxy-dashboard:latest \
	  --push ./dashboard

# ─── Helm (parity with the GHA release helm job) ──────────────────────────────
.PHONY: helm-lint
helm-lint: ## Lint the chart
	helm lint deploy/helm/echoproxy

.PHONY: helm-package
helm-package: ## Package the chart into /tmp/chart/
	@mkdir -p /tmp/chart
	helm package deploy/helm/echoproxy --destination /tmp/chart

.PHONY: helm-push
helm-push: helm-package ## Push the chart to ghcr.io OCI registry (requires helm registry login)
	@v=$$(grep '^version:' deploy/helm/echoproxy/Chart.yaml | awk '{print $$2}'); \
	helm push /tmp/chart/echoproxy-$$v.tgz oci://$(REGISTRY)/charts
