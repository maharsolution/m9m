"""Load repo .env credentials into os.environ without ever printing values.

Only the keys the parity harness needs are imported. Real process
environment always wins, so an operator can override the file without
editing it. Nothing here logs, returns or serialises a secret value:
callers get a boolean "is it present" answer only.
"""
from __future__ import annotations

import os
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
ENV_FILE = ROOT / ".env"

# Keys the harness may read from .env. Anything else in the file (DB
# passwords, registry credentials, encryption keys) is deliberately NOT
# imported — the harness has no use for them and importing them would
# widen the blast radius of a crash dump or a traceback.
ALLOWED_KEYS = frozenset({
    "N8N_API_KEY",
    "M9M_API_KEY",
    "N8N_BASE_URL",
    "M9M_BASE_URL",
})


def _parse(line: str) -> tuple[str, str] | None:
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        return None
    name, _, value = line.partition("=")
    name = name.strip()
    if name.startswith("export "):
        name = name[len("export "):].strip()
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        value = value[1:-1]
    return name, value


def load_env(path: Path | None = None) -> dict[str, bool]:
    """Import ``ALLOWED_KEYS`` from .env into ``os.environ``.

    Returns a presence map (``{"N8N_API_KEY": True}``) — never the
    values themselves. Existing environment variables are not
    overwritten.
    """
    env_path = path or ENV_FILE
    present: dict[str, bool] = {}
    if env_path.is_file():
        for raw in env_path.read_text(encoding="utf-8-sig").splitlines():
            parsed = _parse(raw)
            if not parsed:
                continue
            name, value = parsed
            if name not in ALLOWED_KEYS or not value:
                continue
            os.environ.setdefault(name, value)
    for key in sorted(ALLOWED_KEYS):
        present[key] = bool(os.environ.get(key))
    return present


def describe(present: dict[str, bool]) -> str:
    """One-line, secret-free summary of which credentials are available."""
    return ", ".join(
        f"{key}={'set' if ok else 'absent'}" for key, ok in sorted(present.items()))


# Header names each runtime expects. The n8n public API is protected by
# ``X-N8N-API-KEY``; m9m's own API middleware reads ``X-API-Key``.
N8N_HEADER = "X-N8N-API-KEY"
M9M_HEADER = "X-API-Key"


def api_headers(host: str) -> dict[str, str]:
    """Return the auth headers for one host.

    The n8n API key is NEVER sent to m9m and the m9m API key is never
    sent to n8n. m9m only receives an API key when ``M9M_API_KEY`` is
    actually configured; the shared deployment requires one, because the
    m9m API answers 401 for every ``/api/v1`` path without a credential
    (see ``scripts/parity/authprobe.py`` for the live evidence).
    """
    if host == "n8n":
        key = os.environ.get("N8N_API_KEY")
        return {N8N_HEADER: key} if key else {}
    if host == "m9m":
        key = os.environ.get("M9M_API_KEY")
        return {M9M_HEADER: key} if key else {}
    raise ValueError(f"unknown host: {host!r}")
