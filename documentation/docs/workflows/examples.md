---
title: "Workflow Examples"
description: "Practical workflow examples for common use cases."
keywords: "m9m workflows, n8n workflows, workflow examples, data flow"
---

# Workflow Examples

Practical workflow examples for common use cases.

## HTTP API Integration

Fetch data from an API and process it:

```json
{
  "name": "API Data Fetch",
  "nodes": [
    {
      "id": "start",
      "name": "Start",
      "type": "n8n-nodes-base.start",
      "position": [250, 300],
      "parameters": {}
    },
    {
      "id": "fetch",
      "name": "Fetch Users",
      "type": "n8n-nodes-base.httpRequest",
      "position": [450, 300],
      "parameters": {
        "url": "https://jsonplaceholder.typicode.com/users",
        "method": "GET"
      }
    },
    {
      "id": "filter",
      "name": "Filter Active",
      "type": "n8n-nodes-base.filter",
      "position": [650, 300],
      "parameters": {
        "conditions": [
          {
            "leftValue": "={{ $json.company.name }}",
            "operator": "exists"
          }
        ]
      }
    }
  ],
  "connections": {
    "Start": {"main": [[{"node": "Fetch Users", "type": "main", "index": 0}]]},
    "Fetch Users": {"main": [[{"node": "Filter Active", "type": "main", "index": 0}]]}
  }
}
```

## Webhook to Slack

Receive webhooks and notify Slack:

```json
{
  "name": "Webhook to Slack",
  "nodes": [
    {
      "id": "webhook",
      "name": "Webhook",
      "type": "n8n-nodes-base.webhook",
      "position": [250, 300],
      "parameters": {
        "path": "/alert",
        "httpMethod": "POST"
      }
    },
    {
      "id": "format",
      "name": "Format Message",
      "type": "n8n-nodes-base.set",
      "position": [450, 300],
      "parameters": {
        "assignments": [
          {
            "name": "slackMessage",
            "value": "Alert: {{ $json.body.message }}\nSeverity: {{ $json.body.severity }}"
          }
        ]
      }
    },
    {
      "id": "slack",
      "name": "Send to Slack",
      "type": "n8n-nodes-base.slack",
      "position": [650, 300],
      "parameters": {
        "webhookUrl": "https://hooks.slack.com/services/...",
        "text": "={{ $json.slackMessage }}"
      }
    }
  ],
  "connections": {
    "Webhook": {"main": [[{"node": "Format Message", "type": "main", "index": 0}]]},
    "Format Message": {"main": [[{"node": "Send to Slack", "type": "main", "index": 0}]]}
  }
}
```

## Scheduled Database Backup

Daily database query and notification:

```json
{
  "name": "Daily Stats Report",
  "nodes": [
    {
      "id": "cron",
      "name": "Daily at 9 AM",
      "type": "n8n-nodes-base.cron",
      "position": [250, 300],
      "parameters": {
        "cronExpression": "0 9 * * *"
      }
    },
    {
      "id": "query",
      "name": "Query Stats",
      "type": "n8n-nodes-base.postgres",
      "position": [450, 300],
      "parameters": {
        "connectionUrl": "postgres://user:pass@localhost/db",
        "operation": "executeQuery",
        "query": "SELECT COUNT(*) as users, DATE(created_at) as date FROM users WHERE created_at > NOW() - INTERVAL '1 day' GROUP BY date"
      }
    },
    {
      "id": "format",
      "name": "Format Report",
      "type": "n8n-nodes-base.set",
      "position": [650, 300],
      "parameters": {
        "assignments": [
          {
            "name": "report",
            "value": "Daily Stats:\n- New users: {{ $json.users }}\n- Date: {{ $json.date }}"
          }
        ]
      }
    },
    {
      "id": "email",
      "name": "Send Email",
      "type": "n8n-nodes-base.emailSend",
      "position": [850, 300],
      "parameters": {
        "smtpHost": "smtp.gmail.com",
        "smtpPort": 587,
        "fromEmail": "reports@company.com",
        "toEmail": "team@company.com",
        "subject": "Daily Stats Report",
        "body": "={{ $json.report }}"
      }
    }
  ],
  "connections": {
    "Daily at 9 AM": {"main": [[{"node": "Query Stats", "type": "main", "index": 0}]]},
    "Query Stats": {"main": [[{"node": "Format Report", "type": "main", "index": 0}]]},
    "Format Report": {"main": [[{"node": "Send Email", "type": "main", "index": 0}]]}
  }
}
```

