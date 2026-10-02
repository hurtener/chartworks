# Disposable hosted renderer qualification

This CI-only proposal does not change renderer code or qualify a deployment by
itself. The manual workflow runs the mandatory kernel suite and all eight Phase32
criteria against one exact current same-repository PR head. Missing prerequisites,
failed tests, skipped tests, absent required events, source drift and incomplete
cleanup fail the job. No paid model or live provider credential is configured.

## Trust and installation

Review and merge the provisioning-only workflow and scripts onto the default
branch first. Dispatch `Renderer hosted qualification` from that branch with the
PR number and its full current commit SHA. The workflow checks out its own
`github.workflow_sha` as `trusted`, separately from `candidate`; the PR code is
never used for a privileged command. Both commit identities are recorded.

The job uses only GitHub-hosted `ubuntu-24.04`. There is no pull-request trigger,
self-hosted runner input, production route, deployment or merge action. The
ordinary reporting and final-core workflows remain unchanged and still fail
closed on an unsuitable host. A successful manual run is evidence for its exact
source and disposable host, not an automatic release approval.

## Exact privileged setup requiring approval

The reviewed trusted script performs these temporary host operations:

1. Create a root-owned job directory under `/var/tmp` and private writable build,
   cache and temporary paths owned by the existing `nobody:nogroup` account. Fail
   if that account has other processes or supplemental groups; create no account
2. Add `nsdelegate` to the cgroup2 mount only when absent, preserving its complete
   existing filesystem flags and recording them for restoration
3. Enable `hugetlb` in the root cgroup's `cgroup.subtree_control` only if absent.
   Ubuntu24.04's systemd255 does not manage that controller. The transient service
   goes directly under the root slice to avoid altering `system.slice`
4. If Ubuntu's unprivileged-user-namespace restriction is enabled, load a unique
   named AppArmor profile containing `userns,` with `flags=(unconfined)`. It has
   no executable-path attachment and is selected only for this transient service.
   This is a narrow userns exception for the test process tree, not a full
   application confinement profile. Do not change any AppArmor sysctl or stop it
5. Start a transient systemd service with `User=nobody`, `Group=nogroup`,
   `Delegate=yes`, `DelegateSubgroup=manager`, `NoNewPrivileges=yes`,
   `OOMScoreAdjust=0`, `KillMode=control-group` and a 60-minute lifetime. Hide the
   Docker/containerd and host management sockets from that service
6. On exit or cancellation, stop the unit and its descendants, unload the profile,
   undo only the added root hugetlb and mount changes, and remove job files. An
   uncertain stop or populated unit retains all remaining boundaries and fails
   cleanup. An `always()` cleanup step retries incomplete cleanup; the VM is
   disposable. Never roll back a boundary around a possibly live worker

No candidate shell, hook, script, compiler input or executable runs as root. The
entire dependency build and test command runs with an empty inherited environment
under `nobody`, with zero effective host capabilities, no admin/docker membership
and NoNewPrivileges verified before any candidate code executes. No candidate
code can modify the trusted checkout or the root-owned host-state records.

The unprivileged manager enables `memory`, `hugetlb`, `pids` and `cpu` inside its
own delegated empty domain root and supplies that verified path to the unchanged
tests. It stays in the `manager` sibling of per-render leaves. The runtime still
programs each render's at-most 1 GiB charged-memory budget, 3 GiB address ceiling,
64 tasks, one CPU, zero swap, zero HugeTLB and zero nested cgroups. The build
service itself is not constrained to one render's limits.

## Evidence and current limits

The driver requires the two top-level kernel tests, all eleven memory-contract
subtests, the full Phase32 parent and AC01–AC08 with no failure or skip. The stable
catalog test performs 100 actual renders with two simultaneous jobs. Kernel and
test output, source SHAs, the manager identity, effective cgroup path, mount flags
and selected profile are retained in the workflow artifact.
Structured kernel and Phase32 JSON events are extracted from the streamed log
outside the disposable job directory, including when the test step fails. Source
directory traversal and Go/rustup execution are checked under `nobody` before
candidate code runs; inaccessible preinstalled tools fail the job. Native build
outputs and caches stay outside the read-only candidate checkout.

At proposal time YAML parsing, shell/embedded-Python syntax and static review have
run. Synthetic event fixtures verify that a missing required case, skipped case
and package failure reject qualification. A mocked stop failure verifies cleanup
aborts before boundary teardown; it does not execute host provisioning.
The privileged path, AppArmor/NoNewPrivileges interaction, kernel clone3 placement,
native build and actual renderer tests are unexecuted. A hosted failure must be
diagnosed within the approved scope; it does not authorize disabling restrictions,
running tests as root, changing renderer limits or bypassing mandatory tests.

Primary references: [Linux cgroup v2 delegation](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#delegation),
[systemd255 delegation and DelegateSubgroup](https://github.com/systemd/systemd/blob/v255/man/systemd.resource-control.xml),
[systemd255 controller support](https://github.com/systemd/systemd/blob/v255/src/basic/cgroup-util.h),
[Ubuntu24.04 userns restrictions](https://discourse.ubuntu.com/t/ubuntu-24-04-lts-noble-numbat-release-notes/39890),
[Linux6.8 cgroup mount-flag handling](https://github.com/torvalds/linux/blob/v6.8/kernel/cgroup/cgroup.c).
