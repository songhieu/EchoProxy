package domain

import (
	"context"
	"time"
)

type User struct {
	ID           uint64    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Project struct {
	ID            uint64    `json:"id"`
	OwnerID       uint64    `json:"owner_id"`
	Name          string    `json:"name"`
	RetentionDays int       `json:"retention_days"`
	CreatedAt     time.Time `json:"created_at"`
}

type APIKey struct {
	ID           uint64
	ProjectID    uint64
	Hash         string
	Prefix       string
	Allowlist    []string
	BodyCap      int
	RateLimitRPS int
	RedactRules  []byte // raw JSON; auth-api passes through unchanged
	Status       string
	Description  string
	CreatedAt    time.Time
}

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uint64) (*User, error)
}

type ProjectRepository interface {
	Create(ctx context.Context, ownerID uint64, name string) (*Project, error)
	List(ctx context.Context, ownerID uint64) ([]*Project, error)
	Get(ctx context.Context, id, ownerID uint64) (*Project, error)
	UpdateRetention(ctx context.Context, id, ownerID uint64, days int) (*Project, error)
	Delete(ctx context.Context, id, ownerID uint64) error
}

type APIKeyRepository interface {
	Create(ctx context.Context, k *APIKey) error
	List(ctx context.Context, projectID uint64) ([]*APIKey, error)
	Get(ctx context.Context, id, projectID uint64) (*APIKey, error)
	Update(ctx context.Context, k *APIKey) error
	Revoke(ctx context.Context, id, projectID uint64) error
}
