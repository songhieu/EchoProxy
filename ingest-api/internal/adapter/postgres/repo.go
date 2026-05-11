package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/songhieu/EchoProxy/ingest-api/internal/domain"
	"github.com/songhieu/EchoProxy/pkg/redact"
)

func MustConnect(ctx context.Context, dsn string) *pgxpool.Pool {
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("postgres connect: %w", err))
	}
	return pool
}

type APIKeyRepo struct{ pool *pgxpool.Pool }

func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{pool} }

func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	const q = `SELECT id, project_id, hash, status, COALESCE(rate_limit_rps,0), redact_rules
	           FROM api_keys WHERE hash=$1`
	var (
		k     domain.APIKey
		rules []byte
	)
	if err := r.pool.QueryRow(ctx, q, hash).Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Status, &k.RateLimitRPS, &rules); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("apikey lookup: %w", err)
	}
	rr, _ := redact.FromJSON(rules)
	k.Redactor = redact.New(rr)
	return &k, nil
}
