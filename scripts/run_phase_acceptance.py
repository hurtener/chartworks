#!/usr/bin/env python3
"""Run named Go acceptance tests; never count missing/skipped subtests as passes."""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile

from planning_check import read_json, topological


def validate_events(lines, phase: str, count: int, exit_code: int) -> list[str]:
    if exit_code:
        raise ValueError(f"go test exited {exit_code}")
    parent = f"TestPhase{phase}"
    expected = {f"{parent}/AC{i:02d}" for i in range(1, count + 1)}
    passed, parent_passes, packages = set(), 0, set()
    for line in lines:
        if not line.strip():
            continue
        event = json.loads(line)
        if not isinstance(event, dict):
            raise ValueError("invalid Go test event")
        test, action = event.get("Test", ""), event.get("Action")
        if action == "fail":
            raise ValueError(f"failed test/package: {test or event.get('Package')}")
        if not (test == parent or test.startswith(parent + "/")):
            continue
        packages.add(event.get("Package", ""))
        if action == "skip":
            raise ValueError(f"skipped acceptance: {test}")
        if test == parent:
            if action == "pass":
                parent_passes += 1
            continue
        criterion = "/".join(test.split("/")[:2])
        if criterion not in expected:
            raise ValueError(f"undeclared acceptance: {test}")
        if action == "pass" and test == criterion:
            if criterion in passed:
                raise ValueError(f"duplicate acceptance: {criterion}")
            passed.add(criterion)
    if parent_passes != 1 or len(packages) != 1 or not next(iter(packages), ""):
        raise ValueError(f"missing/ambiguous {parent} parent result")
    missing = expected - passed
    if missing:
        raise ValueError("missing acceptance pass events: " + ", ".join(sorted(missing)))
    return sorted(passed)


def run(root: Path, phase: str, row: dict, release: bool) -> bool:
    """Return True for passed runtime acceptance; False for an explicit planned skip."""
    if release and row["status"] != "shipped":
        raise ValueError(f"phase {phase} is {row['status']}, not reviewed shipped")
    files = list((root / "test/acceptance").glob("*_test.go"))
    declared = any(re.search(r"\bfunc\s+TestPhase" + phase + r"\s*\(",
                             path.read_text(encoding="utf-8")) for path in files)
    if row["status"] == "planned":
        if declared:
            raise ValueError(f"phase {phase} has acceptance code but is still marked planned")
        if not release and os.environ.get("CHARTWORKS_ALLOW_PLANNED_SKIP") == "1":
            print(f"SKIP: phase {phase} unimplemented; planning is not acceptance")
            return False
        raise ValueError(f"phase {phase} is unimplemented; no runtime acceptance")
    if not (root / "go.mod").is_file() or not declared:
        raise ValueError(f"phase {phase}: Go module or named acceptance tests missing")
    seconds = int(os.environ.get("CHARTWORKS_ACCEPTANCE_TIMEOUT_SECONDS", "900"))
    if not 1 <= seconds <= 7200:
        raise ValueError("acceptance timeout must be between 1 and 7200 seconds")
    command = ["go", "test", "-race", "-count=1", "-json", f"-timeout={seconds}s",
               "./test/acceptance", "-run", f"^TestPhase{phase}$"]
    environment = dict(os.environ, CGO_ENABLED="1")
    with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as output:
        process = subprocess.Popen(command, cwd=root, env=environment, stdout=output,
                                   stderr=subprocess.PIPE, text=True, start_new_session=True)
        try:
            _, stderr = process.communicate(timeout=seconds + 30)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.communicate()
            raise ValueError(f"phase {phase}: acceptance command timed out") from None
        if stderr:
            print(stderr, file=sys.stderr, end="")
        output.seek(0)
        try:
            passed = validate_events(output, phase, row["acceptance_count"], process.returncode)
        except (ValueError, json.JSONDecodeError):
            output.seek(0)
            print(output.read()[-16000:], file=sys.stderr)
            raise
    for test in passed:
        print(f"OK: {test}")
    return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--phase")
    group.add_argument("--all", action="store_true")
    parser.add_argument("--release", action="store_true")
    args = parser.parse_args()
    try:
        root = args.root.resolve()
        phases = read_json(root / "docs/plans/phase-registry.json")["phases"]
        selected = topological(phases) if args.all else [str(args.phase).zfill(2)]
        passed = skipped = 0
        for phase in selected:
            if phase not in phases:
                raise ValueError(f"unknown phase {phase}")
            if run(root, phase, phases[phase], args.release):
                passed += 1
            else:
                skipped += 1
        print(f"ACCEPTANCE: passed_phases={passed} unimplemented_skips={skipped}")
        if skipped:
            print("NOTE: development preflight only; release acceptance has NOT passed.")
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"FAIL: acceptance: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
