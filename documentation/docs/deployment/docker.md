---
title: "Docker Deployment"
description: "Deploy m9m using Docker."
keywords: "m9m deployment, Docker, Kubernetes, production workflow automation"
---

# Docker Deployment

Deploy m9m using Docker.

## Quick Start

```bash
docker run -d \
  --name m9m \
  -p 8080:8080 \
  maharsolution/m9m:latest
```

## Docker Images

| Image | Description |
|-------|-------------|
| `maharsolution/m9m:latest` | Latest stable release |
| `maharsolution/m9m:1.0.0` | Specific version |
| `maharsolution/m9m:alpine` | Minimal Alpine-based |

## Basic Configuration

### With Environment Variables

```bash
docker run -d \
  --name m9m \
  -p 8080:8080 \
  -e M9M_LOG_LEVEL=info \
  -e M9M_JWT_SECRET=your-secret \
  maharsolution/m9m:latest
```

### With Persistent Storage

```bash
docker run -d \
  --name m9m \
  -p 8080:8080 \
  -v m9m-data:/data \
  maharsolution/m9m:latest
```

### With Config File

```bash
docker run -d \
  --name m9m \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/etc/m9m/config.yaml \
  maharsolution/m9m:latest
```

## Docker Compose

### Basic Setup

```yaml
# docker-compose.yml
version: '3.8'

services:
  m9m:
    image: maharsolution/m9m:latest
    ports:
      - "8080:8080"
    volumes:
      - m9m-data:/data
    environment:
      - M9M_LOG_LEVEL=info

volumes:
  m9m-data:
```

Start:

```bash
docker compose up -d
```

### With PostgreSQL

```yaml
version: '3.8'

services:
  m9m:
    image: maharsolution/m9m:latest
    ports:
      - "8080:8080"
    environment:
      - M9M_DATABASE_TYPE=postgres
      - M9M_DATABASE_URL=postgres://m9m:password@postgres:5432/m9m
      - M9M_JWT_SECRET=${JWT_SECRET}
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=m9m
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=m9m
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U m9m"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres-data:
```

### With Redis Queue

```yaml
version: '3.8'

services:
  m9m:
    image: maharsolution/m9m:latest
    ports:
      - "8080:8080"
    environment:
      - M9M_DATABASE_TYPE=postgres
      - M9M_DATABASE_URL=postgres://m9m:password@postgres:5432/m9m
      - M9M_QUEUE_TYPE=redis
      - M9M_QUEUE_URL=redis://redis:6379
      - M9M_WORKERS=5
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=m9m
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=m9m
    volumes:
      - postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data

volumes:
  postgres-data:
  redis-data:
```

### Full Production Stack

```yaml
version: '3.8'

services:
  m9m:
    image: maharsolution/m9m:latest
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M
    ports:
      - "8080:8080"
    environment:
      - M9M_DATABASE_TYPE=postgres
      - M9M_DATABASE_URL=postgres://m9m:${DB_PASSWORD}@postgres:5432/m9m
      - M9M_QUEUE_TYPE=redis
      - M9M_QUEUE_URL=redis://redis:6379
      - M9M_JWT_SECRET=${JWT_SECRET}
      - M9M_LOG_FORMAT=json
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=m9m
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=m9m
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U m9m"]
      interval: 10s

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis-data:/data

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    depends_on:
      - m9m

volumes:
  postgres-data:
  redis-data:
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_PORT` | 8080 | Server port |
| `M9M_HOST` | 0.0.0.0 | Listen address |
| `M9M_DATABASE_TYPE` | sqlite | Database type |
| `M9M_DATABASE_URL` | /data/m9m.db | Connection string |
| `M9M_QUEUE_TYPE` | memory | Queue backend |
| `M9M_QUEUE_URL` | - | Queue connection |
| `M9M_WORKERS` | 3 | Worker count |
| `M9M_JWT_SECRET` | - | JWT signing key |
| `M9M_LOG_LEVEL` | info | Log verbosity |
| `M9M_LOG_FORMAT` | text | Log format |

## Volume Mounts

| Path | Purpose |
|------|---------|
| `/data` | SQLite database, file storage |
| `/etc/m9m/config.yaml` | Configuration file |
| `/var/log/m9m` | Log files |

## Networking

### Internal Network

