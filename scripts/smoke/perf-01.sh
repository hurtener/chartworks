#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
report="${CHARTWORKS_PERF_REPORT:-$(mktemp "${TMPDIR:-/tmp}/chartworks-perf01.XXXXXX")}"

cd "$root"
CGO_ENABLED=0 go run ./cmd/chartworks eval perf-smoke \
  --profile examples/evaluation-performance-smoke.json \
  --report "$report" >/dev/null

python3 - "$report" <<'PY'
import json
import pathlib
import sys

report = json.loads(pathlib.Path(sys.argv[1]).read_text())
assert report["correctness_passed"] is True
assert report["evidence_mode"] == "synthetic"
assert len(report["samples"]) == 25
assert {s["step_id"] for s in report["summaries"]} == {
    "cold", "warm", "repeat", "concurrent", "source-change", "rule-change",
    "context-change", "topic-change", "runtime-change", "tenant-negative",
    "context-negative", "actions-negative",
}
PY

printf 'PERF-01 bounded synthetic smoke passed; report=%s\n' "$report"
