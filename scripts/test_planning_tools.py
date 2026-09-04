"""Regression tests for the planning tools, not for the unimplemented service."""
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from planning_check import criteria, read_json, topological, validate_coverage
from run_phase_acceptance import run, validate_events


def events(count=2):
    parent = "TestPhase05"
    values = [{"Action": "pass", "Test": f"{parent}/AC{i:02d}", "Package": "example/acceptance"}
              for i in range(1, count + 1)]
    values.append({"Action": "pass", "Test": parent, "Package": "example/acceptance"})
    return [json.dumps(value) for value in values]


class PlanningToolsTest(unittest.TestCase):
    def test_topological_order(self):
        self.assertEqual(topological({"02": {"depends_on": ["01"]}, "01": {"depends_on": []}}), ["01", "02"])

    def test_cycle_denied(self):
        with self.assertRaisesRegex(ValueError, "cycle"):
            topological({"01": {"depends_on": ["02"]}, "02": {"depends_on": ["01"]}})

    def test_unknown_dependency_denied(self):
        with self.assertRaisesRegex(ValueError, "unknown"):
            topological({"01": {"depends_on": ["02"]}})

    def test_duplicate_dependency_denied(self):
        with self.assertRaisesRegex(ValueError, "duplicate"):
            topological({"01": {"depends_on": ["02", "02"]}, "02": {"depends_on": []}})

    def test_criteria_are_contiguous(self):
        self.assertEqual(criteria("1. **AC01** — real check\n2. **AC02** — another check\n", 2), ["AC01", "AC02"])
        with self.assertRaises(ValueError):
            criteria("1. **AC01** — a\n2. **AC03** — b\n", 2)

    def test_duplicate_json_denied(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "bad.json"
            path.write_text('{"phases":{},"phases":{}}')
            with self.assertRaisesRegex(ValueError, "duplicate JSON"):
                read_json(path)

    def test_coverage_missing_feature_denied(self):
        with self.assertRaisesRegex(ValueError, "63 source"):
            validate_coverage({"features": {}, "gates": {}}, set())

    def test_coverage_refs_resolve(self):
        features = {f"{p}{i:02d}": {"disposition": "discarded_stub" if p == "Q" and i == 11 else "required",
                                    "acceptance": ["05.AC01"]}
                    for p, n in [("B", 20), ("R", 16), ("Q", 11), ("N", 16)] for i in range(1, n+1)}
        coverage = {"features": features, "gates": {f"G{i:02d}": {"acceptance": ["05.AC01"]} for i in range(1, 42)}}
        validate_coverage(coverage, {"05.AC01"})
        coverage["gates"]["G41"]["acceptance"] = ["05.AC99"]
        with self.assertRaisesRegex(ValueError, "unresolved"):
            validate_coverage(coverage, {"05.AC01"})

    def test_complete_event_stream(self):
        self.assertEqual(len(validate_events(events(), "05", 2, 0)), 2)

    def test_parent_without_children_denied(self):
        with self.assertRaisesRegex(ValueError, "missing acceptance"):
            validate_events(events(0), "05", 2, 0)

    def test_no_tests_denied(self):
        with self.assertRaisesRegex(ValueError, "parent"):
            validate_events([], "05", 2, 0)

    def test_nested_skip_denied(self):
        stream = events() + [json.dumps({"Action": "skip", "Test": "TestPhase05/AC01/case", "Package": "example/acceptance"})]
        with self.assertRaisesRegex(ValueError, "skipped"):
            validate_events(stream, "05", 2, 0)

    def test_failure_even_after_pass_denied(self):
        stream = events() + [json.dumps({"Action": "fail", "Package": "example/acceptance"})]
        with self.assertRaisesRegex(ValueError, "failed"):
            validate_events(stream, "05", 2, 0)

    def test_nonzero_exit_denied(self):
        with self.assertRaisesRegex(ValueError, "exited"):
            validate_events(events(), "05", 2, 1)

    def test_duplicate_criterion_denied(self):
        with self.assertRaisesRegex(ValueError, "duplicate"):
            validate_events(events() + events()[:1], "05", 2, 0)

    def test_unknown_criterion_denied(self):
        with self.assertRaisesRegex(ValueError, "undeclared"):
            validate_events(events(3), "05", 2, 0)

    def test_malformed_event_denied(self):
        with self.assertRaises(ValueError):
            validate_events(["this is not JSON"], "05", 2, 0)

    def test_release_rejects_planned_with_skip_flag(self):
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {"CHARTWORKS_ALLOW_PLANNED_SKIP": "1"}):
            with self.assertRaisesRegex(ValueError, "not reviewed shipped"):
                run(Path(temp), "05", {"status": "planned", "acceptance_count": 2}, True)

    def test_planned_skip_is_explicit(self):
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {"CHARTWORKS_ALLOW_PLANNED_SKIP": "1"}):
            self.assertFalse(run(Path(temp), "05", {"status": "planned", "acceptance_count": 2}, False))

    def test_planned_with_tests_cannot_skip(self):
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {"CHARTWORKS_ALLOW_PLANNED_SKIP": "1"}):
            directory = Path(temp) / "test/acceptance"
            directory.mkdir(parents=True)
            (directory / "phase05_test.go").write_text("func TestPhase05(t *testing.T) {}")
            with self.assertRaisesRegex(ValueError, "still marked planned"):
                run(Path(temp), "05", {"status": "planned", "acceptance_count": 2}, False)

    def test_shipped_without_code_fails(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(ValueError, "missing"):
                run(Path(temp), "05", {"status": "shipped", "acceptance_count": 2}, True)


if __name__ == "__main__":
    unittest.main()
