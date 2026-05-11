package domain

import "context"

// EventSink is the boundary for emitting captured HTTP events. Adapters
// implement this with a Kafka producer (production), stdout (dev), or
// in-memory channel (tests).
type EventSink interface {
	Enqueue(ctx context.Context, ev EventPayload)
}

// EventPayload is the raw, transport-agnostic representation. The Kafka
// adapter converts it to the protobuf type for the wire.
type EventPayload struct {
	EventID     string
	TimestampNs int64
	Source      string
	APIKeyID    uint64
	ProjectID   uint64

	Method  string
	Scheme  string
	Host    string
	Path    string
	Query   string
	Status  uint32
	LatencyMs         uint32 // total: arrival → response fully sent to client
	UpstreamLatencyMs uint32 // upstream RoundTrip
	UpstreamTtfbMs    uint32 // upstream time-to-first-byte
	ReqSize uint32
	ResSize uint32

	ReqHeaders map[string]string
	ResHeaders map[string]string
	ReqBody    []byte
	ResBody    []byte
	ReqBodyTruncated bool
	ResBodyTruncated bool

	ClientIP  string
	UserAgent string
	TraceID   string
	Error     string
	Direction string
}
