package domain

import (
	"context"
	"errors"

	"echoproxy/pkg/redact"
)

var (
	ErrAPIKeyNotFound  = errors.New("api key not found")
	ErrAPIKeyRevoked   = errors.New("api key revoked")
	ErrRateLimited     = errors.New("rate limit exceeded")
)

// APIKey carries the bits ingest-api needs to authorize, redact, and rate-limit
// inbound events. Redactor is built once at load and reused on the hot path.
type APIKey struct {
	ID           uint64
	ProjectID    uint64
	Hash         string
	Status       string
	RateLimitRPS int
	Redactor     *redact.Redactor
}

type APIKeyRepository interface {
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
}

type APIKeyCache interface {
	Get(hash string) (*APIKey, bool)
	Set(hash string, k *APIKey)
}
