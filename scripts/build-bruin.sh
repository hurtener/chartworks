#!/usr/bin/env bash
# Build the pinned effective native dependency outside the checkout/module cache.
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$repo/scripts/native-patches/bruin-source.lock"
root="${CHARTWORKS_BRUIN_BUILD_DIR:?set an absolute private build directory}"
case "$root" in /*) ;; *) echo 'absolute build directory required' >&2; exit 1;; esac
mkdir -p "$root"
if [[ ! -d "$root/source/.git" ]]; then
  git init "$root/source"
  git -C "$root/source" remote add origin https://github.com/hurtener/bruin.git
fi
if ! git -C "$root/source" cat-file -e "$commit^{commit}" 2>/dev/null; then
  git -C "$root/source" fetch --depth=1 origin "$commit"
fi
git -C "$root/source" checkout --detach "$commit"
test "$(git -C "$root/source" rev-parse HEAD)" = "$commit"
bash "$repo/scripts/apply-bruin-read-patch.sh" "$root/source" > "$root/native-source-identity.txt"
if ! rustup toolchain list | grep -q '^1.98.1-'; then rustup toolchain install 1.98.1 --profile minimal; fi
cargo +1.98.1 build --release --locked --manifest-path "$root/source/pkg/sqlparser/rustffi/Cargo.toml"
# Compile Chartworks-owned structural evidence against the same pinned parser.
CARGO_TARGET_DIR="$root/source/pkg/sqlparser/rustffi/target" cargo +1.98.1 build --release --locked --manifest-path "$repo/internal/exec/signatureparser/rustffi/Cargo.toml"
source "$repo/scripts/native-build-env.sh"
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$root/source/pkg/sqlparser/rustffi/target/release"
go -C "$root/source" build -tags=bruin_no_duckdb -ldflags "-s -w -X main.version=v0.11.749($commit+cw-read-${patch_sha256:0:12}) -X main.commit=$commit+cw-read-$patch_sha256" -o "$root/bruin" .
(cd "$root" && sha256sum bruin source/pkg/sqlparser/rustffi/target/release/libbruin_rustsqlparser.a source/pkg/sqlparser/rustffi/target/release/libchartworks_signatures.a > native-artifacts.sha256)
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "CGO_LDFLAGS=$CGO_LDFLAGS" >> "$GITHUB_ENV"
  echo "CHARTWORKS_TEST_BRUIN_PATH=$root/bruin" >> "$GITHUB_ENV"
fi
printf 'Native parser library: %s\nBruin executable: %s\n' "$root/source/pkg/sqlparser/rustffi/target/release" "$root/bruin"
