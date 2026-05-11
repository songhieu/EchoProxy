package kafka

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/pkg/event"
)

// Sink wraps the shared producer to satisfy domain.Sink. The fire-and-forget
// callback only logs errors; the producer's internal queue handles
// batching/retries.
type Sink struct {
	p   *event.Producer
	log zerolog.Logger
}

func New(p *event.Producer, log zerolog.Logger) *Sink { return &Sink{p: p, log: log} }

// Push enqueues an event with a background context so the produce survives the
// originating HTTP request's lifecycle (the caller's ctx is canceled as soon
// as the response is written).
func (s *Sink) Push(_ context.Context, ev *event.HttpEvent) error {
	s.p.Produce(context.Background(), ev, func(err error) {
		if err != nil {
			s.log.Warn().Err(err).Msg("kafka produce error")
		}
	})
	return nil
}
