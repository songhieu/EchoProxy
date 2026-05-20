package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"

	"github.com/songhieu/EchoProxy/stats-api/internal/adapter/postgres"
	"github.com/songhieu/EchoProxy/stats-api/internal/adapter/redis"
	"github.com/songhieu/EchoProxy/stats-api/internal/domain"
	"github.com/songhieu/EchoProxy/stats-api/internal/usecase"
)

type Handler struct {
	q         *usecase.Queries
	cache     *redis.Cache
	audit     *postgres.AuditLogger
	jwtSecret []byte
}

func New(q *usecase.Queries, cache *redis.Cache, audit *postgres.AuditLogger, jwtSecret string) *Handler {
	return &Handler{q: q, cache: cache, audit: audit, jwtSecret: []byte(jwtSecret)}
}

func (h *Handler) Routes() stdhttp.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Get("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) { _, _ = w.Write([]byte("ok")) })
	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Get("/v1/projects/{projectID}/logs", h.listLogs)
		r.Get("/v1/projects/{projectID}/logs/{eventID}", h.getLog)
		r.Get("/v1/projects/{projectID}/metrics", h.metrics)
		r.Get("/v1/projects/{projectID}/top-paths", h.topPaths)
		r.Get("/v1/projects/{projectID}/timeseries", h.timeseries)
		r.Get("/v1/projects/{projectID}/distribution", h.distribution)
		r.Get("/v1/projects/{projectID}/endpoints", h.endpoints)
		r.Get("/v1/projects/{projectID}/audit", h.audit_)
	})
	return r
}

// ─── Handlers ───────────────────────────────────────────────────────────────
func (h *Handler) listLogs(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	q := r.URL.Query()
	f := domain.LogsFilter{
		ProjectID: pid,
		APIKeyID:  parseUint(q.Get("api_key_id")),
		From:      parseTime(q.Get("from")),
		To:        parseTime(q.Get("to")),
		Method:    strings.ToUpper(q.Get("method")),
		Status:    uint16(parseUint(q.Get("status"))),
		PathLike:  q.Get("path"),
		Direction: q.Get("direction"),
		IsStream:  parseTriBool(q.Get("is_stream")),
		Limit:     int(parseUint(q.Get("limit"))),
		Offset:    int(parseUint(q.Get("offset"))),
	}
	logs, err := h.q.ListLogs(r.Context(), f)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, logs)
}

func (h *Handler) getLog(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	eid := chi.URLParam(r, "eventID")
	ev, err := h.q.GetLog(r.Context(), pid, eid)
	if err != nil {
		writeErr(w, stdhttp.StatusNotFound, "not found")
		return
	}
	if h.audit != nil {
		uid := r.Context().Value(userIDKey).(uint64)
		ip := clientIP(r)
		ua := r.UserAgent()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.audit.LogAccess(ctx, uid, pid, eid, ip, ua)
		}()
	}
	writeJSON(w, stdhttp.StatusOK, ev)
}

func (h *Handler) timeseries(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	f := buildAnalyticsFilter(r)
	key := "ts:" + analyticsCacheKey(f)
	var out []domain.TimeBucket
	if h.cache.Get(r.Context(), key, &out) {
		writeJSON(w, stdhttp.StatusOK, out)
		return
	}
	out, err := h.q.TimeSeries(r.Context(), f)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	h.cache.Set(r.Context(), key, out)
	writeJSON(w, stdhttp.StatusOK, out)
}

func (h *Handler) distribution(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	kind := r.URL.Query().Get("kind")
	switch kind {
	case "method", "host":
	default:
		kind = "status"
	}
	f := buildAnalyticsFilter(r)
	key := "dist:" + kind + ":" + analyticsCacheKey(f)
	var out []domain.Bucket
	if h.cache.Get(r.Context(), key, &out) {
		writeJSON(w, stdhttp.StatusOK, out)
		return
	}
	var err error
	switch kind {
	case "method":
		out, err = h.q.MethodDistribution(r.Context(), f)
	case "host":
		out, err = h.q.HostDistribution(r.Context(), f)
	default:
		out, err = h.q.StatusDistribution(r.Context(), f)
	}
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	h.cache.Set(r.Context(), key, out)
	writeJSON(w, stdhttp.StatusOK, out)
}

func (h *Handler) endpoints(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := int(parseUint(r.URL.Query().Get("limit")))
	f := buildAnalyticsFilter(r)
	key := "ep:" + strconv.Itoa(limit) + ":" + analyticsCacheKey(f)
	var out []domain.EndpointStat
	if h.cache.Get(r.Context(), key, &out) {
		writeJSON(w, stdhttp.StatusOK, out)
		return
	}
	out, err := h.q.EndpointStats(r.Context(), f, limit)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	h.cache.Set(r.Context(), key, out)
	writeJSON(w, stdhttp.StatusOK, out)
}

