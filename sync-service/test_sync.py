#!/usr/bin/env python3
"""Unit tests for sync.py.

Two groups:

  1. AES-256-CBC credential decrypt — cross-checks with the Go
     implementation in internal/credentials/n8n_cipher_test.go against
     the same known ciphertext vector.

  2. n8n import/export parity — normalization, validation, m9m wire
     adaptation and the local-file import CLI. The expected shapes are
     taken from the real n8n source in ./n8n:
       packages/cli/src/commands/import/workflow.ts
       packages/cli/src/services/import.service.ts
       packages/cli/src/commands/export/workflow.ts
       packages/cli/src/workflow-helpers.ts

Run: `python sync-service/test_sync.py` from the repo root
  or: `pytest sync-service/test_sync.py`
"""
import contextlib
import io
import json
import os
import sys
import tempfile

# Make `import sync` work when invoked from repo root.
sys.path.insert(0, os.path.join(os.path.dirname(__file__)))

import sync


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _workflow(**overrides):
    """A minimal but valid n8n workflow payload."""
    wf = {
        "id": "Wf123",
        "name": "Demo workflow",
        "active": True,
        "nodes": [
            {
                "id": "node-1",
                "name": "Webhook",
                "type": "n8n-nodes-base.webhook",
                "typeVersion": 2,
                "position": [0, 0],
                "parameters": {"path": "demo"},
            },
            {
                "id": "node-2",
                "name": "Set",
                "type": "n8n-nodes-base.set",
                "typeVersion": 1,
                "position": [200, 0],
                "parameters": {},
            },
        ],
        "connections": {
            "Webhook": {"main": [[{"node": "Set", "type": "main", "index": 0}]]},
        },
    }
    wf.update(overrides)
    return wf


# ---------------------------------------------------------------------------
# 1. Credential decrypt (unchanged behaviour)
# ---------------------------------------------------------------------------

def test_decrypt_known_vector():
    """Known ciphertext from a live n8n Postgres DB on 2026-09-09
    (the `basic_credential` envelope). With the right
    N8N_ENCRYPTION_KEY we expect `{"user":"admin","password":"admin123"}`.
    """
    os.environ["N8N_ENCRYPTION_KEY"] = "Default_Secure_Key_Change_Me_!!!"
    # Re-read env into the module-level constant. Python imports
    # happen at module load time, so the env must be set before
    # import — we do this here by re-reading the constant manually.
    sync.N8N_ENCRYPTION_KEY = os.environ["N8N_ENCRYPTION_KEY"]
    ciphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
    pt = sync.decrypt_n8n_credential_data(ciphertext)
    assert pt == {"user": "admin", "password": "admin123"}, f"got {pt!r}"


def test_decrypt_empty():
    """Empty ciphertext -> empty dict. n8n returns "" for
    credentials that have no data (e.g. a newly-created form)."""
    sync.N8N_ENCRYPTION_KEY = "Default_Secure_Key_Change_Me_!!!"
    assert sync.decrypt_n8n_credential_data("") == {}


def test_decrypt_no_key_raises():
    """Missing N8N_ENCRYPTION_KEY must raise rather than silently
    returning junk — silent failure would have us push opaque
    ciphertext blobs to m9m and break the webhook auth path."""
    sync.N8N_ENCRYPTION_KEY = ""
    try:
        sync.decrypt_n8n_credential_data("anything")
    except RuntimeError as e:
        assert "N8N_ENCRYPTION_KEY" in str(e), f"unexpected error: {e}"
    else:
        raise AssertionError("expected RuntimeError when N8N_ENCRYPTION_KEY is unset")


def test_decrypt_wrong_key_returns_garbage():
    """With a wrong key, AES-CBC produces 32 bytes of garbage
    that PKCS#7 unpad will reject. We verify the function raises
    rather than returning the wrong plaintext — silently returning
    garbage would be a much worse failure mode."""
    sync.N8N_ENCRYPTION_KEY = "totally_wrong_key"
    ciphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
    try:
        sync.decrypt_n8n_credential_data(ciphertext)
    except Exception:
        pass  # ValueError from cryptography is fine — padding fails.
    else:
        raise AssertionError("expected decryption to fail with wrong key")


