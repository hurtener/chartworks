#!/usr/bin/env bash
# Build the immutable native dependency outside the checkout/module cache.
set -euo pipefail
commit=83f04505f5a257e7dbd89b98b8276cb6dcb1ec8b
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
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$root/source/pkg/sqlparser/rustffi/target/release"
go -C "$root/source" build -tags=bruin_no_duckdb -ldflags "-s -w -X main.version=v0.11.749($commit) -X main.commit=$commit" -o "$root/bruin" .
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "CGO_LDFLAGS=$CGO_LDFLAGS" >> "$GITHUB_ENV"
  echo "CHARTWORKS_TEST_BRUIN_PATH=$root/bruin" >> "$GITHUB_ENV"
fi
printf 'Native parser library: %s\nBruin executable: %s\n' "$root/source/pkg/sqlparser/rustffi/target/release" "$root/bruin"
