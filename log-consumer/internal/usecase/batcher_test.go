package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/log-consumer/internal/domain"
)

type fakeSink struct {
	batches [][]domain.Row
	err     error
}

func (f *fakeSink) Insert(_ context.Context, batch []domain.Row) error {
	if f.err != nil {
		return f.err
	}
	cp := make([]domain.Row, len(batch))
	copy(cp, batch)
	f.batches = append(f.batches, cp)
	return nil
}

func TestBatcher_FlushOnSize(t *testing.T) {
	s := &fakeSink{}
	b := NewBatcher(s, 2, time.Second, zerolog.Nop())
	ctx := context.Background()

	_ = b.Add(ctx, domain.Row{EventID: "a"})
	if len(s.batches) != 0 {
		t.Fatalf("should not have flushed yet")
	}
	_ = b.Add(ctx, domain.Row{EventID: "b"})
	if len(s.batches) != 1 || len(s.batches[0]) != 2 {
		t.Fatalf("expected one batch of 2, got %v", s.batches)
	}
}

func TestBatcher_FlushOnDeadline(t *testing.T) {
	s := &fakeSink{}
	b := NewBatcher(s, 100, 10*time.Millisecond, zerolog.Nop())
	ctx := context.Background()

	_ = b.Add(ctx, domain.Row{EventID: "a"})
	time.Sleep(15 * time.Millisecond)
	if err := b.MaybeFlush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(s.batches) != 1 {
		t.Fatalf("deadline flush failed, batches=%d", len(s.batches))
	}
}

func TestBatcher_RetainsOnFailure(t *testing.T) {
	s := &fakeSink{err: errors.New("boom")}
	b := NewBatcher(s, 1, time.Second, zerolog.Nop())
	ctx := context.Background()
	_ = b.Add(ctx, domain.Row{EventID: "a"})
	if b.Pending() != 1 {
		t.Fatalf("rows must be retained on failure, pending=%d", b.Pending())
	}
}
