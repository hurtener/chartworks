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
