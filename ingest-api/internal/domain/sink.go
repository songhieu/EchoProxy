package domain

import (
	"context"

	"github.com/songhieu/EchoProxy/pkg/event"
)

// Sink ingests one event. Implementations decide how (Kafka producer, etc).
type Sink interface {
	Push(ctx context.Context, ev *event.HttpEvent) error
}
