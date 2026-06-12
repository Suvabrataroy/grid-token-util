-- 005_create_audit_log.sql
-- Creates the immutable audit log, partitioned by occurred_at.

CREATE TABLE IF NOT EXISTS audit_log (
    id            UUID         NOT NULL DEFAULT uuid_generate_v4(),
    org_unit_id   UUID         NOT NULL,
    actor_type    TEXT         NOT NULL,       -- 'api_key' | 'system'
    actor_id      TEXT         NOT NULL,
    action        TEXT         NOT NULL,
    resource_type TEXT,
    resource_id   TEXT,
    details       JSONB        NOT NULL DEFAULT '{}',
    ip_address    TEXT,
    occurred_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- Default catch-all partition.
CREATE TABLE IF NOT EXISTS audit_log_default PARTITION OF audit_log DEFAULT;

CREATE INDEX IF NOT EXISTS idx_audit_log_org ON audit_log (org_unit_id, occurred_at DESC);

COMMENT ON TABLE  audit_log IS 'Append-only security audit trail.  No UPDATE or DELETE permitted.';
COMMENT ON COLUMN audit_log.actor_type IS 'Who performed the action: api_key or system.';
COMMENT ON COLUMN audit_log.details   IS 'Contextual information such as HTTP status code, duration, etc.';
