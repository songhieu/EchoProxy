package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/auth-api/internal/domain"
	"github.com/songhieu/EchoProxy/auth-api/internal/usecase"
)

type Handler struct {
	auth     *usecase.Auth
	users    domain.UserRepository
	projects *usecase.Projects
	keys     *usecase.APIKeys
	log      zerolog.Logger
}

func NewHandler(a *usecase.Auth, u domain.UserRepository, p *usecase.Projects, k *usecase.APIKeys, log zerolog.Logger) *Handler {
	return &Handler{auth: a, users: u, projects: p, keys: k, log: log}
}

func (h *Handler) Routes() stdhttp.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) { _, _ = w.Write([]byte("ok")) })

	r.Post("/v1/signup", h.signup)
	r.Post("/v1/login", h.login)

	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Get("/v1/me", h.me)
		r.Get("/v1/projects", h.listProjects)
		r.Post("/v1/projects", h.createProject)
		r.Get("/v1/projects/{projectID}", h.getProject)
		r.Patch("/v1/projects/{projectID}", h.updateProject)
		r.Get("/v1/projects/{projectID}/keys", h.listKeys)
		r.Post("/v1/projects/{projectID}/keys", h.createKey)
		r.Get("/v1/projects/{projectID}/keys/{keyID}", h.getKey)
		r.Patch("/v1/projects/{projectID}/keys/{keyID}", h.updateKey)
		r.Delete("/v1/projects/{projectID}/keys/{keyID}", h.revokeKey)
	})

	return r
}

// ─── Auth ───────────────────────────────────────────────────────────────────
type credsReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResp struct {
	Token string  `json:"token"`
	User  userOut `json:"user"`
}

type userOut struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) signup(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "invalid body")
		return
	}
	u, tok, err := h.auth.Signup(r.Context(), in.Email, in.Password)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			writeErr(w, stdhttp.StatusConflict, "email taken")
			return
		}
		writeErr(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusCreated, tokenResp{Token: tok, User: userOut{u.ID, u.Email}})
}

func (h *Handler) login(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "invalid body")
		return
	}
	u, tok, err := h.auth.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		writeErr(w, stdhttp.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, stdhttp.StatusOK, tokenResp{Token: tok, User: userOut{u.ID, u.Email}})
}

func (h *Handler) me(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	writeJSON(w, stdhttp.StatusOK, map[string]uint64{"id": uid})
}

// ─── Projects ───────────────────────────────────────────────────────────────
type createProjectReq struct {
	Name string `json:"name"`
}

func (h *Handler) listProjects(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	ps, err := h.projects.List(r.Context(), uid)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, ps)
}

func (h *Handler) createProject(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	var in createProjectReq
	_ = json.NewDecoder(r.Body).Decode(&in)
	p, err := h.projects.Create(r.Context(), uid, in.Name)
	if err != nil {
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusCreated, p)
}

func (h *Handler) getProject(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, err := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "bad project id")
		return
	}
	p, err := h.projects.Get(r.Context(), pid, uid)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, p)
}

type updateProjectReq struct {
	RetentionDays *int `json:"retention_days"`
}

func (h *Handler) updateProject(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, err := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "bad project id")
		return
	}
	var in updateProjectReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "invalid body")
		return
	}
	if in.RetentionDays == nil {
		writeErr(w, stdhttp.StatusBadRequest, "retention_days required")
		return
	}
	if *in.RetentionDays < 1 || *in.RetentionDays > 90 {
		writeErr(w, stdhttp.StatusBadRequest, "retention_days must be 1..90")
		return
	}
	p, err := h.projects.UpdateRetention(r.Context(), pid, uid, *in.RetentionDays)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, p)
}

// ─── API keys ───────────────────────────────────────────────────────────────
type createKeyReq struct {
	Allowlist    []string        `json:"allowlist"`
	BodyCap      int             `json:"body_cap"`
	RateLimitRPS int             `json:"rate_limit_rps"`
	RedactRules  json.RawMessage `json:"redact_rules"`
	Description  string          `json:"description"`
}

type updateKeyReq createKeyReq

