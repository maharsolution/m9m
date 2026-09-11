---
title: "Productivity Nodes"
description: "Productivity nodes connect m9m to SaaS tools for sheets, documents, and billing."
keywords: "m9m nodes, n8n nodes, workflow nodes, Google Sheets, Notion, Stripe"
---

# Productivity Nodes

Productivity nodes connect m9m to common SaaS tools like Google Sheets, Notion, and Stripe.

---

## GoogleSheets Node

Read, write, and update rows in Google Sheets.

### Type

```
n8n-nodes-base.googleSheets
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `authentication` | string | Yes | `serviceAccount` or `oAuth2` |
| `spreadsheetId` | string | Yes | Spreadsheet id (from the URL) |
| `sheetName` | string | Yes | Tab / sheet name |
| `operation` | string | Yes | `read`, `append`, `update`, `clear` |
| `range` | string | When `read` / `update` | A1 notation range |
| `dataMode` | string | No | `autoMap` (header row), `defineBelow` |
| `values` | array | When `append` / `update` | Rows to write |

### Example — append a row

```json
{
  "type": "n8n-nodes-base.googleSheets",
  "parameters": {
    "authentication": "serviceAccount",
    "spreadsheetId": "1AbC...xyz",
    "sheetName": "Leads",
    "operation": "append",
    "dataMode": "autoMap",
    "values": "={{ [{ name: $json.name, email: $json.email }] }}"
  }
}
```

### Example — read a range

```json
{
  "type": "n8n-nodes-base.googleSheets",
  "parameters": {
    "authentication": "serviceAccount",
    "spreadsheetId": "1AbC...xyz",
    "sheetName": "Orders",
    "operation": "read",
    "range": "A1:F100"
  }
}
```

### Authentication notes

- `serviceAccount`: provide a service-account JSON via credential manager; share the spreadsheet with the service-account email.
- `oAuth2`: a per-user OAuth token. Required when reading user-private sheets.

---

## Notion Node

Create, read, update, and search Notion pages and databases.

### Type

```
n8n-nodes-base.notion
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `authentication` | string | Yes | `apiKey` or `oAuth2` |
| `resource` | string | Yes | `page`, `database`, `block`, `user` |
| `operation` | string | Yes | `create`, `get`, `update`, `query`, `search`, `archive` |
| `databaseId` | string | When resource is `database` | Notion database id |
| `filter` | object | When `query` | Notion filter object |
| `properties` | object | When `create` / `update` | Notion properties object |

### Example — query database

```json
{
  "type": "n8n-nodes-base.notion",
  "parameters": {
    "authentication": "apiKey",
    "resource": "database",
    "operation": "query",
    "databaseId": "d4f8...abc",
    "filter": "={{ { property: 'Status', select: { equals: 'Open' } } }}"
  }
}
```

### Example — create page

```json
{
  "type": "n8n-nodes-base.notion",
  "parameters": {
    "authentication": "apiKey",
    "resource": "page",
    "operation": "create",
    "properties": "={{ { Name: { title: [{ text: { content: $json.title } }] }, Status: { select: { name: 'Open' } } } }}"
  }
}
```

---

## Stripe Node

Create, retrieve, and list Stripe resources (customers, charges, subscriptions, invoices).

### Type

```
n8n-nodes-base.stripe
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `apiKey` | string | Yes | Stripe secret key (`sk_test_*` or `sk_live_*`) |
| `resource` | string | Yes | `customer`, `charge`, `subscription`, `invoice`, `paymentIntent`, `refund` |
| `operation` | string | Yes | `create`, `get`, `list`, `update`, `delete`, `cancel` |
| `id` | string | When operation is `get` / `update` / `delete` | Stripe id |
| `limit` | integer | When operation is `list` | Max results to return |

### Example — create customer

```json
{
  "type": "n8n-nodes-base.stripe",
  "parameters": {
    "apiKey": "={{ $credentials.stripe.secretKey }}",
    "resource": "customer",
    "operation": "create",
    "limit": 0
  }
}
```

> For a full Stripe API call beyond what the convenience fields expose, drop down to the HTTP Request node and target `https://api.stripe.com/v1/...`.

---

## Quick Reference

| Node | Type | Auth |
|------|------|------|
| Google Sheets | `n8n-nodes-base.googleSheets` | Service Account / OAuth2 |
| Notion | `n8n-nodes-base.notion` | API key / OAuth2 |
| Stripe | `n8n-nodes-base.stripe` | Secret API key |
