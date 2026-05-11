-- Raise the http_events table-level TTL from 30 → 90 days. This is now the
-- HARD CAP — older data is deleted by ClickHouse background merges no
-- matter what.
--
-- Per-project shorter retention (the common case) is enforced by the
-- cleanup binary: it reads projects.retention_days and issues
--
--   ALTER TABLE echoproxy.http_events
--   DELETE WHERE project_id = ? AND ts < now() - INTERVAL <days> DAY
--
-- for each project where retention_days < 90. That's an async mutation;
-- CH applies it during the next merge cycle.
--
-- See docs/retention.md for the full picture.

USE echoproxy;

ALTER TABLE http_events
    MODIFY TTL toDateTime(ts) + INTERVAL 90 DAY;
