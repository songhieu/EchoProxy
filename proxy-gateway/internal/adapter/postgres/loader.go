package postgres

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// Loader periodically refreshes the API-key cache from Postgres so the hot
// path never needs to query the DB. A simple full sweep is fine for the
// scale we expect (thousands of keys); upgrade to LISTEN/NOTIFY when needed.
type Loader struct {
	repo     *APIKeyRepo
	cache    domain.APIKeyCache
	interval time.Duration
	log      zerolog.Logger
}

func NewLoader(repo *APIKeyRepo, cache domain.APIKeyCache, interval time.Duration, log zerolog.Logger) *Loader {
	return &Loader{repo: repo, cache: cache, interval: interval, log: log}
}

// Initial loads keys before the server starts accepting traffic.
func (l *Loader) Initial(ctx context.Context) error {
	return l.refresh(ctx)
}

// Run blocks until ctx is done.
func (l *Loader) Run(ctx context.Context) {
	t := time.NewTicker(l.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := l.refresh(ctx); err != nil {
				l.log.Error().Err(err).Msg("apikey refresh failed")
			}
		}
	}
}

func (l *Loader) refresh(ctx context.Context) error {
	keys, err := l.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, k := range keys {
		l.cache.Set(k.Hash, k)
	}
	l.log.Debug().Int("count", len(keys)).Msg("apikey cache refreshed")
	return nil
}
