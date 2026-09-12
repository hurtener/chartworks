#!/usr/bin/env python3
"""Run the compiled foundation against the explicitly configured disposable test DB.

No business credentials are created. The unreachable local HTTPS key fixture must
make readiness fail while liveness remains healthy. Never run against a customer DB.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def request(url: str, method: str = "GET", headers: dict | None = None) -> tuple[int, dict]:
    try:
        with urlopen(Request(url, method=method, headers=headers or {}), timeout=1) as response:
            payload = response.read()
            return response.status, json.loads(payload) if payload else {}
    except HTTPError as error:
        with error:
            payload = error.read(8192)
            return error.code, json.loads(payload) if payload else {}


# These capabilities are always enabled in this smoke configuration. Anchors
# prevent a missing/empty registry from turning an enumeration into a false pass.
REQUIRED_PROTECTED_ROUTES = frozenset({
    ("GET", "/metrics"),
    ("GET", "/v1/retention-policy"),
    ("PUT", "/v1/retention-policy"),
    ("GET", "/v1/audit-events"),
    ("POST", "/v1/retention-sweeps"),
    ("GET", "/v1/access/diagnostics"),
    ("GET", "/v1/charts/catalog"),
    ("POST", "/v1/charts/select"),
    ("POST", "/v1/charts/specify"),
    ("POST", "/v1/charts/build"),
    ("POST", "/v1/charts/rebind"),
    # Phase 29 mounts these services without a model/source credential or MCP.
    # Require every operation, not just the collections, so a missing protected
    # handler cannot pass merely because it disappeared from OpenAPI.
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
PUBLIC_ROUTES = frozenset((method, path)
                          for method in ("GET", "HEAD")
                          for path in ("/healthz", "/readyz", "/capabilities", "/openapi.json"))
# Legacy MCP paths and local credential issuance remain absent. Implemented
# report/dashboard routes above must return 401, never an interchangeable 404.
ABSENT_ROUTES = ("/mcp", "/v1/admin/keys", "/v1/unregistered-smoke-route")


def verify_route_security(base: str, *, mcp_enabled: bool = False) -> None:
    required = REQUIRED_PROTECTED_ROUTES | ({("POST", "/v1/mcp")} if mcp_enabled else set())
    absent = ABSENT_ROUTES + (() if mcp_enabled else ("/v1/mcp",))
    code, document = request(base + "/openapi.json")
    paths = document.get("paths")
    if code != 200 or not isinstance(paths, dict) or not paths:
        raise RuntimeError("compiled OpenAPI paths unavailable or empty")
    for path in absent:
        if path in paths:
            raise RuntimeError(f"unimplemented route advertised: {path}")
    protected = set()
    for path, operations in sorted(paths.items()):
        for method, operation in sorted(operations.items()):
            method = method.upper()
            if (method, path) in PUBLIC_ROUTES:
                if operation.get("x-chartworks-auth") != "none" or operation.get("security"):
                    raise RuntimeError(f"incorrect public registration: {method} {path}")
                continue
            if (operation.get("x-chartworks-auth") != "bearer"
                    or operation.get("security") != [{"penguiBearer": []}]
                    or not operation.get("x-chartworks-action")):
                raise RuntimeError(f"incomplete protected registration: {method} {path}")
            protected.add((method, path))
            concrete = re.sub(r"\{[^{}]+\}", "smoke-missing-resource", path)
            # Neither a missing bearer nor spoofed cookie/header identity may
            # reach decoding or resource lookup, even with keys unavailable.
            for headers in (None, {"Cookie": "token=pretend", "X-Principal": "pretend"}):
                status, body = request(base + concrete, method, headers)
                if status != 401:
                    raise RuntimeError(f"{method} {path}: expected 401, got {status}")
                if method != "HEAD" and body != {"error": "unauthorized"}:
                    raise RuntimeError(f"unsafe authentication error: {method} {path}")
    missing = required - protected
    if missing:
        raise RuntimeError(f"required protected route missing: {sorted(missing)}")
    # An absent route is not a protected capability. Never accept 401-or-404
    # interchangeably: registered routes above require 401, absent routes 404.
    for path in absent:
        status, body = request(base + path)
        if status != 404 or body != {"error": "not_found"}:
            raise RuntimeError(f"{path}: expected 404, got {status}")
    print(f"OK: {len(protected)} registered protected operations reject unauthenticated access; "
          f"{len(absent)} absent routes return 404")


def verify_process(binary: Path, command: str, config: Path, env: dict,
                   address: str, dsn: str, *, mcp_enabled: bool) -> None:
    started = time.perf_counter()
    process = subprocess.Popen([str(binary), command, "--config", str(config)], env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        deadline = time.monotonic() + 15
        while True:
            if process.poll() is not None:
                raise RuntimeError("foundation exited before liveness")
            try:
                code, body = request("http://" + address + "/healthz")
                if code == 200 and body.get("live") is True:
                    break
            except (URLError, TimeoutError, OSError):
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError("compiled binary did not become live")
            time.sleep(0.02)
        print(f"MEASURED: fresh process to observed liveness_ms={(time.perf_counter() - started) * 1000:.2f}")
        code, body = request("http://" + address + "/readyz")
        if code != 503 or body.get("ready") is not False:
            raise RuntimeError("unavailable verification keys incorrectly reported ready")
        code, body = request("http://" + address + "/capabilities")
        if code != 200 or body.get("business_api") is not True or body.get("authentication") is not True:
            raise RuntimeError("incorrect implemented authority capability")
        implemented = body.get("implemented")
        if not isinstance(implemented, list) or ("mcp" in implemented) != mcp_enabled:
            raise RuntimeError("MCP capability does not match the installed transport")
        verify_route_security("http://" + address, mcp_enabled=mcp_enabled)
        status = Path(f"/proc/{process.pid}/status")
        if status.is_file():
            for line in status.read_text().splitlines():
                if line.startswith(("VmRSS:", "Threads:")):
                    print("MEASURED: foundation idle " + line)
        process.send_signal(signal.SIGTERM)
        stdout, stderr = process.communicate(timeout=12)
        if process.returncode != 0 or dsn.encode() in stdout + stderr:
            raise RuntimeError("compiled shutdown/redaction smoke failed")
        try:
            request("http://" + address + "/healthz")
        except (URLError, OSError):
            pass
        else:
            raise RuntimeError("listener survived process shutdown")
        print(f"OK: compiled {command}, MCP enabled={mcp_enabled}, health, negative routes, redaction and SIGTERM shutdown")
    finally:
        if process.poll() is None:
            process.kill()
            process.communicate(timeout=5)


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    dsn = os.environ.get("CHARTWORKS_TEST_STORE_URL", "")
    binary = root / "bin/chartworks"
    if not dsn or not binary.is_file():
        raise RuntimeError("built binary and explicit disposable test database are required")
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        address = "127.0.0.1:" + str(sock.getsockname()[1])
    document = {
        "server": {"listen": address},
        "auth": {"issuer": "https://127.0.0.1/issuer", "audience": "foundation-smoke",
                 "jwks_url": "https://127.0.0.1:1/keys", "request_timeout": "100ms"},
        "store": {"dsn": "env:CHARTWORKS_STORE_URL"},
    }
    env = dict(os.environ, CHARTWORKS_STORE_URL=dsn)
    with tempfile.TemporaryDirectory(prefix="chartworks-smoke-") as directory:
        config = Path(directory) / "foundation.json"
        config.write_text(json.dumps(document))
        config.chmod(0o600)
        for args, expected in ((["version"], 0), (["config-check", "--config", str(config)], 0), (["mcp", "--config", str(config)], 2)):
            result = subprocess.run([str(binary), *args], env=env, capture_output=True, timeout=10, check=False)
            if result.returncode != expected or dsn.encode() in result.stdout + result.stderr:
                raise RuntimeError("compiled command exit/redaction smoke failed")
            if args[0] == "mcp" and b"features.mcp=true" not in result.stderr:
                raise RuntimeError("disabled MCP did not reject startup at its feature gate")
        verify_process(binary, "serve", config, env, address, dsn, mcp_enabled=False)
        # Exercise the actual MCP command and common composition root, not a stub
        # Starter. Charts are real services requiring no model/source credentials.
        document["features"] = {"mcp": True}
        document["mcp"] = {"groups": ["charts"]}
        config.write_text(json.dumps(document))
        verify_process(binary, "mcp", config, env, address, dsn, mcp_enabled=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