# ---------------------------------------------------------------------------
# 2. normalize_n8n_workflow — n8n import parity
# ---------------------------------------------------------------------------

def test_normalize_old_credential_format():
    """n8n's OLD credential shape is `{type: "<name>"}`; import.service.ts
    converts it to `{type: {id, name}}`. The bridge cannot resolve the
    id locally, so it stays None."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {"httpBasicAuth": "My Cred"}
    out = sync.normalize_n8n_workflow(wf)
    creds = out["nodes"][0]["credentials"]
    assert creds == {"httpBasicAuth": {"id": None, "name": "My Cred"}}, creds


def test_normalize_new_credential_format_kept():
    """The NEW shape survives normalization, id included."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {
        "httpBasicAuth": {"id": "abc123", "name": "My Cred"},
    }
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["credentials"] == {
        "httpBasicAuth": {"id": "abc123", "name": "My Cred"},
    }


def test_normalize_credential_null_id_kept():
    """n8n's own converter emits `id: null` when it cannot resolve a
    credential — the bridge must not turn that into an error."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {
        "httpBasicAuth": {"id": None, "name": "My Cred"},
    }
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["credentials"]["httpBasicAuth"]["id"] is None


def test_normalize_generates_missing_node_ids():
    """import.service.ts: `if (!node.id) node.id = uuid()`."""
    wf = _workflow()
    del wf["nodes"][1]["id"]
    out = sync.normalize_n8n_workflow(wf)
    generated = out["nodes"][1]["id"]
    assert generated and isinstance(generated, str), generated
    assert len(generated) == 36, generated  # uuid4 canonical form
    assert out["nodes"][0]["id"] == "node-1"  # existing id untouched


def test_normalize_workflow_id_is_string():
    """n8n workflow ids are strings; a numeric id is coerced, a missing
    one is generated."""
    wf = _workflow(id=12345)
    out = sync.normalize_n8n_workflow(wf)
    assert out["id"] == "12345", out["id"]

    wf = _workflow()
    del wf["id"]
    out = sync.normalize_n8n_workflow(wf)
    assert isinstance(out["id"], str) and len(out["id"]) == 36, out["id"]


def test_normalize_tags_from_strings():
    """n8n tags are objects `{id, name}`. Plain strings are accepted
    and given an id so the result still matches the n8n shape."""
    wf = _workflow(tags=["production", "billing"])
    out = sync.normalize_n8n_workflow(wf)
    tags = out["tags"]
    assert len(tags) == 2, tags
    assert [t["name"] for t in tags] == ["production", "billing"], tags
    for tag in tags:
        assert set(tag.keys()) == {"id", "name"}, tag
        assert isinstance(tag["id"], str) and tag["id"], tag


def test_normalize_tags_objects_preserved():
    """Existing tag objects keep their id."""
    wf = _workflow(tags=[{"id": "tag-1", "name": "production"}])
    out = sync.normalize_n8n_workflow(wf)
    assert out["tags"] == [{"id": "tag-1", "name": "production"}], out["tags"]


def test_normalize_type_version_float():
    """n8n writes minor versions as floats; m9m's Go model wants int."""
    wf = _workflow()
    wf["nodes"][0]["typeVersion"] = 2.1
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["typeVersion"] == 2
    assert isinstance(out["nodes"][0]["typeVersion"], int)


def test_normalize_type_version_string():
    wf = _workflow()
    wf["nodes"][0]["typeVersion"] = "3"
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["typeVersion"] == 3


def test_normalize_type_version_non_numeric_warns():
    """A non-numeric typeVersion falls back to 1 and is reported."""
    wf = _workflow()
    wf["nodes"][0]["typeVersion"] = "not-a-number"
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    assert out["nodes"][0]["typeVersion"] == 1
    assert warnings and "typeVersion" in warnings[0], warnings


def test_normalize_active_forced_false_by_default():
    """n8n's `--activeState=false` default deactivates imports."""
    wf = _workflow(active=True)
    out = sync.normalize_n8n_workflow(wf)
    assert out["active"] is False


