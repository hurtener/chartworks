# Phases 07/08 and phase 09 prerequisite — review checkpoint

This is an implementation checkpoint, not a completion certificate. The existing
`feat/phase-07-08-vindex-sources` work continues the merged phases 01–06 baseline.
Do not replace the branch or relax its acceptance/coverage gates.

## SQL name-resolution findings requiring regression proof

1. Base-table column alias lists operate on physical column positions, not the
   filtered semantic projection. A table with `(secret, id)` must not allow
   `SELECT exposed FROM analytics.leaky AS t(exposed)` when only `id` is authorized.
   Reject positional alias lists on base tables; retain ordinary table aliases
   and fully resolved CTE/derived-output aliases.
2. `GROUP BY` resolves input names before output aliases. An output alias named
   like an excluded input column must not authorize grouping on that input.
   Without complete physical-name collision proof, use input-only resolution.
3. Output aliases in `ORDER BY` are valid as bare names, not as a blanket escape
   inside function arguments, casts or operator expressions. Preserve the bare
   alias case while rejecting expressions that resolve an excluded input column.

Use actual PostgreSQL fixtures as name-resolution oracles and assert that rejected
SQL obtains no validator-issued plan and makes no credential/native-planning call.
Do not treat a parser pass or EXPLAIN pass as signed authority.

## Completion obligations

Run all six phase 07 criteria, all six phase 08 criteria and the six phase 09
prerequisite criteria. Retain all existing phase 01–06 tests. Run the full uncached
race-enabled suite and exact package coverage inventory: 85% storage/security,
exec/vindex; 80% other internal/SDK packages; 70% CLI. Add genuine boundary,
transaction rollback and oversized-result regressions instead of lowering gates.

The final review must distinguish recorded provider/pure-vector fixtures from live
model execution; distinguish the PostgreSQL source subset from later engine
support; and leave later execution/semantic consumers assigned to their phases.
Remove temporary source-preparation workflows/scripts before opening the completed
PR, retain read-only exact-source CI, and record final head plus actual CI results.

No new runtime pass, completed adversarial review, deployment or merge is asserted
by this checkpoint.
