-- One-shot schema fix for m9m Postgres deployments that pre-date the
-- raw_data table. Run this once against your m9m database, then re-activate
-- your workflows so RegisterWorkflowWebhooks() can persist webhook state.
--
-- Safe to run multiple times: CREATE TABLE IF NOT EXISTS, and ADD COLUMN
-- IF NOT EXISTS where supported. All statements are no-ops if the schema
-- is already current.
--
-- Apply with:
--   psql "postgres://user:pass@host:5432/m9m" -f scripts/fix-raw-data.sql

BEGIN;

-- 1. raw_data: required by PersistentWebhookStorage (webhook persistence).
--    Was being read/written by the webhook subsystem without a matching
--    CREATE TABLE in the original initSchema() — fixed in upstream.
CREATE TABLE IF NOT EXISTS raw_data (
    key        TEXT PRIMARY KEY,
    value      BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_raw_data_updated_at ON raw_data(updated_at);

-- 2. workspace_id columns: idempotent — Postgres 9.6+ supports IF NOT EXISTS
--    on ADD COLUMN. If you're on an older Postgres, drop the IF NOT EXISTS
--    or run ensureColumn() manually from the m9m helper.
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE executions
    ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- 3. Default workspace row — needed if you haven't created one yet.
INSERT INTO workspaces (id, name, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000000', 'default', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- 4. Verify: should return one row each.
-- SELECT 'raw_data'        AS table_name, COUNT(*) AS rows FROM raw_data
-- UNION ALL
-- SELECT 'workspaces'       AS table_name, COUNT(*) AS rows FROM workspaces;
