# Accepted generation explanations

Status: AP-06A implementation in PR #62. Runtime results belong to the exact
commit's test evidence, not this document. Extends phase 18 AC01/AC03/AC06 without
changing SQL authorization, analytical receipt versions or frozen execution.

## Custody and lifecycle

The existing `assumptions` and `ambiguities` query fields retain the descriptive
text from the accepted model candidate, after known-value redaction. Fresh Plan,
Run, saved-query projection and terminal replay read that same durable value.
Preflight has no generated candidate and retains its existing routing description.
An explicitly empty model list remains empty; it does not acquire a generic route
assertion during Run. Ordering and duplicates are preserved.

Only the final native/analytically accepted validation candidate contributes notes.
Failed proposals are not merged into the accepted description or sent as repair
instructions. An execution correction replaces notes only after native validation,
exact-binding restrictions, SQL equivalence and any analytical proof have passed.
If correction fails those checks, the original accepted notes remain. Notes on an
accepted second physical attempt describe that attempt even if execution fails.
A refined child retains its own accepted notes and never rewrites its parent.

Descriptive text is model-authored and can be wrong. It is neither selection
state, approval, executable SQL, an analytical certificate nor an instruction.
Storing `ambiguities` does NOT resolve them or implement the separately tracked
AP-08 discriminated ambiguity gate. Idempotent Plan lookup intentionally remains
an opaque query-ID-only response; it must not expose stored descriptive content
without the normal current-resource checks. Run owns its reauthorized result view.

## Privacy and bounds

Before storing or returning notes, reuse the reviewed clarification literal
redactor for known sensitive canonical values, supplied answer spellings and the
active sensitive value's reviewed aliases. SQL parameters have no public sensitivity
contract, so their scalar spellings are conservatively private. Refined children
also redact known parent parameter/resolution values and aliases; a removed or
replaced filter must not disclose its old value through new explanatory prose.
No complete vocabulary or discarded explanation is appended to provider input.

This is known-literal redaction, not general data-loss prevention or semantic
verification of prose. It cannot identify invented/encoded/paraphrased secrets.
Normal SQL inspection permission and source/context authorization remain separate.
Strings remain subject to the existing 32-item and 1,024-byte per-item bounds;
invalid UTF-8 is rejected. If redaction markers expand a note beyond the byte
limit, replace that whole note with a fixed withheld marker, not a byte-truncated
fragment. Stored and response slices are detached from model input and each other.
Ordinary logs do not gain model text, SQL, values or credentials.

## Compatibility and tests

Use existing database columns and transport fields; no migration, scope, remote
role or public operation is added. Historical rows are not backfilled, inferred,
rewritten or regenerated. A read preserves the historical notes already stored.
Retained/frozen operations add no provider calls. The existing correction ceiling,
exact SQL/parameter fences and analytical versions are unchanged.

`TestSQLRecoveryExplanations*` covers detachment, empty lists, privacy, bounded
redaction, validation-candidate choice, execution-correction acceptance/rejection,
legacy reads and concurrent reuse. PostgreSQL/recorded-provider acceptance is
`TestSQLRecoveryExplanationLifecycleAcceptance`,
`TestSQLRecoveryExplanationCorrectionAcceptance` and
`TestSQLRecoveryExplanationPrivacyAcceptance`: EN/ES Plan, service reconstruction,
Run, saved views, zero-work replay, child lineage, validation correction and private
answer replacement. These software fixtures are not live-model comprehension,
parameterized-learning completion or general migration parity.
