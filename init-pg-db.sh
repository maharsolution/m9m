#!/bin/bash
# Postgres first-run init for n8n. Runs once on the first `docker compose up`
# when the data volume is empty. The POSTGRES_DB / POSTGRES_USER /
# POSTGRES_PASSWORD env vars already create the default database before
# this script runs, so this file is essentially a placeholder that
# documents where to add n8n-specific grants or extensions later.

set -euo pipefail

# n8n's official image wants the `btree_gin` extension on the
# `n8n_prod` database to speed up tag/filter queries. Adding it here
# avoids manually running CREATE EXTENSION after the first boot.
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<SQL
CREATE EXTENSION IF NOT EXISTS btree_gin;
SQL

echo "[init-pg-db] btree_gin extension ensured in ${POSTGRES_DB}"
