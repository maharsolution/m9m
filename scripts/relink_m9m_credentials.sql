-- Relink the surviving m9m credentials rows to use the n8n-stable
-- credential id, so the sync bridge (post-fix commit 1c34d64) upserts
-- them in place on subsequent cycles instead of creating new rows.
--
-- Background:
--   After dedupe_m9m_credentials.sql collapses duplicates, each
--   (name, type) group has ONE row — but its id is still the
--   m9m-generated `cred_<ts>_<seq>`. The next sync POST will send
--   `{id: "<n8n-stable-id>", ...}`, which MySQL's ON DUPLICATE KEY
--   UPDATE will treat as a NEW row (no PK collision) — producing a
--   duplicate again.
--
--   This script does the lookup and UPDATE on the m9m side, mapping
--   each surviving m9m row to its n8n-stable id by (name, type). The
--   n8n-stable ids are baked in here because:
--
--     - They never change (n8n uses a random 14-char alphanumeric
--       generated once at credential create time).
--     - Pulling them at run-time would require the bridge to query
--       n8n's REST API again, which is more moving parts than this
--       one-shot script deserves.
--
--   Mapping below was captured live from n8n's GET /api/v1/credentials
--   on 2026-09-09 (see tmp/validation-2026-09-09/n8n_credentials.json):
--
--     basic_credential / httpBasicAuth     -> 7yYBGnRIN6EZRLD5
--     header auth - apikey / httpHeaderAuth -> cGt9OpKCfFxBmKnR
--     JWT Auth account / jwtAuth           -> 7Toxe8olSwP9Q5B2
--     Kvm8 - m9m_prod / mySql              -> yTExVVLoThzj0EJo
--     Kvm8-n8n_prod / postgres             -> MvNyaYifN5VYIubg
--
--   The 6th surviving row (`mybasic` / httpBasicAuth) was created
--   directly in m9m (id `cred_1788421061072353676_1`) and has no
--   n8n counterpart — leave it untouched.
--
-- Safety:
--   - Wrapped in a transaction.
--   - Only targets rows whose current id still starts with `cred_`
--     (the legacy scheme); any future n8n-stable rows are untouched.
--   - The UPDATE changes only the `id` column; the secret `data`,
--     `name`, `type` stay as-is.
--
-- Usage:
--   docker exec -i m9m-mysql mysql -u m9m_admin -pSecure_Db_Pass_2026 m9m_prod \
--     < scripts/relink_m9m_credentials.sql

START TRANSACTION;

-- Preview: which rows are about to be updated.
SELECT '--- BEFORE: rows to relink ---' AS status;
SELECT id, name, type
FROM credentials
WHERE (name = 'basic_credential' AND type = 'httpBasicAuth')
   OR (name = 'header auth - apikey' AND type = 'httpHeaderAuth')
   OR (name = 'JWT Auth account' AND type = 'jwtAuth')
   OR (name = 'Kvm8 - m9m_prod' AND type = 'mySql')
   OR (name = 'Kvm8-n8n_prod' AND type = 'postgres');

-- Relink each row to its n8n-stable id. Using the WHERE id LIKE
-- 'cred_%' guard so this script is safe to re-run — if the row is
-- already relinked (its id no longer starts with `cred_`), the
-- UPDATE matches 0 rows.
UPDATE credentials SET id = '7yYBGnRIN6EZRLD5'
WHERE name = 'basic_credential' AND type = 'httpBasicAuth' AND id LIKE 'cred_%';

UPDATE credentials SET id = 'cGt9OpKCfFxBmKnR'
WHERE name = 'header auth - apikey' AND type = 'httpHeaderAuth' AND id LIKE 'cred_%';

UPDATE credentials SET id = '7Toxe8olSwP9Q5B2'
WHERE name = 'JWT Auth account' AND type = 'jwtAuth' AND id LIKE 'cred_%';

UPDATE credentials SET id = 'yTExVVLoThzj0EJo'
WHERE name = 'Kvm8 - m9m_prod' AND type = 'mySql' AND id LIKE 'cred_%';

UPDATE credentials SET id = 'MvNyaYifN5VYIubg'
WHERE name = 'Kvm8-n8n_prod' AND type = 'postgres' AND id LIKE 'cred_%';

-- Final state.
SELECT '--- AFTER: relinked rows ---' AS status;
SELECT id, name, type
FROM credentials
ORDER BY name;

COMMIT;
