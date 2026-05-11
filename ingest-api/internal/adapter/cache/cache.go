package cache

import (
	"github.com/dgraph-io/ristretto"

	"echoproxy/ingest-api/internal/domain"
)

type RistrettoCache struct{ c *ristretto.Cache }

func New() (*RistrettoCache, error) {
	c, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1 << 20, MaxCost: 1 << 24, BufferItems: 64})
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

func (r *RistrettoCache) Set(hash string, k *domain.APIKey) { r.c.Set(hash, k, 1) }
