# Renderer memory isolation continuation

### D-093 — Separate charged-memory and virtual-address renderer limits

Date: 2026-10-01. Status: accepted code/contract change; deployment qualification pending.
Supersedes only the memory-accounting portion of D-083 and D-091. Host provisioning
and changes to host security settings require separate operator authorization.

## Problem and decision

The supported Go runtime reserves virtual address ranges exceeding the former
1 GiB RLIMIT_AS even with small resident use. Raising that ceiling alone would
weaken the intended memory boundary. Each render therefore requires a fresh
cgroup v2 leaf with kernel charged-memory enforcement, plus a separate fixed
3 GiB soft-and-hard address-space ceiling in the trusted worker before input is
read. The default charged-memory budget remains 1 GiB; configurations may tighten
it to 32 MiB but cannot exceed 1 GiB. Go's advisory heap target is half that budget.

The parent and worker use worker protocol v2 and the fixed
`RENDER_MEMORY_CONTRACT=charged-memory-v1` marker in the clean launch environment.
A new worker refuses an old supervisor before applying the new limit. This marker
is compatibility binding, not a substitute for the kernel admission checks.

The kernel memory controller accounts charged anonymous and file-backed memory
across the process and its descendants. This is not a promise about exact RSS:
Linux permits temporary memory.max overshoot and previously shared/cached pages
may retain charges outside this cgroup. The service admits at most MaxConcurrent such jobs, so the aggregate job budget
is MaxConcurrent times the configured per-job charged limit, in addition to the
manager and its bounded input/output buffers. This is not a whole-service 1 GiB
limit. Swap is disabled for the leaf. HugeTLB
limits are set to zero independently; memory.max alone does not cover that pool.

## Mandatory deployment admission

A trusted Linux deployment supplies `rendering.cgroup_root`, an absolute,
symlink-free, manager-owned cgroup v2 domain with memory, hugetlb, pids and cpu controllers
already enabled for children. The actual mount must have `nsdelegate`. The
manager must not inherit OOM immunity (`oom_score_adj=-1000`) or share its host
identity with unrelated untrusted processes. Provisioning/remounting the hierarchy
is outside Chartworks. Missing enforcement fails before executing a worker.

For each job Chartworks creates a fresh empty leaf, writes and reads back:

- memory.max: the configured budget rounded down to a whole kernel page, at most
  1073741824 bytes; exact readback verifies the effective bound never increases
- memory.swap.max: 0; memory.oom.group: 1
- pids.max: 64 tasks; cpu.max: 100000 per 100000 microseconds (one aggregate CPU)
- cgroup.max.descendants: 0; cgroup.max.depth: 0
- every available hugetlb size's max and reservation max: 0

The leaf must expose cgroup.kill. Atomic clone3 placement uses UseCgroupFD and
CLONE_INTO_CGROUP together with CLONE_NEWCGROUP and the existing user, mount,
network, IPC, UTS and PID namespaces. There is no ordinary-clone or in-process
fallback. Existing empty chroot, fixed executable, clean environment, sealed input,
time/concurrency and byte bounds remain. Descriptors use CLOEXEC and are never
passed through ExtraFiles.

nsdelegate protects the worker's namespace-root controller files even after it
mounts cgroup2 itself. Zero descendants prevents moving to a new child namespace
and reopening an ancestor's controls. A writable descriptor opened in the manager
namespace must never leak into the worker. The manager retains the cgroup directory and preopened kill/event controller
descriptors through launch and cleanup so descriptor numbers cannot be reused early.
Cleanup and accounting do not reopen worker-owned paths after launch; revoking file
or directory modes cannot disable the already admitted descriptors. Repeated reads
reset offsets and concurrent control access is serialized. Kernel qualification
includes mode revocation under a manager without DAC-override privileges.

Cancellation kills the entire group; WaitDelay bounds descendant-held output
pipes. Every terminal path kills remaining descendants, waits for populated=0,
and removes the leaf. Cleanup failure suppresses output and disables further
admission through that supervisor. The two-second polling bound does not claim a
hard bound on kernel filesystem syscalls. Setup cleanup failure is also explicit.

## Qualification

Pure admission/configuration tests cannot prove kernel enforcement. The mandatory
`renderer_integration` gate requires an already provisioned hierarchy and fails,
rather than skips, when it is unavailable. It tests self-remount/control mutation,
nested namespace attempts, leaked descriptors, anonymous/file-backed/aggregate
child memory, independent address reservation, cancellation and retained pipes.
At least 100 representative actual renders and simultaneous jobs must also pass.
The ordinary Phase32 gate still requires actual content and isolation behavior.

The current development executor has no cgroup mount, and the previously tested
hosted environment rejected namespace launch. This change does not qualify either
deployment or authorize changing their settings. The optional repository variable `CHARTWORKS_RENDER_RUNNER` selects an already
provisioned trusted Linux runner label; `CHARTWORKS_RENDER_CGROUP_ROOT` identifies
its existing delegated hierarchy. Neither variable provisions or changes host
security. Without the runner label the hosted default remains fail-closed.
A supported operator-provisioned runner must pass the kernel and full Phase32 gates before release readiness.

Primary references: [Linux cgroup v2](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html),
[clone3](https://man7.org/linux/man-pages/man2/clone.2.html),
[Go process creation](https://go.dev/src/syscall/exec_linux.go).