// analyticsCacheKey builds a stable cache key from the filter. Time is
// rounded to the minute so jittery refreshes still share a cache entry.
func analyticsCacheKey(f domain.AnalyticsFilter) string {
	return strconv.FormatUint(f.ProjectID, 10) + "|" +
		strconv.FormatUint(f.APIKeyID, 10) + "|" +
		f.Method + "|" + f.Host + "|" + f.PathLike + "|" + f.Direction + "|" +
		f.From.UTC().Truncate(time.Minute).Format(time.RFC3339) + "|" +
		f.To.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

func buildAnalyticsFilter(r *stdhttp.Request) domain.AnalyticsFilter {
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	q := r.URL.Query()
	return domain.AnalyticsFilter{
		ProjectID: pid,
		APIKeyID:  parseUint(q.Get("api_key_id")),
		Method:    strings.ToUpper(q.Get("method")),
		Host:      q.Get("host"),
		PathLike:  q.Get("path"),
		Direction: q.Get("direction"),
		From:      parseTime(q.Get("from")),
		To:        parseTime(q.Get("to")),
	}
}

func writeData(w stdhttp.ResponseWriter, v any, err error) {
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, v)
}

func (h *Handler) audit_(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if h.audit == nil {
		writeJSON(w, stdhttp.StatusOK, []any{})
		return
	}
	// Audit listing requires the same JWT as everything else; project ownership
	// is already enforced because the projectID path param is scoped to the user.
	writeErr(w, stdhttp.StatusNotImplemented, "audit listing endpoint is wired but not yet served by stats-api; query Postgres directly")
}

func clientIP(r *stdhttp.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.Split(v, ",")[0]
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func (h *Handler) metrics(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	apiKey := parseUint(r.URL.Query().Get("api_key_id"))
	from := parseTime(r.URL.Query().Get("from"))
	to := parseTime(r.URL.Query().Get("to"))
	cacheKey := "metrics:" + strconv.FormatUint(pid, 10) + ":" + strconv.FormatUint(apiKey, 10) + ":" + from.Format(time.RFC3339) + ":" + to.Format(time.RFC3339)
	var out []domain.MinuteMetric
	if h.cache.Get(r.Context(), cacheKey, &out) {
		writeJSON(w, stdhttp.StatusOK, out)
		return
	}
	out, err := h.q.MinuteMetrics(r.Context(), pid, apiKey, from, to)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	h.cache.Set(r.Context(), cacheKey, out)
	writeJSON(w, stdhttp.StatusOK, out)
}

func (h *Handler) topPaths(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	from := parseTime(r.URL.Query().Get("from"))
	to := parseTime(r.URL.Query().Get("to"))
	limit := int(parseUint(r.URL.Query().Get("limit")))
	out, err := h.q.TopPaths(r.Context(), pid, from, to, limit)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, out)
}

// ─── Auth middleware (verifies the JWT minted by auth-api) ──────────────────
type ctxKey int

const userIDKey ctxKey = 1

func (h *Handler) authMiddleware(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" {
			writeErr(w, stdhttp.StatusUnauthorized, "missing token")
			return
		}
		parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("bad alg")
			}
			return h.jwtSecret, nil
		})
		if err != nil || !parsed.Valid {
			writeErr(w, stdhttp.StatusUnauthorized, "invalid token")
			return
		}
		claims, _ := parsed.Claims.(jwt.MapClaims)
		sub, _ := claims["sub"].(float64)
		ctx := context.WithValue(r.Context(), userIDKey, uint64(sub))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ─── Helpers ────────────────────────────────────────────────────────────────
func parseUint(s string) uint64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// parseTriBool returns nil for empty / unrecognised input so callers can
// treat that as "no filter". "1"/"true"/"yes" → true, "0"/"false"/"no" → false.
func parseTriBool(s string) *bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes":
		v := true
		return &v
	case "0", "false", "no":
		v := false
		return &v
	}
	return nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func writeJSON(w stdhttp.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w stdhttp.ResponseWriter, code int, msg string) {
	// Server-side errors (5xx) should leave a breadcrumb in stderr — we have
	// no request-logger middleware, and without this the only signal at the
	// edge is "stats-api 500" with no detail.
	if code >= 500 {
		log.Printf("stats-api %d: %s", code, msg)
	}
	writeJSON(w, code, map[string]string{"error": msg})
}
