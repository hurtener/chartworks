# Renderer protected job domain

### D-095 — Keep resource controls outside the worker cgroup namespace

Date: 2026-10-03. Status: accepted implementation change; exact-source deployment
qualification pending. Supersedes only D-093's single-leaf topology and its claim
that namespace delegation prevents rewriting every admitted control. All budgets,
atomic placement, process isolation, cleanup and host-authorization requirements
remain.

## Problem and decision

The disposable-host run at source `0629745` proved the startup correction and all
100 stable renders, but its hostile controller test still failed. Linux marks
`memory.oom.group` as namespace-delegatable, so placing that control on the worker's
namespace root allows the worker to disable whole-job OOM killing. Deleting that
negative assertion would conceal an incorrect containment assumption.

Each job now owns two fresh cgroups under the existing delegated root:

- The outer job owns the unchanged charged-memory budget, zero swap/HugeTLB,
  `memory.oom.group=1`, 64-task bound and one-CPU bound. Its subtree-control list
  remains empty. It permits exactly one descendant at depth one.
- The inner `worker` leaf is the atomic clone3 target and cgroup namespace root.
  It has zero allowed descendants/depth and no available or enabled resource
  controllers. Thus its processes use the outer resource domain, and no inner
  memory controller can narrow the OOM domain or disable its group-kill policy.

The worker cannot see the outer cgroup through its cgroup mount, empty chroot,
process namespace or inherited descriptors. Resource-control writes and attempts
to enable controllers or create descendants must fail. No outer descriptor is
passed to the child; all manager descriptors and the clone target use CLOEXEC.
The separate empty sibling kill-write probe is still destroyed before either job
cgroup is created, avoiding the earlier kill-sequence startup defect.

The manager retains the outer kill and event descriptors plus both directory
descriptors. Cancellation kills the entire outer tree. Cleanup waits for outer
`populated=0`, restores traversal on the owned empty outer directory through its
retained descriptor, removes the inner leaf, then removes the outer job. Removal
failure remains typed and disables that supervisor. Application-owned transient
cgroup changes do not provision or relax host security settings.

## Required proof

The kernel gate checks outer limit readback before launch and after hostile
tampering; absent inner controls and failed controller activation; no ancestor,
sibling or descriptor escape; and unchanged independent-job isolation. Anonymous,
file-backed and descendant memory probes require an outer `oom_group_kill` receipt
and an empty outer job before manager cleanup, without a deadline kill masking
survivors. Permission-revocation coverage changes inner modes from the worker and
outer modes from the manager, preserving retained-descriptor and two-level cleanup
proof under the unprivileged identity.

Child-process probes explicitly reuse stdin because the sealed chroot deliberately
contains no `/dev/null`. Build/race checks can verify source behavior locally, but
the full kernel and Phase32 gates remain mandatory on the exact candidate source.

Primary references: [Linux memory controller source](https://code.googlesource.com/linux/torvalds/linux/+/a0300e8cf0ed685851232973ccadbf62681f7f6d/mm/memcontrol.c),
[cgroup v2 controller delegation and OOM semantics](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html),
[Go subprocess stdin handling](https://go.dev/src/os/exec/exec.go).
