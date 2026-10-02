#!/usr/bin/env bash
# Trusted test driver. The entire candidate build and test process is unprivileged.
set -euo pipefail
source_dir=${1:?candidate checkout}
source_sha=${2:?exact candidate commit}
unit=${3:?delegated unit}
source_digest=${4:?exact staged source digest}
export GIT_OPTIONAL_LOCKS=0
[[ $(id -u) == $(id -u nobody) && $(id -Gn) == nogroup ]]
[[ $(awk '/^CapEff:/ {print $2}' /proc/self/status) == 0000000000000000 ]]
[[ $(awk '/^NoNewPrivs:/ {print $2}' /proc/self/status) == 1 ]]
[[ $(cat /proc/self/oom_score_adj) == 0 ]]
[[ ! -w /run/docker.sock ]]
mkdir -p "$TMPDIR"
cd "$source_dir"
test "$(git -c safe.directory="$source_dir" rev-parse HEAD)" = "$source_sha"
test -z "$(git -c safe.directory="$source_dir" status --porcelain)"
test ! -w .
test -z "$(find "$source_dir" \( -type f -o -type d \) -writable -print -quit)"
python3 -I "$(dirname "${BASH_SOURCE[0]}")/source.py" verify "$source_dir" "$source_digest"
# Go's VCS stamping also calls git; trust only this exact read-only checkout in
# this job's private configuration, never safe.directory=* on the runner.
git config --global --add safe.directory "$source_dir"
# Check actual traversal/execution under the test identity, before compiling any
# candidate code. Inaccessible preinstalled tools are an explicit host failure.
go version
rustup --version

# Derive the delegated root from the actual process placement, not a fabricated
# systemd-to-cgroup mapping. DelegateSubgroup keeps the domain root empty.
relative=$(awk -F: '$1 == "0" && $2 == "" {print $3}' /proc/self/cgroup)
[[ $relative == /*/manager && ${relative%/manager} == /"$unit" ]]
export CHARTWORKS_TEST_RENDER_CGROUP_ROOT="/sys/fs/cgroup${relative%/manager}"
test -z "$(cat "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT/cgroup.procs")"
chmod 0700 "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT"
printf '%s\n' '+memory +hugetlb +pids +cpu' > "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT/cgroup.subtree_control"
printf 'MANAGER_UID=%s\nMANAGER_CGROUP=%s\n' "$(id -u)" "$relative"
cat /proc/self/attr/current
cat "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT/cgroup.controllers"
cat "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT/cgroup.subtree_control"

export CGO_ENABLED=1 GOFLAGS=-p=2 GOMAXPROCS=2 GOTOOLCHAIN=local
export CHARTWORKS_BRUIN_BUILD_DIR="$HOME/bruin"
export CHARTWORKS_TEST_BRUIN_PATH="$HOME/bruin/bruin"
export CGO_LDFLAGS="-L$HOME/bruin/source/pkg/sqlparser/rustffi/target/release"
# Disposable CI fixture only; never a production credential or model endpoint.
export CHARTWORKS_TEST_STORE_URL='postgres://chartworks:chartworks@localhost:5434/chartworks?sslmode=disable'
go mod verify
bash scripts/build-bruin.sh
source scripts/native-build-env.sh

go test -race -tags renderer_integration -count=1 -json -timeout=10m \
  ./internal/rendering -run '^TestRendererKernel(MemoryContract|StableCatalog)$' | tee "$HOME/kernel.jsonl"
go test -race -count=1 -json -timeout=15m ./test/acceptance -run '^TestPhase32$' | tee "$HOME/phase32.jsonl"
python3 - "$HOME/kernel.jsonl" "$HOME/phase32.jsonl" <<'PY'
import json, pathlib, sys
required = {'TestRendererKernelMemoryContract', 'TestRendererKernelStableCatalog', 'TestPhase32'}
required |= {'TestRendererKernelMemoryContract/' + case for case in (
    'non-page-aligned-budget', 'revoked-controls', 'independent-jobs',
    'worker-limits', 'tamper', 'address', 'anonymous', 'descendants',
    'file', 'pipes', 'sleep')}
required |= {f'TestPhase32/AC{i:02d}' for i in range(1, 9)}
events = [json.loads(line) for path in sys.argv[1:]
          for line in pathlib.Path(path).read_text().splitlines()]
assert not any(e.get('Action') in ('fail', 'skip') for e in events)
passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
assert required <= passed, sorted(required - passed)
print(f'QUALIFICATION=passed required_events={len(required)}')
PY
test -z "$(git -c safe.directory="$source_dir" status --porcelain)"
python3 -I "$(dirname "${BASH_SOURCE[0]}")/source.py" verify "$source_dir" "$source_digest"
test -z "$(find "$CHARTWORKS_TEST_RENDER_CGROUP_ROOT" -mindepth 1 -maxdepth 1 -type d ! -name manager -print)"
