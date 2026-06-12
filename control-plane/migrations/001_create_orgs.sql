-- 001_create_orgs.sql
-- Creates the organisations (tenant) table.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS orgs (
    id         UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       TEXT         NOT NULL UNIQUE,
    plan_tier  TEXT         NOT NULL DEFAULT 'free',
    policy     JSONB        NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE orgs IS 'Organisational units (tenants) within the grid.';
COMMENT ON COLUMN orgs.policy IS 'OrgPolicy JSON: execution windows, agent allowlist, auto-merge rules, etc.';
