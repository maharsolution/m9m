#!/usr/bin/env bash
# Queue soak harness for BUG-ARCH-2026-10-12-001.
#
# Runs N concurrent workflows against the configured queue
# for `DURATION` seconds, then writes a verdict to stdout
# and a JSON report to `OUT`. Used as the entry criterion
# for the 0.5.0 queue-mode GA milestone.
#
# Required env:
#   M9M_URL        — base URL of the m9m backend (e.g. http://localhost:8080)
#   M9M_API_KEY    — API key with workflow:execute permission
#   WORKFLOW_ID    — workflow to fan out (must be a noop or webhook-shaped
#                    so the test isolates queue behaviour from workflow logic)
# Optional env:
#   DURATION       — seconds to run (default 60)
#   CONCURRENCY    — simultaneous in-flight workflows (default 100)
#   OUT            — output JSON path (default ./queue-soak-report.json)
#
# Usage:
#   M9M_URL=http://localhost:8080 \
#   M9M_API_KEY=m9m-*** \
#   WORKFLOW_ID=w-1 \
#   DURATION=120 CONCURRENCY=200 \
#   ./scripts/queue-soak.sh

set -euo pipefail

: "${M9M_URL:?M9M_URL is required}"
: "${M9M_API_KEY:?M9M_API_KEY is required}"
: "${WORKFLOW_ID:?WORKFLOW_ID is required}"
DURATION="${DURATION:-60}"
CONCURRENCY="${CONCURRENCY:-100}"
OUT="${OUT:-./queue-soak-report.json}"

echo "queue-soak: url=$M9M_URL workflow=$WORKFLOW_ID duration=${DURATION}s concurrency=$CONCURRENCY"
echo "queue-soak: report -> $OUT"

START_TS=$(date +%s)
END_TS=$((START_TS + DURATION))

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Fire-and-track. Each `curl` is a workflow trigger; we
# capture the execution id and final status. The python
# verifier is the source of truth on pass/fail.
python3 - "$M9M_URL" "$M9M_API_KEY" "$WORKFLOW_ID" "$DURATION" "$CONCURRENCY" "$OUT" <<'PY'
import json
import os
import sys
import time
import urllib.request
import urllib.parse
import threading
import queue
from collections import Counter

base, api_key, wf_id, duration, concurrency, out_path = sys.argv[1:7]
duration = int(duration)
concurrency = int(concurrency)

end = time.time() + duration
results = queue.Queue()
lock = threading.Lock()
total = 0
errors = Counter()

def fire():
    global total
    while time.time() < end:
        url = f"{base}/api/v1/workflows/{wf_id}/execute?mode=webhook"
        req = urllib.request.Request(
            url,
            data=b'{}',
            headers={
                'Content-Type': 'application/json',
                'X-N8N-API-KEY': api_key,
            },
            method='POST',
        )
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                body = json.loads(r.read())
                results.put(('ok', body.get('id', '')))
        except Exception as e:
            results.put(('err', str(e)))
        with lock:
            total += 1

threads = [threading.Thread(target=fire, daemon=True) for _ in range(concurrency)]
for t in threads:
    t.start()

# Wait for the duration.
time.sleep(duration)

# Drain the result queue; gather counts.
ok = 0
errs = Counter()
exec_ids = []
while not results.empty():
    try:
        kind, val = results.get_nowait()
    except queue.Empty:
        break
    if kind == 'ok':
        ok += 1
        exec_ids.append(val)
    else:
        errs[val[:80]] += 1

# Poll each execution for its final status.
final = {'success': 0, 'failed': 0, 'running': 0, 'unknown': 0}
for eid in exec_ids:
    if not eid:
        final['unknown'] += 1
        continue
    for _ in range(30):
        try:
            with urllib.request.urlopen(f"{base}/api/v1/executions/{eid}", timeout=5) as r:
                body = json.loads(r.read())
            status = body.get('status', '')
            if status in ('success', 'failed', 'running'):
                final[status] += 1
                break
        except Exception:
            time.sleep(0.5)
    else:
        final['unknown'] += 1

report = {
    'started_at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime(end - duration)),
    'duration_s': duration,
    'concurrency': concurrency,
    'total_triggered': total,
    'http_ok': ok,
    'http_errors': dict(errs),
    'final_status': final,
    'verdict': 'PASS' if (final['failed'] + final['unknown'] == 0 and errs.get('500', 0) == 0) else 'FAIL',
}

with open(out_path, 'w') as f:
    json.dump(report, f, indent=2)

print(json.dumps(report, indent=2))
sys.exit(0 if report['verdict'] == 'PASS' else 1)
PY
