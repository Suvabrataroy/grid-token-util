-- 003_create_tasks.sql
-- Creates the tasks table partitioned by queued_at for time-based archiving.

CREATE TABLE IF NOT EXISTS tasks (
    id                  UUID         NOT NULL DEFAULT uuid_generate_v4(),
    org_unit_id         UUID         NOT NULL REFERENCES orgs(id),
    submitter_id        UUID,
    title               TEXT         NOT NULL,
    description         TEXT         NOT NULL,
    task_type           TEXT         NOT NULL,
    priority            INT          NOT NULL DEFAULT 5,
    ai_agent            TEXT         NOT NULL,
    state               TEXT         NOT NULL DEFAULT 'queued',
    assigned_worker_id  UUID         REFERENCES workers(id),
    queued_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    assigned_at         TIMESTAMPTZ,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    failed_at           TIMESTAMPTZ,
    error_message       TEXT,
    retry_count         INT          NOT NULL DEFAULT 0,
    max_retries         INT          NOT NULL DEFAULT 3,
    options             JSONB        NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, queued_at)
) PARTITION BY RANGE (queued_at);

-- Default catch-all partition for rows that don't match a monthly partition.
CREATE TABLE IF NOT EXISTS tasks_default PARTITION OF tasks DEFAULT;

CREATE INDEX IF NOT EXISTS idx_tasks_org    ON tasks (org_unit_id);
CREATE INDEX IF NOT EXISTS idx_tasks_state  ON tasks (state);
CREATE INDEX IF NOT EXISTS idx_tasks_worker ON tasks (assigned_worker_id)
    WHERE assigned_worker_id IS NOT NULL;

COMMENT ON TABLE  tasks IS 'AI coding tasks submitted by org members and executed by workers.';
COMMENT ON COLUMN tasks.priority    IS 'Scheduling priority 1-10; higher value = processed first.';
COMMENT ON COLUMN tasks.ai_agent    IS 'Identifier of the AI agent required to execute this task.';
COMMENT ON COLUMN tasks.options     IS 'Freeform JSON options passed to the AI agent.';