def test_normalize_active_from_json_opt_in():
    """`--activeState=fromJson` honours the JSON flag."""
    wf = _workflow(active=True)
    out = sync.normalize_n8n_workflow(wf, active_state=sync.ACTIVE_STATE_FROM_JSON)
    assert out["active"] is True

    wf = _workflow(active=False)
    out = sync.normalize_n8n_workflow(wf, active_state=sync.ACTIVE_STATE_FROM_JSON)
    assert out["active"] is False


def test_normalize_assigns_fresh_version_id():
    """Every import assigns a NEW versionId."""
    wf = _workflow(versionId="old-version")
    out = sync.normalize_n8n_workflow(wf)
    assert out["versionId"] != "old-version"
    assert len(out["versionId"]) == 36, out["versionId"]


def test_normalize_requires_nodes_and_connections():
    """n8n aborts with 'File does not seem to contain valid workflows.'
    when either property is missing. The bridge raises per entry so
    the batch can continue."""
    wf = _workflow()
    del wf["connections"]
    try:
        sync.normalize_n8n_workflow(wf)
    except sync.InvalidWorkflowError as e:
        assert "connections" in str(e), e
    else:
        raise AssertionError("expected InvalidWorkflowError for missing connections")

    wf = _workflow()
    del wf["nodes"]
    try:
        sync.normalize_n8n_workflow(wf)
    except sync.InvalidWorkflowError as e:
        assert "nodes" in str(e), e
    else:
        raise AssertionError("expected InvalidWorkflowError for missing nodes")


def test_normalize_does_not_mutate_input():
    wf = _workflow(tags=["a"])
    wf["nodes"][0]["credentials"] = {"httpBasicAuth": "My Cred"}
    sync.normalize_n8n_workflow(wf)
    assert wf["active"] is True
    assert wf["tags"] == ["a"]
    assert wf["nodes"][0]["credentials"] == {"httpBasicAuth": "My Cred"}


# ---------------------------------------------------------------------------
# 2b. Editor-side import rules (reference §3)
#    n8n/packages/frontend/editor-ui/src/app/composables/useCanvasOperations.ts
#    importWorkflowData() L3230-3345 + app/utils/workflowUtils.ts
# ---------------------------------------------------------------------------

def test_normalize_drops_nodes_without_type():
    """The editor import filters out nodes with no `type` and reports the
    count. The bridge does the same — as a WARNING, never a hard failure."""
    wf = _workflow()
    wf["nodes"].append({
        "id": "node-3",
        "name": "Broken",
        "position": [400, 0],
        "parameters": {},
    })
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    assert len(out["nodes"]) == 2, out["nodes"]
    assert any("without a 'type'" in w for w in warnings), warnings


def test_normalize_all_nodes_typeless_becomes_empty_and_is_rejected():
    """Every node dropped -> no nodes left -> the workflow is rejected."""
    wf = _workflow(nodes=[{"id": "n1", "name": "Broken", "parameters": {}}])
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"] == []
    check = sync.validate_n8n_workflow(out)
    assert check["valid"] is False, check
    assert any("no nodes" in e for e in check["errors"]), check


def test_normalize_position_missing_becomes_zero():
    """`ensureNodePosition()` normalises a missing/invalid position."""
    wf = _workflow()
    del wf["nodes"][0]["position"]
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    assert out["nodes"][0]["position"] == [0, 0]
    assert any("position" in w for w in warnings), warnings


def test_normalize_position_invalid_becomes_zero():
    wf = _workflow()
    wf["nodes"][0]["position"] = ["x", "y"]
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["position"] == [0, 0]


def test_normalize_position_preserved_when_valid():
    wf = _workflow()
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["position"] == [0, 0]
    wf = _workflow()
    wf["nodes"][0]["position"] = [120, 240]
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["position"] == [120, 240]


def test_normalize_names_unnamed_nodes_uniquely():
    """The editor import gives unnamed nodes a unique name."""
    wf = _workflow()
    del wf["nodes"][0]["name"]
    del wf["nodes"][1]["name"]
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    names = [n["name"] for n in out["nodes"]]
    assert names[0] == "Webhook", names
    assert names[1] == "Edit Fields (Set)", names
    assert len(set(names)) == len(names), names
    assert any("had no name" in w for w in warnings), warnings


