-- 006_create_outputs.sql
-- Creates the output packages table for worker-submitted task results.

CREATE TABLE IF NOT EXISTS outputs (
    id             UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id        UUID         NOT NULL,
    worker_id      UUID         NOT NULL REFERENCES workers(id),
    hmac_sha256    TEXT         NOT NULL,
    artifacts      JSONB        NOT NULL DEFAULT '[]',
    metadata       JSONB        NOT NULL DEFAULT '{}',
    review_status  TEXT         NOT NULL DEFAULT 'pending',
    reviewer_id    UUID,
    submitted_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    reviewed_at    TIMESTAMPTZ,

    CONSTRAINT reviewer_required_when_approved CHECK (
        review_status NOT IN ('approved') OR reviewer_id IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_outputs_task          ON outputs (task_id);
CREATE INDEX IF NOT EXISTS idx_outputs_review_status ON outputs (review_status);

COMMENT ON TABLE  outputs IS 'Artefacts submitted by workers when a task completes.';
COMMENT ON COLUMN outputs.hmac_sha256   IS 'HMAC-SHA256 of the serialised artifacts list for tamper detection.';
COMMENT ON COLUMN outputs.artifacts     IS 'Array of file path strings or blob references.';
COMMENT ON COLUMN outputs.review_status IS 'pending | approved | rejected | changes_requested';
