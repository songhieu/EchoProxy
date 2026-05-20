package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/songhieu/EchoProxy/ingest-api/internal/domain"
	"github.com/songhieu/EchoProxy/ingest-api/internal/usecase"
	"github.com/songhieu/EchoProxy/pkg/event"
)

// jsonEvent is the wire shape SDKs emit when using HTTP/JSON. Field names mirror
// the proto JSON mapping so a future switch to protojson is drop-in.
type jsonEvent struct {
	EventID          string            `json:"event_id"`
	TimestampNs      int64             `json:"timestamp_ns"`
	Source           string            `json:"source"`
	SDKVersion       string            `json:"sdk_version"`
	Method           string            `json:"method"`
	Scheme           string            `json:"scheme"`
	Host             string            `json:"host"`
	Path             string            `json:"path"`
	Query            string            `json:"query"`
	Status            uint32            `json:"status"`
	LatencyMs         uint32            `json:"latency_ms"`
	UpstreamLatencyMs uint32            `json:"upstream_latency_ms"`
	UpstreamTtfbMs    uint32            `json:"upstream_ttfb_ms"`
	ReqSize           uint32            `json:"req_size"`
	ResSize          uint32            `json:"res_size"`
	ReqHeaders       map[string]string `json:"req_headers"`
	ResHeaders       map[string]string `json:"res_headers"`
	ReqBody          []byte            `json:"req_body"`
	ResBody          []byte            `json:"res_body"`
	ReqBodyTruncated bool              `json:"req_body_truncated"`
	ResBodyTruncated bool              `json:"res_body_truncated"`
	ClientIP         string            `json:"client_ip"`
	UserAgent        string            `json:"user_agent"`
	TraceID          string            `json:"trace_id"`
	Attributes       map[string]string `json:"attributes"`
	Error            string            `json:"error"`
	Direction        string            `json:"direction"`
}

type ingestReq struct {
	Events []jsonEvent `json:"events"`
}

type ingestResp struct {
	Accepted uint32 `json:"accepted"`
	Rejected uint32 `json:"rejected"`
	Reason   string `json:"reason,omitempty"`
}

type Handler struct{ uc *usecase.Ingest }

func New(uc *usecase.Ingest) *Handler { return &Handler{uc: uc} }

func (h *Handler) Routes() stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/events:batch", h.batch)
	return mux
}

func (h *Handler) batch(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if r.Method != stdhttp.MethodPost {
		stdhttp.Error(w, "method not allowed", stdhttp.StatusMethodNotAllowed)
		return
	}
	apiKey := r.Header.Get("X-Echo-Key")
	if apiKey == "" {
		stdhttp.Error(w, "X-Echo-Key required", stdhttp.StatusUnauthorized)
		return
	}

	var req ingestReq
	if err := json.NewDecoder(stdhttp.MaxBytesReader(w, r.Body, 16<<20)).Decode(&req); err != nil {
		stdhttp.Error(w, "invalid body", stdhttp.StatusBadRequest)
		return
	}
	if len(req.Events) == 0 {
		writeJSON(w, stdhttp.StatusOK, ingestResp{})
		return
	}

	events := make([]*event.HttpEvent, 0, len(req.Events))
	for _, e := range req.Events {
		events = append(events, fromJSON(&e))
	}
	res, err := h.uc.Execute(r.Context(), apiKey, events)
	switch {
	case errors.Is(err, domain.ErrAPIKeyNotFound):
		stdhttp.Error(w, "unauthorized", stdhttp.StatusUnauthorized)
		return
	case errors.Is(err, domain.ErrAPIKeyRevoked):
		stdhttp.Error(w, "api key revoked", stdhttp.StatusUnauthorized)
		return
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "1")
		stdhttp.Error(w, "rate limit exceeded", stdhttp.StatusTooManyRequests)
		return
	}
	writeJSON(w, stdhttp.StatusAccepted, ingestResp(res))
}

func fromJSON(j *jsonEvent) *event.HttpEvent {
	return &event.HttpEvent{
		EventId:          j.EventID,
		TimestampNs:      j.TimestampNs,
		Source:           j.Source,
		SdkVersion:       j.SDKVersion,
		Method:           j.Method,
		Scheme:           j.Scheme,
		Host:             j.Host,
		Path:             j.Path,
		Query:            j.Query,
		Status:            j.Status,
		LatencyMs:         j.LatencyMs,
		UpstreamLatencyMs: j.UpstreamLatencyMs,
		UpstreamTtfbMs:    j.UpstreamTtfbMs,
		ReqSize:           j.ReqSize,
		ResSize:          j.ResSize,
		ReqHeaders:       j.ReqHeaders,
		ResHeaders:       j.ResHeaders,
		ReqBody:          j.ReqBody,
		ResBody:          j.ResBody,
		ReqBodyTruncated: j.ReqBodyTruncated,
		ResBodyTruncated: j.ResBodyTruncated,
		ClientIp:         j.ClientIP,
		UserAgent:        j.UserAgent,
		TraceId:          j.TraceID,
		Attributes:       j.Attributes,
		Error:            j.Error,
		Direction:        j.Direction,
	}
}

func writeJSON(w stdhttp.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Compile-time mapping check: ingestResp shape matches usecase.Result for clarity.
var _ = ingestResp(usecase.Result{})
