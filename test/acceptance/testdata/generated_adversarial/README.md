# Held-out generated-topic adversaries

This synthetic corpus is deliberately harder than the small generated-topic smoke tests.

- `warehouse.json` and `schema.sql` are synthetic source records and warehouse structure
- `authoring_inputs.json` contains explicit business wording, caller-supplied vocabulary and privacy annotations, with no questions or answer values
- `held_out.json` contains evaluation questions and expected outcome classes; it is loaded only after topic publication
- `oracle.json` contains expected values computed with Python Decimal arithmetic, checked independently by the Go rational-arithmetic oracle over warehouse records

The recorded authoring harness creates four actual source profiles, assembles structural-only PlanProfile outputs, explicitly classifies privacy, requests paginated Enhance proposals, confirms only complete proposed composite relationships, obtains a new whole-topic advisory after confirmation, then explicitly reviews and publishes. Recorded gateway output tests service transport and validation; it does not measure live model reasoning.

`TestGeneratedAdversarialHeldOutRecorded` is a correctness gate and deliberately fails for unmet expectations. A clarification caused by unsupported wording is not a pass for an answerable question. A generic or wrong clarification is not a pass for a genuine ambiguity. Known-amount totals are not definitive totals when money is NULL.

Current evaluation has separate denominators: eight answer cases, five clarification cases, and six rejection cases. The lifecycle itself is assessed separately. This corpus does not silently pre-author count or net measures, nor extend same-dataset vocabulary to unproved joined populations. Missing generated definitions and unsupported consumers remain capability gaps.

The richer generated-net variant also uses `net_business.json`: explicit business
choices for cohort/activity definitions and unknown-count obligations, without
questions, answers, dates or hand-written semantic entities. Every measure, KPI,
filter, period mapping and completeness link still arrives through recorded model
proposals and an explicit synthetic operator review. The source records and all
original held-out questions remain fixed.

Monthly controls use the already documented PostgreSQL instant representation
`date_trunc(unit, instant, zone)`. The original two-argument `AT TIME ZONE` civil
representation remains a separate unsupported-compatibility test; no source dates
or expected month/NULL/zero groups are changed to make it pass. Gross/monthly
correctness requires proof-bound structured amount-completeness evidence as well
as numeric answers. This is separate from transport/result truncation status.
