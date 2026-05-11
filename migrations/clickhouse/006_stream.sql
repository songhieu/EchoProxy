-- Streaming response fields. The proxy now detects SSE / chunked / gRPC
-- responses and flushes each chunk to the client immediately. Zero values
-- mean the response was not a stream — old rows preserve that meaning.
--
--   is_stream            = 1 if upstream returned a streaming response
--   stream_chunk_count   = number of chunks flushed to the client
--   stream_duration_ms   = body-streaming duration (first byte → close)
--   stream_idle_timeout  = 1 if the watchdog terminated the stream

USE echoproxy;

ALTER TABLE http_events
    ADD COLUMN IF NOT EXISTS is_stream           UInt8  DEFAULT 0 AFTER direction,
    ADD COLUMN IF NOT EXISTS stream_chunk_count  UInt32 DEFAULT 0 AFTER is_stream,
    ADD COLUMN IF NOT EXISTS stream_duration_ms  UInt32 DEFAULT 0 AFTER stream_chunk_count,
    ADD COLUMN IF NOT EXISTS stream_idle_timeout UInt8  DEFAULT 0 AFTER stream_duration_ms;
