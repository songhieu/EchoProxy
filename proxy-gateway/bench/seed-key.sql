-- Seed the bench API key (sk_test_demo).
-- Idempotent: safe to run multiple times. The raw key is sha256-hashed
-- because that is what the proxy looks up.
--
-- Attaches to the smallest existing project_id by default so the bench
-- traffic shows up under whatever project you already see in the dashboard.
-- Falls back to creating a dedicated bench user + project if the DB is empty.

DO $$
DECLARE
    target_project_id BIGINT;
    bench_user_id     BIGINT;
BEGIN
    SELECT id INTO target_project_id FROM projects ORDER BY id LIMIT 1;

    IF target_project_id IS NULL THEN
        INSERT INTO users (email, password_hash)
        VALUES ('bench@echoproxy.local', 'x')
        ON CONFLICT (email) DO NOTHING;

        SELECT id INTO bench_user_id FROM users WHERE email = 'bench@echoproxy.local';

        INSERT INTO projects (owner_id, name) VALUES (bench_user_id, 'bench')
        RETURNING id INTO target_project_id;
    END IF;

    INSERT INTO api_keys (project_id, hash, prefix, allowlist, status, description)
    VALUES (
        target_project_id,
        encode(digest('sk_test_demo', 'sha256'), 'hex'),
        'sk_test_',
        '{}',                       -- empty allowlist = any host
        'active',
        'bench: stress test key'
    )
    ON CONFLICT (hash) DO UPDATE
       SET project_id = EXCLUDED.project_id,
           status     = 'active',
           revoked_at = NULL;
END $$;