```yaml
services:
  m9m:
    networks:
      - internal
      - frontend

networks:
  internal:
    internal: true
  frontend:
```

### Expose Specific Ports

```yaml
services:
  m9m:
    ports:
      - "127.0.0.1:8080:8080"  # Local only
```

## Resource Limits

```yaml
services:
  m9m:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
```

## Health Checks

```yaml
services:
  m9m:
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

## Logging

### JSON Logging

```yaml
services:
  m9m:
    environment:
      - M9M_LOG_FORMAT=json
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

### External Logging

```yaml
services:
  m9m:
    logging:
      driver: syslog
      options:
        syslog-address: "tcp://logserver:514"
```

## Building Custom Image

### Dockerfile

```dockerfile
FROM maharsolution/m9m:latest

# Add custom config
COPY config.yaml /etc/m9m/config.yaml

# Add custom scripts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
```

Build:

```bash
docker build -t my-m9m:latest .
```

## Troubleshooting

### View Logs

```bash
docker logs m9m
docker logs -f m9m  # Follow
```

### Shell Access

```bash
docker exec -it m9m /bin/sh
```

### Check Health

```bash
docker inspect --format='{{.State.Health.Status}}' m9m
```

### Restart Container

```bash
docker restart m9m
```

## Upgrading

### Pull New Image

```bash
docker pull maharsolution/m9m:latest
```

### Upgrade with Docker Compose

```bash
docker compose pull
docker compose up -d
```

### Backup Before Upgrade

```bash
# Backup volume
docker run --rm -v m9m-data:/data -v $(pwd):/backup alpine tar czf /backup/m9m-backup.tar.gz /data

# Backup database
docker exec m9m m9m backup /data/backup.sql
```

## Image registry (Harbor + GitHub Actions) + auto-deploy

Starting with the 2026-09 cycle, the m9m Docker image is **built by a GitHub
Action and pushed to the project's Harbor registry**, instead of being
built in-place by `/root/bin/build` on the deploy server. The end-to-end
shape is:

```
git push origin main
     │
     ▼
┌─────────────────────────────────────────────┐
│ .github/workflows/harbor-image.yml          │
│   (ubuntu-latest, buildx, single-arch)      │
│                                             │
│   docker buildx build                       │
│     --platform linux/amd64                  │
│     --tag harbor.solutiontech.id/           │
│          m9m/m9m-core:latest                │
│     --push                                  │
└─────────────────────────────────────────────┘
     │
     ▼  (needs: build-and-push)
┌─────────────────────────────────────────────┐
│ deploy job (same workflow, ssh-action)      │
│                                             │
│   ssh root@<server> -p 2212                 │
│     bash /root/bin/pull-compose.sh          │
│       # docker compose pull m9m-backend     │
│       # docker compose up -d m9m-backend    │
└─────────────────────────────────────────────┘
     │
     ▼
m9m-backend running the new image
```

### Tag strategy — `latest` only

Every push to `main` (or every manual **Run workflow**) overwrites the
single `harbor.solutiontech.id/m9m/m9m-core:latest` tag. There is no
`main-<sha>`, no version matrix, no per-commit reference. This keeps
the registry catalog minimal and the deploy server's
`docker-compose.yaml` declarative (it already pins `:latest`).

**Why not per-commit tags?** GitHub-hosted runners preserve the build
context (including the GHA cache) for ~7 days. To roll back to a
specific commit you can re-checkout that commit and push it (which
re-runs the same workflow) — that is simpler than maintaining a tag
catalog nobody queries.

**Why only `linux/amd64`?** The production deploy target
(`187.77.113.218`) is amd64. Dropping arm64 removes the qemu-emulated
cross-build of `go build` (which alone takes ~20 minutes on GH-hosted
runners) and cuts the end-to-end job time in half. Re-introduce
multi-arch by changing `platforms:` in the workflow back to
`linux/amd64,linux/arm64` and adding back the `docker/setup-qemu-action`
step.

### Required secrets

Configure these once under
`Settings → Secrets and variables → Actions` on the GitHub side:

