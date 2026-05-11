-- Per-project data retention, configurable from the dashboard.
--
-- The ClickHouse table-level TTL (90 days, see CH migration 005) acts as a
-- hard ceiling. Per-project shorter retention is enforced by the cleanup
-- binary (cleanup/cmd/cleanup) which issues async DELETE mutations against
-- ClickHouse for projects with retention_days < 90.
--
-- Why a per-project knob: Sentry, Datadog, Honeycomb all expose this.
-- Some traffic (login attempts, sensitive bodies) people want to keep for
-- the shortest possible window; some (audit) they want to keep longer.

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS retention_days INTEGER NOT NULL DEFAULT 30
        CHECK (retention_days BETWEEN 1 AND 90);
