#!/usr/bin/env python3
"""Live byte-equal verification for n8n vs m9m webhook responses."""
import json
import subprocess
import sys

URLS = [
    ("simple_webhook", 5678, 8080, {"x": 1}),
    ("bocahtuanakal", 5678, 8080, {"test": "test123"}),
]


def fetch(port: str, path: str, payload: dict) -> bytes:
    body = json.dumps(payload)
    cmd = [
        "curl.exe", "-sS", "-X", "POST",
        f"http://187.77.113.218:{port}/webhook/{path}",
        "-H", "Content-Type: application/json",
        "-d", body,
    ]
    return subprocess.check_output(cmd)


def main() -> int:
    rc = 0
    for path, n8n_port, m9m_port, payload in URLS:
        n8n = fetch(n8n_port, path, payload)
        m9m = fetch(m9m_port, path, payload)

        print(f"\n=== /webhook/{path} payload={payload} ===")
        print(f"n8n ({len(n8n)} bytes): {n8n.decode()}")
        print(f"m9m ({len(m9m)} bytes): {m9m.decode()}")

        try:
            n8n_j = json.loads(n8n)
            m9m_j = json.loads(m9m)
            n8n_pretty = json.dumps(n8n_j, indent=2, sort_keys=True)
            m9m_pretty = json.dumps(m9m_j, indent=2, sort_keys=True)
        except Exception as exc:
            print(f"  ! JSON parse error: {exc}")
            rc = 2
            continue

        if n8n_pretty == m9m_pretty:
            print("  -> byte-equal: YES (semantic)")
        else:
            print("  -> byte-equal: NO (semantic diff)")
            # Print field-by-field diff for headers.
            if isinstance(n8n_j, list) and isinstance(m9m_j, list):
                if n8n_j and m9m_j:
                    if isinstance(n8n_j[0], dict) and isinstance(m9m_j[0], dict):
                        nh = n8n_j[0].get("headers", {})
                        mh = m9m_j[0].get("headers", {})
                        all_keys = sorted(set(nh) | set(mh))
                        for k in all_keys:
                            if nh.get(k) != mh.get(k):
                                print(f"    header[{k}]: n8n={nh.get(k)!r} m9m={mh.get(k)!r}")
            rc = 1

    return rc


if __name__ == "__main__":
    sys.exit(main())