func (h *Handler) listKeys(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, err := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "bad project id")
		return
	}
	keys, err := h.keys.List(r.Context(), pid, uid)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyToWire(k, ""))
	}
	writeJSON(w, stdhttp.StatusOK, out)
}

func (h *Handler) createKey(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, err := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "bad project id")
		return
	}
	var in createKeyReq
	_ = json.NewDecoder(r.Body).Decode(&in)
	out, err := h.keys.Create(r.Context(), usecase.CreateAPIKeyInput{
		ProjectID:    pid,
		OwnerID:      uid,
		Allowlist:    in.Allowlist,
		BodyCap:      in.BodyCap,
		RateLimitRPS: in.RateLimitRPS,
		RedactRules:  in.RedactRules,
		Description:  in.Description,
	})
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, keyToWire(out.Key, out.Raw))
}

func (h *Handler) getKey(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	kid, _ := strconv.ParseUint(chi.URLParam(r, "keyID"), 10, 64)
	k, err := h.keys.Get(r.Context(), pid, uid, kid)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, keyToWire(k, ""))
}

func (h *Handler) updateKey(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	kid, _ := strconv.ParseUint(chi.URLParam(r, "keyID"), 10, 64)
	var in updateKeyReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, stdhttp.StatusBadRequest, "invalid body")
		return
	}
	k, err := h.keys.Update(r.Context(), usecase.UpdateAPIKeyInput{
		ProjectID:    pid,
		OwnerID:      uid,
		KeyID:        kid,
		Allowlist:    in.Allowlist,
		BodyCap:      in.BodyCap,
		RateLimitRPS: in.RateLimitRPS,
		RedactRules:  in.RedactRules,
		Description:  in.Description,
	})
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, keyToWire(k, ""))
}

func (h *Handler) revokeKey(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	uid := userID(r.Context())
	pid, _ := strconv.ParseUint(chi.URLParam(r, "projectID"), 10, 64)
	kid, _ := strconv.ParseUint(chi.URLParam(r, "keyID"), 10, 64)
	if err := h.keys.Revoke(r.Context(), pid, uid, kid); err != nil {
		writeProjectErr(w, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func keyToWire(k *domain.APIKey, raw string) map[string]any {
	rules := json.RawMessage(k.RedactRules)
	if len(rules) == 0 {
		rules = json.RawMessage("{}")
	}
	out := map[string]any{
		"id":             k.ID,
		"prefix":         k.Prefix,
		"allowlist":      k.Allowlist,
		"body_cap":       k.BodyCap,
		"rate_limit_rps": k.RateLimitRPS,
		"redact_rules":   rules,
		"status":         k.Status,
		"description":    k.Description,
		"created_at":     k.CreatedAt,
	}
	if raw != "" {
		out["raw"] = raw
	}
	return out
}

// ─── Helpers ────────────────────────────────────────────────────────────────
type ctxKey int

const userIDKey ctxKey = 1

func userID(ctx context.Context) uint64 {
	if v := ctx.Value(userIDKey); v != nil {
		return v.(uint64)
	}
	return 0
}

func (h *Handler) authMiddleware(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		raw := r.Header.Get("Authorization")
		raw = strings.TrimPrefix(raw, "Bearer ")
		if raw == "" {
			writeErr(w, stdhttp.StatusUnauthorized, "missing token")
			return
		}
		uid, err := h.auth.Verify(raw)
		if err != nil {
			writeErr(w, stdhttp.StatusUnauthorized, "invalid token")
			return
		}
		// JWT signature is valid, but the user may have been deleted, or the
		// DB may have been recreated under us. Without this check the next
		// handler would explode on a foreign-key violation with a 500 — we
		// want a clean 401 so the dashboard can auto-logout.
		if _, err := h.users.FindByID(r.Context(), uid); err != nil {
			writeErr(w, stdhttp.StatusUnauthorized, "user no longer exists")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON(w stdhttp.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w stdhttp.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeProjectErr(w stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrAPIKeyNotFound):
		writeErr(w, stdhttp.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, stdhttp.StatusForbidden, err.Error())
	default:
		writeErr(w, stdhttp.StatusInternalServerError, err.Error())
	}
}
