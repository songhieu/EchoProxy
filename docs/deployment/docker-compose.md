# Deploying with docker-compose (single VM)

`docker-compose.yml` at the repo root is production-runnable as-is for
~100M req/day on a single beefy VM (8 vCPU, 16 GB RAM, NVMe SSD). Above
that, switch to [Kubernetes](kubernetes.md).

## Why this fits

- One file, one VM, one `make up`. Easy mental model.
- Migrations auto-apply via `docker-entrypoint-initdb.d` on first boot.
- Volumes persist across `make up/down`. Survive restarts.
- The cleanup container loops forever (`INTERVAL=24h`) — no external cron needed.

## Limitations

- **No HA**. VM dies → service down. Acceptable for staging or
  small-team internal tools.
- **Single broker / single CH node**. Survive single-disk failure only
  if you snapshot the volume. No replication.
- **Vertical scaling only**. Above 100M req/day on a 16-core VM you'll
  notice CPU contention between Go services and Kafka.

## 1. Provision

Anything from a single Hetzner/DigitalOcean VM to a bare-metal box
works. Recommended baseline:

| Spec | Reason |
|------|--------|
| 8 vCPU | Kafka + CH + 6 Go services all want CPU |
| 16 GB RAM | CH eats ~4 GB at idle, Kafka ~2 GB, rest ~2 GB |
| 200 GB NVMe SSD | 30 days of bodies at 1M req/day ≈ 30 GB compressed; pad for growth |
| Ubuntu 22.04 LTS or similar | docker + docker-compose available |

```bash
# On the VM (Ubuntu)
sudo apt update && sudo apt install -y docker.io docker-compose-plugin git make
sudo usermod -aG docker $USER
# log out + log back in
```

## 2. Clone + boot

```bash
git clone https://github.com/songhieu/EchoProxy.git
cd echoproxy

# Override defaults via .env (docker-compose auto-reads it)
cat > .env <<EOF
JWT_SECRET=$(openssl rand -hex 32)
NEXTAUTH_SECRET=$(openssl rand -hex 32)
EOF

make up
```

`make up` builds every image (first time: 2-5 min) and starts the stack.
Migrations auto-apply via the postgres + clickhouse init-db hooks.

## 3. TLS + public hostname

`docker-compose.yml` doesn't ship a TLS reverse-proxy by default — add
Caddy as a sibling service:

```yaml
# Append to docker-compose.yml under services:
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    depends_on: [dashboard, proxy-gateway]
# And append to volumes:
volumes:
  caddy-data:
  caddy-config:
```

And a `Caddyfile`:

```Caddyfile
echoproxy.example.com {
  reverse_proxy dashboard:3000
}

proxy.echoproxy.example.com {
  reverse_proxy proxy-gateway:8080
}

ingest.echoproxy.example.com {
  reverse_proxy ingest-api:8081
}
```

Caddy fetches Let's Encrypt certs automatically. Make sure DNS points
at the VM first.

Update the dashboard env so NextAuth knows its public URL:

```yaml
# In docker-compose.yml under dashboard.environment:
NEXTAUTH_URL: https://echoproxy.example.com
```

## 4. Backups

The only stateful volumes that matter:

| Volume | What | Backup priority |
|--------|------|-----------------|
| `clickhouse-data` | all captured events + aggregates | **critical** |
| `postgres-data` | users / projects / api_keys | **critical** |
| `kafka-data` | buffer only — events already persisted in CH | optional |
| `redis-data` | rate-limit counters + stats cache | optional |
| `grafana-data` | dashboard configs you've created | nice-to-have |

Simple cron on the VM:

```bash
# /etc/cron.daily/echoproxy-backup
#!/bin/bash
set -e
TS=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR=/backup/echoproxy
mkdir -p $BACKUP_DIR

# Postgres
docker exec echoproxy-postgres-1 \
  pg_dump -U echoproxy echoproxy | gzip > $BACKUP_DIR/pg_$TS.sql.gz

# ClickHouse (use BACKUP statement for online, consistent snapshot)
docker exec echoproxy-clickhouse-1 \
  clickhouse-client -d echoproxy --query "BACKUP DATABASE echoproxy TO Disk('backups', '$TS.zip')"
docker cp echoproxy-clickhouse-1:/var/lib/clickhouse/disks/backups/$TS.zip $BACKUP_DIR/

# Rotate (keep 14 days)
find $BACKUP_DIR -mtime +14 -delete
```

Push the `/backup/echoproxy/` directory off-VM (rsync, restic, B2, S3).
**An on-host-only backup isn't a backup.**

## 5. Updates

```bash
git pull
make build         # rebuild every image
docker-compose up -d   # rolling restart, in-flight requests drain

# If new migrations were added:
make migrate
```

Downtime: a few seconds per service while containers swap. The proxy is
behind nothing here, so users do see request errors during the swap —
acceptable for the single-VM model. For zero-downtime updates, use the
Kubernetes deployment.

## 6. Resource monitoring

`make up` includes Prometheus + Grafana. Dashboards live at
http://VM_IP:3001 (anonymous admin in dev — gate it behind Caddy +
auth in prod). Prometheus auto-scrapes every Go service's `/metrics`.

Key panels you'll want:

- `rate(proxy_request_duration_seconds_count[1m])` — RPS
- `histogram_quantile(0.99, ...)` — p99 latency
- `proxy_dropped_events_total` — should be flat (=0)
- `clickhouse_part_count` — CH merge backlog
- Host CPU + memory

## 7. Going from single-VM to k8s later

The migration path:

1. Dump Postgres + ClickHouse (see backups above).
2. Stand up the k8s cluster + managed infra per
   [kubernetes.md](kubernetes.md).
3. Restore both dumps into the new instances.
4. `helm install echoproxy ./deploy/helm/echoproxy ...`.
5. Cut over DNS from the VM to the k8s ingress.
6. Decommission the VM after a few days of overlap.

API keys, users, projects, retention settings, and the entire event
history come across intact. No re-onboarding for your apps — the proxy
URL changes, that's it.
