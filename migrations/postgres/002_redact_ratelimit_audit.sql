-- Per-key redaction overrides, rate limiting, and body-access audit log.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS redact_rules JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS rate_limit_rps INTEGER NOT NULL DEFAULT 0;

-- Audit log: every time a dashboard user opens a single event with full body.
-- Keep this table append-only; deletion handled by a future TTL job.
CREATE TABLE IF NOT EXISTS body_access_log (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_id    TEXT NOT NULL,
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip          INET,
    user_agent  TEXT
);

CREATE INDEX IF NOT EXISTS body_access_user_idx    ON body_access_log(user_id, accessed_at DESC);
CREATE INDEX IF NOT EXISTS body_access_project_idx ON body_access_log(project_id, accessed_at DESC);
