-- 004_create_api_keys.sql
-- Creates the API key credentials table.

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_unit_id UUID         NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    worker_id   UUID         REFERENCES workers(id) ON DELETE CASCADE,
    key_hash    TEXT         NOT NULL,
    key_prefix  TEXT         NOT NULL,
    name        TEXT         NOT NULL,
    scopes      TEXT[]       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys (key_prefix);
CREATE INDEX IF NOT EXISTS idx_api_keys_org    ON api_keys (org_unit_id);

COMMENT ON TABLE  api_keys IS 'API keys used to authenticate against the control-plane.';
COMMENT ON COLUMN api_keys.key_hash   IS 'Argon2id hash of the plaintext key.';
COMMENT ON COLUMN api_keys.key_prefix IS 'First few characters of the key for prefix lookup without Argon2 overhead.';
COMMENT ON COLUMN api_keys.scopes     IS 'Permission scopes granted to this key.';
COMMENT ON COLUMN api_keys.worker_id  IS 'If set, this key is bound to a specific worker.';