| Secret | Example | Purpose |
|---|---|---|
| `REGISTRY_USERNAME` | `m9m` | Harbor robot account or personal username |
| `REGISTRY_PASSWORD` | `Abcd.12345!` | Matching password |
| `SSH_HOST` | `187.77.113.218` | m9m-server host (port 22 is firewalled) |
| `SSH_PORT` | `2212` | SSH port (port 22 is firewalled on the deploy box) |
| `SSH_USER` | `mhr` | SSH user (was `root` before the move to `/home/mhr/` on 2026-09-11) |
| `SSH_PRIVATE_KEY` | (full PEM content) | Private half of a key whose public half is in `~/.ssh/authorized_keys` on the server. Currently the operator's existing `id_rsa`. |

### Automatic deploy after build

`harbor-image.yml` defines **two jobs** chained by `needs:`:

```
push to main ──► build-and-push ──► deploy ──► server: pull + restart
                (linux/amd64)   (ssh via      (m9m-backend)
                               appleboy/ssh)
```

The `deploy` job uses the `appleboy/ssh-action@v1` Marketplace action to
SSH into the deploy server (port 2212) and execute:

```bash
bash /root/bin/pull-compose.sh
```

The host key for `[187.77.113.218]:2212` is pinned in the workflow so a
man-in-the-middle cannot silently redirect the SSH session.

#### Skipping the auto-deploy

If you push a commit that should not trigger a redeploy (e.g. a
doc-only change), include `[no-deploy]` anywhere in the commit message:

```
docs(readme): refresh install instructions [no-deploy]
```

The `deploy` job's `if:` condition short-circuits and the server is
left untouched. The build, on the other hand, still runs so the
Harbor `:latest` is refreshed even if you decide the server should not
restart yet.

#### Troubleshooting the SSH step

| Symptom | Likely cause | Fix |
|---|---|---|
| `Permission denied (publickey)` | `SSH_PRIVATE_KEY` not set, or doesn't match an entry in `~/.ssh/authorized_keys` on the server | Compare the public half of the secret against `ssh -p 2212 root@$SSH_HOST 'cat ~/.ssh/authorized_keys'` |
| `Host key verification failed` | The `fingerprint` pin in the workflow is outdated (server was rebuilt) | Re-read the new fingerprint with `ssh-keyscan -p 2212 $SSH_HOST \| ssh-keygen -lf -` (OpenSSH base64 form, no `SHA256:` prefix) and paste it into the workflow |
| `bash: /root/bin/pull-compose.sh: Permission denied` or `No such file or directory` | Script lives at `/home/mhr/bin/pull-compose.sh` since the operator move on 2026-09-11 | Update the `script:` body in the workflow to `bash /home/mhr/bin/pull-compose.sh` |

### Switching the bundled docker-compose.yml to the registry

The repo's `docker-compose.yml` pins `image: ${REGISTRY_IMAGE_URL:-m9m}`.
Set `REGISTRY_IMAGE_URL` in your environment file:

```bash
# .env (operator-local, never committed)
REGISTRY_IMAGE_URL=https://harbor.solutiontech.id/m9m/m9m-core:latest
REGISTRY_URL=https://harbor.solutiontech.id
REGISTRY_USER=m9m
REGISTRY_PASSWORD=...        # mirrors the GitHub Secret
```

Then start the stack as usual:

```bash
docker compose pull m9m-backend
docker compose up -d
```

`pull_policy: always` ensures the compose stack fetches the latest tag
on every `up`, so a new push to `main` is picked up the next time
someone (or a Harbor webhook) runs `docker compose up -d`.

### Local development without Harbor

If you want to test a code change before pushing, fall back to a local
build:

```bash
unset REGISTRY_IMAGE_URL          # falls back to `image: m9m`
docker build -t m9m .
docker compose up -d
```

The bundled Dockerfile lives at the repo root and is identical to what
the GitHub Action builds; only the tag and registry differ.

### Replacing the legacy `/root/bin/build` pipeline

Before this workflow landed, deploys ran through `/root/bin/build` on
the server itself. The replacement script on the server is now roughly:

```bash
#!/usr/bin/env bash
# /root/bin/pull-compose.sh — runs on the deploy server
set -euo pipefail
cd /root/gabungan
docker compose pull m9m-backend
docker compose up -d m9m-backend
```

Pair it with a Harbor webhook (project → Webhooks → Push notification)
that hits this script over SSH, or run it manually after each push. The
old `docker build --no-cache` step is now redundant because the GitHub
Action always builds from a clean Buildx cache.
