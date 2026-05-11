-- Initial echoproxy schema: users, projects, api_keys.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    email           CITEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    owner_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS projects_owner_idx ON projects(owner_id);

CREATE TABLE IF NOT EXISTS api_keys (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    hash         TEXT UNIQUE NOT NULL,        -- sha256 of the raw key
    prefix       TEXT,                        -- visible prefix shown in UI
    allowlist    TEXT[] NOT NULL DEFAULT '{}', -- exact hostnames; empty = allow all
    body_cap     INTEGER NOT NULL DEFAULT 0,  -- 0 = use proxy default
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    description  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS api_keys_project_idx ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS api_keys_hash_idx    ON api_keys(hash);
