# m9m scripts

One-shot SQL fixes and operational scripts.

## `fix-raw-data-postgres.sql`

Run against **Postgres** deployments that pre-date the `raw_data` table.

Background: the `PersistentWebhookStorage` reads/writes the `raw_data`
table, but the original `PostgresStorage.initSchema()` never created it.
Without this table, `LoadActiveWebhooks()` fails on startup and webhook
requests return "Webhook not found".

```bash
psql "postgres://user:pass@host:5432/m9m" -f scripts/fix-raw-data-postgres.sql
```

Then re-activate workflows to repopulate the webhook cache:

```bash
curl -X POST http://host:8080/api/v1/workflows/{id}/deactivate
curl -X POST http://host:8080/api/v1/workflows/{id}/activate
```

## `fix-raw-data-mysql.sql`

Run against **MySQL** deployments that pre-date the `raw_data` table, or
for legacy schemas that need the `workspace_id` columns on `workflows`
and `executions`.

The current upstream `MySQLStorage.initSchema()` creates all of these
tables automatically on first server start, so this script is only
needed to repair an out-of-sync schema.

```bash
mysql -u root -p m9m < scripts/fix-raw-data-mysql.sql
```

The script uses a temporary stored procedure (`add_column_if_missing`)
for the `ALTER TABLE ADD COLUMN` step so it is safe to re-run. The
procedure is dropped at the end of the script.
