#!/usr/bin/env python3
"""Strict package coverage from one full real-driver, race-enabled test suite."""
from __future__ import annotations

from collections import defaultdict
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile


def bands(text: str) -> dict[str, int]:
    result = {}
    for line in text.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) != 2 or not re.fullmatch(r"(?:internal|cmd|sdk|eval)(?:/[A-Za-z0-9_-]+)+", parts[0]):
            raise ValueError("invalid coverage band")
        name, minimum = parts[0], int(parts[1])
        if name in result or not 1 <= minimum <= 100:
            raise ValueError("duplicate or invalid coverage threshold")
        result[name] = minimum
    if not result:
        raise ValueError("implemented code requires coverage bands")
    return result


def measure(text: str, module: str, packages: set[str]) -> dict[str, tuple[int, int]]:
    lines = text.splitlines()
    if not lines or lines[0] != "mode: atomic":
        raise ValueError("race-compatible atomic coverage profile required")
    blocks = {}
    for line in lines[1:]:
        fields = line.rsplit(" ", 2)
        if len(fields) != 3 or ":" not in fields[0]:
            raise ValueError("malformed coverage block")
        location, statements, count = fields[0], int(fields[1]), int(fields[2])
        if statements < 0 or count < 0:
            raise ValueError("negative coverage value")
        previous = blocks.get(location)
        if previous and previous[0] != statements:
            raise ValueError("inconsistent duplicate coverage block")
        blocks[location] = (statements, bool(count) or bool(previous and previous[1]))
    totals = defaultdict(lambda: [0, 0])
    for location, (statements, covered) in blocks.items():
        filename = location.rsplit(":", 1)[0]
        if not filename.startswith(module + "/"):
            continue
        package = filename[len(module) + 1:].rsplit("/", 1)[0]
        if package in packages:
            totals[package][0] += statements if covered else 0
            totals[package][1] += statements
    if set(totals) != packages or any(total == 0 for _, total in totals.values()):
        raise ValueError("missing instrumented package coverage")
    return {name: tuple(value) for name, value in totals.items()}


def command(args: list[str], root: Path, *, capture: bool = False) -> str:
    result = subprocess.run(args, cwd=root, env=dict(os.environ, CGO_ENABLED="1"),
                            text=True, stdout=subprocess.PIPE if capture else None,
                            timeout=900, check=False)
    if result.returncode:
        raise ValueError("coverage command failed; no package may be silently skipped")
    return result.stdout.strip() if capture else ""


def main() -> int:
    try:
        root = Path(__file__).resolve().parents[1]
        limits = bands((root / "scripts/coverage-bands.conf").read_text())
        module = command(["go", "list", "-m"], root, capture=True)
        listed = command(["go", "list", "./..."], root, capture=True).splitlines()
        packages = {name[len(module) + 1:] for name in listed if name.startswith(module + "/") and name[len(module) + 1:].split("/", 1)[0] in ("internal", "cmd", "sdk", "eval")}
        if not packages or packages != set(limits):
            raise ValueError("production package inventory and exact coverage bands disagree")
        with tempfile.TemporaryDirectory(prefix="chartworks-coverage-") as directory:
            profile = Path(directory) / "coverage.out"
            targets = ",".join(module + "/" + name for name in sorted(packages))
            try:
                command(["go", "test", "-race", "-count=1", "-timeout=10m", "-covermode=atomic",
                         "-coverpkg=" + targets, "-coverprofile=" + str(profile), "./..."], root)
                totals = measure(profile.read_text(), module, packages)
            finally:
                # Preserve real instrumentation, including failed-suite evidence, when CI asks.
                # This does not change test selection, thresholds or failure propagation.
                destination = os.environ.get("CHARTWORKS_COVERAGE_OUTPUT")
                if destination and profile.is_file():
                    shutil.copyfile(profile, destination)
        failed = False
        for name in sorted(packages):
            covered, total = totals[name]
            passed = covered * 100 >= limits[name] * total
            failed |= not passed
            print(f"{'OK' if passed else 'FAIL'}: {name} {100 * covered / total:.2f}% ({covered}/{total}), required {limits[name]}%")
        return int(failed)
    except (OSError, ValueError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: coverage: {type(error).__name__}: coverage unavailable or invalid", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