def test_normalize_drops_connections_to_unknown_nodes():
    """`sanitizeConnections()` drops edges pointing at unknown node names."""
    wf = _workflow()
    wf["connections"]["Ghost"] = {"main": [[{"node": "Set", "type": "main", "index": 0}]]}
    wf["connections"]["Webhook"]["main"][0].append(
        {"node": "Missing", "type": "main", "index": 0}
    )
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    assert "Ghost" not in out["connections"], out["connections"]
    targets = [entry["node"] for entry in out["connections"]["Webhook"]["main"][0]]
    assert targets == ["Set"], targets
    assert any("connection" in w for w in warnings), warnings


def test_normalize_keeps_valid_connections():
    wf = _workflow()
    out = sync.normalize_n8n_workflow(wf)
    assert out["connections"] == wf["connections"], out["connections"]


def test_normalize_drops_malformed_connection_entries():
    wf = _workflow()
    wf["connections"]["Webhook"]["main"][0].append({"node": "Set"})
    out = sync.normalize_n8n_workflow(wf)
    assert out["connections"]["Webhook"]["main"][0] == [
        {"node": "Set", "type": "main", "index": 0}
    ], out["connections"]


# ---------------------------------------------------------------------------
# 2c. Credential skip rules (reference §2.3, workflow-helpers.ts L274-335)
# ---------------------------------------------------------------------------

def test_normalize_skips_credentials_on_disabled_nodes():
    """n8n does not repair credentials of DISABLED nodes."""
    wf = _workflow()
    wf["nodes"][0]["disabled"] = True
    wf["nodes"][0]["credentials"] = {"httpBasicAuth": "My Cred"}
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    # Left exactly as it came in — no shape conversion, no repair.
    assert out["nodes"][0]["credentials"] == {"httpBasicAuth": "My Cred"}, \
        out["nodes"][0]["credentials"]
    assert any("disabled" in w for w in warnings), warnings


def test_normalize_skips_ai_gateway_managed_credentials():
    """`__aiGatewayManaged` entries have no record behind them; n8n
    skips them (workflow-helpers.ts L301-302)."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {
        "openAiApi": {"__aiGatewayManaged": True},
    }
    warnings = []
    out = sync.normalize_n8n_workflow(wf, warnings=warnings)
    assert out["nodes"][0]["credentials"] == {
        "openAiApi": {"__aiGatewayManaged": True},
    }, out["nodes"][0]["credentials"]
    assert any("__aiGatewayManaged" in w for w in warnings), warnings


def test_to_m9m_payload_drops_ai_gateway_managed_credentials():
    """m9m has no AI Gateway credential concept — the entry is dropped."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {"openAiApi": {"__aiGatewayManaged": True}}
    payload = sync.to_m9m_payload(sync.normalize_n8n_workflow(wf))
    assert payload["nodes"][0]["credentials"] == {}, payload["nodes"][0]["credentials"]


