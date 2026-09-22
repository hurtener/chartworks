#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$root"
CGO_ENABLED=0 go test ./internal/evaluation -run \
  'TestPerformance(SmokeMeasuresRawReuseAndNegatives|AuthorityFixturesUseVerifiedBearerAndProtectedResources|DerivesOutcomeFromIndependentReceipts|AuthorityExpectationCannotControlDenial|NearestRankP95|WorkerPoolJoinsOnCancellation)$' \
  -count=1

printf 'PERF-01 bounded test-only synthetic smoke passed\n'
