#!/usr/bin/env bash
# Bind Go/CGo build-cache identity to the exact external native parser patch.
# This is needed even on a native-artifact cache hit: Go does not fingerprint
# arbitrary external archives solely because their filesystem path is unchanged.
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$repo/scripts/native-patches/bruin-source.lock"
test "$(sha256sum "$repo/scripts/native-patches/bruin-transparent-paren.patch" | cut -d' ' -f1)" = "$patch_sha256"
flag="-DCHARTWORKS_NATIVE_READ_POLICY_${patch_sha256}=1"
: "${CGO_CFLAGS:=-O2 -g}"
case " ${CGO_CFLAGS:-} " in *" $flag "*) ;; *) export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }$flag";; esac
if [[ -n "${GITHUB_ENV:-}" ]]; then
  printf 'CGO_CFLAGS=%s\n' "$CGO_CFLAGS" >> "$GITHUB_ENV"
fi
printf 'Native CGo identity: %s + %s\n' "$commit" "$patch_sha256"
