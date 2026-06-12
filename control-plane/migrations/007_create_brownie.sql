-- 007_create_brownie.sql
-- Creates the brownie points ledger, materialised leaderboard view, and
-- token usage table.

-- ── Brownie points ledger ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS brownie_points_ledger (
    id           UUID   PRIMARY KEY DEFAULT uuid_generate_v4(),
    worker_id    UUID   NOT NULL REFERENCES workers(id),
    org_unit_id  UUID   NOT NULL REFERENCES orgs(id),
    points       INT    NOT NULL,           -- positive = award, negative = deduction
    reason       TEXT   NOT NULL,
    reference_id UUID,                      -- e.g. task_id or output_id
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_brownie_worker ON brownie_points_ledger (worker_id);
CREATE INDEX IF NOT EXISTS idx_brownie_org    ON brownie_points_ledger (org_unit_id);

COMMENT ON TABLE  brownie_points_ledger IS 'Append-only ledger of brownie point transactions.';
COMMENT ON COLUMN brownie_points_ledger.points IS 'Signed integer: positive = award, negative = deduction.';

-- ── Materialised leaderboard ──────────────────────────────────────────────────
CREATE MATERIALIZED VIEW IF NOT EXISTS brownie_leaderboard AS
    SELECT
        worker_id,
        org_unit_id,
        SUM(points)::BIGINT  AS total_points,
        COUNT(*)::BIGINT     AS event_count
    FROM brownie_points_ledger
    GROUP BY worker_id, org_unit_id
    ORDER BY total_points DESC;

CREATE UNIQUE INDEX IF NOT EXISTS idx_brownie_leaderboard
    ON brownie_leaderboard (worker_id, org_unit_id);

COMMENT ON MATERIALIZED VIEW brownie_leaderboard
    IS 'Pre-aggregated worker point totals.  Refresh with REFRESH MATERIALIZED VIEW CONCURRENTLY brownie_leaderboard.';

-- ── Token usage ───────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS token_usage (
    id            UUID   PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id       UUID   NOT NULL,
    worker_id     UUID   NOT NULL REFERENCES workers(id),
    org_unit_id   UUID   NOT NULL REFERENCES orgs(id),
    ai_agent      TEXT   NOT NULL,
    input_tokens  INT    NOT NULL DEFAULT 0,
    output_tokens INT    NOT NULL DEFAULT 0,
    total_tokens  INT    NOT NULL DEFAULT 0,
    recorded_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_usage_org ON token_usage (org_unit_id, recorded_at DESC);

COMMENT ON TABLE token_usage IS 'Per-task AI token consumption records for billing and auditing.';
