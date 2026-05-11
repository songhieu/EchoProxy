package cache

import (
	"github.com/dgraph-io/ristretto"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// RistrettoCache implements domain.APIKeyCache. Read-heavy hot path so we
// pick ristretto for its concurrent, lock-free reads.
type RistrettoCache struct {
	c *ristretto.Cache
}

func NewRistrettoCache() (*RistrettoCache, error) {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1 << 20,
		MaxCost:     1 << 24, // ~16M cost units; a key is small so this is plenty
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &RistrettoCache{c: c}, nil
}

func (r *RistrettoCache) Get(hash string) (*domain.APIKey, bool) {
	v, ok := r.c.Get(hash)
	if !ok {
		return nil, false
	}
	k, ok := v.(*domain.APIKey)
	return k, ok
}

func (r *RistrettoCache) Set(hash string, key *domain.APIKey) {
	r.c.Set(hash, key, 1)
}

func (r *RistrettoCache) Invalidate(hash string) {
	r.c.Del(hash)
}
