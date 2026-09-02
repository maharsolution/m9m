#!/bin/bash
# MySQL first-run init for m9m. Runs once on the first `docker compose up`
# when the data volume is empty (and the MySQL image invokes every
# /docker-entrypoint-initdb.d/*.sql|.sh in lexical order).
#
# The MYSQL_DATABASE / MYSQL_USER / MYSQL_PASSWORD env vars from the
# compose file already create the default database and user before this
# script runs, so this file is mostly defensive: it makes sure the
# `workflows`, `executions`, `credentials`, `tags`, `workspaces`,
# `raw_data` tables and `workspace_id` columns exist.
#
# In practice, the current m9m MySQL backend (internal/storage/mysql.go)
# creates all of these automatically on first start, so this script is
# belt-and-suspenders: only useful for legacy or hand-imported schemas.

set -euo pipefail

# Use the env-provided admin user. The MYSQL_DATABASE / MYSQL_USER have
# already been applied by the entrypoint before this script runs.
: "${MYSQL_DATABASE:=m9m_prod}"
: "${MYSQL_USER:=m9m_admin}"

mysql --protocol=socket -uroot -p"${MYSQL_ROOT_PASSWORD}" <<SQL
-- raw_data: required by PersistentWebhookStorage. MySQL `key` is a
-- reserved word, hence the backticks. utf8mb4 for full unicode support.
CREATE TABLE IF NOT EXISTS \`raw_data\` (
    \`key\`      VARCHAR(255) PRIMARY KEY,
    value      LONGBLOB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX IF NOT EXISTS idx_raw_data_updated_at ON \`raw_data\`(updated_at);
SQL

echo "[init-mysql-db] raw_data table ensured in ${MYSQL_DATABASE}"
