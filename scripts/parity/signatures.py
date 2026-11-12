"""Request body fingerprints used to pair sheet rows with harness cases.

The parity sheet stores a curl command per row. Two rows can share the
same webhook path and still be different cases (the MySQL and the
PostgreSQL branch of ``db_quiry`` for example). The sheet's own "no"
column is a 1..N row number, not a harness case id, so it cannot be
used for pairing.

The fingerprint is a SHA-256 over the canonical JSON form of the
request body: key order does not matter, but a different payload
produces a different digest. Only the digest is stored - request bodies
are never written to a report or to the sheet.
"""
from __future__ import annotations

import hashlib
import json
import re
from typing import Any

_CURL_DATA = re.compile(
    r"(?:--data(?:-binary|-raw)?|-d)\s+(?:'([^']*)'|\"([^\"]*)\"|([^\s]+))")


def canonical_body_text(body: Any) -> str:
    """Canonical text form of a request body."""
    if body in (None, ""):
        return ""
    if isinstance(body, (dict, list)):
        return json.dumps(body, sort_keys=True, separators=(",", ":"))
    text = str(body)
    try:
        parsed = json.loads(text)
    except (json.JSONDecodeError, TypeError):
        return text.strip()
    return json.dumps(parsed, sort_keys=True, separators=(",", ":"))


def body_signature(body: Any) -> str:
    """SHA-256 (truncated) of the canonical body text."""
    text = canonical_body_text(body)
    if not text:
        return ""
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:32]


def case_signature(case: dict[str, Any]) -> str:
    """Body fingerprint of a harness case."""
    return body_signature(case.get("body"))


def curl_body_signature(request: str) -> str:
    """Body fingerprint of the payload carried by a curl command.

    Returns an empty string when the command has no inline payload.
    """
    if not request:
        return ""
    match = _CURL_DATA.search(request)
    if not match:
        return ""
    payload = next((group for group in match.groups() if group is not None), "")
    return body_signature(payload)