## AI Content Generation

Use OpenAI to generate content:

```json
{
  "name": "AI Content Generator",
  "nodes": [
    {
      "id": "webhook",
      "name": "Webhook",
      "type": "n8n-nodes-base.webhook",
      "position": [250, 300],
      "parameters": {
        "path": "/generate",
        "httpMethod": "POST"
      }
    },
    {
      "id": "openai",
      "name": "Generate Content",
      "type": "n8n-nodes-base.openAi",
      "position": [450, 300],
      "parameters": {
        "apiKey": "={{ $credentials.openai.apiKey }}",
        "model": "gpt-4",
        "prompt": "Write a blog post about: {{ $json.body.topic }}",
        "maxTokens": 1000
      }
    },
    {
      "id": "response",
      "name": "Format Response",
      "type": "n8n-nodes-base.set",
      "position": [650, 300],
      "parameters": {
        "assignments": [
          {"name": "content", "value": "={{ $json.response }}"},
          {"name": "topic", "value": "={{ $json.body.topic }}"}
        ]
      }
    }
  ],
  "connections": {
    "Webhook": {"main": [[{"node": "Generate Content", "type": "main", "index": 0}]]},
    "Generate Content": {"main": [[{"node": "Format Response", "type": "main", "index": 0}]]}
  }
}
```

## GitHub Issue Monitor

Watch for new issues and notify:

```json
{
  "name": "GitHub Issue Monitor",
  "nodes": [
    {
      "id": "cron",
      "name": "Every 5 Minutes",
      "type": "n8n-nodes-base.cron",
      "position": [250, 300],
      "parameters": {
        "cronExpression": "*/5 * * * *"
      }
    },
    {
      "id": "github",
      "name": "Get Issues",
      "type": "n8n-nodes-base.github",
      "position": [450, 300],
      "parameters": {
        "accessToken": "={{ $credentials.github.accessToken }}",
        "resource": "issue",
        "operation": "list",
        "owner": "my-org",
        "repository": "my-repo"
      }
    },
    {
      "id": "filter",
      "name": "New Issues",
      "type": "n8n-nodes-base.filter",
      "position": [650, 300],
      "parameters": {
        "conditions": [
          {
            "leftValue": "={{ new Date($json.created_at) > new Date(Date.now() - 300000) }}",
            "operator": "equals",
            "rightValue": true
          }
        ]
      }
    },
    {
      "id": "slack",
      "name": "Notify",
      "type": "n8n-nodes-base.slack",
      "position": [850, 300],
      "parameters": {
        "webhookUrl": "https://hooks.slack.com/...",
        "text": "New issue: {{ $json.title }}\n{{ $json.html_url }}"
      }
    }
  ],
  "connections": {
    "Every 5 Minutes": {"main": [[{"node": "Get Issues", "type": "main", "index": 0}]]},
    "Get Issues": {"main": [[{"node": "New Issues", "type": "main", "index": 0}]]},
    "New Issues": {"main": [[{"node": "Notify", "type": "main", "index": 0}]]}
  }
}
```

## Data Transformation Pipeline

Complex data processing:

```json
{
  "name": "Data Pipeline",
  "nodes": [
    {
      "id": "start",
      "name": "Start",
      "type": "n8n-nodes-base.start",
      "position": [250, 300],
      "parameters": {}
    },
    {
      "id": "fetch",
      "name": "Fetch Data",
      "type": "n8n-nodes-base.httpRequest",
      "position": [450, 300],
      "parameters": {
        "url": "https://api.example.com/data",
        "method": "GET"
      }
    },
    {
      "id": "code",
      "name": "Transform",
      "type": "n8n-nodes-base.code",
      "position": [650, 300],
      "parameters": {
        "language": "javascript",
        "code": "return items.map(item => ({\n  json: {\n    id: item.json.id,\n    name: item.json.name.toUpperCase(),\n    processed: true,\n    timestamp: new Date().toISOString()\n  }\n}));"
      }
    },
    {
      "id": "save",
      "name": "Save to DB",
      "type": "n8n-nodes-base.postgres",
      "position": [850, 300],
      "parameters": {
        "connectionUrl": "postgres://...",
        "operation": "executeQuery",
        "query": "INSERT INTO processed (id, name, timestamp) VALUES ('{{ $json.id }}', '{{ $json.name }}', '{{ $json.timestamp }}')"
      }
    }
  ],
  "connections": {
    "Start": {"main": [[{"node": "Fetch Data", "type": "main", "index": 0}]]},
    "Fetch Data": {"main": [[{"node": "Transform", "type": "main", "index": 0}]]},
    "Transform": {"main": [[{"node": "Save to DB", "type": "main", "index": 0}]]}
  }
}
```

