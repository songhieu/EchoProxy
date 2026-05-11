package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/songhieu/EchoProxy/auth-api/internal/domain"
)

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

// ─── User ───────────────────────────────────────────────────────────────────
type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool} }

func (r *UserRepo) Create(ctx context.Context, email, hash string) (*domain.User, error) {
	const q = `INSERT INTO users(email, password_hash) VALUES($1, $2) RETURNING id, created_at`
	u := &domain.User{Email: email, PasswordHash: hash}
	err := r.pool.QueryRow(ctx, q, email, hash).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") {
			return nil, domain.ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `SELECT id, email, password_hash, created_at FROM users WHERE email=$1`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByID(ctx context.Context, id uint64) (*domain.User, error) {
	const q = `SELECT id, email, password_hash, created_at FROM users WHERE id=$1`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

// ─── Project ────────────────────────────────────────────────────────────────
type ProjectRepo struct{ pool *pgxpool.Pool }

func NewProjectRepo(pool *pgxpool.Pool) *ProjectRepo { return &ProjectRepo{pool} }

func (r *ProjectRepo) Create(ctx context.Context, ownerID uint64, name string) (*domain.Project, error) {
	const q = `INSERT INTO projects(owner_id, name) VALUES($1, $2) RETURNING id, retention_days, created_at`
	p := &domain.Project{OwnerID: ownerID, Name: name}
	if err := r.pool.QueryRow(ctx, q, ownerID, name).Scan(&p.ID, &p.RetentionDays, &p.CreatedAt); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProjectRepo) List(ctx context.Context, ownerID uint64) ([]*domain.Project, error) {
	const q = `SELECT id, owner_id, name, retention_days, created_at FROM projects WHERE owner_id=$1 ORDER BY id`
	rows, err := r.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Name, &p.RetentionDays, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) Get(ctx context.Context, id, ownerID uint64) (*domain.Project, error) {
	const q = `SELECT id, owner_id, name, retention_days, created_at FROM projects WHERE id=$1 AND owner_id=$2`
	var p domain.Project
	if err := r.pool.QueryRow(ctx, q, id, ownerID).Scan(&p.ID, &p.OwnerID, &p.Name, &p.RetentionDays, &p.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepo) UpdateRetention(ctx context.Context, id, ownerID uint64, days int) (*domain.Project, error) {
	const q = `UPDATE projects SET retention_days = $3
	           WHERE id = $1 AND owner_id = $2
	           RETURNING id, owner_id, name, retention_days, created_at`
	var p domain.Project
	if err := r.pool.QueryRow(ctx, q, id, ownerID, days).Scan(
		&p.ID, &p.OwnerID, &p.Name, &p.RetentionDays, &p.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Delete removes the project. API keys cascade via the FK in
// migrations/postgres/001_init.sql (api_keys.project_id REFERENCES projects(id)
// ON DELETE CASCADE). Owner check is enforced in WHERE so users can't
// nuke each other's projects via id-guessing.
func (r *ProjectRepo) Delete(ctx context.Context, id, ownerID uint64) error {
	const q = `DELETE FROM projects WHERE id = $1 AND owner_id = $2`
	tag, err := r.pool.Exec(ctx, q, id, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProjectNotFound
	}
	return nil
}

// ─── API Key ────────────────────────────────────────────────────────────────
type APIKeyRepo struct{ pool *pgxpool.Pool }

func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{pool} }

func (r *APIKeyRepo) Create(ctx context.Context, k *domain.APIKey) error {
	const q = `INSERT INTO api_keys(project_id, hash, prefix, allowlist, body_cap, rate_limit_rps, redact_rules, status, description)
	           VALUES($1,$2,$3,$4,$5,$6,COALESCE($7,'{}'::jsonb),$8,$9)
	           RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, k.ProjectID, k.Hash, k.Prefix, k.Allowlist, k.BodyCap, k.RateLimitRPS, k.RedactRules, k.Status, k.Description).
		Scan(&k.ID, &k.CreatedAt)
}

func (r *APIKeyRepo) List(ctx context.Context, projectID uint64) ([]*domain.APIKey, error) {
	const q = `SELECT id, project_id, hash, COALESCE(prefix,''), allowlist, body_cap,
	                  COALESCE(rate_limit_rps,0), redact_rules, status, COALESCE(description,''), created_at
	           FROM api_keys WHERE project_id=$1 ORDER BY id DESC`
	rows, err := r.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &k.Allowlist, &k.BodyCap,
			&k.RateLimitRPS, &k.RedactRules, &k.Status, &k.Description, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &k)
	}
	return out, rows.Err()
}

func (r *APIKeyRepo) Get(ctx context.Context, id, projectID uint64) (*domain.APIKey, error) {
	const q = `SELECT id, project_id, hash, COALESCE(prefix,''), allowlist, body_cap,
	                  COALESCE(rate_limit_rps,0), redact_rules, status, COALESCE(description,''), created_at
	           FROM api_keys WHERE id=$1 AND project_id=$2`
	var k domain.APIKey
	if err := r.pool.QueryRow(ctx, q, id, projectID).Scan(&k.ID, &k.ProjectID, &k.Hash, &k.Prefix, &k.Allowlist, &k.BodyCap,
		&k.RateLimitRPS, &k.RedactRules, &k.Status, &k.Description, &k.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepo) Update(ctx context.Context, k *domain.APIKey) error {
	const q = `UPDATE api_keys
	           SET allowlist=$3, body_cap=$4, rate_limit_rps=$5, redact_rules=COALESCE($6,'{}'::jsonb), description=$7
	           WHERE id=$1 AND project_id=$2`
	tag, err := r.pool.Exec(ctx, q, k.ID, k.ProjectID, k.Allowlist, k.BodyCap, k.RateLimitRPS, k.RedactRules, k.Description)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAPIKeyNotFound
	}
	return nil
}

func (r *APIKeyRepo) Revoke(ctx context.Context, id, projectID uint64) error {
	const q = `UPDATE api_keys SET status='revoked', revoked_at=NOW() WHERE id=$1 AND project_id=$2`
	tag, err := r.pool.Exec(ctx, q, id, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAPIKeyNotFound
	}
	return nil
}
