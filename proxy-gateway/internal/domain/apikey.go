package domain

import (
	"context"
	"strings"

	"echoproxy/pkg/redact"
)

// APIKey is the value object that drives proxy authorization. It is loaded
// from Postgres (via APIKeyRepository) and cached in-memory (APIKeyCache) so
// the hot path stays I/O-free. The Redactor is pre-built at load time so the
// hot path doesn't have to compile rules per request.
type APIKey struct {
	ID           uint64
	ProjectID    uint64
	Hash         string
	Prefix       string
	Allowlist    []string // exact hostnames; empty list = ALLOW ALL (dev mode)
	BodyCap      int      // 0 falls back to the proxy default
	RateLimitRPS int      // 0 disables rate limiting
	Status       Status
	Description  string
	Redactor     *redact.Redactor
}

type Status int

const (
	StatusActive Status = iota
	StatusRevoked
)

// AllowsHost reports whether the API key permits forwarding to a target host.
// Empty Allowlist is treated as "allow all" (development convenience).
func (k *APIKey) AllowsHost(host string) bool {
	if len(k.Allowlist) == 0 {
		return true
	}
	host = strings.ToLower(host)
	for _, h := range k.Allowlist {
		if strings.ToLower(h) == host {
			return true
		}
	}
	return false
}

// APIKeyRepository is the persistence boundary. Adapters implement it.
type APIKeyRepository interface {
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	List(ctx context.Context) ([]*APIKey, error)
}

// APIKeyCache is the hot-path cache (e.g. ristretto). Adapters implement it.
type APIKeyCache interface {
	Get(hash string) (*APIKey, bool)
	Set(hash string, key *APIKey)
	Invalidate(hash string)
}
