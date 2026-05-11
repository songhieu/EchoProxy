package http

import (
	"errors"
	stdhttp "net/http"
	"strconv"
	"time"

	"github.com/songhieu/EchoProxy/pkg/ratelimit"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/usecase"
)

type ProxyHandler struct {
	validate *usecase.ValidateAPIKey
	proxy    *usecase.ProxyRequest
	limiter  *ratelimit.Limiter
}

func NewProxyHandler(validate *usecase.ValidateAPIKey, proxy *usecase.ProxyRequest, limiter *ratelimit.Limiter) *ProxyHandler {
	if limiter == nil {
		limiter = ratelimit.Disabled()
	}
	return &ProxyHandler{validate: validate, proxy: proxy, limiter: limiter}
}

func (h *ProxyHandler) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	arrival := time.Now()
	rawKey := r.Header.Get("X-Echo-Key")
	rawTarget := r.Header.Get("X-Echo-Target")

	target, err := usecase.ParseTarget(rawTarget)
	if err != nil {
		status, code := mapErr(err)
		writeError(w, err)
		// target may be nil (header missing/invalid). Still log the failure
		// so it shows up in the dashboard.
		h.proxy.EmitAuthFailure(r, target, nil, status, code, arrival)
		return
	}

	key, err := h.validate.Execute(r.Context(), rawKey, target.Host)
	if err != nil {
		status, code := mapErr(err)
		writeError(w, err)
		// key may be nil (not found) or non-nil (revoked / host not allowed).
		// In the non-nil case we get to attribute the failure to its project.
		h.proxy.EmitAuthFailure(r, target, key, status, code, arrival)
		return
	}

	if d := h.limiter.Allow(r.Context(), key.ID, key.RateLimitRPS); !d.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.RetryAfter.Seconds()+1)))
		stdhttp.Error(w, "rate limit exceeded", stdhttp.StatusTooManyRequests)
		h.proxy.EmitAuthFailure(r, target, key, stdhttp.StatusTooManyRequests, "rate_limited", arrival)
		return
	}

	h.proxy.Execute(w, r, key, target)
}

// mapErr translates a domain auth/target error to an HTTP status code +
// short machine-readable tag stored in the audit event's `error` column.
func mapErr(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrAPIKeyNotFound):
		return stdhttp.StatusUnauthorized, "invalid_api_key"
	case errors.Is(err, domain.ErrAPIKeyRevoked):
		return stdhttp.StatusUnauthorized, "revoked_api_key"
	case errors.Is(err, domain.ErrTargetMissing):
		return stdhttp.StatusBadRequest, "target_missing"
	case errors.Is(err, domain.ErrTargetInvalid):
		return stdhttp.StatusBadRequest, "target_invalid"
	case errors.Is(err, domain.ErrTargetNotAllowed):
		return stdhttp.StatusForbidden, "target_not_allowed"
	case errors.Is(err, domain.ErrTargetUnsafe):
		return stdhttp.StatusForbidden, "target_unsafe"
	default:
		return stdhttp.StatusInternalServerError, "internal_error"
	}
}

func writeError(w stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAPIKeyNotFound), errors.Is(err, domain.ErrAPIKeyRevoked):
		stdhttp.Error(w, "unauthorized", stdhttp.StatusUnauthorized)
	case errors.Is(err, domain.ErrTargetMissing):
		stdhttp.Error(w, "X-Echo-Target header required", stdhttp.StatusBadRequest)
	case errors.Is(err, domain.ErrTargetInvalid):
		stdhttp.Error(w, "X-Echo-Target invalid", stdhttp.StatusBadRequest)
	case errors.Is(err, domain.ErrTargetNotAllowed):
		stdhttp.Error(w, "target host not allowed for this api key", stdhttp.StatusForbidden)
	case errors.Is(err, domain.ErrTargetUnsafe):
		stdhttp.Error(w, "target host blocked", stdhttp.StatusForbidden)
	default:
		stdhttp.Error(w, "internal error", stdhttp.StatusInternalServerError)
	}
}
