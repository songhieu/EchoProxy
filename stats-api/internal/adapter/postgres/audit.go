package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogger persists body access events to Postgres so admins can later
// review who looked at which event body. It is intentionally fire-and-forget
// from the caller's perspective: a logging failure must not break the user.
type AuditLogger struct{ pool *pgxpool.Pool }

func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger { return &AuditLogger{pool: pool} }

func MustConnect(ctx context.Context, dsn string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		panic(fmt.Errorf("postgres parse dsn: %w", err))
	}
	cfg.MaxConns = 5
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("postgres connect: %w", err))
	}
	return pool
}

// LogAccess records a single body-access event. Errors are returned but the
// caller is expected to log and continue.
func (a *AuditLogger) LogAccess(ctx context.Context, userID, projectID uint64, eventID, ip, userAgent string) error {
	const q = `INSERT INTO body_access_log(user_id, project_id, event_id, ip, user_agent)
	           VALUES($1, $2, $3, NULLIF($4,'')::inet, NULLIF($5,''))`
	_, err := a.pool.Exec(ctx, q, userID, projectID, eventID, ip, userAgent)
	return err
}
