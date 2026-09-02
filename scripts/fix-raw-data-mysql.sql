-- One-shot schema fix for m9m MySQL deployments that pre-date the raw_data
-- table or that pre-date the MySQL storage backend itself. Run this once
-- against your m9m database, then re-activate your workflows so
-- RegisterWorkflowWebhooks() can persist webhook state.
--
-- Safe to run multiple times: CREATE TABLE IF NOT EXISTS, and the
-- conditional procedure makes ALTER safe on re-runs.
--
-- Apply with:
--   mysql -u root -p m9m < scripts/fix-raw-data-mysql.sql
--
-- The current upstream MySQL backend (internal/storage/mysql.go) creates
-- all of these tables automatically on server start, so this script is
-- only needed for legacy deployments or to repair an out-of-sync schema.

-- 1. raw_data: required by PersistentWebhookStorage (webhook persistence).
CREATE TABLE IF NOT EXISTS raw_data (
    `key`      VARCHAR(255) PRIMARY KEY,
    value      LONGBLOB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_raw_data_updated_at ON raw_data(updated_at);

-- 2. workspace_id columns: MySQL 8.0.29+ supports ADD COLUMN IF NOT EXISTS;
--    for older versions this stored-procedure trick is used instead.
DROP PROCEDURE IF EXISTS add_column_if_missing;
DELIMITER //
CREATE PROCEDURE add_column_if_missing(
    IN tbl       VARCHAR(128),
    IN col_name  VARCHAR(128),
    IN col_def   TEXT
)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME   = tbl
          AND COLUMN_NAME  = col_name
    ) THEN
        SET @ddl = CONCAT('ALTER TABLE ', tbl, ' ADD COLUMN ', col_name, ' ', col_def);
        PREPARE stmt FROM @ddl;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END //
DELIMITER ;

CALL add_column_if_missing(
    'workflows', 'workspace_id',
    "VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'"
);
CALL add_column_if_missing(
    'executions', 'workspace_id',
    "VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'"
);

-- 3. Default workspace row — needed if you haven't created one yet.
INSERT INTO workspaces (id, name, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000000', 'default', NOW(), NOW())
ON DUPLICATE KEY UPDATE name = VALUES(name);

-- 4. Verify: should return one row each.
-- SELECT 'raw_data'  AS table_name, COUNT(*) AS rows FROM raw_data
-- UNION ALL
-- SELECT 'workspaces' AS table_name, COUNT(*) AS rows FROM workspaces;

-- 5. Cleanup the helper procedure (it has no other use after this script).
DROP PROCEDURE IF EXISTS add_column_if_missing;
