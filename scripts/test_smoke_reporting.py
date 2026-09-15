"""Require every Phase 29 operation in the compiled security smoke.

The expected set is independent of the smoke's anchor constant. The shared
request fixture tests the checker only; full CI separately probes the real
compiled binary and its actual OpenAPI registry, in both serve and MCP modes.
"""
import re
import unittest

import smoke_foundation as smoke
import test_smoke_foundation as foundation_tests


REPORTING_ROUTES = frozenset({
    ("GET", "/v1/reports"),
    ("POST", "/v1/reports"),
    ("POST", "/v1/reports/import"),
    ("GET", "/v1/reports/{id}"),
    ("PUT", "/v1/reports/{id}"),
    ("POST", "/v1/reports/{id}/review"),
    ("POST", "/v1/reports/{id}/publish"),
    ("POST", "/v1/reports/{id}/reject"),
    ("POST", "/v1/reports/{id}/archive"),
    ("POST", "/v1/reports/{id}/runs"),
    ("GET", "/v1/dashboards"),
    ("POST", "/v1/dashboards"),
    ("POST", "/v1/dashboards/import"),
    ("GET", "/v1/dashboards/{id}"),
    ("PUT", "/v1/dashboards/{id}"),
    ("POST", "/v1/dashboards/{id}/review"),
    ("POST", "/v1/dashboards/{id}/publish"),
    ("POST", "/v1/dashboards/{id}/reject"),
    ("POST", "/v1/dashboards/{id}/archive"),
    ("POST", "/v1/dashboards/{id}/runs"),
    ("GET", "/v1/composition-runs/{id}"),
    ("GET", "/v1/composition-runs/{id}/receipt"),
    ("POST", "/v1/composition-runs/{id}/execute"),
    ("GET", "/v1/composition-runs/{id}/widget"),
    ("POST", "/v1/composition-runs/{id}/cancel"),
    ("POST", "/v1/composition-retention"),
})


class ReportingSmokeSecurityTest(unittest.TestCase):
    def probe(self, *args, **kwargs):
        return foundation_tests.SmokeRouteSecurityTest().run_probe(*args, **kwargs)

    def test_all_26_operations_are_required_in_both_modes(self):
        self.assertEqual(len(REPORTING_ROUTES), 26)
        self.assertTrue(REPORTING_ROUTES <= smoke.REQUIRED_PROTECTED_ROUTES)
        for _, path in REPORTING_ROUTES:
            self.assertNotIn(path, smoke.ABSENT_ROUTES)
        for enabled in (False, True):
            with self.subTest(mcp_enabled=enabled):
                calls = self.probe(mcp_enabled=enabled)
                for method, path in REPORTING_ROUTES:
                    concrete = path.replace("{id}", "smoke-missing-resource")
                    self.assertIn((method, concrete, None), calls)
                    self.assertIn((method, concrete,
                                   {"Cookie": "token=pretend", "X-Principal": "pretend"}), calls)

    def test_each_operation_is_required_even_when_others_remain(self):
        for method, path in sorted(REPORTING_ROUTES):
            doc = foundation_tests.document()
            del doc["paths"][path][method.lower()]
            with self.subTest(method=method, path=path):
                with self.assertRaisesRegex(RuntimeError, "required protected route missing"):
                    self.probe(doc)

    def test_each_operation_rejects_success_and_missing_handler(self):
        for method, path in sorted(REPORTING_ROUTES):
            concrete = path.replace("{id}", "smoke-missing-resource")
            for status in (200, 404):
                with self.subTest(method=method, path=path, status=status):
                    with self.assertRaisesRegex(RuntimeError, f"expected 401, got {status}"):
                        self.probe(override=lambda actual_method, actual_path, _: (status, {})
                                   if (actual_method, actual_path) == (method, concrete) else None)

    def test_each_operation_rejects_spoofed_identity(self):
        for method, path in sorted(REPORTING_ROUTES):
            concrete = path.replace("{id}", "smoke-missing-resource")
            with self.subTest(method=method, path=path):
                with self.assertRaisesRegex(RuntimeError, "expected 401, got 200"):
                    self.probe(override=lambda actual_method, actual_path, headers: (200, {})
                               if headers and (actual_method, actual_path) == (method, concrete) else None)

    def test_each_operation_requires_all_security_metadata(self):
        for method, path in sorted(REPORTING_ROUTES):
            for field in ("security", "x-chartworks-auth", "x-chartworks-action"):
                doc = foundation_tests.document()
                del doc["paths"][path][method.lower()][field]
                with self.subTest(method=method, path=path, field=field):
                    with self.assertRaisesRegex(RuntimeError, "incomplete protected registration"):
                        self.probe(doc)

    def test_each_operation_requires_safe_authentication_errors(self):
        for method, path in sorted(REPORTING_ROUTES):
            concrete = path.replace("{id}", "smoke-missing-resource")
            with self.subTest(method=method, path=path):
                with self.assertRaisesRegex(RuntimeError, re.escape(f"unsafe authentication error: {method} {path}")):
                    self.probe(override=lambda actual_method, actual_path, _: (401, {"error": "private-canary"})
                               if (actual_method, actual_path) == (method, concrete) else None)


if __name__ == "__main__":
    unittest.main()
