package domain

import "context"

// Sink is the persistence boundary for the consumer.
type Sink interface {
	Insert(ctx context.Context, batch []Row) error
}

// Row mirrors the http_events ClickHouse columns. Adapters convert from the
// protobuf event into Row.
type Row struct {
	TimestampMs   int64
	EventID       string
	ProjectID     uint64
	APIKeyID      uint64
	Source        string
	SDKVersion    string
	Direction     string

	Method        string
	Scheme        string
	Host          string
	Path          string
	Query         string
	Status            uint16
	LatencyMs         uint32
	UpstreamLatencyMs uint32
	UpstreamTtfbMs    uint32
	ReqSize           uint32
	ResSize           uint32

	ReqHeaders    map[string]string
	ResHeaders    map[string]string
	ReqBody       string
	ResBody       string
	ReqTruncated  uint8
	ResTruncated  uint8

	ClientIP      string
	UserAgent     string
	TraceID       string
	Attributes    map[string]string
	Error         string

	// Streaming. Zero values = not a stream.
	IsStream           uint8
	StreamChunkCount   uint32
	StreamDurationMs   uint32
	StreamIdleTimeout  uint8
}
