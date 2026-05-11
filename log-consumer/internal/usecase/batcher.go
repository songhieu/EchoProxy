package usecase

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/log-consumer/internal/domain"
)

// Batcher accumulates rows and flushes when the batch is full or the
// interval elapses, whichever comes first. It owns the lifetime of a single
// Kafka consumer worker.
type Batcher struct {
	sink     domain.Sink
	maxSize  int
	maxWait  time.Duration
	log      zerolog.Logger
	pending  []domain.Row
	deadline time.Time
}

func NewBatcher(sink domain.Sink, maxSize int, maxWait time.Duration, log zerolog.Logger) *Batcher {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if maxWait <= 0 {
		maxWait = time.Second
	}
	return &Batcher{
		sink:    sink,
		maxSize: maxSize,
		maxWait: maxWait,
		log:     log,
		pending: make([]domain.Row, 0, maxSize),
	}
}

// Add appends a row and flushes if the batch is full.
func (b *Batcher) Add(ctx context.Context, row domain.Row) error {
	if len(b.pending) == 0 {
		b.deadline = time.Now().Add(b.maxWait)
	}
	b.pending = append(b.pending, row)
	if len(b.pending) >= b.maxSize {
		return b.Flush(ctx)
	}
	return nil
}

// MaybeFlush flushes if the deadline has passed. Caller (the consumer loop)
// invokes this on a tick.
func (b *Batcher) MaybeFlush(ctx context.Context) error {
	if len(b.pending) == 0 {
		return nil
	}
	if time.Now().Before(b.deadline) {
		return nil
	}
	return b.Flush(ctx)
}

// Flush forces the current batch to the sink.
func (b *Batcher) Flush(ctx context.Context) error {
	if len(b.pending) == 0 {
		return nil
	}
	if err := b.sink.Insert(ctx, b.pending); err != nil {
		// Keep the batch in place so the consumer's commit loop will retry.
		b.log.Error().Err(err).Int("batch", len(b.pending)).Msg("clickhouse insert failed")
		return err
	}
	b.log.Debug().Int("batch", len(b.pending)).Msg("clickhouse insert ok")
	b.pending = b.pending[:0]
	return nil
}

// Pending returns the in-memory batch size (for metrics).
func (b *Batcher) Pending() int { return len(b.pending) }
