# Deploying on Kubernetes (Helm)

The Helm chart at `deploy/helm/echoproxy/` deploys all six Go services +
the Next.js dashboard + the cleanup CronJob. Stateful infra (Kafka,
ClickHouse, Postgres, Redis) is **bring-your-own** — see [infra
choices](#stateful-infra-choices) below.

## Prerequisites

- Kubernetes 1.25+
- Helm 3
- Container registry where you push the EchoProxy images
- Running Kafka, ClickHouse, Postgres, Redis reachable from the cluster

## 1. Build + push images

Each service is a separate image. The repo ships a multi-service
`deploy/Dockerfile.go` parameterised by `SERVICE` and `BIN`. Use your
preferred build pipeline; the Makefile shorthand for local Mac/Linux:

```bash
export REGISTRY=ghcr.io/songhieu
export TAG=$(git rev-parse --short HEAD)

for s in proxy-gateway ingest-api log-consumer auth-api stats-api cleanup; do
  bin=$(case $s in proxy-gateway) echo proxy ;;
                  ingest-api)    echo ingest ;;
                  log-consumer)  echo consumer ;;
                  auth-api)      echo auth ;;
                  stats-api)     echo stats ;;
                  cleanup)       echo cleanup ;; esac)
  docker build \
    -f deploy/Dockerfile.go \
    --build-arg SERVICE=$s --build-arg BIN=$bin \
    -t $REGISTRY/echoproxy-$s:$TAG .
  docker push $REGISTRY/echoproxy-$s:$TAG
done

# Dashboard has its own Dockerfile
docker build -t $REGISTRY/echoproxy-dashboard:$TAG ./dashboard
docker push $REGISTRY/echoproxy-dashboard:$TAG
```

In CI: GitHub Actions can do this via `docker/build-push-action`. There
isn't a workflow file shipped in the repo — add one matching your
registry strategy.

## 2. Stateful infra

Pick **one** option per database. All approaches work as long as the
DSNs you put into `secrets:` are reachable from the cluster.

### Option A — Managed (recommended for prod)

| Service | Recommended managed providers |
|---------|------------------------------|
| Kafka | Confluent Cloud, Aiven, AWS MSK, Upstash Kafka, Redpanda Cloud |
| ClickHouse | ClickHouse Cloud, Aiven, Altinity.Cloud, DoubleCloud |
| Postgres | RDS, Cloud SQL, Neon, Supabase, Aiven |
| Redis | ElastiCache, Memorystore, Upstash, Aiven |

Pros: zero ops, backups handled, upgrades managed.
Cons: costs ~2x DIY, data leaves your VPC.

### Option B — In-cluster operators

Run each as a sibling Helm release. These are the most production-ready
operators per service:

```bash
# Kafka via Strimzi
helm repo add strimzi https://strimzi.io/charts/
helm install kafka strimzi/strimzi-kafka-operator -n kafka --create-namespace

# ClickHouse via Altinity operator
kubectl apply -f https://raw.githubusercontent.com/Altinity/clickhouse-operator/master/deploy/operator/clickhouse-operator-install-bundle.yaml

# Postgres via CrunchyData PGO
kubectl apply -k https://github.com/CrunchyData/postgres-operator-examples/kustomize/install/default

# Redis via Bitnami chart (simpler, no operator needed)
helm install redis oci://registry-1.docker.io/bitnamicharts/redis -n redis --create-namespace
```

For each, create a CR / Helm values matching the capacity recommendations
in [README.md](README.md#capacity-sizing-rule-of-thumb), then read the
DSN out of the resulting Secret and feed it to EchoProxy below.

### Option C — Quick bring-up via Bitnami

For staging or POC where you want everything in-cluster fast:

```bash
helm install pg     oci://registry-1.docker.io/bitnamicharts/postgresql --set auth.username=echoproxy --set auth.password=$(openssl rand -hex 16) --set auth.database=echoproxy -n echoproxy --create-namespace
helm install redis  oci://registry-1.docker.io/bitnamicharts/redis      --set auth.password=$(openssl rand -hex 16) -n echoproxy
helm install kafka  oci://registry-1.docker.io/bitnamicharts/kafka      --set listeners.client.protocol=PLAINTEXT -n echoproxy
# ClickHouse: no first-party Bitnami chart. Use the Altinity operator above
# or run a single-node deployment manually for staging.
```

## 3. Install the chart

```bash
# Create the namespace
kubectl create ns echoproxy

# Generate secrets (don't reuse the chart defaults in prod!)
JWT_SECRET=$(openssl rand -hex 32)
NEXTAUTH_SECRET=$(openssl rand -hex 32)

helm install echoproxy ./deploy/helm/echoproxy \
  --namespace echoproxy \
  --set image.registry=$REGISTRY \
  --set image.tag=$TAG \
  --set secrets.kafkaBrokers="kafka-bootstrap.kafka:9092" \
  --set secrets.clickhouseDSN="clickhouse://user:pass@clickhouse.echoproxy:9000/echoproxy?secure=true" \
  --set secrets.postgresDSN="postgres://echoproxy:$PG_PASS@pg-rw.echoproxy:5432/echoproxy?sslmode=require" \
  --set secrets.redisAddr="redis-master.echoproxy:6379" \
  --set secrets.jwtSecret="$JWT_SECRET" \
  --set dashboard.nextauthSecret="$NEXTAUTH_SECRET" \
  --set dashboard.publicURL="https://echoproxy.example.com" \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts.proxy.host=proxy.echoproxy.example.com \
  --set ingress.hosts.dashboard.host=echoproxy.example.com \
  --set "ingress.annotations.cert-manager\.io/cluster-issuer=letsencrypt-prod"
```

Prefer a values file to long `--set` chains:

```bash
helm install echoproxy ./deploy/helm/echoproxy -n echoproxy -f my-values.yaml
```

## 4. Apply migrations

The chart does NOT bundle a migration Job (intentional — you control how
secrets reach the migrator). One-shot via `kubectl run`:

```bash
# Postgres
kubectl run -n echoproxy pg-migrate --rm -i --restart=Never \
  --image=postgres:16-alpine \
  --env=PGPASSWORD=$PG_PASS \
  -- psql -h pg-rw.echoproxy -U echoproxy -d echoproxy < migrations/postgres/001_init.sql

# Loop for all numbered files:
for f in migrations/postgres/*.sql; do
  kubectl run -n echoproxy pg-migrate-$RANDOM --rm -i --restart=Never \
    --image=postgres:16-alpine --env=PGPASSWORD=$PG_PASS \
    -- psql -h pg-rw.echoproxy -U echoproxy -d echoproxy < $f
done

# ClickHouse
for f in migrations/clickhouse/*.sql; do
  kubectl run -n echoproxy ch-migrate-$RANDOM --rm -i --restart=Never \
    --image=clickhouse/clickhouse-server:24.8-alpine \
    -- clickhouse-client --host clickhouse.echoproxy --user echoproxy --password $CH_PASS --database echoproxy --multiquery < $f
done
```

For a Helm-native flow add a `Job` with `helm.sh/hook: post-install,post-upgrade`
template — left out of the default chart to keep behavior explicit.

## 5. Verify

```bash
kubectl -n echoproxy get pods -l app.kubernetes.io/part-of=echoproxy
# All Running + 1/1 Ready

kubectl -n echoproxy port-forward svc/echoproxy-dashboard 3000:3000
# Open http://localhost:3000 → sign up
```

Then send a smoke request:

```bash
KEY=<key from dashboard>
curl -H "X-Echo-Key: $KEY" -H "X-Echo-Target: https://httpbin.org" \
  https://proxy.echoproxy.example.com/get
```

## 6. Observability

The chart ships a `ServiceMonitor` (disabled by default). If you run
kube-prometheus-stack:

```bash
helm upgrade echoproxy ./deploy/helm/echoproxy -n echoproxy --reuse-values \
  --set serviceMonitor.enabled=true
```

Key metrics + alerts:

- `proxy_request_duration_seconds_bucket` — p99 SLO 20ms
- `proxy_dropped_events_total` — should stay at 0; rising = log-consumer falling behind
- `kafka_consumergroup_lag` (Strimzi exporter) — log-consumer health
- Cleanup CronJob's last completion time — alert if no success in 48h

## 7. Upgrading

```bash
helm upgrade echoproxy ./deploy/helm/echoproxy -n echoproxy \
  --set image.tag=$NEW_TAG --reuse-values
```

Zero-downtime — every Deployment is configured with default
`RollingUpdate` strategy and readiness probes.

## 8. Rotating JWT_SECRET

The auth middleware verifies the user still exists on every request, so
rotating JWT_SECRET cleanly forces all sessions to 401 → dashboard
auto-logs everyone out:

```bash
NEW_JWT=$(openssl rand -hex 32)
helm upgrade echoproxy ./deploy/helm/echoproxy -n echoproxy --reuse-values \
  --set secrets.jwtSecret=$NEW_JWT

kubectl -n echoproxy rollout restart deploy/echoproxy-auth-api deploy/echoproxy-stats-api
```

## Stateful infra choices — TL;DR

| You have / want | Use |
|-----------------|-----|
| Multi-cloud, vendor-neutral | Strimzi (Kafka) + Altinity (CH) + CrunchyData (PG) + Bitnami (Redis) |
| GCP only | Cloud SQL (PG) + Memorystore (Redis) + Confluent on GKE Marketplace + ClickHouse Cloud |
| AWS only | RDS (PG) + ElastiCache (Redis) + MSK (Kafka) + ClickHouse Cloud or Altinity |
| Smallest blast radius | All managed (Aiven across the board) |
| Smallest bill | Bitnami all-in-cluster (lower SLA, you on-call for upgrades) |
