-- Add upstream-latency breakdown columns. The proxy now records:
--   latency_ms          = total: arrival → response sent (existing)
--   upstream_latency_ms = upstream RoundTrip duration
--   upstream_ttfb_ms    = upstream time-to-first-byte
-- Proxy overhead can be derived: latency_ms - upstream_latency_ms.

USE echoproxy;

ALTER TABLE http_events
    ADD COLUMN IF NOT EXISTS upstream_latency_ms UInt32 DEFAULT 0 AFTER latency_ms,
    ADD COLUMN IF NOT EXISTS upstream_ttfb_ms    UInt32 DEFAULT 0 AFTER upstream_latency_ms;
