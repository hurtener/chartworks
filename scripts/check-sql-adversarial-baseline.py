#!/usr/bin/env python3
"""Reproduce exact historical regressions without treating a build failure as proof.

Runs only ten new tests, copied into an isolated worktree at the pinned review
baseline. The working implementation is not altered. Expected assertion failures
are recorded separately from the current-head passing suites.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

BASE = "c8c0a8db9327b147f313830037fcc56353536909"
CASES = {
    "TestSQLRecoveryAdversarialOnlyCannotChangePopulation": "physical-parent-only scan acquired a complete relation proof",
    "TestSQLRecoveryAdversarialUnknownLiteralCannotProveDecimalDivision": "truncated integer result acquired a decimal-ratio proof",
    "TestSQLRecoveryAdversarialBigintParserStorageIsNotNumericType": "bigint fval acquired a decimal-division proof",
    "TestSQLRecoveryAdversarialDurableFailureReceiptGatesCorrection": "durable stopped query failure could not reach correction",
    "TestSQLRecoveryAdversarialRepairContextFailureFinalizesOperation": "failed physical attempt not finalized before correction",
    "TestSQLRecoveryAdversarialSourceFailureSurvivesRevisionFence": "source failure lost its closed classification",
    "TestSQLRecoveryAdversarialMissingReceiptFinalizesUncertain": "missing physical receipt left an unfinished query or fabricated success",
    "TestSQLRecoveryAdversarialFailedRerunDiscardsPreviousRows": "failed operation returned stale successful rows",
    "TestSQLRecoveryAdversarialCancelledCallerCanFinalize": "caller cancellation abandoned known finalization",
    "TestSQLRecoveryAdversarialReceiptErrorsNeverExposeRows": "invalid/missing receipt acquired a result or success",
}
FILES = (
    "internal/exec/analytical_adversarial_test.go",
    "internal/nlqexec/adversarial_execution_test.go",
    "internal/nlqexec/adversarial_finalization_test.go",
    "internal/store/postgres/sql_query_error_test.go",
)


def main():
    root = Path(subprocess.check_output(["git", "rev-parse", "--show-toplevel"], text=True).strip())
    output = Path(os.environ["RUNNER_TEMP"]) / "sql-adversarial-baseline"
    output.mkdir(exist_ok=True)
    baseline = Path(tempfile.mkdtemp(prefix="review-baseline-", dir=os.environ["RUNNER_TEMP"]))
    command = ["go", "test", "-race", "-count=1", "-timeout=5m", "-json", "./internal/exec", "./internal/nlqexec", "./internal/store/postgres", "-run", "^TestSQLRecoveryAdversarial"]
    try:
        subprocess.run(["git", "worktree", "add", "--detach", str(baseline), BASE], cwd=root, check=True)
        for path in FILES:
            shutil.copyfile(root / path, baseline / path)
        with (output / "baseline-tests.log").open("w") as stream:
            result = subprocess.run(command, cwd=baseline, stdout=stream, stderr=subprocess.STDOUT, timeout=420)
        events = []
        for line in (output / "baseline-tests.log").read_text().splitlines():
            try:
                events.append(json.loads(line))
            except ValueError:
                pass
        failed = {e.get("Test") for e in events if e.get("Action") == "fail" and e.get("Test")}
        checks = {name: any(e.get("Test") == name and message in e.get("Output", "") for e in events) for name, message in CASES.items()}
        valid = result.returncode == 1 and failed == set(CASES) and all(checks.values())
        valid = valid and not any(e.get("Action") in ("skip", "build-fail") or "panic:" in e.get("Output", "") for e in events)
        report = {"baseline": BASE, "command": command, "returncode": result.returncode, "expected_failure_tests": sorted(failed), "expected_assertions": checks, "confirmed": valid}
        (output / "summary.json").write_text(json.dumps(report, indent=2) + "\n")
        if not valid:
            raise SystemExit("Baseline reproduction did not produce the exact intended assertion failures; inspect its separate log.")
        print(json.dumps(report, indent=2))
    finally:
        subprocess.run(["git", "worktree", "remove", "--force", str(baseline)], cwd=root, check=True)


if __name__ == "__main__":
    main()
