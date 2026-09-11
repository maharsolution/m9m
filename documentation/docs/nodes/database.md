---
title: "Database Nodes"
description: "Database nodes allow workflows to interact with relational databases."
keywords: "m9m nodes, n8n nodes, workflow nodes, HTTP, database, AI, messaging, integrations"
---

# Database Nodes

Database nodes allow workflows to interact with relational databases.

## PostgreSQL Node

Execute queries against PostgreSQL databases.

### Type

```
n8n-nodes-base.postgres
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `connectionUrl` | string | No* | PostgreSQL connection URL |
| `host` | string | No* | Database host |
| `port` | number | No | Port (default: 5432) |
| `database` | string | No* | Database name |
| `user` | string | No* | Username |
| `password` | string | No* | Password |
| `operation` | string | Yes | `executeQuery`, `insert`, `update`, `delete` |
| `query` | string | Depends | SQL query (for executeQuery) |

*Either `connectionUrl` OR individual connection parameters required.

### Connection Examples

#### Using Connection URL

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "connectionUrl": "postgres://user:password@localhost:5432/mydb",
    "operation": "executeQuery",
    "query": "SELECT * FROM users"
  }
}
```

#### Using Individual Parameters

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "host": "localhost",
    "port": 5432,
    "database": "mydb",
    "user": "dbuser",
    "password": "={{ $credentials.postgres.password }}",
    "operation": "executeQuery",
    "query": "SELECT * FROM users WHERE status = 'active'"
  }
}
```

### Operations

#### Execute Query

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "connectionUrl": "postgres://...",
    "operation": "executeQuery",
    "query": "SELECT id, name, email FROM users WHERE created_at > '2024-01-01'"
  }
}
```

Output:
```json
[
  {"json": {"id": 1, "name": "John", "email": "john@example.com"}},
  {"json": {"id": 2, "name": "Jane", "email": "jane@example.com"}}
]
```

#### Insert

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "connectionUrl": "postgres://...",
    "operation": "insert",
    "table": "users",
    "columns": ["name", "email"],
    "values": ["={{ $json.name }}", "={{ $json.email }}"]
  }
}
```

#### Update

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "connectionUrl": "postgres://...",
    "operation": "update",
    "table": "users",
    "updateKey": "id",
    "updateValue": "={{ $json.id }}",
    "columns": ["status"],
    "values": ["inactive"]
  }
}
```

#### Delete

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "connectionUrl": "postgres://...",
    "operation": "delete",
    "table": "users",
    "deleteKey": "id",
    "deleteValue": "={{ $json.id }}"
  }
}
```

---

## MySQL Node

Execute queries against MySQL databases.

### Type

```
n8n-nodes-base.mysql
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `connectionUrl` | string | No* | MySQL connection URL |
| `host` | string | No* | Database host |
| `port` | number | No | Port (default: 3306) |
| `database` | string | No* | Database name |
| `user` | string | No* | Username |
| `password` | string | No* | Password |
| `operation` | string | Yes | `executeQuery`, `insert`, `update`, `delete` |
| `query` | string | Depends | SQL query |

### Examples

#### Select Query

```json
{
  "type": "n8n-nodes-base.mysql",
  "parameters": {
    "host": "localhost",
    "port": 3306,
    "database": "myapp",
    "user": "root",
    "password": "={{ $credentials.mysql.password }}",
    "operation": "executeQuery",
    "query": "SELECT * FROM orders WHERE status = 'pending'"
  }
}
```

#### Insert with Expression

```json
{
  "type": "n8n-nodes-base.mysql",
  "parameters": {
    "connectionUrl": "mysql://user:pass@localhost:3306/myapp",
    "operation": "executeQuery",
    "query": "INSERT INTO logs (message, timestamp) VALUES ('{{ $json.message }}', NOW())"
  }
}
```

---

## SQLite Node

Execute queries against SQLite databases.

### Type

