# Canonical calendar vocabulary at topic authoring

Generated temporal proposals support the Gregorian calendar already understood
by governed calendar grouping. The closed enhancement schema accepts exactly
`gregorian`, `Gregorian` and `GREGORIAN`. The latter two are display spellings of
that same calendar, not alternate business calendars. The producer is instructed
to emit `gregorian` and to leave other calendar meaning unresolved.

`ApplyRichEnhancements` canonicalizes these recognized proposal spellings to
`gregorian` on a detached temporal policy before compiling or saving the new
draft. It preserves grains and timezone, never mutates the caller or prior model,
and rejects unknown names rather than assigning them Gregorian meaning.
Recognized spellings produce identical canonical semantic digests for the same
proposal and revision. The provider schema and normalizer share one vocabulary.

This is an authoring boundary correction. Downstream SQL/calendar proof remains
exact; it does not become case-insensitive or infer unsupported calendars. No
retained or manually authored definition is silently rewritten. No new grains,
timezones, fiscal calendars, source reach or publication authority are added.
Normal explicit review/publication and current source/profile fences still apply.

## Evidence and limits

A bounded synthetic live enhancement returned `Gregorian`. Offline replay of
that exact semantic output (with only ephemeral synthetic source IDs rebound)
showed it passed the former schema/domain validation and survived durable draft
reload, while the actual month-bucket consumer rejected it. Changing only the
calendar spelling to `gregorian` made the same consumer accept. The returned
supplemental COUNT of the fact identifier was valid and is not removed.

Regression tests cover the closed producer schema, immutable canonicalization,
unknown-name rejection, durable rollback after an invalid proposal, saved/reloaded
canonical metadata, explicit publication and independent gross/month query-result
oracles through real PostgreSQL and the SDK. Recorded cases use the actual 4,096
output reserve and unchanged admission caps. Hosted required-name checks retain
each recognized-spelling variant. Executed counts are reported with the source
checkpoint; this test inventory does not itself establish a pass.

This correction and offline replay are not a new paid live end-to-end run or a
complete topic-quality qualification. Broader topic/calendar and release gates
remain separate.

## Current catalog key evidence

Topic enhancement and quality review receive complete non-null unique keys from
current authorized source discovery, projected onto the selected candidate column
IDs. Source, context and revision must match the profiled source. Every key
component must be selected, safe and non-null; hidden, nullable or unsafe
components omit the entire key. Missing keys and sampled distinct counts never
create uniqueness evidence. Ambiguous relations, stale catalogs and malformed
keys fail closed. The evidence is included in the authoring context digest.

This metadata describes physical uniqueness only. It does not establish business
meaning, authorize joins, widen source reach or waive the advisory review gate.
Normal explicit publication and independently validated read plans remain
required. `TestGeneratedTopicCatalogKeyEvidenceRecorded` checks the real catalog
projection in both enhancement pages and review, then exercises the unchanged
publication and numeric/month result oracles. Recorded provider responses do not
establish live model quality.

## Lossless aggregate transport and unchanged budgets

Profile aggregate columns use `profile-column-table-v1` in both enhancement and
independent whole-candidate review. Each table names the seven original fields
once, in order: `column`, `observed`, `nulls`, `sample_distinct`, `distinct_exact`,
`families`, `disclosure`. Every row supplies all seven values. Counts retain exact
integers, `false` remains distinct from absent evidence, and `null` families mean
no family disclosure. No column, count, family, disclosure or business definition
is pruned. Explicit source/context/revision/profile origins, policy and sampling,
relation names, all complete catalog keys and `current_authorized_catalog`
provenance stay in the same packet on every page and in the final review.

The context digest binds the exact encoding version, ordered field header, every
row and all surrounding evidence. Unknown or missing versions, shifted headers,
ragged rows and invalid types are rejected by the inverse inspector. The existing
128 KiB authoring material bound, 65,536 operation reservation, 4,096 recorded
output reserve and strict output schemas are unchanged. Oversized complete
material still fails closed; compaction does not promise every topic will fit.

Only internal model-input representation changes. Published topic packs, saved
reviews, generation checkpoints and their retained digests are not rewritten.
Historical reviews still validate against their exact candidate and coverage;
newly generated pages and reviews bind their newly encoded context. There is no
migration or advisory/publication waiver. Recorded provider fixtures establish
transport and lifecycle behavior, not live model-quality qualification.

`TestAuthoringColumnTableLosslessRoundTrip` reconstructs every aggregate through
an independent field-name oracle, including integers beyond binary64 precision.
`TestAuthoringColumnTableRejectsAmbiguousInterpretation` and
`TestAuthoringColumnTablePreservesBoundProvenance` cover closed decoding and digest
sensitivity. `TestGeneratedAdversarialCatalogEnvelopeRecorded` checks complete
current catalog/profile evidence on both eleven-page authoring passes and both
independent reviews, then retains explicit publication and held-out result
oracles. Exact-source execution results are reported separately.