def test_normalize_keeps_unresolved_credential_as_null_id():
    """An id that cannot be resolved stays null, the name is kept."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {
        "httpBasicAuth": {"id": None, "name": "My Cred"},
    }
    out = sync.normalize_n8n_workflow(wf)
    assert out["nodes"][0]["credentials"] == {
        "httpBasicAuth": {"id": None, "name": "My Cred"},
    }, out["nodes"][0]["credentials"]


# ---------------------------------------------------------------------------
# 3. validate_n8n_workflow
# ---------------------------------------------------------------------------

def test_validate_accepts_valid_workflow():
    result = sync.validate_n8n_workflow(sync.normalize_n8n_workflow(_workflow()))
    assert result["valid"] is True, result
    assert result["errors"] == [], result
    assert result["unsupportedTypes"] == [], result


def test_validate_reports_empty_nodes():
    wf = _workflow(nodes=[])
    result = sync.validate_n8n_workflow(wf)
    assert result["valid"] is False
    assert any("no nodes" in e for e in result["errors"]), result


def test_validate_reports_missing_node_name_and_type():
    wf = _workflow()
    wf["nodes"][1].pop("name")
    wf["nodes"][1].pop("type")
    result = sync.validate_n8n_workflow(wf)
    assert result["valid"] is False
    assert any("missing name" in e for e in result["errors"]), result
    assert any("missing type" in e for e in result["errors"]), result


def test_validate_reports_missing_workflow_name():
    wf = _workflow()
    wf.pop("name")
    result = sync.validate_n8n_workflow(wf)
    assert result["valid"] is False
    assert any("name is missing" in e for e in result["errors"]), result


def test_validate_unsupported_type_is_warning_not_error():
    """n8n imports workflows with unknown node types; m9m cannot run
    them, so the gap is reported but the workflow stays importable."""
    wf = _workflow()
    wf["nodes"][1]["type"] = "n8n-nodes-base.someNodeWeDoNotHave"
    result = sync.validate_n8n_workflow(wf)
    assert result["valid"] is True, result
    assert "n8n-nodes-base.someNodeWeDoNotHave" in result["unsupportedTypes"], result
    assert result["warnings"], result


def test_validate_tolerated_types_are_not_unsupported():
    """Sticky notes / manual triggers are skipped by m9m's engine, so
    they must not be reported as unsupported."""
    wf = _workflow()
    wf["nodes"][1]["type"] = "n8n-nodes-base.stickyNote"
    result = sync.validate_n8n_workflow(wf)
    assert result["unsupportedTypes"] == [], result
    assert result["warnings"] == [], result


def test_validate_uses_live_known_types():
    """`known_types` (from GET /api/v1/node-types) extends the static map."""
    wf = _workflow()
    wf["nodes"][1]["type"] = "n8n-nodes-base.liveOnly"
    result = sync.validate_n8n_workflow(wf, known_types={"n8n-nodes-base.liveOnly"})
    assert result["unsupportedTypes"] == [], result


# ---------------------------------------------------------------------------
# 4. to_m9m_payload — n8n shape -> m9m wire format
# ---------------------------------------------------------------------------

def test_to_m9m_payload_converts_tags_to_strings():
    """m9m's Go model has `Tags []string`; n8n sends tag objects."""
    normalized = sync.normalize_n8n_workflow(_workflow(tags=["production"]))
    payload = sync.to_m9m_payload(normalized)
    assert payload["tags"] == ["production"], payload["tags"]
    # The n8n-shaped representation stays untouched.
    assert normalized["tags"][0]["name"] == "production"


def test_to_m9m_payload_adapts_node_credentials():
    """m9m's Credential model is `{id, name, type}`; n8n's is
    `{id, name}` keyed by credential type."""
    wf = _workflow()
    wf["nodes"][0]["credentials"] = {"httpBasicAuth": "My Cred"}
    normalized = sync.normalize_n8n_workflow(wf)
    payload = sync.to_m9m_payload(normalized)
    creds = payload["nodes"][0]["credentials"]
    assert creds == {
        "httpBasicAuth": {"id": "", "name": "My Cred", "type": "httpBasicAuth"},
    }, creds


def test_to_m9m_payload_keeps_nodes_and_connections():
    normalized = sync.normalize_n8n_workflow(_workflow())
    payload = sync.to_m9m_payload(normalized)
    assert len(payload["nodes"]) == 2
    assert "Webhook" in payload["connections"]
    assert payload["id"] == normalized["id"]
    assert payload["versionId"] == normalized["versionId"]


# ---------------------------------------------------------------------------
# 5. File import (single object + array, lenient batch)
# ---------------------------------------------------------------------------

def _write_json(tmpdir, name, payload):
    path = os.path.join(tmpdir, name)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh)
    return path


def test_import_single_object_file():
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "wf.json", _workflow())
        verdicts = sync.import_workflows_from_file(path, dry_run=True)
        assert len(verdicts) == 1, verdicts
        assert verdicts[0]["status"] == "ok", verdicts[0]
        assert verdicts[0]["normalized"]["active"] is False


