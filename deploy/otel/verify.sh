#!/usr/bin/env bash
# verify.sh — vendor-agnostic smoke test for m9m OTLP tracing.
#
# Usage:
#   ./deploy/otel/verify.sh                         # uses defaults
#   M9M_ENDPOINT=otel.example.com:4318 ./verify.sh  # override endpoint
#
# What it does:
#   1. Boots an m9m serve with the smoke-test env on this host.
#   2. Waits for the API to come up.
#   3. Sends a no-op test span via POST /api/v1/otel/test.
#   4. Sends a webhook that triggers a workflow (so workflow.execute /
#      node.execute chains are exercised too).
#   5. Prints a vendor-neutral checklist the operator can tick off in
#      whichever collector UI they happen to use.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

M9M_HOST="${M9M_HOST:-127.0.0.1}"
M9M_PORT="${M9M_PORT:-8080}"
M9M_ENDPOINT="${M9M_ENDPOINT:-127.0.0.1:4317}"
M9M_PROTOCOL="${M9M_PROTOCOL:-grpc}"
M9M_SERVICE_NAME="${M9M_SERVICE_NAME:-m9m}"

echo "▶ Verifying m9m OTel pipeline (endpoint=$M9M_ENDPOINT, protocol=$M9M_PROTOCOL)"
echo

# --- 1. Source the env file (the otel/ values override only if the operator
# has not set them; we force-set the ones we care about so the test stays
# deterministic).
set -a
# shellcheck disable=SC1091
source "$SCRIPT_DIR/smoke-test.env"
set +a

export M9M_OTEL_ENABLED=true
export M9M_OTEL_PROTOCOL="$M9M_PROTOCOL"
export M9M_OTEL_ENDPOINT="$M9M_ENDPOINT"
export M9M_OTEL_SERVICE_NAME="$M9M_SERVICE_NAME"

# --- 2. Boot m9m in the background.
pushd "$REPO_ROOT" >/dev/null
./m9m serve --host "$M9M_HOST" --port "$M9M_PORT" > /tmp/m9m-otel-verify.log 2>&1 &
PID=$!
trap 'kill -9 $PID 2>/dev/null || true' EXIT
popd >/dev/null

# --- 3. Wait for the API to come up.
echo "⏳ Waiting for m9m to listen on $M9M_HOST:$M9M_PORT"
for i in {1..30}; do
  if curl -fsS "http://$M9M_HOST:$M9M_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! curl -fsS "http://$M9M_HOST:$M9M_PORT/healthz" >/dev/null 2>&1; then
  echo "❌ m9m failed to come up. Tail of /tmp/m9m-otel-verify.log:"
  tail -n 30 /tmp/m9m-otel-verify.log
  exit 1
fi

# --- 4. Confirm the boot line includes the OTel config.
if grep -q "OpenTelemetry initialised" /tmp/m9m-otel-verify.log; then
  echo "✅ Boot line confirms OTEL pipeline is up."
else
  echo "⚠️  Boot didn't print 'OpenTelemetry initialised'. Tail of log:"
  tail -n 15 /tmp/m9m-otel-verify.log
fi
echo

# --- 5. POST /api/v1/otel/test to send a no-op span.
echo "▶ POST /api/v1/otel/test (no-op smoke span)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "http://$M9M_HOST:$M9M_PORT/api/v1/otel/test")
if [ "$HTTP_CODE" = "204" ]; then
  echo "✅ Smoke span dispatched."
else
  echo "⚠️  Smoke span endpoint returned HTTP $HTTP_CODE (expected 204)."
fi
echo

# --- 6. Send a webhook that triggers a workflow if any are registered.
echo "▶ POST /webhook/anything (workflow.execute path)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "http://$M9M_HOST:$M9M_PORT/webhook/anything" \
  -H 'content-type: application/json' \
  -d '{"hello":"otel"}' 2>/dev/null || true)
echo "   returned HTTP $HTTP_CODE (404 is fine — we just want the handler to run.)"
echo

# --- 7. Vendor-neutral checklist.
cat <<EOF
✅ Done. Look for the following in your backend UI:

  - service.name = $M9M_SERVICE_NAME
  - span name    = "m9m.otel.test"         (one of these, from /otel/test)
  - span name    = "workflow.execute"      (from the webhook run)
    └── child    = "node.execute"          (per-node, if M9M_OTEL_INCLUDE_NODE_SPANS=true)

Spans typically appear within 5-15s of being exported (depends on your
collector's batch interval).

If the spans don't appear:
  - Confirm the collector UI accepts the service name. Most collectors
    discover new services automatically; some require explicit whitelist.
  - Check $M9M_PROTOCOL://$M9M_ENDPOINT from this host: \`nc -vz ${M9M_ENDPOINT%:*} ${M9M_ENDPOINT##*:}\`
  - Re-run with OTEL_EXPORTER_OTLP_DEBUG=true (the OTEL SDK respects this)
    to see raw request-level errors on stderr.

Cleanup: the script traps SIGINT/SIGTERM and kills m9m. Just Ctrl+C.
EOF
