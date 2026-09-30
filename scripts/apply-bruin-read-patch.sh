#!/usr/bin/env bash
# Apply exactly the owned native read patch; never discard unrelated changes.
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$repo/scripts/native-patches/bruin-source.lock"
patch="$repo/scripts/native-patches/bruin-transparent-paren.patch"
test "$(sha256sum "$patch" | cut -d' ' -f1)" = "$patch_sha256"
source_dir="${1:?absolute private dependency checkout required}"
case "$source_dir" in /*) ;; *) echo 'absolute source checkout required' >&2; exit 1;; esac
test "$(git -C "$source_dir" rev-parse HEAD)" = "$commit"
git -C "$source_dir" diff --cached --quiet
if [[ -n "$(git -C "$source_dir" ls-files -v | sed -n '/^[a-zS]/p')" ]]; then
  echo 'hidden or sparse dependency source entries are not permitted' >&2; exit 1
fi
if [[ -n "$(git -C "$source_dir" ls-files --others --exclude-standard)" ]]; then
  echo 'untracked dependency source changes are not permitted' >&2; exit 1
fi
if git -C "$source_dir" diff --quiet; then
  git -C "$source_dir" apply --check "$patch"
  git -C "$source_dir" apply "$patch"
fi
if ! cmp -s <(git -C "$source_dir" diff --binary --full-index --no-ext-diff --no-textconv "$commit") "$patch"; then
  echo 'dependency changes do not match the exact owned patch' >&2; exit 1
fi
printf 'base=%s\npatch_sha256=%s\n' "$commit" "$patch_sha256"