def test_import_array_file_skips_invalid_entry_and_continues():
    """n8n's `--separate` mode skips an invalid file and keeps importing
    the rest of the batch."""
    invalid = _workflow(name="Missing connections")
    del invalid["connections"]
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "batch.json", [_workflow(name="Good one"), invalid])
        verdicts = sync.import_workflows_from_file(path, dry_run=True)

    assert len(verdicts) == 2, verdicts
    assert verdicts[0]["status"] == "ok", verdicts[0]
    assert verdicts[0]["name"] == "Good one"
    assert verdicts[1]["status"] == "skipped", verdicts[1]
    assert "connections" in verdicts[1]["reason"], verdicts[1]


def test_import_entry_missing_nodes_is_skipped():
    invalid = _workflow()
    del invalid["nodes"]
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "wf.json", invalid)
        verdicts = sync.import_workflows_from_file(path, dry_run=True)
    assert verdicts[0]["status"] == "skipped", verdicts[0]
    assert "nodes" in verdicts[0]["reason"], verdicts[0]


# ---------------------------------------------------------------------------
# 6. CLI
# ---------------------------------------------------------------------------

def _no_http(*args, **kwargs):
    raise AssertionError("dry-run must not perform any HTTP request")


def test_cli_dry_run_makes_zero_http_requests():
    original_http = sync.http_json
    original_fetch = sync.fetch_m9m_node_types
    sync.http_json = _no_http
    sync.fetch_m9m_node_types = _no_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", [_workflow()])
            buffer = io.StringIO()
            with contextlib.redirect_stdout(buffer):
                exit_code = sync.main(["--import", path, "--dry-run"])
            output = buffer.getvalue()
    finally:
        sync.http_json = original_http
        sync.fetch_m9m_node_types = original_fetch

    assert exit_code == 0, output
    assert "[import] ok" in output, output
    # The normalized JSON is printed in a dry run.
    assert '"versionId"' in output, output


def test_cli_dry_run_reports_skipped_entries():
    invalid = _workflow(name="Broken")
    del invalid["connections"]
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "batch.json", [_workflow(), invalid])
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            exit_code = sync.main(["--import", path, "--dry-run"])
        output = buffer.getvalue()
    assert exit_code == 0, output
    assert "skipped" in output, output
    assert "1 ok, 1 skipped, 0 failed" in output, output


def test_cli_active_state_from_json_flag():
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "wf.json", _workflow(active=True))
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            exit_code = sync.main(["--import", path, "--dry-run", "--active-state=from-json"])
        output = buffer.getvalue()
    assert exit_code == 0, output
    assert '"active": true' in output, output


def test_cli_active_state_from_json_camel_case_alias():
    """n8n spells the flag `--activeState=fromJson`; the bridge accepts
    both spellings."""
    with tempfile.TemporaryDirectory() as tmp:
        path = _write_json(tmp, "wf.json", _workflow(active=True))
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            exit_code = sync.main(["--import", path, "--dry-run", "--activeState=fromJson"])
        output = buffer.getvalue()
    assert exit_code == 0, output
    assert '"active": true' in output, output


def test_cli_missing_file_exits_nonzero():
    buffer = io.StringIO()
    with contextlib.redirect_stdout(buffer):
        exit_code = sync.main(["--import", os.path.join(tempfile.gettempdir(), "does-not-exist-42.json"), "--dry-run"])
    assert exit_code == 2, buffer.getvalue()


def test_cli_without_import_boots_service(monkeypatch=None):
    """With no --import the module keeps its uvicorn behaviour."""
    calls = {}

    def fake_run(app, **kwargs):
        calls["app"] = app
        calls["kwargs"] = kwargs

    original = sys.modules.get("uvicorn")
    fake_uvicorn = type(sys)("uvicorn")
    fake_uvicorn.run = fake_run
    sys.modules["uvicorn"] = fake_uvicorn
    try:
        exit_code = sync.main([])
    finally:
        if original is not None:
            sys.modules["uvicorn"] = original
        else:
            sys.modules.pop("uvicorn", None)

    assert exit_code == 0
    assert calls.get("app") == "sync:app", calls


