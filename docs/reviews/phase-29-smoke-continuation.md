# Phase 29 — compiled smoke continuation

The parent source is `138524199b1ff724f56022118a030b2587f4bad9`.
Its full race-coverage step passed, but PR CI run `34708280749` subsequently
failed in the compiled foundation smoke. The smoke still classified
`/v1/reports` as an unimplemented, absent route even though the actual Phase 29
HTTP registry mounts it. The same source's push CI run `34708278786` failed
earlier in cumulative acceptance; its detailed failing criterion still needs
inspection. A passing coverage step is not a complete CI result.

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

This source change has not been represented as verified before its checks run.
The existing read-only CI must pass script tests, the compiled smoke and all
remaining cumulative acceptance, fuzz and preflight steps on the new commit
before PR #20 is marked ready. Prior successful coverage and dedicated Phase 29
runs do not waive those remaining checks. No merge or deployment is authorized.
