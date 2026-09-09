"""Regression tests for the compiled smoke's registered/absent route contract."""
import copy
import unittest
from unittest.mock import patch
from urllib.request import Request

import smoke_foundation as smoke


def document(mcp_enabled=False):
    paths = {}
    for method, path in smoke.REQUIRED_PROTECTED_ROUTES:
        paths.setdefault(path, {})[method.lower()] = {
            "x-chartworks-auth": "bearer",
            "x-chartworks-action": "fixture.read",
            "security": [{"penguiBearer": []}],
        }
    if mcp_enabled:
        paths["/v1/mcp"] = {"post": {
            "x-chartworks-auth": "bearer", "x-chartworks-action": "mcp.use",
            "security": [{"penguiBearer": []}],
        }}
    paths["/healthz"] = {"get": {"x-chartworks-auth": "none"}}
    paths["/v1/fixture/{id}"] = {"delete": {
        "x-chartworks-auth": "bearer", "x-chartworks-action": "fixture.delete",
        "security": [{"penguiBearer": []}],
    }}
    return {"openapi": "3.1.1", "paths": paths}


class SmokeRouteSecurityTest(unittest.TestCase):
    def run_probe(self, doc=None, override=None, *, mcp_enabled=False):
        doc = document(mcp_enabled) if doc is None else doc
        calls = []

        def request(url, method="GET", headers=None):
            path = url.removeprefix("http://localhost")
            calls.append((method, path, headers))
            if path == "/openapi.json":
                return 200, doc
            if override is not None:
                result = override(method, path, headers)
                if result is not None:
                    return result
            if path in smoke.ABSENT_ROUTES or (path == "/v1/mcp" and not mcp_enabled):
                return 404, {"error": "not_found"}
            return 401, {"error": "unauthorized"}

        with patch.object(smoke, "request", side_effect=request):
            smoke.verify_route_security("http://localhost", mcp_enabled=mcp_enabled)
        return calls

    def test_registered_denied_and_absent_not_found(self):
        calls = self.run_probe()
        protected = [(method, path, headers) for method, path, headers in calls
                     if path not in ("/openapi.json", "/v1/mcp") and path not in smoke.ABSENT_ROUTES]
        self.assertEqual(len(protected), 2 * (len(smoke.REQUIRED_PROTECTED_ROUTES) + 1))
        self.assertIn(("DELETE", "/v1/fixture/smoke-missing-resource", None), calls)
        self.assertTrue(any(headers and "Cookie" in headers for _, _, headers in protected))
        self.assertFalse(any("{" in path for _, path, _ in calls))

    def test_disabled_mcp_is_absent(self):
        self.assertIn(("GET", "/v1/mcp", None), self.run_probe())
        with self.assertRaisesRegex(RuntimeError, "unimplemented route advertised"):
            self.run_probe(document(mcp_enabled=True))
        with self.assertRaisesRegex(RuntimeError, r"/v1/mcp.*expected 404, got 401"):
            self.run_probe(override=lambda method, path, _: (401, {}) if path == "/v1/mcp" else None)

    def test_enabled_mcp_is_required_and_protected(self):
        calls = self.run_probe(mcp_enabled=True)
        self.assertIn(("POST", "/v1/mcp", None), calls)
        self.assertTrue(any(path == "/v1/mcp" and headers for _, path, headers in calls))
        with self.assertRaisesRegex(RuntimeError, "required protected route missing"):
            self.run_probe(document(), mcp_enabled=True)
        for status in (200, 404):
            with self.subTest(status=status), self.assertRaisesRegex(RuntimeError, "expected 401"):
                self.run_probe(mcp_enabled=True, override=lambda method, path, _: (status, {})
                               if path == "/v1/mcp" else None)

    def test_protected_404_is_not_accepted(self):
        with self.assertRaisesRegex(RuntimeError, r"GET /metrics.*expected 401, got 404"):
            self.run_probe(override=lambda method, path, _: (404, {}) if path == "/metrics" else None)

    def test_spoofed_identity_cannot_succeed(self):
        with self.assertRaisesRegex(RuntimeError, "expected 401, got 200"):
            self.run_probe(override=lambda method, path, headers: (200, {}) if headers else None)

    def test_new_protected_operation_is_also_checked(self):
        with self.assertRaisesRegex(RuntimeError, r"DELETE /v1/fixture/\{id\}"):
            self.run_probe(override=lambda method, path, _: (200, {}) if method == "DELETE" else None)

    def test_absent_route_401_is_not_accepted(self):
        with self.assertRaisesRegex(RuntimeError, r"/mcp.*expected 404, got 401"):
            self.run_probe(override=lambda method, path, _: (401, {}) if path == "/mcp" else None)

    def test_absent_route_cannot_be_advertised(self):
        doc = document()
        doc["paths"]["/v1/admin/keys"] = copy.deepcopy(doc["paths"]["/metrics"])
        with self.assertRaisesRegex(RuntimeError, "unimplemented route advertised"):
            self.run_probe(doc)

    def test_missing_required_route_cannot_pass(self):
        doc = document()
        del doc["paths"]["/v1/charts/build"]
        with self.assertRaisesRegex(RuntimeError, "required protected route missing"):
            self.run_probe(doc)
        with self.assertRaisesRegex(RuntimeError, "OpenAPI paths"):
            self.run_probe({"paths": {}})

    def test_business_route_cannot_be_relabeled_public(self):
        doc = document()
        doc["paths"]["/metrics"]["get"] = {"x-chartworks-auth": "none"}
        with self.assertRaisesRegex(RuntimeError, "protected registration"):
            self.run_probe(doc)

    def test_incomplete_security_contract_fails(self):
        for field in ("security", "x-chartworks-action", "x-chartworks-auth"):
            doc = document()
            del doc["paths"]["/metrics"]["get"][field]
            with self.subTest(field=field), self.assertRaisesRegex(RuntimeError, "protected registration"):
                self.run_probe(doc)

    def test_safe_error_body_required(self):
        with self.assertRaisesRegex(RuntimeError, "unsafe authentication error"):
            self.run_probe(override=lambda method, path, _: (401, {"error": "private-data"})
                           if path == "/metrics" else None)

    def test_request_preserves_method_and_spoof_headers(self):
        from io import BytesIO
        from urllib.error import HTTPError
        error = HTTPError("http://localhost/v1/fixture/missing", 401, "Unauthorized", {},
                          BytesIO(b'{"error":"unauthorized"}'))
        with patch.object(smoke, "urlopen", side_effect=error) as opened:
            code, body = smoke.request(error.url, "DELETE", {"Cookie": "token=pretend"})
        self.assertEqual((code, body), (401, {"error": "unauthorized"}))
        req = opened.call_args.args[0]
        self.assertIsInstance(req, Request)
        self.assertEqual(req.get_method(), "DELETE")
        self.assertEqual(req.get_header("Cookie"), "token=pretend")


if __name__ == "__main__":
    unittest.main()
