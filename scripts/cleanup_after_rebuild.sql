-- One-shot cleanup to run AFTER rebuilding m9m with the inbound-id fix
-- (commit 1c34d64). Does three things:
--
--   1. Drops any rows whose id starts with `cred_` (legacy server-
--      generated ids). The sync bridge, now honoring inbound ids,
--      will re-create these rows with n8n-stable ids on its next
--      cycle. Cleaner than RELINK because it leaves a single source
--      of truth (n8n).
--
--   2. Keeps the one m9m-original row (`mybasic / httpBasicAuth /
--      cred_1788421061072353676_1`) which has no n8n counterpart.
--
--   3. Cleans up any debug / test rows left behind by hand testing.
--
-- After this, run `curl -sS -X POST http://localhost:8001/sync` to
-- let the bridge re-populate from n8n. The result should be EXACTLY
-- 5 rows (one per n8n credential), each with an id matching the
-- n8n-stable value.

START TRANSACTION;

SELECT '--- BEFORE ---' AS status;
SELECT id, name, type FROM credentials ORDER BY name;

-- Remove legacy cred_% rows. Safe to re-run (idempotent: matches 0 rows after first run).
DELETE FROM credentials WHERE id LIKE 'cred_%';

-- Remove the debug row I created while testing inbound-id handling.
-- Adjust the name below if your test row used a different name.
DELETE FROM credentials WHERE name = 'test_credential_debug';

SELECT '--- AFTER ---' AS status;
SELECT id, name, type FROM credentials ORDER BY name;

SELECT '--- total ---' AS status;
SELECT COUNT(*) AS total FROM credentials;

COMMIT;
