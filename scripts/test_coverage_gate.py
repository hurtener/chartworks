from contextlib import redirect_stderr, redirect_stdout
import io
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import coverage_gate
from coverage_gate import band_percentage, bands, measure, passes_band


class CoverageTests(unittest.TestCase):
    def test_exact_bands(self):
        self.assertEqual(bands('# comment\ninternal/store 85\ncmd/chartworks 70\n'), {'internal/store': 8500, 'cmd/chartworks': 7000})
        for value in ('', 'internal/store 85\ninternal/store 80', 'internal/store 101', '../store 80', 'internal/store zero'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                bands(value)

    def test_decimal_bands_are_exact_basis_points(self):
        for text, expected in (('84.5', 8450), ('84.50', 8450), ('84.51', 8451), ('1.01', 101), ('100.00', 10000)):
            with self.subTest(text=text):
                self.assertEqual(bands('internal/store/postgres ' + text), {'internal/store/postgres': expected})
        for text in ('0.99', '100.01', '84.500', '84.', '.5', '8.45e1', '+84.5', '-84.5', 'NaN', 'Infinity'):
            with self.subTest(text=text), self.assertRaises(ValueError):
                bands('internal/store/postgres ' + text)
        self.assertEqual(band_percentage(8500), '85')
        self.assertEqual(band_percentage(8450), '84.5')
        self.assertEqual(band_percentage(8451), '84.51')

    def test_fractional_threshold_does_not_round_up_coverage(self):
        minimum = bands('internal/store/postgres 84.5')['internal/store/postgres']
        self.assertTrue(passes_band(169, 200, minimum))
        self.assertFalse(passes_band(168, 200, minimum))
        self.assertTrue(passes_band(1974, 2336, minimum))
        self.assertFalse(passes_band(1973, 2336, minimum))
        self.assertFalse(passes_band(84499, 100000, minimum))  # Displays as 84.50%.
        total = 10 ** 30
        self.assertFalse(passes_band(845 * total // 1000 - 1, total, minimum))
        self.assertFalse(passes_band(169, 200, bands('internal/store 85')['internal/store']))
        self.assertTrue(passes_band(170, 200, bands('internal/store 85')['internal/store']))

    def test_weighted_statements_and_merged_instrumentation(self):
        profile = 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 8 0\nexample/internal/store/a.go:1.1,2.1 8 1\nexample/internal/store/b.go:1.1,2.1 2 0\n'
        self.assertEqual(measure(profile, 'example', {'internal/store'}), {'internal/store': (8, 10)})

    def test_missing_package_cannot_pass(self):
        with self.assertRaises(ValueError):
            measure('mode: atomic\nexample/internal/a.go:1.1,2.1 1 1', 'example', {'internal/store'})

    def test_bad_profiles(self):
        for value in ('', 'mode: set\n', 'mode: atomic\nbad', 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 -1 2', 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 1 1\nexample/internal/store/a.go:1.1,2.1 2 1'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                measure(value, 'example', {'internal/store'})


class CoverageCommandTests(unittest.TestCase):
    def test_default_discovery_budget_and_environment(self):
        result = subprocess.CompletedProcess(["go", "list", "-m"], 0, "example\n")
        with patch.dict(os.environ, {"CGO_ENABLED": "0", "GOFLAGS": "-p=2"}), \
             patch.object(coverage_gate.subprocess, "run", return_value=result) as run:
            self.assertEqual(coverage_gate.command(["go", "list", "-m"], Path("."), capture=True), "example")
        self.assertEqual(run.call_args.kwargs["timeout"], 1500)
        self.assertEqual(run.call_args.kwargs["env"]["CGO_ENABLED"], "1")
        self.assertEqual(run.call_args.kwargs["env"]["GOFLAGS"], "-p=2")
        self.assertEqual(run.call_args.kwargs["stdout"], subprocess.PIPE)
        self.assertFalse(run.call_args.kwargs["check"])

    def test_nonzero_command_cannot_pass(self):
        result = subprocess.CompletedProcess(["go", "test"], 1)
        with patch.object(coverage_gate.subprocess, "run", return_value=result), \
             self.assertRaises(ValueError):
            coverage_gate.command(["go", "test"], Path("."))


class CoverageRunnerTests(unittest.TestCase):
    def run_gate(self, *, suite_result=0, covered=1, discovery_timeout=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").mkdir()
            (root / "scripts/coverage-bands.conf").write_text("internal/store 85\n")
            saved_profile = root / "saved-coverage.out"
            output, errors = io.StringIO(), io.StringIO()
            calls = []

            def run(args, **kwargs):
                calls.append((args, kwargs))
                self.assertEqual(kwargs["cwd"], root)
                self.assertEqual(kwargs["env"]["CGO_ENABLED"], "1")
                if args == ["go", "list", "-m"]:
                    if discovery_timeout:
                        raise subprocess.TimeoutExpired(["private-config-value"], kwargs["timeout"])
                    return subprocess.CompletedProcess(args, 0, "example\n")
                if args == ["go", "list", "./..."]:
                    return subprocess.CompletedProcess(args, 0, "example/internal/store\nexample/test/acceptance\n")
                self.assertEqual(args[:2], ["go", "test"])
                profile_arg = next(value for value in args if value.startswith("-coverprofile="))
                Path(profile_arg.split("=", 1)[1]).write_text(
                    f"mode: atomic\nexample/internal/store/a.go:1.1,2.1 1 {covered}\n")
                if suite_result == "timeout":
                    raise subprocess.TimeoutExpired(["private-config-value"], kwargs["timeout"])
                return subprocess.CompletedProcess(args, suite_result)

            with patch.object(coverage_gate, "__file__", str(root / "scripts/coverage_gate.py")), \
                 patch.dict(os.environ, {"CHARTWORKS_COVERAGE_OUTPUT": str(saved_profile)}), \
                 patch.object(coverage_gate.subprocess, "run", side_effect=run), \
                 redirect_stdout(output), redirect_stderr(errors):
                status = coverage_gate.main()
            retained = saved_profile.read_text() if saved_profile.exists() else None
            return status, output.getvalue(), errors.getvalue(), calls, retained

    def test_distinct_aggregate_budget_preserves_race_and_package_deadline(self):
        status, output, errors, calls, retained = self.run_gate()
        self.assertEqual(status, 0, errors)
        self.assertEqual([kwargs["timeout"] for _, kwargs in calls], [1500, 1500, 3600])
        args = calls[-1][0]
        self.assertEqual(args[:7], ["go", "test", "-race", "-count=1", "-timeout=20m",
                                   "-covermode=atomic", "-coverpkg=example/internal/store"])
        self.assertEqual(args[-1], "./...")
        self.assertEqual(sum(value.startswith("-timeout=") for value in args), 1)
        self.assertIn("OK: internal/store 100.00%", output)
        self.assertIn("required 85%", output)
        self.assertTrue(retained.startswith("mode: atomic\n"))

    def test_failed_suite_with_full_coverage_is_still_failure(self):
        status, output, errors, _, retained = self.run_gate(suite_result=1)
        self.assertEqual(status, 1)
        self.assertNotIn("OK:", output)
        self.assertIn("FAIL: coverage: ValueError", errors)
        self.assertIsNotNone(retained)

    def test_timeout_retains_evidence_without_claiming_success_or_logging_command(self):
        status, output, errors, _, retained = self.run_gate(suite_result="timeout")
        self.assertEqual(status, 1)
        self.assertNotIn("OK:", output)
        self.assertIn("exceeded 3600s aggregate wall-clock limit", errors)
        self.assertIn("partial coverage is not passing test evidence", errors)
        self.assertNotIn("private-config-value", errors)
        self.assertIsNotNone(retained)

    def test_discovery_timeout_never_runs_suite(self):
        status, output, errors, calls, retained = self.run_gate(discovery_timeout=True)
        self.assertEqual(status, 1)
        self.assertEqual(len(calls), 1)
        self.assertIn("exceeded 1500s aggregate wall-clock limit", errors)
        self.assertNotIn("private-config-value", errors)
        self.assertNotIn("OK:", output)
        self.assertIsNone(retained)

    def test_insufficient_coverage_remains_failure_after_successful_suite(self):
        status, output, errors, _, retained = self.run_gate(covered=0)
        self.assertEqual(status, 1)
        self.assertIn("FAIL: internal/store 0.00%", output)
        self.assertIn("required 85%", output)
        self.assertEqual(errors, "")
        self.assertIsNotNone(retained)
