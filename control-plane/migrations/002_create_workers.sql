-- 002_create_workers.sql
-- Creates the worker nodes table.

CREATE TABLE IF NOT EXISTS workers (
    id                UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_unit_id       UUID         NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    hostname_hash     TEXT         NOT NULL,
    agents            TEXT[]       NOT NULL DEFAULT '{}',
    state             TEXT         NOT NULL DEFAULT 'offline',
    capacity_score    INT          NOT NULL DEFAULT 100,
    last_heartbeat_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_unit_id, hostname_hash)
);

CREATE INDEX IF NOT EXISTS idx_workers_org    ON workers (org_unit_id);
CREATE INDEX IF NOT EXISTS idx_workers_state  ON workers (state);
CREATE INDEX IF NOT EXISTS idx_workers_agents ON workers USING GIN (agents);

COMMENT ON TABLE  workers IS 'Registered compute nodes that execute AI coding tasks.';
COMMENT ON COLUMN workers.hostname_hash IS 'SHA-256 of the hostname — stored hashed for privacy.';
COMMENT ON COLUMN workers.agents        IS 'List of AI agent identifiers this worker can serve.';
COMMENT ON COLUMN workers.capacity_score IS 'Scheduler hint: higher = prefer this worker.';