## Running Examples

```bash
# Save example to file
cat > example.json << 'EOF'
{...workflow json...}
EOF

# Run the workflow
m9m run example.json
```

---

## Conditional routing with IF

Branch on a boolean expression; downstream nodes pick up only the items that match.

```json
{
  "name": "Amount threshold",
  "nodes": [
    { "id": "s",     "name": "Start",       "type": "n8n-nodes-base.start",     "position": [250, 300], "parameters": {} },
    { "id": "fetch", "name": "Fetch Order", "type": "n8n-nodes-base.httpRequest", "position": [450, 300],
      "parameters": { "url": "={{ $json.orderUrl }}", "method": "GET" } },
    { "id": "if",    "name": "Big order?",  "type": "n8n-nodes-base.if",        "position": [650, 300],
      "parameters": {
        "conditions": [
          { "leftValue": "={{ $json.amount }}", "operator": "greaterThan", "rightValue": 1000 }
        ]
      } },
    { "id": "high",  "name": "Notify Sales","type": "n8n-nodes-base.slack",      "position": [850, 200],
      "parameters": { "channel": "#sales", "text": "Big order: {{ $json.amount }}" } },
    { "id": "low",   "name": "Log Only",    "type": "n8n-nodes-base.set",        "position": [850, 400],
      "parameters": { "assignments": [{ "name": "logged", "value": true }] } }
  ],
  "connections": {
    "Start":         { "main": [[{ "node": "Fetch Order", "type": "main", "index": 0 }]] },
    "Fetch Order":   { "main": [[{ "node": "Big order?",  "type": "main", "index": 0 }]] },
    "Big order?":    { "main": [
      [{ "node": "Notify Sales", "type": "main", "index": 0 }],
      [{ "node": "Log Only",    "type": "main", "index": 0 }]
    ] }
  }
}
```

Operators (`equals`, `notEquals`, `contains`, `greaterThan`, `lessThan`, `startsWith`, `endsWith`, `exists`, `notExists`) are coerced to a common type before comparison. Combine multiple conditions with `combineOperation: "and"` (default) or `"or"`.

