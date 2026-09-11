# syntax=docker/dockerfile:1

# Stage 1 — build the Vue frontend. web/dist is .gitignored, so it MUST
# be produced here, otherwise `//go:embed all:dist/*` in the Go binary
# captures an empty directory and the running container serves a
# stale bundle.
FROM node:20-alpine AS web-builder

WORKDIR /web

# Copy package files first so the npm install layer caches across
# source-only edits.
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund

# Now copy the rest of the source and build. Defensively wipe any
# pre-existing dist/ first — .dockerignore excludes web/dist from the
# build context but we belt-and-braces it here so a stale dist can
# never contaminate the freshly-built output.
COPY web/ ./
RUN rm -rf dist && npm run build

# Stage 2 — build the Go binary. The dist directory produced above is
# the one //go:embed captures.
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Bring in the freshly-built frontend dist. CRITICAL: this must land in
# internal/web/dist because that's what //go:embed dist/* in
# internal/web/handler.go captures at compile time. Putting it in
# web/dist/ instead leaves Go compiling with whatever stale files
# happen to live in internal/web/dist (e.g. files tracked in git from
# an old local build) — the resulting binary then serves those old
# bundles regardless of how fresh web/dist/ is.
COPY --from=web-builder /web/dist ./internal/web/dist

# Build the application
# CGO is needed for sqlite3
#
# COMMIT / BUILD_DATE are passed as build args (typically by /root/bin/build)
# so the resulting binary exposes its exact source identity on
# /api/v1/version. Without them, the binary defaults to "unknown" and the
# UI footer cannot tell whether the running container is fresh or stale.
ARG COMMIT="unknown"
ARG BUILD_DATE="unknown"
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s -extldflags '-static' \
              -X main.Commit=${COMMIT} \
              -X main.BuildDate=${BUILD_DATE}" \
    -o m9m ./cmd/m9m

# Runtime stage
FROM alpine:latest

# Install runtime dependencies.
# - ca-certificates / tzdata: needed by m9m + python https calls.
# - supervisor: runs both `m9m serve` and the sync uvicorn app in one PID-1.
# - python3 / py3-pip: powers the sync bridge. We use --break-system-packages
#   when installing pip deps because Alpine's PEP 668 enforcement would
#   otherwise reject the system-wide install.
RUN apk --no-cache add ca-certificates tzdata supervisor python3 py3-pip

# Defensive symlinks: Alpine installs python3 at /usr/bin/python3, but older
# supervisord configs (or scripts that hardcode /usr/local/bin/python3)
# expect the binary there too. Symlink rather than copy so the two stay in
# lock-step across apk version bumps.
RUN ln -sf /usr/bin/python3 /usr/local/bin/python3 && \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip3 2>/dev/null || true

# Create non-root user for m9m itself. supervisor still runs as root (see below).
RUN addgroup -g 1000 n8n && \
    adduser -D -u 1000 -G n8n n8n

WORKDIR /app

# Copy the m9m binary from the builder.
COPY --from=builder /build/m9m /usr/local/bin/m9m
RUN chmod +x /usr/local/bin/m9m

# Install the sync bridge's Python deps BEFORE copying its source so the
# pip install layer is cached across source-only edits.
COPY sync-service/requirements.txt /app/sync-service/requirements.txt
RUN pip install --no-cache-dir --break-system-packages \
    -r /app/sync-service/requirements.txt

# Copy the sync bridge source.
COPY sync-service/ /app/sync-service/
RUN cp /app/sync-service/sync.py /usr/local/bin/sync.py && \
    chmod +x /usr/local/bin/sync.py

# Drop in the supervisor config. This file defines the two programs that
# run in the container (see supervisord.conf for the full content).
COPY supervisord.conf /etc/supervisord.conf

# Create directories for data persistence, logs, and the supervisor pidfile.
# /app/logs is what supervisord writes stdout/stderr to for both programs.
# /app/run holds supervisord.pid so it isn't living in /tmp.
RUN mkdir -p /app/data /app/logs /app/config /app/run && \
    chown -R n8n:n8n /app/data /app/logs /app/config /app/run

# We intentionally stay as root: supervisord needs to fork both child
# processes (m9m as the n8n user would be ideal, but uvicorn running
# as n8n while supervisor stays as root is the simplest correct setup).
# The previous Dockerfile had a USER root -> USER n8n ordering quirk
# that left the binary owned wrong; copying straight to /usr/local/bin
# above sidesteps that.
USER root

# Expose ports:
#   8080: m9m HTTP server (main)
#   9090: m9m metrics
#   8001: sync bridge FastAPI (uvicorn)
EXPOSE 8080 9090 8001

# Health check probes m9m's own /health endpoint.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Environment variables with defaults
ENV N8N_GO_PORT=8080 \
    N8N_GO_HOST=0.0.0.0 \
    N8N_GO_LOG_LEVEL=info \
    N8N_GO_METRICS_PORT=9090 \
    N8N_GO_DATA_DIR=/app/data \
    N8N_GO_LOG_DIR=/app/logs \
    GOPROXY="https://proxy.golang.org,direct" \
    SYNC_PORT=8001

# Volume for persistent data
VOLUME ["/app/data", "/app/logs", "/app/config"]

# Run both processes under supervisor.
ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
