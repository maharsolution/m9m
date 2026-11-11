#!/usr/bin/env python3
"""Comparison primitives for the m9m <-> n8n parity harness.

This module is deliberately dependency-free (stdlib only) so it can run
on the operator's Windows box, inside the dev container, or on the
deploy host without a virtualenv.

Three comparison strategies are needed to decide whether an m9m
response matches the n8n response for the same request:

  * ``semantic_json_equal``  — JSON bodies compared *after* removing the
    fields that legitimately differ between the two runtimes (the
    listener host, the webhook URL, parser-specific error hints) and
    after tolerating one-day timezone drift on date-only fields.

  * ``xml_round_trip_equivalent`` — XML bodies compared as element
    trees. n8n's ``jsonToxml`` collapses attributes into child
    elements while m9m preserves them as attributes, and the two
    serialisers differ in whitespace and in the XML declaration. Both
    shapes describe the same logical tree, so the comparator folds
    attributes into virtual children before diffing.

  * ``headers_equivalent`` — response headers compared on their base
    media type (``application/xml; charset=utf-8`` and ``application/xml``
    are the same media type; only the charset differs).

Nothing in this module may be used to *force* a response into shape —
the hard constraint in ``workflow-test.MD`` Section G says a parity gap
is a bug in ``internal/nodes/...`` and must be fixed there, never
papered over in the harness.
"""
from __future__ import annotations

import datetime as _dt
import json
import re
import xml.etree.ElementTree as ET
from typing import Any

# Fields that legitimately differ between n8n and m9m because they are
# bound to the listener or to operator-configured environment values.
# Comparing them would report a false regression on every run.
#
# Only dotted paths are listed on purpose: a bare key such as ``id``
# would strip meaningful payload fields (``{"id": 1, "name": "Andi"}``
# in the loop workflow) and hide real regressions.
ENV_NOISE_FIELDS = frozenset({
    "headers.host",
    "webhookUrl",
})

# Error hints produced by the two JSON parsers for the same malformed
# payload. The V8 parser says "Unexpected token 'x' …", Go's
# encoding/json says "invalid character 'x' …". Both mean the same
# thing, so a mismatch between them is not a parity regression.
_V8_HINT = re.compile(
    r"Unexpected token|Unexpected end of JSON input|Expected property name|"
    r"is not valid JSON")
_GO_HINT = re.compile(
    r"invalid character|unexpected end of JSON input|unexpected EOF|"
    r"cannot unmarshal")


def _scrub(node: Any, prefix: str = "") -> None:
    """Delete environment-driven keys in-place.

    The noise-field set uses dotted paths so nested keys (e.g.
    ``headers.host``) can be targeted without dropping the whole
    ``headers`` map.
    """
    if isinstance(node, dict):
        for key in list(node.keys()):
            full = f"{prefix}.{key}" if prefix else key
            if full in ENV_NOISE_FIELDS:
                node.pop(key, None)
                continue
            _scrub(node[key], full)
    elif isinstance(node, list):
        for item in node:
            _scrub(item, prefix)


def _json_eq(a: Any, b: Any) -> bool:
    """Structural JSON equality, ignoring object key order.

    Date-only strings are allowed to differ by one day: n8n resolves
    ``$now`` in the workflow timezone (default ``America/New_York``)
    while m9m may resolve it in the container timezone, so a
    ``processedAt`` field can land on either side of midnight.
    """
    if isinstance(a, dict) and isinstance(b, dict):
        if set(a.keys()) != set(b.keys()):
            return False
        return all(_json_eq(a[k], b[k]) for k in a)
    if isinstance(a, list) and isinstance(b, list):
        if len(a) != len(b):
            return False
        return all(_json_eq(x, y) for x, y in zip(a, b))
    if isinstance(a, str) and isinstance(b, str):
        if a == b:
            return True
        try:
            da = _dt.date.fromisoformat(a)
            db = _dt.date.fromisoformat(b)
        except ValueError:
            return False
        return abs((da - db).days) <= 1
    return a == b


def _error_hints_equivalent(a: str, b: str) -> bool:
    """True when two parser hints describe the same parse failure."""
    if a == b:
        return True
    return bool(_V8_HINT.search(a) and _GO_HINT.search(b)) or bool(
        _V8_HINT.search(b) and _GO_HINT.search(a))


