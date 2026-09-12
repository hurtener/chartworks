# Phase 29 — compiled smoke continuation

The parent source is `138524199b1ff724f56022118a030b2587f4bad9`.
Full race coverage and cumulative acceptance passed on both PR CI run
`34708280749` and push CI run `34708278786`. Both then failed in the compiled
foundation smoke with `unimplemented route advertised: /v1/reports`.
The checker still classified this implemented route as absent. Bounded windows
of the original job logs confirmed the identical failure; an earlier status
inspection did not establish a separate cumulative-acceptance defect.
A passing coverage step alone is not a complete CI result.

The smoke now requires all 26 implemented report, dashboard, composition-run
and composition-retention operations. None is allowed to disappear from the
OpenAPI registry, return 404 instead of 401, accept spoofed cookie/header
identity, omit bearer/action metadata, or return a data-bearing authentication
error. The legacy MCP and local issuer routes remain absent; MCP feature-gate
expectations are unchanged. No production handler, signed authority check,
coverage threshold, timeout or acceptance criterion is weakened.

`scripts/test_smoke_reporting.py` independently enumerates the 26 operations and
checks both serve/MCP inventories, each missing operation, invalid 200/404
responses, spoofed identity, all required security metadata fields and safe
error bodies. These are checker regressions, not substitutes for the real
compiled-binary smoke, Phase 29 acceptance or full CI.

At commit `7efb091c186471299ae8ee3bfc7f251626c6efe4`, CI's planning job ran all
59 script tests successfully, including the six new smoke-security tests with
per-operation negative cases. Planning coherence and drift also passed.
The local execution service was unavailable during this continuation; no new
local Go pass is claimed. The small log-reading workflow ran on an isolated
diagnostic branch with read-only permissions, not in the implementation PR.

The existing read-only CI must pass the compiled smoke and all remaining
cumulative acceptance, fuzz and preflight steps on the final commit before
PR #20 is marked ready. Prior successful coverage and dedicated Phase 29 runs
do not waive those checks. Final results belong in the PR evidence record;
this document does not pre-declare unobserved results. No merge or deployment
is authorized.
