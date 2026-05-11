package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// ValidateAPIKey is the hot-path use case that authorizes a single proxy
// request. It must avoid I/O whenever the cache is warm.
type ValidateAPIKey struct {
	repo  domain.APIKeyRepository
	cache domain.APIKeyCache
}

func NewValidateAPIKey(repo domain.APIKeyRepository, cache domain.APIKeyCache) *ValidateAPIKey {
	return &ValidateAPIKey{repo: repo, cache: cache}
}

// Execute returns the API key if the raw key is valid and the target host is
// permitted. All errors are mapped to domain sentinels so the delivery layer
// can map them to HTTP status codes uniformly.
func (uc *ValidateAPIKey) Execute(ctx context.Context, rawKey, targetHost string) (*domain.APIKey, error) {
	if rawKey == "" {
		return nil, domain.ErrAPIKeyNotFound
	}
	hash := HashKey(rawKey)

	key, ok := uc.cache.Get(hash)
	if !ok {
		k, err := uc.repo.GetByHash(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("validate apikey: %w", err)
		}
		uc.cache.Set(hash, k)
		key = k
	}

	if key.Status == domain.StatusRevoked {
		// Return the key alongside the error so the caller can attribute the
		// audit event to the right project.
		return key, domain.ErrAPIKeyRevoked
	}
	if !key.AllowsHost(targetHost) {
		return key, domain.ErrTargetNotAllowed
	}
	return key, nil
}

// HashKey returns a stable, hex-encoded SHA-256 of the raw key. The DB stores
// only this hash so a leak of `api_keys.hash` doesn't reveal usable keys.
func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