# ---------------------------------------------------------------------------
# 7. Batch push to m9m's n8n import endpoint
#    (internal/api/workflow_import_export.go -> POST /api/v1/workflows/import)
# ---------------------------------------------------------------------------

def _recording_http(responses):
    """Build a fake `sync.http_json` that records its calls.

    `responses` is either a single `(status, body)` tuple or a list of
    tuples consumed in order; the last one repeats.
    """
    calls = []

    if isinstance(responses, tuple):
        responses = [responses]

    def fake_http(method, url, headers=None, body=None):
        calls.append({"method": method, "url": url, "headers": headers, "body": body})
        if len(responses) == 1:
            status, resp_body = responses[0]
        else:
            status, resp_body = responses.pop(0)
        return status, resp_body

    fake_http.calls = calls
    return fake_http


def test_import_batch_pushes_to_m9m_import_endpoint():
    """The import CLI pushes the whole batch to m9m's n8n import
    endpoint - the endpoint that mirrors n8n's batch import."""
    fake_http = _recording_http((201, {
        "imported": 1,
        "failed": 0,
        "activeState": "false",
        "workflows": [{"index": 0, "id": "Wf123", "name": "Demo workflow",
                       "versionId": "v-1", "active": False}],
        "errors": [],
    }))
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", _workflow())
            verdicts = sync.import_workflows_from_file(path)
    finally:
        sync.http_json = original_http

    assert len(fake_http.calls) == 1, fake_http.calls
    call = fake_http.calls[0]
    assert call["method"] == "POST", call
    assert call["url"].endswith("/api/v1/workflows/import"), call
    assert isinstance(call["body"], list), call
    assert len(call["body"]) == 1, call
    assert verdicts[0]["status"] == "ok", verdicts[0]

def test_import_batch_from_json_asks_for_the_active_flag():
    """`--active-state=from-json` maps to n8n's `activeState=fromJson`
    on the wire - m9m only honours the JSON `active` flag when the
    request asks for it."""
    fake_http = _recording_http((201, {
        "imported": 1, "failed": 0, "activeState": "fromJson",
        "workflows": [{"index": 0, "id": "Wf123", "active": True}],
        "errors": [],
    }))
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", _workflow(active=True))
            verdicts = sync.import_workflows_from_file(
                path, active_state=sync.ACTIVE_STATE_FROM_JSON
            )
    finally:
        sync.http_json = original_http

    assert "activeState=fromJson" in fake_http.calls[0]["url"], fake_http.calls
    assert fake_http.calls[0]["body"][0]["active"] is True, fake_http.calls
    assert verdicts[0]["status"] == "ok", verdicts[0]

def test_import_batch_reports_per_item_errors():
    """One rejected entry never aborts the batch - the summary of the
    import endpoint is mapped back onto the per-entry verdicts."""
    fake_http = _recording_http((201, {
        "imported": 1, "failed": 1, "activeState": "false",
        "workflows": [{"index": 0, "id": "Wf123", "name": "Good one"}],
        "errors": [{"index": 1, "name": "Bad one",
                    "message": "File does not seem to contain valid workflows: "
                               "missing \"connections\" property"}],
    }))
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "batch.json",
                               [_workflow(name="Good one"), _workflow(name="Bad one")])
            verdicts = sync.import_workflows_from_file(path)
    finally:
        sync.http_json = original_http

    assert verdicts[0]["status"] == "ok", verdicts[0]
    assert verdicts[1]["status"] == "failed", verdicts[1]
    assert "connections" in verdicts[1]["reason"], verdicts[1]

def test_import_batch_surfaces_server_warnings():
    """Warnings of the import endpoint (e.g. an unresolved credential)
    are added to the verdict."""
    fake_http = _recording_http((201, {
        "imported": 1, "failed": 0, "activeState": "false",
        "workflows": [{"index": 0, "id": "Wf123",
                       "warnings": ["credential \"My Cred\" (httpBasicAuth) could not "
                                    "be resolved in m9m"]}],
        "errors": [],
    }))
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", _workflow())
            verdicts = sync.import_workflows_from_file(path)
    finally:
        sync.http_json = original_http

    assert verdicts[0]["status"] == "ok", verdicts[0]
    assert any("could not be resolved" in w for w in verdicts[0]["warnings"]), verdicts[0]

