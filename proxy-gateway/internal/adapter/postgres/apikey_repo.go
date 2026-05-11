package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/songhieu/EchoProxy/pkg/redact"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// APIKeyRepo is the Postgres-backed implementation of domain.APIKeyRepository.
type APIKeyRepo struct{ pool *pgxpool.Pool }

func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{pool: pool} }

func MustConnect(ctx context.Context, dsn string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		panic(fmt.Errorf("postgres parse dsn: %w", err))
	}
	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("postgres connect: %w", err))
	}
	return pool
}

func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	const q = `
		SELECT id, project_id, hash, COALESCE(prefix,''), allowlist, body_cap, rate_limit_rps,
		       redact_rules, status, COALESCE(description,'')
		FROM api_keys WHERE hash = $1
	`
	var (
		k         domain.APIKey
		statusStr string
		rules     []byte
	)
	err := r.pool.QueryRow(ctx, q, hash).Scan(
		&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &k.Allowlist, &k.BodyCap, &k.RateLimitRPS,
		&rules, &statusStr, &k.Description,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres apikey: %w", err)
	}
	if statusStr == "revoked" {
		k.Status = domain.StatusRevoked
	}
	k.Redactor = buildRedactor(rules)
	return &k, nil
}

func (r *APIKeyRepo) List(ctx context.Context) ([]*domain.APIKey, error) {
	const q = `SELECT id, project_id, hash, COALESCE(prefix,''), allowlist, COALESCE(body_cap,0),
	                  COALESCE(rate_limit_rps,0), redact_rules, status, COALESCE(description,'')
	           FROM api_keys`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("postgres list: %w", err)
	}
	defer rows.Close()
	var out []*domain.APIKey
	for rows.Next() {
		var (
			k         domain.APIKey
			statusStr string
			rules     []byte
		)
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &k.Allowlist, &k.BodyCap, &k.RateLimitRPS,
			&rules, &statusStr, &k.Description); err != nil {
			return nil, fmt.Errorf("postgres scan: %w", err)
		}
		if statusStr == "revoked" {
			k.Status = domain.StatusRevoked
		}
		k.Redactor = buildRedactor(rules)
		out = append(out, &k)
	}
	return out, rows.Err()
}

// buildRedactor parses per-key JSON rules and constructs a Redactor that
// merges them with the package defaults. On parse error we fall back to the
// defaults rather than rejecting the key (defense in depth still applies).
func buildRedactor(rules []byte) *redact.Redactor {
	r, _ := redact.FromJSON(rules)
	return redact.New(r)
}
