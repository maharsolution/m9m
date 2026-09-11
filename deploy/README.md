# m9m Deployment Guide

Deploy **m9m — the n8n alternative without the bugs** — with one click. Drop-in n8n alternative in Go: 5–10× faster, 70% lower memory, deterministic execution, no Node.js required.

> m9m is built on the shoulders of [n8n](https://n8n.io) — n8n's workflow JSON, expressions, credential envelopes, REST surface, and webhook wire shape are the public contract m9m targets. Originally created by **Dipankar Sarkar** ([historical repo](https://github.com/neul-labs/m9m)); currently maintained by **Mahar Solution** at [`github.com/maharsolution/m9m`](https://github.com/maharsolution/m9m). See the root [README](../README.md#credits-and-acknowledgements) for full credits.

## One-Click Deploy

### Railway
[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/m9m)

```bash
railway login
railway init
railway up
```

### Fly.io
[![Deploy on Fly.io](https://fly.io/button.svg)](https://fly.io/docs/speedrun/)

```bash
fly auth login
fly launch
fly deploy
```

### Render
[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy)

Connect your GitHub repo and Render will automatically deploy.

### DigitalOcean
[![Deploy to DO](https://www.deploytodo.com/do-btn-blue.svg)](https://cloud.digitalocean.com/apps/new)

Use the App Platform with our Dockerfile.

## Docker

```bash
# Pull and run
docker pull ghcr.io/maharsolution/m9m:latest
docker run -d -p 8080:8080 ghcr.io/maharsolution/m9m:latest

# With docker-compose
docker-compose up -d
```

## Kubernetes

```bash
# Apply manifests
kubectl apply -f deploy/kubernetes.yaml

# Check status
kubectl get pods -n m9m
kubectl get svc -n m9m
```

## Helm Chart (Coming Soon)

```bash
helm repo add m9m https://charts.m9m.io
helm install m9m m9m/m9m
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `M9M_PORT` | HTTP server port | `8080` |
| `M9M_HOST` | Bind address | `0.0.0.0` |
| `M9M_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `M9M_DB_TYPE` | Database type (sqlite/postgres/badger) | `sqlite` |
| `M9M_DB_POSTGRES_URL` | PostgreSQL connection URL | - |
| `M9M_QUEUE_TYPE` | Queue type (memory/redis/rabbitmq) | `memory` |
| `M9M_QUEUE_URL` | Queue connection URL | - |
| `M9M_AI_PROVIDER`     | AI provider (openai/anthropic/minimax/ollama)        | - |
| `M9M_AI_API_KEY`      | AI API key                                            | - |
| `M9M_AI_BASE_URL`     | AI base URL (overrides provider default)              | - |
| `M9M_AI_MODEL`        | AI model name                                         | `gpt-4o` |
| `M9M_AI_MAX_TOKENS`   | AI max tokens                                         | `4096` |
| `M9M_AI_TEMPERATURE`  | AI temperature                                        | `0.7` |
| `M9M_AI_TIMEOUT_MS`   | AI request timeout (ms)                               | `60000` |
| `M9M_AI_ENABLED`      | Master switch for the in-app AI assistant             | `false` |

> The env-var path is preserved for headless deployments; the recommended
> path is now **Settings → AI** in the web UI, which writes to the same
> DB store and takes effect live (no restart). The legacy
> `M9M_COPILOT_*` variables are still honoured as deprecated aliases
> for backward compatibility.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Load Balancer                         │
└─────────────────────────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │ Control  │    │ Control  │    │ Control  │
    │  Plane   │    │  Plane   │    │  Plane   │
    └──────────┘    └──────────┘    └──────────┘
          │               │               │
          └───────────────┼───────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │  Worker  │    │  Worker  │    │  Worker  │
    └──────────┘    └──────────┘    └──────────┘
          │               │               │
          └───────────────┼───────────────┘
                          │
                    ┌─────┴─────┐
                    │ PostgreSQL│
                    │   Redis   │
                    └───────────┘
```

## Production Checklist

- [ ] Use PostgreSQL for storage
- [ ] Enable Redis for queue/caching
- [ ] Configure SSL/TLS
- [ ] Set up monitoring (Prometheus/Grafana)
- [ ] Enable audit logging
- [ ] Configure backups
- [ ] Set resource limits
- [ ] Enable horizontal scaling

## Support

- Documentation: https://github.com/maharsolution/m9m/tree/main/docs
- GitHub Issues: https://github.com/maharsolution/m9m/issues
- Discord: https://discord.gg/m9m
