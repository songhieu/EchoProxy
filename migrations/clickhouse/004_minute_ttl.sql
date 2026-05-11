-- Tiered retention, matching the pattern used by Datadog (live / indexed /
-- archived), Sentry (default 30/90d, per-project override), Honeycomb (60d
-- events), Cloudflare Logpush (7d on platform).
--
--   http_events         (raw, full bodies)         30 days   — already in 001
--   http_events_minute  (aggregates, no bodies)   180 days   — set here
--
-- Aggregates compress ~100x vs raw rows and serve dashboard charts, so they
-- can outlive the raw events without blowing up storage. Pick a number that
-- matches the longest "trend over time" question you let users ask.

USE echoproxy;

ALTER TABLE http_events_minute
    MODIFY TTL toDateTime(minute) + INTERVAL 180 DAY;
