#!/usr/bin/env python3
"""Explicit reviewed edits, committed before separate read-only verification."""
import json
from pathlib import Path
import subprocess


def replace(path: str, before: str, after: str) -> None:
    file = Path(path)
    text = file.read_text()
    if text.count(before) != 1:
        raise RuntimeError(f"{path}: expected exactly one reviewed anchor")
    file.write_text(text.replace(before, after))


replace("test/acceptance/phase11_test.go", "receipt.Manifest.Validation.Context", "receipt.Manifest.Receipt.Context")
replace("test/acceptance/phase11_test.go", "client := f.client(t)", "client := f.binaryClient(t)")
replace("test/acceptance/engineering_surface_test.go", "\tdefer server.Close()\n", "\tdefer func() { owner.Close(); server.Close() }()\n")

# Discovery describes implemented capabilities, not future reporting features.
replace("internal/foundation/server.go", '"01-06-gateway-jobs", s.implemented()', '"01-12-engineering", s.implemented()')
replace("internal/foundation/server.go", '''	if s.values.Jobs.Enabled {
		out = append(out, "durable_operations", "scheduling")
	}
	return out''', '''	if s.values.Jobs.Enabled {
		out = append(out, "durable_operations", "scheduling")
	}
	if s.values.Sources.Enabled {
		out = append(out, "governed_sources", "validated_read_execution")
	}
	if s.values.Uploads.Enabled {
		out = append(out, "governed_uploads")
	}
	if s.values.Profiling.Enabled {
		out = append(out, "versioned_profile_evidence")
	}
	return out''')

# Existing code must never evade strict acceptance behind a planned skip. These
# phases are not shipped until all their actual acceptance/coverage gates pass.
paths = ["docs/plans/phase-11-uploads-workspace.md", "docs/plans/phase-12-engineering-profiling.md"]
for path in paths:
    replace(path, "Status: planned.", "Status: in_progress.")
registry_path = Path("docs/plans/phase-registry.json")
registry = json.loads(registry_path.read_text())
for phase in ("11", "12"):
    if registry["phases"][phase]["status"] != "planned":
        raise RuntimeError("unexpected phase status; do not overwrite concurrent progress")
    registry["phases"][phase]["status"] = "in_progress"
registry_path.write_text(json.dumps(registry, indent=2) + "\n")
paths.append(str(registry_path))
subprocess.run(["git", "add", "--", *paths], check=True)
print("Corrected SDK acceptance field, enabled compiled-binary coverage, bounded test cleanup, and marked phases in progress.")
