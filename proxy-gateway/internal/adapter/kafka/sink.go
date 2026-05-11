package kafka

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/domain"
)

// Sink implements domain.EventSink. It exposes a non-blocking Enqueue that
// drops on overflow. A worker pool drains the channel and produces to Kafka
// asynchronously.
type Sink struct {
	ch       chan domain.EventPayload
	producer *event.Producer
	workers  int
	dropped  *uint64
	log      zerolog.Logger
}

type SinkConfig struct {
	BufferSize int
	Workers    int
}

func NewSink(producer *event.Producer, cfg SinkConfig, log zerolog.Logger) *Sink {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 100_000
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 8
	}
	var dropped uint64
	return &Sink{
		ch:       make(chan domain.EventPayload, cfg.BufferSize),
		producer: producer,
		workers:  cfg.Workers,
		dropped:  &dropped,
		log:      log,
	}
}

// Enqueue is non-blocking. On overflow we drop and increment the counter.
func (s *Sink) Enqueue(_ context.Context, ev domain.EventPayload) {
	select {
	case s.ch <- ev:
	default:
		atomic.AddUint64(s.dropped, 1)
	}
}

// Run starts the worker pool. Returns when ctx is done and all workers exit.
func (s *Sink) Run(ctx context.Context) {
	for i := 0; i < s.workers; i++ {
		go s.worker(ctx)
	}
}

func (s *Sink) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-s.ch:
			ev := toProto(payload)
			s.producer.Produce(ctx, ev, func(err error) {
				if err != nil {
					s.log.Warn().Err(err).Msg("kafka produce error")
				}
			})
		}
	}
}

// Dropped returns the number of events dropped due to channel overflow.
func (s *Sink) Dropped() uint64 { return atomic.LoadUint64(s.dropped) }

func toProto(p domain.EventPayload) *event.HttpEvent {
	return &event.HttpEvent{
		EventId:          p.EventID,
		TimestampNs:      p.TimestampNs,
		Source:           p.Source,
		ApiKeyId:         p.APIKeyID,
		ProjectId:        p.ProjectID,
		Method:           p.Method,
		Scheme:           p.Scheme,
		Host:             p.Host,
		Path:             p.Path,
		Query:            p.Query,
		Status:            p.Status,
		LatencyMs:         p.LatencyMs,
		UpstreamLatencyMs: p.UpstreamLatencyMs,
		UpstreamTtfbMs:    p.UpstreamTtfbMs,
		ReqSize:           p.ReqSize,
		ResSize:           p.ResSize,
		ReqHeaders:       p.ReqHeaders,
		ResHeaders:       p.ResHeaders,
		ReqBody:          p.ReqBody,
		ResBody:          p.ResBody,
		ReqBodyTruncated: p.ReqBodyTruncated,
		ResBodyTruncated: p.ResBodyTruncated,
		ClientIp:         p.ClientIP,
		UserAgent:        p.UserAgent,
		TraceId:          p.TraceID,
		Error:            p.Error,
		Direction:        p.Direction,
	}
}
