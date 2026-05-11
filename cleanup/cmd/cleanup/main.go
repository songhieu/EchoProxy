// cleanup runs the periodic retention sweeps that don't have a built-in
// TTL mechanism. The hard cap (max 90d) is enforced by ClickHouse's
// table-level TTL (CH migrations 001 + 005). This binary handles the two
// things CH TTL cannot:
//
//   1. Per-project retention shorter than the cap — for each project where
//      projects.retention_days < 90, issue an async ALTER TABLE DELETE
//      mutation against http_events to drop the older rows.
//
//   2. Postgres body_access_log prune — audit table has no native TTL.
//
// Two run modes:
//
//   - One-shot (default, INTERVAL unset): runs once, exits. Use this for
//     k8s CronJob, systemd timer, plain crontab — the scheduler fires the
//     binary on each tick.
//
//   - Loop (INTERVAL=24h): runs forever, sleeping INTERVAL between sweeps.
//     Use this for docker-compose, ECS service, or any environment where
//     there is no external cron and you just want a long-running container.
//
// See docs/retention.md for the full picture.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HardCapDays mirrors the ClickHouse table-level TTL on http_events.
// Projects with retention_days < HardCapDays get a per-project mutation;
// projects at the cap rely entirely on CH's automatic TTL sweep.
const HardCapDays = 90

func main() {
	pgDSN := env("POSTGRES_DSN", "postgres://echoproxy:echoproxy@localhost:5432/echoproxy?sslmode=disable")
	chDSN := env("CLICKHOUSE_DSN", "clickhouse://echoproxy:echoproxy@localhost:9000/echoproxy")
	auditDays := envInt("AUDIT_RETENTION_DAYS", 90)

	// INTERVAL switches between one-shot (k8s CronJob / systemd) and loop
	// mode (docker-compose / ECS). Empty = one-shot.
	var interval time.Duration
	if v := os.Getenv("INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "INTERVAL %q invalid: %v\n", v, err)
			os.Exit(2)
		}
		interval = d
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Persistent pools — reused across iterations in loop mode.
	pgCtx, pgCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pgCancel()
	pg, err := pgxpool.New(pgCtx, pgDSN)
	if err != nil {
		log.Error("postgres connect", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	chOpts, err := clickhouse.ParseDSN(chDSN)
	if err != nil {
		log.Error("clickhouse parse dsn", "err", err)
		os.Exit(1)
	}
	ch, err := clickhouse.Open(chOpts)
	if err != nil {
		log.Error("clickhouse connect", "err", err)
		os.Exit(1)
	}
	defer ch.Close()

	if interval == 0 {
		// One-shot.
		if err := runOnce(context.Background(), log, pg, ch, auditDays); err != nil {
			os.Exit(1)
		}
		return
	}

	// Loop mode: SIGTERM/SIGINT-aware, runs immediately then on each tick.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	log.Info("cleanup loop started", "interval", interval.String())
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := runOnce(ctx, log, pg, ch, auditDays); err != nil {
			// Don't exit — next tick is the right retry.
			log.Warn("sweep failed; will retry next interval", "err", err)
		}
		select {
		case <-ctx.Done():
			log.Info("cleanup loop stopping")
			return
		case <-tick.C:
		}
	}
}

func runOnce(parent context.Context, log *slog.Logger, pg *pgxpool.Pool, ch driver.Conn, auditDays int) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()

	deleted, err := pruneBodyAccessLog(ctx, pg, auditDays)
	if err != nil {
		log.Error("prune body_access_log", "err", err)
		return err
	}
	log.Info("prune body_access_log", "deleted", deleted, "older_than_days", auditDays)

	projects, err := loadProjects(ctx, pg)
	if err != nil {
		log.Error("load projects", "err", err)
		return err
	}
	for _, p := range projects {
		if p.RetentionDays >= HardCapDays {
			// CH's table TTL already covers this — skip to avoid a no-op mutation.
			continue
		}
		if err := enforceProjectRetention(ctx, ch, p); err != nil {
			log.Error("enforce project retention", "project_id", p.ID, "err", err)
			// Don't bail — try other projects.
			continue
		}
		log.Info("enforce project retention",
			"project_id", p.ID, "name", p.Name, "retention_days", p.RetentionDays)
	}
	return nil
}

type project struct {
	ID            uint64
	Name          string
	RetentionDays int
}

func loadProjects(ctx context.Context, pg *pgxpool.Pool) ([]project, error) {
	rows, err := pg.Query(ctx, `SELECT id, name, retention_days FROM projects`)
	if err != nil {
		return nil, fmt.Errorf("select projects: %w", err)
	}
	defer rows.Close()
	var out []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.Name, &p.RetentionDays); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// enforceProjectRetention fires an async ClickHouse mutation that deletes
// rows for this project older than its retention_days. The mutation is
// idempotent — running it daily costs at most a metadata write when there's
// nothing new to delete (mutations on empty selections are cheap).
func enforceProjectRetention(ctx context.Context, ch driver.Conn, p project) error {
	q := fmt.Sprintf(
		`ALTER TABLE echoproxy.http_events DELETE WHERE project_id = %d AND ts < now() - INTERVAL %d DAY`,
		p.ID, p.RetentionDays,
	)
	return ch.Exec(ctx, q)
}

// pruneBodyAccessLog deletes audit rows older than `days`. Returns the row
// count deleted. The table is append-only and project-FK-cascaded, so a
// plain DELETE is safe; no soft-delete column to update.
func pruneBodyAccessLog(ctx context.Context, pg *pgxpool.Pool, days int) (int64, error) {
	if days <= 0 {
		return 0, errors.New("days must be > 0")
	}
	cmd, err := pg.Exec(ctx,
		`DELETE FROM body_access_log WHERE accessed_at < now() - make_interval(days => $1)`,
		days,
	)
	if err != nil {
		return 0, fmt.Errorf("delete: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
