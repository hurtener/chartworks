#!/usr/bin/env bash
# Trusted, manual CI provisioning only. Never invoke this from candidate source.
set -euo pipefail
[[ $EUID == 0 && ${GITHUB_ACTIONS:-} == true && ${RUNNER_ENVIRONMENT:-} == github-hosted ]]
operation=${1:?run or cleanup}
run_id=${2:?GitHub run ID and attempt}
[[ $run_id =~ ^[0-9]+-[0-9]+$ ]]
unit="chartworks-render-$run_id.service"
profile="chartworks-render-$run_id"
state="/var/tmp/chartworks-render-$run_id"

cleanup() {
  local failed=0 group="/$unit" load active actual_group profiles
  if [[ ! -e $state ]]; then return; fi
  [[ -d $state && ! -L $state && $(stat -c %u "$state") == 0 ]] || return 1
  # Kill the whole unit before removing the userns exception or delegation.
  load=$(systemctl show "$unit" --property=LoadState --value) || return 1
  if [[ $load != not-found ]]; then
    actual_group=$(systemctl show "$unit" --property=ControlGroup --value) || return 1
    [[ -z $actual_group || $actual_group == "$group" ]] || return 1
    if ! systemctl stop "$unit"; then
      echo 'FAIL: stop uncertain; retain profile and cgroup mount boundaries' >&2
      return 1
    fi
    active=$(systemctl show "$unit" --property=ActiveState --value) || return 1
    [[ $active == inactive || $active == failed ]] || return 1
  fi
  systemctl reset-failed "$unit" 2>/dev/null || true
  if [[ -d /sys/fs/cgroup$group ]]; then
    if ! grep -qx 'populated 0' "/sys/fs/cgroup$group/cgroup.events"; then
      echo 'FAIL: service still populated; retain all host boundaries' >&2
      return 1
    fi
    for ((attempt=0; attempt<50; attempt++)); do
      [[ ! -d /sys/fs/cgroup$group ]] && break
      sleep 0.1
    done
    if [[ -d /sys/fs/cgroup$group ]]; then
      echo 'FAIL: delegated cgroup still present; retain remaining boundaries' >&2
      return 1
    fi
  fi
  if [[ -f $state/apparmor-loaded ]]; then
    profiles=$(cat /sys/kernel/security/apparmor/profiles) || return 1
    if ! grep -q "^$profile (" <<< "$profiles" ||
       apparmor_parser -R "$state/apparmor"; then
      rm "$state/apparmor-loaded"
    else
      failed=1
    fi
  fi
  if [[ -f $state/added-hugetlb ]]; then
    if printf '%s\n' -hugetlb > /sys/fs/cgroup/cgroup.subtree_control; then
      rm "$state/added-hugetlb"
    else
      failed=1
    fi
  fi
  if [[ -f $state/added-nsdelegate ]]; then
    # cgroup2 has no "nonsdelegate" option. Reapply the original complete flags.
    if mount -t cgroup2 -o "remount,$(cat "$state/original-mount-options")" cgroup2 /sys/fs/cgroup; then
      rm "$state/added-nsdelegate"
    else
      failed=1
    fi
  fi
  if (( failed )); then
    echo 'FAIL: transient host cleanup incomplete' >&2
    return 1
  fi
  rm -rf --one-file-system -- "$state"
  echo 'HOST_CLEANUP=passed'
}

if [[ $operation == cleanup ]]; then cleanup; exit; fi
[[ $operation == run && $# == 6 ]]
source_dir=$(realpath -e "${3:?candidate checkout}")
source_sha=${4:?candidate SHA}
go_bin=$(realpath -e "${5:?Go binary directory}")
rust_bin=$(realpath -e "${6:?rustup binary directory}")
[[ $source_sha =~ ^[0-9a-f]{40}$ ]]
trusted_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
[[ -x $go_bin/go && -x $rust_bin/rustup ]]
source /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 24.04 ]]
[[ $(stat -fc %T /sys/fs/cgroup) == cgroup2fs ]]
[[ $(id -u nobody) != 0 && $(id -Gn nobody) == nogroup ]]
if pgrep -u nobody >/dev/null; then
  echo 'FAIL: nobody already has processes; a distinct manager identity is required' >&2
  exit 1
