-- Dedupe the m9m credentials table after the credential-bridge sync ran
-- multiple cycles before the inbound-id fix landed (commit 1c34d64).
--
-- Background:
--   Before the fix, m9m's POST /api/v1/credentials ignored the inbound
--   `id` field and generated a fresh server-side id (`cred_<ts>_<seq>`)
--   on every call. The sync bridge POSTed each n8n credential every 30s,
--   producing ~100+ rows named the same 5 things ("basic_credential",
--   "header auth - apikey", "JWT Auth account", ...) all with different
--   m9m-generated ids.
--
--   After the fix, new POSTs carry the n8n-stable id (e.g. "7yYBGnRIN6EZRLD5")
--   and MySQL's ON DUPLICATE KEY UPDATE keeps the row count stable.
--
--   This script removes the legacy duplicates so the store collapses to
--   one row per (name, type).
--
-- Safety:
--   - Runs inside a transaction; rolls back on any error.
--   - Targets ONLY rows whose id matches `cred_%` (the server-generated
--     scheme). Rows whose id came from n8n (no `cred_` prefix) are the
--     canonical ones and are NEVER deleted.
--   - The IN-subquery is wrapped in a derived table because MySQL 8
--     rejects self-referencing DELETE/UPDATE targets (error 1093).
--
-- Usage:
--   docker exec -i m9m-mysql mysql -u m9m_admin -pSecure_Db_Pass_2026 m9m_prod \
--     < scripts/dedupe_m9m_credentials.sql

START TRANSACTION;

-- Preview: groups with more than 1 row.
SELECT '--- BEFORE: groups with duplicates ---' AS status;
SELECT name, type, COUNT(*) AS rows_in_group, SUM(id LIKE 'cred_%') AS legacy_rows
FROM credentials
GROUP BY name, type
HAVING rows_in_group > 1
ORDER BY rows_in_group DESC;

-- Delete legacy duplicates for groups that have at least one
-- n8n-stable id row (id NOT LIKE 'cred_%'). For those groups, every
-- legacy `cred_<ts>_<seq>` row is a duplicate.
--
-- The subquery is wrapped in another SELECT because MySQL 8 refuses
-- to let DELETE reference the target table in a subquery directly
-- (error 1093: "You can't specify target table for update in FROM
-- clause"). The outer SELECT forces materialisation.
DELETE FROM credentials
WHERE id LIKE 'cred_%'
  AND (name, type) IN (
    SELECT name, type FROM (
      SELECT name, type
      FROM credentials
      WHERE id NOT LIKE 'cred_%'
      GROUP BY name, type
    ) AS n8n_groups
  );

-- Delete legacy duplicates for groups that have NO n8n-stable id row.
-- For those, keep the row with the OLDEST created_at as the canonical
-- one (the first insert is closest to "what n8n had before the bridge
-- bug started").
DELETE c FROM credentials c
INNER JOIN (
    SELECT name, type, MIN(created_at) AS keep_created_at
    FROM credentials
    GROUP BY name, type
    HAVING SUM(id NOT LIKE 'cred_%') = 0  -- group has no n8n-stable id row
) AS keepers
ON c.name = keepers.name
AND c.type = keepers.type
AND c.id LIKE 'cred_%'                   -- only delete legacy id rows
AND c.created_at > keepers.keep_created_at;

-- Final state.
SELECT '--- AFTER: should be 1 row per (name, type) ---' AS status;
SELECT name, type, id
FROM credentials
ORDER BY name;

SELECT '--- total row count ---' AS status;
SELECT COUNT(*) AS total FROM credentials;

COMMIT;
