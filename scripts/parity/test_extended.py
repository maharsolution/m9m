"""Offline regression tests: python -m unittest discover -s scripts/parity -v.

The suite is deterministic and never touches the network.
"""
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import compare
import env
import extended
import load
import report
import runner
import signatures


class Contracts(unittest.TestCase):
    def response(self, value, status=200):
        return {"status_code": status, "headers": {"Content-Type": "application/json"},
                "body_raw": json.dumps(value), "error": None, "elapsed_ms": 0}

    def fake_api(self, url, case, timeout):
        """Transport double for the API contracts (no network)."""
        if "credentials" in url:
            return self.response({"data": [{"name": "db", "type": "postgres", "password": "SECRET"}]})
        if "workflows?limit" in url:
            return self.response({"data": [{"id": "w1", "name": "Simple Webhook"}]})
        if "workflows/w1" in url:
            return self.response({"data": {"name": "Simple Webhook", "nodes": [], "connections": {}}})
        return self.response({"data": []})

    def test_transport_failure_never_passes(self):
        self.assertEqual(compare.classify({}, self.response({}, 0), self.response({}, 0))[0], "FAIL")

    def test_disabling_headers_does_not_disable_body(self):
        self.assertEqual(compare.classify({"compare_content_type": False}, self.response({"x": 1}), self.response({"x": 2}))[0], "FAIL")

    def test_xml_root_is_significant(self):
        self.assertFalse(compare.xml_round_trip_equivalent("<a/>", "<b/>"))

    def test_percentile_nearest_rank(self):
        self.assertEqual(load.percentile(list(range(1, 51)), 95), 48)

    def test_zero_load_rejected(self):
        with self.assertRaises(SystemExit):
            load.main(["--count", "0"])

    def test_no_matching_cases_is_error(self):
        self.assertEqual(runner.main(["--suite", "positive", "--case", "nonexistent"]), 2)

    def test_registered_webhook_prefix_preserved(self):
        self.assertEqual(runner._case_url("http://test", {"path": "/webhook/user-signup"}), "http://test/webhook/webhook/user-signup")

    def test_suites_unique_ids(self):
        cases = [c for suite in runner.load_suites(None) for c in suite["cases"]]
        self.assertEqual(len(cases), len({c["id"] for c in cases}))

    def test_missing_api_not_pass(self):
        result = extended.run_contract({"id": "x", "name": "test", "check": "credentials"}, "n", "m", 1,
                                       lambda *a: self.response({}, 401))
        self.assertEqual(result["verdict"], "FAIL")

    def test_credential_metadata_redacted(self):
        result = extended.run_contract({"id": "x", "name": "test", "check": "credentials"}, "n", "m", 1,
                                       self.fake_api)
        self.assertEqual(result["verdict"], "PASS")
        self.assertNotIn("SECRET", json.dumps(result))

    def test_prerequisite_blocked(self):
        result = extended.run_contract({"id": "x", "name": "test", "check": "prerequisite", "blocker": "no fixture"}, "n", "m", 1, None)
        self.assertEqual(result["verdict"], "BLOCKED")

    def test_workflow_shape_keeps_real_parameters(self):
        a = {"nodes": [{"name": "DB", "parameters": {"sslMode": "disable"}}]}
        b = {"nodes": [{"name": "DB", "parameters": {"sslMode": "require"}}]}
        self.assertNotEqual(extended.workflow_shape(a), extended.workflow_shape(b))

    def test_sheet_path_from_curl(self):
        self.assertEqual(report._path_of({"request": "curl 'http://host/webhook/db_quiry' -d '{}'"}), "db_quiry")

    def test_unavailable_sheet_not_pass(self):
        with patch.object(report, "_sheet_rows", return_value=[]):
            self.assertNotEqual(report.drift_check({"results": []}), 0)

    def test_document_contradictory_sections(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "workflow.md"
            path.write_text("| Suite | Case | Path | Status |\n|---|---|---|---|\n| positive | `p01` | `a` | FAILED |\n| positive | `p01` | `a` | PASS |", encoding="utf-8")
            self.assertEqual(report.document_drift({"results": [{"id": "p01", "path": "a", "verdict": "PASS"}]}, path), 1)

    def test_document_unmatched_not_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "workflow.md"
            path.write_text("No evidence", encoding="utf-8")
            self.assertEqual(report.document_drift({"results": []}, path), 1)


class EnvironmentAndAuth(unittest.TestCase):
    """C1 - the n8n key must never leak to m9m."""

    def setUp(self):
        self._n8n = os.environ.get("N8N_API_KEY")
        self._m9m = os.environ.get("M9M_API_KEY")

    def tearDown(self):
        for key, value in (("N8N_API_KEY", self._n8n), ("M9M_API_KEY", self._m9m)):
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

    def test_n8n_key_never_goes_to_m9m(self):
        os.environ["N8N_API_KEY"] = "n8n-secret"
        os.environ.pop("M9M_API_KEY", None)
        self.assertEqual(env.api_headers("n8n"), {"X-N8N-API-KEY": "n8n-secret"})
        self.assertEqual(env.api_headers("m9m"), {})

    def test_m9m_key_only_when_configured(self):
        os.environ["M9M_API_KEY"] = "m9m-secret"
        self.assertEqual(env.api_headers("m9m"), {"X-API-Key": "m9m-secret"})
        os.environ.pop("M9M_API_KEY", None)
        self.assertEqual(env.api_headers("m9m"), {})

    def test_unknown_host_rejected(self):
        with self.assertRaises(ValueError):
            env.api_headers("other")

    def test_load_env_reports_presence_only(self):
        # The ambient environment is isolated: the parity runner imports
        # the REAL repo .env at module load, so M9M_API_KEY / N8N_API_KEY
        # may already be set in os.environ. Without the isolation this
        # test would report the AMBIENT key instead of the one of the
        # temporary file.
        with patch.dict(os.environ, {}, clear=True):
            with tempfile.TemporaryDirectory() as directory:
                env_file = Path(directory) / ".env"
                env_file.write_text("N8N_API_KEY=super-secret\nM9M_API_KEY=\n# comment\n",
                                    encoding="utf-8")
                present = env.load_env(env_file)
                self.assertNotIn("super-secret", json.dumps(present))
                self.assertTrue(present["N8N_API_KEY"])
                self.assertFalse(present["M9M_API_KEY"])


class BodySignatures(unittest.TestCase):
    """C12 - sheet rows are paired by path AND request body."""

    MYSQL = {"db_type": "mysql", "operation": "inquiry", "table_name": "credentials"}
    POSTGRES = {"db_type": "postgresql", "operation": "inquiry", "table_name": "credentials_entity"}

    def test_key_order_does_not_change_the_signature(self):
        first = signatures.body_signature('{"a":1,"b":2}')
        second = signatures.body_signature('{"b":2,"a":1}')
        self.assertEqual(first, second)

    def test_different_payloads_get_different_signatures(self):
        self.assertNotEqual(signatures.body_signature(self.MYSQL),
                            signatures.body_signature(self.POSTGRES))

    def test_curl_payload_is_extracted(self):
        curl = ("curl -X POST http://host/webhook/db_quiry -H 'Content-Type: application/json' "
                "-d '{\"db_type\":\"mysql\",\"operation\":\"inquiry\","
                "\"table_name\":\"credentials\"}'")
        self.assertEqual(signatures.curl_body_signature(curl),
                         signatures.body_signature(self.MYSQL))

    def test_curl_without_payload_has_no_signature(self):
        self.assertEqual(signatures.curl_body_signature("curl http://host/webhook/x"), "")


class DriftGuards(unittest.TestCase):
    """C12 - document and sheet drift, no double counting."""

    def write(self, directory: str, name: str, text: str) -> Path:
        path = Path(directory) / name
        path.write_text(text, encoding="utf-8")
        return path

    def test_bullet_lines_are_parsed(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **PASS**\n")
            payload = {"results": [{"id": "p01", "path": "simple_webhook", "verdict": "PASS"}]}
            self.assertEqual(report.document_drift(payload, path), 0)

    def test_bullet_disagreement_is_drift(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **FAIL**\n")
            payload = {"results": [{"id": "p01", "path": "simple_webhook", "verdict": "PASS"}]}
            self.assertEqual(report.document_drift(payload, path), 1)

    def test_bullet_and_table_lines_are_both_understood(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(
                directory, "workflow-test.MD",
                "| Suite | Case | Path | Status |\n|---|---|---|---|\n"
                "| positive | `p01` | `simple_webhook` | PASS |\n"
                "- `p02` `simple_webhook_2`: **PASS**\n")
            payload = {"results": [
                {"id": "p01", "path": "simple_webhook", "verdict": "PASS"},
                {"id": "p02", "path": "simple_webhook_2", "verdict": "PASS"},
            ]}
            self.assertEqual(report.document_drift(payload, path), 0)

    def test_one_case_is_counted_once(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(
                directory, "workflow-test.MD",
                "- `p01` `simple_webhook`: **PASS**\n"
                "- `p01` `simple_webhook`: **PASS**\n")
            payload = {"results": [{"id": "p01", "path": "simple_webhook", "verdict": "PASS"}]}
            self.assertEqual(report.document_drift(payload, path), 0)

    def _sheet_rows(self):
        mysql = ('{"db_type":"mysql","operation":"inquiry","table_name":"credentials"}')
        postgres = ('{"db_type":"postgresql","operation":"inquiry",'
                    '"table_name":"credentials_entity"}')
        return [
            {"no": "9", "name": "db mysql", "status": "OK", "code": "200", "suite": "positive",
             "request": f"curl -X POST http://host/webhook/db_quiry -d '{mysql}'"},
            {"no": "", "name": "db postgres", "status": "OK", "code": "200", "suite": "positive",
             "request": f"curl -X POST http://host/webhook/db_quiry -d '{postgres}'"},
        ]

    def _payload(self, mysql_verdict="PASS"):
        mysql = ('{"db_type":"mysql","operation":"inquiry","table_name":"credentials"}')
        postgres = ('{"db_type":"postgresql","operation":"inquiry",'
                    '"table_name":"credentials_entity"}')
        return {"results": [
            {"id": "p12", "path": "db_quiry", "suite": "positive", "verdict": mysql_verdict,
             "body_hash": signatures.body_signature(mysql)},
            {"id": "p14", "path": "db_quiry", "suite": "positive", "verdict": "PASS",
             "body_hash": signatures.body_signature(postgres)},
        ]}

    def test_sheet_rows_pair_by_path_and_body(self):
        with patch.object(report, "_sheet_rows", side_effect=[self._sheet_rows(), []]), \
                patch.object(report, "document_drift", return_value=0):
            self.assertEqual(report.drift_check(self._payload()), 0)

    def test_sheet_row_number_does_not_decide_the_pairing(self):
        """The sheet's own 1..N numbering must not override the body signature."""
        rows = self._sheet_rows()
        rows[0]["no"] = "99"  # a row number that matches no harness case
        with patch.object(report, "_sheet_rows", side_effect=[rows, []]), \
                patch.object(report, "document_drift", return_value=0):
            self.assertEqual(report.drift_check(self._payload()), 0)

    def test_each_harness_case_is_matched_once(self):
        rows = self._sheet_rows()
        rows.append(dict(rows[0]))  # the same request stored twice
        with patch.object(report, "_sheet_rows", side_effect=[rows, []]), \
                patch.object(report, "document_drift", return_value=0):
            self.assertEqual(report.drift_check(self._payload()), 0)

    def test_real_status_difference_is_drift(self):
        with patch.object(report, "_sheet_rows", side_effect=[self._sheet_rows(), []]), \
                patch.object(report, "document_drift", return_value=0):
            self.assertEqual(report.drift_check(self._payload(mysql_verdict="FAIL")), 1)

    def test_skipped_live_verdict_is_not_drift(self):
        """A case that did not run has no verdict - it cannot drift."""
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **PASS**\n")
            payload = {"results": [{"id": "p01", "path": "simple_webhook",
                                    "verdict": "SKIPPED"}]}
            self.assertEqual(report.document_drift(payload, path), 0)

    def test_offline_document_check_needs_no_live_evidence(self):
        """Offline: the document is parsed and judged on its own."""
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **PASS**\n"
                              "- `p02` `simple_webhook_2`: **FAIL**\n")
            payload = {"results": []}
            self.assertEqual(report.document_drift(payload, path, require_evidence=False), 0)
            self.assertEqual(report.document_drift(payload, path), 1)

    def test_contradiction_is_drift_even_without_live_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **PASS**\n"
                              "| positive | `p01` | `simple_webhook` | FAIL |\n")
            payload = {"results": []}
            self.assertEqual(report.document_drift(payload, path, require_evidence=False), 1)

    def test_offline_drift_contract_is_evaluated_without_the_sheet(self):
        relative = "tmp/parity-selftest/workflow-test.MD"
        target = Path(__file__).resolve().parent.parent.parent / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("- `p01` `simple_webhook`: **PASS**\n", encoding="utf-8")
        try:
            verdict, reason = extended.evaluate_drift(
                {"id": "c1201", "name": "drift", "document": relative},
                {"results": []}, offline=True)
            self.assertEqual(verdict, "PASS")
            self.assertIn("consistent", reason)
        finally:
            target.unlink(missing_ok=True)

    def test_offline_mode_does_not_compare_verdicts(self):
        """The document records the live run; the offline run has no live verdicts."""
        with tempfile.TemporaryDirectory() as directory:
            path = self.write(directory, "workflow-test.MD",
                              "- `p01` `simple_webhook`: **BLOCKED**\n")
            payload = {"results": [{"id": "p01", "path": "simple_webhook",
                                    "verdict": "PASS"}]}
            self.assertEqual(
                report.document_drift(payload, path, require_evidence=False,
                                      compare_verdicts=False), 0)
            # The live run still compares the document with the report.
            self.assertEqual(report.document_drift(payload, path), 1)

    def test_offline_contract_tolerates_a_stale_document_status(self):
        relative = "tmp/parity-selftest/workflow-test.MD"
        target = Path(__file__).resolve().parent.parent.parent / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("- `c1001` `ui_surface`: **BLOCKED**\n", encoding="utf-8")
        try:
            verdict, reason = extended.evaluate_drift(
                {"id": "c1201", "name": "drift", "document": relative},
                {"results": [{"id": "c1001", "verdict": "PASS"}]}, offline=True)
            self.assertEqual(verdict, "PASS")
            self.assertIn("live run", reason)
        finally:
            target.unlink(missing_ok=True)


class ShapeDiff(unittest.TestCase):
    def test_type_version_difference_is_named(self):
        n8n = {"nodes": [{"name": "Webhook", "type": "n8n-nodes-base.webhook", "typeVersion": 2.1,
                          "parameters": {}}], "connections": {}}
        m9m = {"nodes": [{"name": "Webhook", "type": "n8n-nodes-base.webhook", "typeVersion": 2,
                          "parameters": {}}], "connections": {}}
        differences = extended.shape_diff(n8n, m9m)
        self.assertTrue(any("typeVersion" in entry for entry in differences))

        ignored = extended.shape_diff(n8n, m9m, {"typeVersion"})
        self.assertEqual(ignored, [])

    def test_parameter_difference_is_named(self):
        n8n = {"nodes": [{"name": "Set", "type": "n8n-nodes-base.set", "parameters": {"a": 1}}],
               "connections": {}}
        m9m = {"nodes": [{"name": "Set", "type": "n8n-nodes-base.set", "parameters": {"a": 2}}],
               "connections": {}}
        differences = extended.shape_diff(n8n, m9m)
        self.assertTrue(any("parameter" in entry for entry in differences))


class SuiteInventory(unittest.TestCase):
    """The case files must cover C1..C12 and stay offline-safe."""

    def test_every_plan_suite_has_a_case_file(self):
        suites = {suite["suite"] for suite in runner.load_suites(None)}
        expected = {"credentials", "C1_auth", "C2_roundtrip", "C3_expressions",
                    "content_type", "C5_telemetry", "C6_sync", "C7_cron_async",
                    "negative", "C8_faults", "C9_stability", "C10_ui",
                    "C11_regression", "C12_drift"}
        self.assertTrue(expected.issubset(suites), expected - suites)

    def test_offline_cases_do_not_need_the_shared_hosts(self):
        for suite in runner.load_suites(None):
            for case in suite["cases"]:
                if not case.get("offline"):
                    continue
                self.assertIn(case.get("check"),
                              {"go_test", "load", "local_error", "ui_surface", "ui_journey", "drift"})

    def test_body_hash_is_recorded_for_every_case(self):
        for suite in runner.load_suites(None):
            for case in suite["cases"]:
                if case.get("body"):
                    self.assertTrue(runner.body_signature(case))

class UiJourneyArtifact(unittest.TestCase):
    """C10 - the recorded browser journey artifact drives the verdict."""

    def write(self, payload):
        directory = tempfile.mkdtemp()
        path = Path(directory) / "journey.json"
        path.write_text(json.dumps(payload), encoding="utf-8")
        self.addCleanup(lambda: path.unlink(missing_ok=True))
        return path

    def call(self, payload):
        path = self.write(payload)
        with patch.object(extended, "ROOT", path.parent):
            return extended._contract_ui_journey({"artifact": path.name})

    def test_all_steps_pass_is_pass(self):
        verdict, reason = self.call({"steps": [{"name": "a", "result": "PASS"},
                                               {"name": "b", "result": "PASS"}]})
        self.assertEqual(verdict, "PASS")
        self.assertIn("2 recorded UI journey steps", reason)

    def test_blocked_step_is_blocked_not_fail(self):
        verdict, reason = self.call({"steps": [{"name": "a", "result": "PASS"},
                                               {"name": "preview", "result": "BLOCKED",
                                                "missing_piece": "web/src ParameterField.vue"}]})
        self.assertEqual(verdict, "BLOCKED")
        self.assertIn("ParameterField.vue", reason)

    def test_failed_step_fails(self):
        verdict, _ = self.call({"steps": [{"name": "a", "result": "FAIL"}]})
        self.assertEqual(verdict, "FAIL")

    def test_unknown_step_result_fails(self):
        verdict, _ = self.call({"steps": [{"name": "a", "result": "MAYBE"}]})
        self.assertEqual(verdict, "FAIL")

    def test_empty_artifact_fails(self):
        verdict, _ = self.call({"steps": []})
        self.assertEqual(verdict, "FAIL")

    def test_missing_artifact_is_blocked(self):
        with patch.object(extended, "ROOT", Path(tempfile.mkdtemp())):
            verdict, reason = extended._contract_ui_journey({"artifact": "absent.json"})
        self.assertEqual(verdict, "BLOCKED")
        self.assertIn("no recorded browser journey", reason)


class LiveContractFixtures(unittest.TestCase):
    """Contracts that must judge the shared hosts correctly (no network)."""

    def test_empty_collection_envelope_is_not_a_parse_error(self):
        self.assertEqual(extended.rows({"data": None, "total": 0}, "executions"), [])
        self.assertEqual(extended.rows({"executions": None}, "executions"), [])
        self.assertEqual(extended.rows({"data": [1]}, "executions"), [1])

    def test_auth_probe_requires_an_anonymous_n8n_rejection(self):
        def fake_http(url, method="GET", payload=None, headers=None, timeout=30):
            headers = headers or {}
            if "/api/v1/workflows" in url and "n8n" in url:
                status = 200 if headers.get("X-N8N-API-KEY") else 401
                return status, {"data": []}, "{}"
            if url.startswith("http://m9m") and "/api/v1" in url:
                # The m9m API is closed by default: 401 without a key.
                status = 200 if headers.get("X-API-Key") else 401
                return status, {"data": []}, "{}"
            return 200, {"data": []}, "{}"

        case = {"paths": ["/healthz", "/api/v1/node-types", "/api/v1/workflows"],
                "protected_paths": ["/api/v1/workflows"]}
        with patch.object(extended, "_http", side_effect=fake_http):
            verdict, reason = extended._contract_auth_probe(
                case, "http://n8n", "http://m9m", 1)
        self.assertEqual(verdict, "PASS")
        self.assertIn("rejects anonymous", reason)

    def test_auth_probe_fails_when_m9m_serves_api_without_a_key(self):
        def fake_http(url, method="GET", payload=None, headers=None, timeout=30):
            headers = headers or {}
            if "/api/v1/workflows" in url and "n8n" in url:
                status = 200 if headers.get("X-N8N-API-KEY") else 401
                return status, {"data": []}, "{}"
            return 200, {"data": []}, "{}"

        case = {"paths": ["/healthz", "/api/v1/node-types", "/api/v1/workflows"],
                "protected_paths": ["/api/v1/workflows"]}
        with patch.object(extended, "_http", side_effect=fake_http):
            with self.assertRaises(ValueError):
                extended._contract_auth_probe(case, "http://n8n", "http://m9m", 1)

    def test_auth_probe_accepts_the_legacy_m9m_open_expectation(self):
        def fake_http(url, method="GET", payload=None, headers=None, timeout=30):
            headers = headers or {}
            if "/api/v1/workflows" in url and "n8n" in url:
                status = 200 if headers.get("X-N8N-API-KEY") else 401
                return status, {"data": []}, "{}"
            return 200, {"data": []}, "{}"

        case = {"paths": ["/healthz", "/api/v1/node-types", "/api/v1/workflows"],
                "protected_paths": ["/api/v1/workflows"],
                "expect": "m9m-open"}
        with patch.object(extended, "_http", side_effect=fake_http):
            verdict, reason = extended._contract_auth_probe(
                case, "http://n8n", "http://m9m", 1)
        self.assertEqual(verdict, "PASS")
        self.assertIn("without any key", reason)

    def test_auth_probe_fails_when_n8n_accepts_anonymous_calls(self):
        def fake_http(url, method="GET", payload=None, headers=None, timeout=30):
            return 200, {"data": []}, "{}"

        case = {"paths": ["/api/v1/workflows"],
                "protected_paths": ["/api/v1/workflows"]}
        with patch.object(extended, "_http", side_effect=fake_http):
            with self.assertRaises(ValueError):
                extended._contract_auth_probe(case, "http://n8n", "http://m9m", 1)

    def test_cron_without_any_execution_is_blocked_not_failed(self):
        def api(host, path, method="GET", payload=None, redact=True):
            if path.startswith("workflows"):
                return {"data": [{"id": "w1", "name": "Simple CronWorkflow"}]}
            return {"data": []}

        verdict, reason = extended._contract_cron(
            {"workflow_name": "Simple CronWorkflow", "poll_seconds": 0.1}, api, None)
        self.assertEqual(verdict, "BLOCKED")
        self.assertIn("either host", reason)

    def test_environment_blocked_case_is_blocked_when_hosts_agree(self):
        def fake_request(url, case, timeout):
            return {"status_code": 403, "headers": {}, "body_raw": "{}",
                    "elapsed_ms": 1, "error": None}

        case = {"id": "pXX", "name": "fixture", "path": "x", "method": "POST",
                "expected_status": 200, "compare": "json",
                "environment_blocked": "the fixture credential is not available"}
        with patch.object(runner, "_request", side_effect=fake_request):
            result = runner.run_case(case, "http://n8n", "http://m9m", 1, 0)
        self.assertEqual(result["verdict"], "BLOCKED")
        self.assertIn("fixture credential", result["reason"])

    def test_environment_blocked_case_still_fails_when_hosts_differ(self):
        def fake_request(url, case, timeout):
            status = 403 if "n8n" in url else 500
            return {"status_code": status, "headers": {}, "body_raw": "{}",
                    "elapsed_ms": 1, "error": None}

        case = {"id": "pXX", "name": "fixture", "path": "x", "method": "POST",
                "expected_status": 200, "compare": "json",
                "environment_blocked": "the fixture credential is not available"}
        with patch.object(runner, "_request", side_effect=fake_request):
            result = runner.run_case(case, "http://n8n", "http://m9m", 1, 0)
        self.assertEqual(result["verdict"], "FAIL")


if __name__ == "__main__":
    unittest.main()