```
n8n-nodes-base.sqlite
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `filename` | string | Yes | Path to SQLite database file |
| `operation` | string | Yes | `executeQuery`, `insert`, `update`, `delete` |
| `query` | string | Depends | SQL query |

### Examples

#### Query SQLite File

```json
{
  "type": "n8n-nodes-base.sqlite",
  "parameters": {
    "filename": "/data/myapp.db",
    "operation": "executeQuery",
    "query": "SELECT * FROM settings"
  }
}
```

#### Insert Record

```json
{
  "type": "n8n-nodes-base.sqlite",
  "parameters": {
    "filename": "./local.db",
    "operation": "executeQuery",
    "query": "INSERT INTO events (name, data) VALUES ('{{ $json.event }}', '{{ JSON.stringify($json.data) }}')"
  }
}
```

---

## Common Patterns

### Parameterized Queries

Prevent SQL injection with expressions:

```json
{
  "query": "SELECT * FROM users WHERE id = {{ parseInt($json.userId) }}"
}
```

### Batch Insert

Loop through items:

```json
{
  "type": "n8n-nodes-base.postgres",
  "parameters": {
    "operation": "executeQuery",
    "query": "INSERT INTO items (name, value) VALUES ('{{ $json.name }}', {{ $json.value }})"
  }
}
```

### Check Query Results

Use Filter node after database query:

```json
{
  "type": "n8n-nodes-base.filter",
  "parameters": {
    "conditions": [
      {
        "leftValue": "={{ $json.length }}",
        "operator": "greaterThan",
        "rightValue": 0
      }
    ]
  }
}
```

---

## Quick Reference

| Node | Type | Default Port |
|------|------|--------------|
| PostgreSQL | `n8n-nodes-base.postgres` | 5432 |
| MySQL | `n8n-nodes-base.mysql` | 3306 |
| SQLite | `n8n-nodes-base.sqlite` | N/A (file) |
| MongoDB | `n8n-nodes-base.mongoDb` | 27017 |
| Redis | `n8n-nodes-base.redis` | 6379 |
| Elasticsearch | `n8n-nodes-base.elasticsearch` | 9200 |

---

## MongoDB Node

Run queries against a MongoDB collection.

### Type

```
n8n-nodes-base.mongoDb
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `connectionUrl` | string | No* | MongoDB connection string |
| `host` / `port` | string / number | No* | Individual host/port |
| `database` | string | Yes | Database name |
| `collection` | string | Yes | Collection name |
| `operation` | string | Yes | `find`, `insert`, `update`, `delete`, `aggregate` |
| `query` | object | When `find` / `update` / `delete` | MongoDB query document |
| `update` | object | When `update` | Update document (e.g. `{ "$set": { ... } }`) |

### Example — find

```json
{
  "type": "n8n-nodes-base.mongoDb",
  "parameters": {
    "host": "localhost",
    "port": 27017,
    "database": "app",
    "collection": "users",
    "operation": "find",
    "query": "={{ { active: true } }}"
  }
}
```

### Example — aggregate

```json
{
  "type": "n8n-nodes-base.mongoDb",
  "parameters": {
    "database": "app",
    "collection": "events",
    "operation": "aggregate",
    "query": "={{ [{ $match: { type: 'click' } }, { $group: { _id: '$userId', count: { $sum: 1 } } }] }}"
  }
}
```

---

## Redis Node

Issue commands against a Redis server.

### Type

```
n8n-nodes-base.redis
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `connectionUrl` | string | No* | Redis URL (e.g. `redis://localhost:6379/0`) |
| `host` / `port` / `password` | string | No* | Individual connection params |
| `operation` | string | Yes | `get`, `set`, `del`, `incr`, `lpush`, `rpush`, `lpop`, `rpop`, `publish`, `keys`, `info` |
| `key` | string | Depends | Redis key |
| `value` | string | When `set` / `lpush` / `rpush` | Value to write |
| `ttl` | integer | No | TTL in seconds (when supported) |

### Example — set with TTL

```json
{
  "type": "n8n-nodes-base.redis",
  "parameters": {
    "connectionUrl": "redis://localhost:6379/0",
    "operation": "set",
    "key": "session:{{ $json.sessionId }}",
    "value": "={{ JSON.stringify($json.user) }}",
    "ttl": 3600
  }
}
```

---

## Elasticsearch Node

Index, search, and aggregate documents in an Elasticsearch cluster.

### Type

```
n8n-nodes-base.elasticsearch
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `connectionUrl` | string | No* | Elasticsearch URL |
| `username` / `password` | string | No* | Basic auth credentials |
| `index` | string | Yes | Index name |
| `operation` | string | Yes | `index`, `update`, `delete`, `get`, `search`, `multiSearch` |
| `query` | object | When `search` | Elasticsearch query DSL |
| `body` | object | When `update` | Update body |

### Example — search

```json
{
  "type": "n8n-nodes-base.elasticsearch",
  "parameters": {
    "connectionUrl": "http://localhost:9200",
    "index": "logs",
    "operation": "search",
    "query": "={{ { query: { match: { message: $json.text } } } }}"
  }
}
```

---

## MySQL Node

Run SQL queries against a MySQL / MariaDB server.

### Type

```
n8n-nodes-base.mySql
```

### Description

- Supports the same operations as the PostgreSQL node (`executeQuery`, `insert`, `update`, `delete`).
- Connection can be expressed as `connectionUrl` (e.g. `root:pw@tcp(127.0.0.1:3306)/db`) or as host/port/database/user/password.
- TLS via the `ssl` parameter; off by default to match n8n's behaviour.

### Example

```json
{
  "type": "n8n-nodes-base.mySql",
  "parameters": {
    "connectionUrl": "root:pw@tcp(127.0.0.1:3306)/app",
    "operation": "executeQuery",
    "query": "SELECT * FROM users WHERE id = ?"
  }
}
```

> Postgres and MySQL default `sslMode`/`ssl` to **disable** to match n8n's behaviour against local databases. To enable TLS, pass `sslmode=require` in the `connectionUrl` or set the `ssl` flag explicitly.