Full reference: [IF node](../nodes/transform.md#if-node).

---

## Loop and batch

Iterate one item at a time (with a Wait between iterations if you need to throttle), or process in fixed-size batches.

```json
{
  "name": "Throttled email blast",
  "nodes": [
    { "id": "s",     "name": "Start",       "type": "n8n-nodes-base.start",     "position": [250, 300], "parameters": {} },
    { "id": "list",  "name": "Get Recipients", "type": "n8n-nodes-base.postgres", "position": [450, 300],
      "parameters": { "operation": "executeQuery", "query": "SELECT email FROM recipients WHERE active = true" } },
    { "id": "loop",  "name": "Loop",        "type": "n8n-nodes-base.loop",      "position": [650, 300],
      "parameters": { "loopOver": "items", "batchSize": 1 } },
    { "id": "send",  "name": "Send",        "type": "n8n-nodes-base.sendGrid",   "position": [850, 300],
      "parameters": {
        "apiKey": "={{ $credentials.sendgrid.apiKey }}",
        "from":   { "email": "alerts@example.com" },
        "to":     [{ "email": "={{ $json.email }}" }],
        "subject": "Hello",
        "text":   "Welcome."
      } },
    { "id": "wait",  "name": "Throttle",    "type": "n9m-nodes-base.wait",      "position": [1050, 300],
      "parameters": { "resume": "time", "amount": 1, "unit": "seconds" } }
  ],
  "connections": {
    "Start":          { "main": [[{ "node": "Get Recipients", "type": "main", "index": 0 }]] },
    "Get Recipients": { "main": [[{ "node": "Loop", "type": "main", "index": 0 }]] },
    "Loop":           { "main": [[{ "node": "Send", "type": "main", "index": 0 }]] },
    "Send":           { "main": [[{ "node": "Throttle", "type": "main", "index": 0 }]] },
    "Throttle":       { "main": [[{ "node": "Loop", "type": "main", "index": 0 }]] }
  }
}
```

The self-loop on `Throttle → Loop` (re-connecting to the loop's main[0]) is what makes the iteration continuous; without it the loop runs once.

Alternative: [SplitInBatches](../nodes/transform.md#splitinbatches-node) for batch processing.

---

## Sub-workflow

A workflow invokes another workflow by id, passes inputs, and continues from the sub-workflow's output.

### Parent (`wf-parent`)

```json
{
  "name": "Per-user report",
  "nodes": [
    { "id": "s",     "name": "Start",        "type": "n9m-nodes-base.start",            "position": [250, 300], "parameters": {} },
    { "id": "each",  "name": "For Each User", "type": "n8n-nodes-base.splitInBatches", "position": [450, 300],
      "parameters": { "batchSize": 1 } },
    { "id": "call",  "name": "Render Report", "type": "n8n-nodes-base.executeWorkflow","position": [650, 300],
      "parameters": {
        "workflowId": "wf-child-render",
        "workflowInputs": { "userId": "={{ $json.userId }}", "feature": "report" }
      } },
    { "id": "mail",  "name": "Email",        "type": "n8n-nodes-base.sendGrid",         "position": [850, 300],
      "parameters": {
        "apiKey": "={{ $credentials.sendgrid.apiKey }}",
        "from":   { "email": "reports@example.com" },
        "to":     [{ "email": "={{ $json.email }}" }],
        "subject": "Your report",
        "text":   "={{ $json.reportUrl }}"
      } }
  ],
  "connections": {
    "Start":           { "main": [[{ "node": "For Each User", "type": "main", "index": 0 }]] },
    "For Each User":   { "main": [[{ "node": "Render Report", "type": "main", "index": 0 }], [{ "node": "Render Report", "type": "main", "index": 0 }]] },
    "Render Report":   { "main": [[{ "node": "Email", "type": "main", "index": 0 }]] }
  }
}
```

### Child (`wf-child-render`)

```json
{
  "name": "Render report",
  "nodes": [
    { "id": "t",     "name": "Called From Parent", "type": "n8n-nodes-base.executeWorkflowTrigger", "position": [250, 300], "parameters": {} },
    { "id": "render", "name": "Render", "type": "n8n-nodes-base.code", "position": [450, 300],
      "parameters": {
        "language": "python",
        "code": "user_id = data[0]['json']['userId']\nfeature = data[0]['json']['feature']\n# ...render...\nreturn [{ 'json': { 'reportUrl': f'https://reports.example.com/{user_id}-{feature}.pdf' } }]"
      } }
  ],
  "connections": {
    "Called From Parent": { "main": [[{ "node": "Render", "type": "main", "index": 0 }]] }
  }
}
```

Sub-workflow edges are recorded separately from the parent's; see [execution-edges](execution-edges.md#sub-workflow-edge-preservation) for the trace shape.

---

## Error workflow

A workflow that catches failures from any other workflow and posts to Slack.

```json
{
  "name": "On Failure",
  "nodes": [
    { "id": "t",     "name": "Error Trigger",   "type": "n8n-nodes-base.errorTrigger",  "position": [250, 300], "parameters": {} },
    { "id": "log",   "name": "Format Error",    "type": "n8n-nodes-base.set",           "position": [450, 300],
      "parameters": { "assignments": [
        { "name": "text", "value": "={{ $json.execution.workflowName }} failed at {{ $json.error.node.name }}: {{ $json.error.message }}" }
      ] } },
    { "id": "alert", "name": "Slack Alert",     "type": "n8n-nodes-base.slack",         "position": [650, 300],
      "parameters": { "channel": "#alerts", "text": "={{ $json.text }}" } }
  ],
  "connections": {
    "Error Trigger": { "main": [[{ "node": "Format Error", "type": "main", "index": 0 }]] },
    "Format Error":  { "main": [[{ "node": "Slack Alert",  "type": "main", "index": 0 }]] }
  }
}
```

Set the error workflow for any other workflow via:

```bash
curl -X PATCH http://localhost:8080/api/v1/workflows/{id} \
  -H "Content-Type: application/json" \
  -d '{"settings": {"errorWorkflow": "wf-error-handler-id"}}'
```
