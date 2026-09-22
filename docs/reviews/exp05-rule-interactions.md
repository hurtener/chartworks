# EXP-05 rule interaction and semantic-edit evidence

Date: 2026-09-22. Scope: deterministic rule interaction, semantic publication
edits, exact replay and reporting template provenance. Fixtures are neutral and
synthetic.

## Adversarial evidence

| Risk | Resolution and executable evidence |
| --- | --- |
| Several applicable rules overwrite one another or lose the reason each was selected. | Phase 16 shadow evidence retains every sorted selection, required and excluded references, and the attributable violation while the approved metric remains pinned. The public authoring service rejects a required-metric/excluded-dependency conflict before persistence. |
| A template selector changes which rules apply without an exact reviewed pin. | The closed evaluator distinguishes exact match, mismatch and omitted selection. Replay persists the canonical template coordinates. Overlapping template require/exclude rules fail compilation; disjoint selectors remain valid and their selection reasons stay observable. |
| A relationship rule silently follows a removed or meaningfully rebound relationship. | The compiler-level relationship regression removes a reviewed join and receives `missing_reference`. Reusing the old rule definition against a changed relationship digest receives `evidence_mismatch`; an explicit newly reviewed topic/digest pin is required before it can compile. Public lifecycle tests separately prove exact topic/ruleset replacement and rollback; relationship-specific publication lifecycle remains final qualification evidence. |
| Replacing, retiring or rolling back semantics mutates historical evidence. | Topic/rule acceptance retains exact publications, replays old versions after replacement, denies current evaluation while topic/rule heads are incompatible or retired, and restores evaluation only after an explicit CAS rollback. PostgreSQL immutability triggers reject publication and comparison rewrites. |
| Replay relies on historical authority. | Replay rereads the exact topic/ruleset under the current verified envelope. The protected HTTP/SDK route denies a signed token with the correct topic but wrong execution-context reach and proves no comparison row is written. |
| Clearing capture provenance leaves a path to add it back with an ordinary edit. | A real query capture may be edited to remove its template selection. A later edit that re-adds the old coordinates is rejected; only a new source-backed query capture establishes fresh provenance. |
| Migration 040 was only exercised against query rows, not the reporting table it constrains. | The populated PostgreSQL upgrade now includes an immutable pre-040 reporting revision with the legacy singular template. The production migration preserves its bytes, then the production repository reads the singular pin and verifies stored definition/execution digests. Mixed singular/plural representation is rejected by the installed constraint. |

## Boundary

This evidence closes the deterministic software/runtime portion of EXP-05. It
does not claim that a synthetic corpus measures whether end users understand
every conflict explanation, nor does it claim live-model or representative-data
quality. Phase 24 retains held-out and human calibration, with exact topic,
ruleset, context, model and source revisions recorded for those measurements.
