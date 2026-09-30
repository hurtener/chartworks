#!/usr/bin/env bash
# Build the immutable native dependency outside the checkout/module cache.
set -euo pipefail
commit=5f562c2959496a04d57f5f199f5e3ad22159fa9f
root="${CHARTWORKS_BRUIN_BUILD_DIR:?set an absolute private build directory}"
case "$root" in /*) ;; *) echo 'absolute build directory required' >&2; exit 1;; esac
mkdir -p "$root"
if [[ ! -d "$root/source/.git" ]]; then
  git init "$root/source"
  git -C "$root/source" remote add origin https://github.com/hurtener/bruin.git
fi
git -C "$root/source" fetch --depth=1 origin "$commit"
git -C "$root/source" checkout --detach FETCH_HEAD
test "$(git -C "$root/source" rev-parse HEAD)" = "$commit"
rustup toolchain install 1.98.1 --profile minimal
cargo +1.98.1 build --release --locked --manifest-path "$root/source/pkg/sqlparser/rustffi/Cargo.toml"
# Compile Chartworks-owned structural evidence against the same pinned parser.
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CARGO_TARGET_DIR="$root/signatures-target" cargo +1.98.1 build --release --locked --manifest-path "$repo/internal/exec/signatureparser/rustffi/Cargo.toml"
cp "$root/signatures-target/release/libchartworks_signatures.a" "$root/source/pkg/sqlparser/rustffi/target/release/"
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$root/source/pkg/sqlparser/rustffi/target/release"
go -C "$root/source" build -tags=bruin_no_duckdb -ldflags "-s -w -X main.version=v0.11.749($commit) -X main.commit=$commit" -o "$root/bruin" .
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "CGO_LDFLAGS=$CGO_LDFLAGS" >> "$GITHUB_ENV"
  echo "CHARTWORKS_TEST_BRUIN_PATH=$root/bruin" >> "$GITHUB_ENV"
fi
printf 'Native parser library: %s\nBruin executable: %s\n' "$root/source/pkg/sqlparser/rustffi/target/release" "$root/bruin"