def semantic_json_equal(n8n_raw: str, m9m_raw: str) -> bool:
    """Compare two JSON bodies modulo environment noise.

    Falls back to raw string equality when either side is not valid
    JSON so a shape regression (e.g. m9m returning plain text where
    n8n returned JSON) still surfaces as a mismatch.
    """
    try:
        a = json.loads(n8n_raw)
        b = json.loads(m9m_raw)
    except (json.JSONDecodeError, TypeError):
        return n8n_raw.strip() == m9m_raw.strip()

    _scrub(a)
    _scrub(b)
    if _json_eq(a, b):
        return True

    # Error envelopes: same code + message, different parser hint.
    if isinstance(a, dict) and isinstance(b, dict):
        if a.get("code") == b.get("code") and a.get("message") == b.get("message"):
            hint_a, hint_b = a.get("hint"), b.get("hint")
            if isinstance(hint_a, str) and isinstance(hint_b, str):
                return _error_hints_equivalent(hint_a, hint_b)
    return False


def _canon_xml(elem: ET.Element) -> dict[str, Any]:
    """Canonicalise an XML element into a comparable dict.

    Attributes are folded into the same namespace as child elements so
    that n8n's ``<buku><id>001</id></buku>`` and m9m's
    ``<buku id="001">…</buku>`` compare equal. Whitespace-only text is
    dropped; significant text is kept under ``_text``.
    """
    children: dict[str, list[Any]] = {}

    for key, value in elem.attrib.items():
        children.setdefault(key, []).append({"_text": (value or "").strip()})

    for child in list(elem):
        children.setdefault(child.tag, []).append(_canon_xml(child))

    out: dict[str, Any] = {}
    text = (elem.text or "").strip()
    if text:
        out["_text"] = text
    for tag, values in children.items():
        out[tag] = values if len(values) > 1 else values[0]
    return out


def xml_round_trip_equivalent(n8n_xml: str, m9m_xml: str) -> bool:
    """Compare two XML bodies as logical element trees.

    Returns ``False`` when either side fails to parse — a body that is
    not XML at all is a parity regression, not an equivalent
    serialisation.
    """
    try:
        a = ET.fromstring(n8n_xml)
        b = ET.fromstring(m9m_xml)
    except ET.ParseError:
        return n8n_xml.strip() == m9m_xml.strip()
    return _canon_xml(a) == _canon_xml(b)


def base_media_type(content_type: str) -> str:
    """``application/xml; charset=utf-8`` -> ``application/xml``."""
    return (content_type or "").split(";")[0].strip().lower()


def headers_equivalent(n8n_headers: dict[str, str], m9m_headers: dict[str, str]) -> bool:
    """Compare the response headers that matter for parity.

    Only ``Content-Type`` is compared, and only on its base media type:
    n8n appends ``charset=utf-8`` to its XML responses while m9m does
    not, and that difference is not a behavioural gap. Header names are
    compared case-insensitively.
    """
    def pick(headers: dict[str, str]) -> str:
        for key, value in (headers or {}).items():
            if key.lower() == "content-type":
                return base_media_type(value)
        return ""

    return pick(n8n_headers) == pick(m9m_headers)


def classify(case: dict[str, Any], n8n: dict[str, Any], m9m: dict[str, Any]) -> tuple[str, str]:
    """Decide PASS/FAIL for one case.

    Returns ``(verdict, reason)``. ``verdict`` is ``PASS`` or ``FAIL``;
    ``reason`` explains a failure in one short line.
    """
    if n8n.get("status_code", 0) != m9m.get("status_code", 0):
        return "FAIL", (
            f"HTTP status differs: n8n={n8n.get('status_code')} "
            f"m9m={m9m.get('status_code')}")

    expected = case.get("expected_status")
    if expected is not None and n8n.get("status_code") != expected:
        return "FAIL", (
            f"n8n status {n8n.get('status_code')} != expected {expected}")

    if not case.get("compare_content_type", True):
        return "PASS", "status only (content type comparison disabled)"

    if not headers_equivalent(n8n.get("headers", {}), m9m.get("headers", {})):
        return "FAIL", (
            "Content-Type differs: n8n="
            f"{base_media_type(n8n.get('headers', {}).get('Content-Type', ''))} "
            f"m9m={base_media_type(m9m.get('headers', {}).get('Content-Type', ''))}")

    n8n_body = (n8n.get("body_raw") or "").strip()
    m9m_body = (m9m.get("body_raw") or "").strip()

    if not n8n_body and not m9m_body:
        return "FAIL", "both responses are empty"

    kind = (case.get("compare") or "json").lower()
    if kind == "xml":
        if xml_round_trip_equivalent(n8n_body, m9m_body):
            return "PASS", "XML round-trip equivalent"
        return "FAIL", "XML bodies are not equivalent"
    if kind == "none":
        return "PASS", "status + headers only"

    if semantic_json_equal(n8n_body, m9m_body):
        return "PASS", "JSON bodies equivalent"
    return "FAIL", "JSON bodies differ"
