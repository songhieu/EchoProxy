package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/songhieu/EchoProxy/ingest-api/internal/domain"
	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/pkg/ratelimit"
	"github.com/songhieu/EchoProxy/pkg/redact"
)

// Ingest is the single entry point for both HTTP and gRPC adapters. It
// validates the API key once, stamps the events, and pushes them to Kafka.
type Ingest struct {
	repo            domain.APIKeyRepository
	cache           domain.APIKeyCache
	sink            domain.Sink
	defaultRedactor *redact.Redactor
	limiter         *ratelimit.Limiter
}

func NewIngest(repo domain.APIKeyRepository, cache domain.APIKeyCache, sink domain.Sink, redactor *redact.Redactor, limiter *ratelimit.Limiter) *Ingest {
	if redactor == nil {
		redactor = redact.New(redact.Rules{})
	}
	if limiter == nil {
		limiter = ratelimit.Disabled()
	}
	return &Ingest{repo: repo, cache: cache, sink: sink, defaultRedactor: redactor, limiter: limiter}
}

// Result captures partial-success semantics. Accepted = events pushed; Rejected = events dropped.
type Result struct {
	Accepted uint32
	Rejected uint32
	Reason   string
}

func (uc *Ingest) Execute(ctx context.Context, rawKey string, events []*event.HttpEvent) (Result, error) {
	if rawKey == "" {
		return Result{}, domain.ErrAPIKeyNotFound
	}
	hash := HashKey(rawKey)
	key, ok := uc.cache.Get(hash)
	if !ok {
		k, err := uc.repo.GetByHash(ctx, hash)
		if err != nil {
			return Result{}, fmt.Errorf("ingest: %w", err)
		}
		uc.cache.Set(hash, k)
		key = k
	}
	if key.Status == "revoked" {
		return Result{}, domain.ErrAPIKeyRevoked
	}

	if d := uc.limiter.Allow(ctx, key.ID, key.RateLimitRPS); !d.Allowed {
		return Result{}, domain.ErrRateLimited
	}

	red := key.Redactor
	if red == nil {
		red = uc.defaultRedactor
	}

	res := Result{}
	for _, ev := range events {
		if err := event.Validate(ev); err != nil {
			res.Rejected++
			if res.Reason == "" {
				res.Reason = err.Error()
			}
			continue
		}
		// Stamp authoritative identity onto the event so SDKs cannot lie.
		ev.ProjectId = key.ProjectID
		ev.ApiKeyId = key.ID
		if ev.EventId == "" {
			ev.EventId = event.NewEventID()
		}
		if ev.TimestampNs == 0 {
			ev.TimestampNs = event.NowNanos()
		}
		// Server-side scrub even if the SDK forgot — defense in depth.
		ev.ReqHeaders = red.Headers(ev.ReqHeaders)
		ev.ResHeaders = red.Headers(ev.ResHeaders)
		reqCT := ev.ReqHeaders["Content-Type"]
		resCT := ev.ResHeaders["Content-Type"]
		ev.ReqBody = red.Body(ev.ReqBody, reqCT)
		ev.ResBody = red.Body(ev.ResBody, resCT)
		if err := uc.sink.Push(ctx, ev); err != nil {
			res.Rejected++
			if res.Reason == "" {
				res.Reason = err.Error()
			}
			continue
		}
		res.Accepted++
	}
	if res.Accepted == 0 && res.Rejected > 0 {
		return res, errors.New("all events rejected: " + res.Reason)
	}
	return res, nil
}

func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
