# Pinned native expression wrappers

The warehouse primary read inspector is built from the immutable dependency commit
and exact SHA-256-checked patch in `scripts/native-patches/bruin-source.lock`.
The bounded dependency patch recognizes `Paren` and `AtTimeZone` AST wrappers plus the Boolean `NullSafeEq` binary node. Their
children still participate in the original complete node/function/dependency,
comment, parameter, depth and work-bound checks. This does not strip or rewrite
SQL, ignore arbitrary unsupported nodes, loosen source scopes, or replace native
source EXPLAIN. The independent structural signature and scoped column checks
also continue to validate every child.

This is required runtime behavior: the service's own typed predicate binder emits
parenthesized WHERE/HAVING conjuncts to preserve Boolean meaning. Previously those
valid typed forms stopped at the primary inspector before native planning, even
though the narrower analytical normalizer could compare them. Merely passing
analytical AST fixtures did not establish actual-engine predicate support.

`apply-bruin-read-patch.sh` accepts only the exact base and exact owned diff, is
idempotent, and rejects staged, unrelated tracked/untracked, hidden and sparse
source changes. Builds occur outside the application checkout and module cache.
The builder records the base/patch identity and resulting executable/archive hashes;
the Bruin executable version contains the patch identity. Docker and native CI cache
keys include the lock, patch and build helpers. A deterministic patch-hash CGo
compiler macro changes Go's action/cache identity even when an external archive
occupies the same path; cache hits run that environment step too.

Tests must pass the service-bound predicate through actual native planning and
source execution, and reject wrong parameter values and extra restrictions. The
native grammar regression also nests unknown functions, forbidden columns, missing
bindings and excessive depth inside wrappers, requiring denial before EXPLAIN;
cancellation behavior stays unchanged. Exact-source executed results are recorded
separately from this contract. No change is pushed to the dependency repository.

The `AtTimeZone` extension has a narrower Chartworks structural consumer: only
MySQL physical TIMESTAMP(0..6) fields, literal `+00:00` or `UTC`, and an enclosing
CAST to DATETIME(6) are admitted. Standalone nodes, civil or derived inputs,
arbitrary expressions/zones, hidden columns and reduced precision fail before
EXPLAIN. The expression normalizer only uses this form for exact reviewed UTC
calendar partitions; recognizing a native wrapper never grants a generic timezone
conversion policy. The exact null-safe equality node normalizes to IS NOT DISTINCT FROM semantics for grouped NULL-key alignment; ordinary equality cannot borrow that proof. Required native tests exercise all added nodes' hidden children across the five warehouse grammars.

Parameter inventory is independently corroborated for every warehouse query,
including the native-zero/supplied-zero case. This prevents a positional marker
omitted by the native inventory from acquiring an unbound plan. The native count
is a lower bound, never something the corroboration may erase. Actual MySQL tests
verify missing/excess null-safe-comparison bindings stop before source EXPLAIN and
an exact binding passes native planning and returns the expected row.