fi
[[ ! -e $state ]]
install -d -m 0755 "$state"
install -d -o nobody -g nogroup -m 0700 "$state/work"
install -o root -g root -m 0555 "$trusted_dir/renderer-host-tests.sh" "$state/tests.sh"
install -o root -g root -m 0555 "$trusted_dir/renderer-source.py" "$state/source.py"
trap 'result=$?; trap - EXIT; cleanup || result=1; exit "$result"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# The hosted runner's home may be non-traversable by nobody. Stage immutable
# bytes in this job's existing root-owned directory; do not open the home tree.
source_digest=$(python3 -I "$state/source.py" prepare "$source_dir" "$state/source" "$source_sha")
source_dir="$state/source"
printf 'SOURCE_SNAPSHOT_SHA256=%s\n' "$source_digest"
install -d -o root -g root -m 0755 "$state/tools/bin"
# rustup lives below the same private home. Copy only the official executable;
# job-private CARGO_HOME/RUSTUP_HOME still receive the pinned native toolchain.
install -o root -g root -m 0555 "$rust_bin/rustup" "$state/tools/bin/rustup"
rustup_digest=$(sha256sum "$rust_bin/rustup" | cut -d' ' -f1)
[[ $(sha256sum "$state/tools/bin/rustup" | cut -d' ' -f1) == "$rustup_digest" ]]
printf 'STAGED_RUSTUP_SHA256=%s\n' "$rustup_digest"
for shim in cargo rustc rustdoc rustfmt cargo-fmt cargo-clippy clippy-driver; do
  ln -s rustup "$state/tools/bin/$shim"
done
rust_bin="$state/tools/bin"

has_word() { [[ " $(cat "$1") " == *" $2 "* ]]; }
for controller in memory hugetlb pids cpu; do
  has_word /sys/fs/cgroup/cgroup.controllers "$controller" || {
    echo "FAIL: missing cgroup v2 controller $controller" >&2; exit 1;
  }
done
options=$(findmnt -n -o FS-OPTIONS /sys/fs/cgroup)
if [[ ,$options, != *,nsdelegate,* ]]; then
  printf '%s\n' "$options" > "$state/original-mount-options"
  touch "$state/added-nsdelegate"
  mount -t cgroup2 -o "remount,$options,nsdelegate" cgroup2 /sys/fs/cgroup
fi
# systemd 255 does not know hugetlb. Put this one service directly in the
# root slice so only the root needs this additional controller enabled.
if ! has_word /sys/fs/cgroup/cgroup.subtree_control hugetlb; then
  touch "$state/added-hugetlb"
  printf '%s\n' +hugetlb > /sys/fs/cgroup/cgroup.subtree_control
fi
properties=()
if [[ -r /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]] &&
   [[ $(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns) == 1 ]]; then
  # Named profile without a path attachment: only this service opts into it.
  profiles=$(cat /sys/kernel/security/apparmor/profiles)
  if grep -q "^$profile (" <<< "$profiles"; then
    echo 'FAIL: job AppArmor profile already exists' >&2; exit 1
  fi
  printf 'abi <abi/4.0>,\nprofile %s flags=(unconfined) {\n  userns,\n}\n' "$profile" > "$state/apparmor"
  touch "$state/apparmor-loaded"
  apparmor_parser -a "$state/apparmor"
  properties+=(--property="AppArmorProfile=$profile")
fi
printf 'KERNEL=%s\n' "$(uname -r)"
systemd --version | head -n 1
findmnt -n -o TARGET,FSTYPE,FS-OPTIONS /sys/fs/cgroup
# No candidate shell, script, binary, hook or dependency runs as root. nobody
# has neither sudo rights nor docker/admin groups. No credentials are inherited.
test_result=0
systemd-run --unit="$unit" --wait --pipe --service-type=exec \
  --property=Slice=-.slice --property=User=nobody --property=Group=nogroup \
  --property=Delegate=yes --property=DelegateSubgroup=manager \
  --property=NoNewPrivileges=yes --property=UMask=0077 \
  --property=OOMScoreAdjust=0 --property=KillMode=control-group \
  --property=TimeoutStopSec=15s --property=RuntimeMaxSec=60m \
  --property='InaccessiblePaths=-/run/docker.sock -/run/containerd -/run/systemd/private -/run/dbus/system_bus_socket' \
  "${properties[@]}" \
  /usr/bin/env -i \
  PATH="$go_bin:$rust_bin:/usr/local/bin:/usr/bin:/bin" \
  HOME="$state/work" GOPATH="$state/work/go" GOCACHE="$state/work/go-cache" \
  CARGO_HOME="$state/work/cargo" RUSTUP_HOME="$state/work/rustup" \
  TMPDIR="$state/work/tmp" \
  /bin/bash "$state/tests.sh" "$source_dir" "$source_sha" "$unit" "$source_digest" || test_result=$?
# Independent trusted verification precedes cleanup, including on test failure.
python3 -I "$state/source.py" verify "$source_dir" "$source_digest" || test_result=1
exit "$test_result"