def test_import_batch_falls_back_when_the_import_endpoint_is_missing():
    """m9m builds without the n8n import endpoint (HTTP 404) fall back
    to the per-workflow endpoint `POST /api/v1/workflows`."""
    fake_http = _recording_http([
        (404, {"error": "not found"}),
        (200, {"id": "Wf123"}),
    ])
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", _workflow())
            verdicts = sync.import_workflows_from_file(path)
    finally:
        sync.http_json = original_http

    assert len(fake_http.calls) == 2, fake_http.calls
    assert fake_http.calls[0]["url"].endswith("/api/v1/workflows/import"), fake_http.calls
    assert fake_http.calls[1]["url"].endswith("/api/v1/workflows"), fake_http.calls
    assert verdicts[0]["status"] == "ok", verdicts[0]

def test_import_batch_maps_the_summary_of_a_rejected_request():
    """m9m answers HTTP 400 when NOTHING could be imported; the
    per-item errors live in the `summary` of the response body."""
    fake_http = _recording_http((400, {
        "error": True,
        "message": "No workflow was imported: File does not seem to contain valid workflows",
        "code": 400,
        "summary": {
            "imported": 0,
            "failed": 1,
            "activeState": "false",
            "workflows": [],
            "errors": [{"index": 0, "name": "Demo workflow",
                        "message": "File does not seem to contain valid workflows: "
                                   "missing \"connections\" property"}],
        },
    }))
    original_http = sync.http_json
    sync.http_json = fake_http
    try:
        with tempfile.TemporaryDirectory() as tmp:
            path = _write_json(tmp, "wf.json", _workflow())
            verdicts = sync.import_workflows_from_file(path)
    finally:
        sync.http_json = original_http

    assert verdicts[0]["status"] == "failed", verdicts[0]
    assert "connections" in verdicts[0]["reason"], verdicts[0]

def test_payload_matches_the_m9m_import_contract():
    """The payload the bridge produces must be accepted by m9m's Go
    import endpoint (internal/api/workflow_import_export.go ->
    internal/compatibility/workflow_importer.go):

      * the workflow id is a string;
      * `tags` are plain strings (m9m stores tag names);
      * node credentials use the new shape, keyed by credential type;
      * `connections` use n8n's source-node map shape;
      * the entry carries BOTH `nodes` and `connections` (the Go
        endpoint rejects an entry without either).
    """
    wf = _workflow(tags=["production"])
    wf["nodes"][0]["credentials"] = {"httpBasicAuth": "My Cred"}
    payload = sync.to_m9m_payload(sync.normalize_n8n_workflow(wf))

    assert isinstance(payload["id"], str), payload["id"]
    assert payload["tags"] == ["production"], payload["tags"]
    assert "nodes" in payload and "connections" in payload, payload
    assert payload["nodes"][0]["credentials"] == {
        "httpBasicAuth": {"id": "", "name": "My Cred", "type": "httpBasicAuth"},
    }, payload["nodes"][0]["credentials"]
    assert payload["connections"] == {
        "Webhook": {"main": [[{"node": "Set", "type": "main", "index": 0}]]},
    }, payload["connections"]
    assert isinstance(payload["nodes"][0]["typeVersion"], int), payload["nodes"][0]
    assert payload["nodes"][0]["position"] == [0, 0], payload["nodes"][0]

# ---------------------------------------------------------------------------
# Runner (pytest-compatible; also runnable as a plain script)
# ---------------------------------------------------------------------------

def _main():
    tests = [(name, obj) for name, obj in sorted(globals().items())
             if name.startswith("test_") and callable(obj)]
    failures = 0
    for name, fn in tests:
        try:
            fn()
        except Exception as e:
            failures += 1
            print(f"{name} FAIL: {e}")
            import traceback
            traceback.print_exc()
        else:
            print(f"{name} PASS")
    print(f"\n{len(tests) - failures}/{len(tests)} passed")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(_main())
