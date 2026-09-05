#!/usr/bin/env python3
"""Run the compiled foundation against the explicitly configured disposable test DB.

No business credentials are created. The unreachable local HTTPS key fixture must
make readiness fail while liveness remains healthy. Never run against a customer DB.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import urlopen


def request(url: str) -> tuple[int, dict]:
    try:
        with urlopen(url, timeout=1) as response:
            return response.status, json.load(response)
    except HTTPError as error:
        with error:
            payload = error.read(8192)
            return error.code, json.loads(payload) if payload else {}


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
        for args, expected in ((["version"], 0), (["config-check", "--config", str(config)], 0), (["mcp"], 3)):
            result = subprocess.run([str(binary), *args], env=env, capture_output=True, timeout=10, check=False)
            if result.returncode != expected or dsn.encode() in result.stdout + result.stderr:
                raise RuntimeError("compiled command exit/redaction smoke failed")
        started = time.perf_counter()
        process = subprocess.Popen([str(binary), "serve", "--config", str(config)], env=env,
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
            if code != 200 or body.get("business_api") is not False or body.get("authentication") is not False:
                raise RuntimeError("unimplemented business/auth capability advertised")
            for path in ("/metrics", "/mcp", "/v1/admin/keys", "/v1/reports"):
                if request("http://" + address + path)[0] != 404:
                    raise RuntimeError("unprotected or unimplemented route exposed")
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
            print("OK: compiled commands, health, negative routes, redaction and SIGTERM shutdown")
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate(timeout=5)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
