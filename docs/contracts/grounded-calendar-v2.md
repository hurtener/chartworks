# Grounded calendar phrase producer v2

Optional `interpretation_policy: grounded-calendar-v2` enables this bounded
producer through existing Route, Plan, saved-selection and Refine transports.
`deterministic-grounded-calendar-v2` is part of the interpretation digest. An
absent policy and `continuation-v1` retain their old grammar and evidence. Refine
preserves v2 even after removing every filter; explicit policy changes remain
ordinary new requests. Retained policy stripping fails reconstruction. No new
migration, SQL authority, model role, native proof or review bypass is introduced.

The producer supports English local/calendar-year `for`, `of`, `in`, `during`
forms and Spanish `en`, `durante`, `para`, `del` reviewed calendar-year forms.
Bounded month/quarter/year grouping forms include English zone-qualified booking
or calendar grains and Spanish `por mes de registro en ...` equivalents. This
is a finite grammar, not general language comprehension. A grouping phrase needs
an independently interpreted period or explicit logical grouping. Repeated years
still require the existing reviewed per-metric period mapping and every connector
must be valid; different, malformed and unanchored years are never deduplicated.

Explicit supported zone spellings are `New York` (English), `Nueva York`
(Spanish), normalized `America/New_York`, and `UTC`. Each is an assertion against
the exact current reviewed timezone. No alias equivalence such as `US/Eastern`,
fixed offset, DST abbreviation, fiscal calendar or unknown named place overrides
or silently disappears into that policy. Contradictory assertions clarify. Native
calendar boundaries still enforce unique local midnight and original temporal
types. Zone spelling does not resolve multiple otherwise eligible dimensions.
Every named-zone assertion needs at least one actual resolved temporal target.
All fresh and retained periods and selected temporal grouping keys are checked
after merging; explicit scalar or categorical-only intent cannot leave a zone
unbound. Retained selection merging preserves the v2 parser digest.
Current explicit metric facts and reviewed per-metric period mappings keep their
existing ownership semantics. Explicit logical grouping selects its own grain.

Inference uses the shared opaque private-answer surface. Calendar-only quote
masking preserves token coordinates while public governed-value parsing keeps
quoted non-sensitive values. Only exact zone token occurrences consumed by
calendar syntax are excluded from independent value inference; a separate city
mention is preserved. Mixed private/public calendar phrases and changed polarity
clarify before provider work. Calendar words inside quoted text and inside reviewed
metric labels do not become grouping intent. No private raw text or new origin
proof is retained by this producer.

## Qualification boundary

The frozen offline paired cohort remains unchanged. D uses B's same reviewed
semantic publication and frozen source/questions/oracles, opts into only v2, and
runs before training without examples. A/B/C remain under their original policy;
their failed answer expectations remain failed in every report. The repair
requalification test may pass by asserting exact predecessor outcomes and D's
answer contracts separately, never by relabeling A/B/C refusals as expected answers.
The original red test source and exact baseline report remain immutable at
`c15e64b`. The integration `TestPairedCohortRecorded` explicitly asserts the
unchanged failed evaluation (`evaluation.ErrGate`, quality gate false), including
all eight failed expected-answer contracts per arm. Mutation controls reject
wrong classifications, false quality-pass flags, changed expected answers,
missing cells, and corrupted D correctness. No gate is skipped or refusal
reclassified as the desired answer. Calibration is
unknown. Any measured D gain is a producer/policy repair, not learning quality,
live-model quality, or approval of the original business-gross live refusal.

Implementation tests and native run evidence are reported separately; this
contract alone is not qualification evidence.

## Combined grouping policies

When v2 produces a reviewed grouping and model grouping is also requested, the
proved grouping takes the existing override path with zero additional grouping-role
model calls. The effective grouping-model policy is cleared and no grouping-model
proof is claimed. Ordinary embedding, reranking and SQL generation keep their
normal roles. If v2 has no proved grouping, the combined policies return a
content-free typed clarification before grouping-model dispatch. Model-only legacy
requests retain their existing behavior.
This bounded contract prevents a later model selection from overwriting checked
calendar grain or zone meaning. Hosted coverage requires the actual CI run;
workflow selection and required-root inventory alone establish no hosted result.

Local exact-source results and limitations are in the [offline qualification](../reviews/grounded-calendar-v2-offline.md).
