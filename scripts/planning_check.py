#!/usr/bin/env python3
"""Validate planning coherence only. This is not runtime or security evidence."""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys

AC = re.compile(r"^\d+\.\s+\*\*(AC\d{2})\*\*", re.M)
REF = re.compile(r"^(\d{2})\.(AC\d{2})$")


def read_json(path: Path) -> dict:
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key {key!r} in {path}")
            result[key] = value
        return result
    data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique)
    if not isinstance(data, dict):
        raise ValueError(f"expected object: {path}")
    return data


def topological(phases: dict) -> list[str]:
    ordered, visiting, visited = [], set(), set()
    def visit(pid):
        if pid not in phases:
            raise ValueError(f"unknown phase dependency {pid}")
        if pid in visiting:
            raise ValueError(f"dependency cycle at {pid}")
        if pid in visited:
            return
        visiting.add(pid)
        deps = phases[pid]["depends_on"]
        if not isinstance(deps, list) or len(deps) != len(set(deps)):
            raise ValueError(f"invalid/duplicate dependencies for {pid}")
        for dependency in deps:
            visit(dependency)
        visiting.remove(pid)
        visited.add(pid)
        ordered.append(pid)
    for pid in sorted(phases):
        visit(pid)
    return ordered


def criteria(text: str, count: int) -> list[str]:
    found = AC.findall(text)
    expected = [f"AC{i:02d}" for i in range(1, count + 1)]
    if found != expected:
        raise ValueError(f"criteria {found!r} do not equal {expected!r}")
    return found


def validate_coverage(coverage: dict, known: set[str]) -> None:
    expected_features = {f"{prefix}{i:02d}" for prefix, size in
                         [("B", 20), ("R", 16), ("Q", 11), ("N", 16)]
                         for i in range(1, size + 1)}
    if set(coverage["features"]) != expected_features:
        raise ValueError("feature ledger must preserve all 63 source IDs")
    expected_gates = {f"G{i:02d}" for i in range(1, 42)}
    if set(coverage["gates"]) != expected_gates:
        raise ValueError("gate ledger must contain G01-G41, including Bifrost-only G41")
    for group in ("features", "gates"):
        for key, row in coverage[group].items():
            refs = row.get("acceptance", [])
            if not refs or len(refs) != len(set(refs)):
                raise ValueError(f"missing/duplicate acceptance mapping: {key}")
            for ref in refs:
                if not REF.fullmatch(ref) or ref not in known:
                    raise ValueError(f"unresolved acceptance {key}: {ref}")
            if group == "features":
                expected = "discarded_stub" if key == "Q11" else "required"
                if row.get("disposition") != expected:
                    raise ValueError(f"unapproved feature disposition: {key}")


def active_documents(root: Path) -> list[Path]:
    files = list(root.glob("RFC-*.md"))
    files += [root / name for name in ("AGENTS.md", "CLAUDE.md", "README.md",
                                      "00_KICKSTART-PROMPT.md", "00_CHARTWORKS_CONSUMER-REQUEST.md")]
    for directory in ("docs/plans", "docs/contracts", "docs/reporting"):
        files.extend((root / directory).glob("*.md"))
    return sorted({path for path in files if path.exists()})


def check(root: Path) -> dict:
    phases = read_json(root / "docs/plans/phase-registry.json")["phases"]
    if set(phases) != {f"{i:02d}" for i in range(1, 35)}:
        raise ValueError("active phase IDs must be 01-34")
    order = topological(phases)
    known, expected_files = set(), set()
    headings = ("Brief findings incorporated", "Findings I'm departing from",
                "Scope", "Non-goals", "Acceptance criteria", "Tests")
    for pid, row in sorted(phases.items()):
        if row["status"] not in ("planned", "in_progress", "shipped"):
            raise ValueError(f"invalid phase status: {pid}")
        count = row["acceptance_count"]
        if type(count) is not int or count < 1 or count > 99:
            raise ValueError(f"invalid acceptance count: {pid}")
        path = root / f"docs/plans/phase-{pid}-{row['slug']}.md"
        expected_files.add(path)
        text = path.read_text(encoding="utf-8")
        for heading in headings:
            if not re.search(r"^## .*" + re.escape(heading), text, re.M):
                raise ValueError(f"{path.name}: missing heading {heading}")
        metadata = re.search(r"Status: (\w+)\. Owner: ([^.]+(?:/[^.]+)?)\. Hard dependencies: ([^.]+)\.", text)
        if not metadata:
            raise ValueError(f"{path.name}: malformed status/owner/dependency header")
        if metadata[1] != row["status"]:
            raise ValueError(f"{path.name}: status disagrees with registry")
        dependencies = re.findall(r"\b\d{2}\b", metadata[3])
        if dependencies != row["depends_on"]:
            raise ValueError(f"{path.name}: dependency header disagrees with registry")
        for ac in criteria(text, count):
            known.add(f"{pid}.{ac}")
        smoke = root / f"scripts/smoke/phase-{pid}.sh"
        if not smoke.is_file() or "run_phase_acceptance.py" not in smoke.read_text():
            raise ValueError(f"phase {pid}: missing real acceptance-runner smoke")
    if set((root / "docs/plans").glob("phase-*.md")) != expected_files:
        raise ValueError("active phase files and registry disagree")
    coverage = read_json(root / "docs/plans/coverage.json")
    validate_coverage(coverage, known)
    if (root / "AGENTS.md").read_bytes() != (root / "CLAUDE.md").read_bytes():
        raise ValueError("contributor rules are not byte-identical")
    decision_files = [root / "docs/decisions.md"] + sorted((root / "docs/decisions").glob("*.md"))
    ids = [ident for path in decision_files for ident in
           re.findall(r"^### (D-\d{3})\b", path.read_text(), re.M)]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate decision IDs")
    decisions = set(ids)
    for path in active_documents(root):
        text = path.read_text(encoding="utf-8")
        for decision in set(re.findall(r"\bD-\d{3}\b", text)):
            # D-043 is explicitly reserved for the separate historical branch.
            if decision not in decisions and decision != "D-043":
                raise ValueError(f"{path.relative_to(root)}: unresolved {decision}")
        for target in re.findall(r"\[[^\]]*\]\(([^)\s]+)\)", text):
            if "://" in target or target.startswith(("#", "mailto:")):
                continue
            destination = (path.parent / target.split("#", 1)[0]).resolve()
            if not destination.is_relative_to(root.resolve()) or not destination.exists():
                raise ValueError(f"{path.relative_to(root)}: broken link {target}")
    policy = read_json(root / "docs/contracts/model-gateway-policy.json")
    if policy.get("production_driver") != "bifrost" or policy.get("local_inference") is not False:
        raise ValueError("Bifrost-only/no-local-inference policy missing")
    example = read_json(root / "examples/chartworks.gateway.json")["gateway"]
    if example["driver"] != "bifrost":
        raise ValueError("production example does not select Bifrost SDK")
    if example["roles"]["embedding"]["dimensions"] != policy["reference_embedding_dimensions"]:
        raise ValueError("example dimensions disagree with gateway policy")
    return {"phases": len(phases), "acceptance_criteria": len(known),
            "features": len(coverage["features"]), "gates": len(coverage["gates"]),
            "topological_order": order}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()
    try:
        result = check(args.root.resolve())
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"FAIL: planning coherence: {error}", file=sys.stderr)
        return 1
    print("OK: planning coherence " + json.dumps(result, sort_keys=True))
    print("NOTE: mappings and document checks are not runtime or security proof.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
