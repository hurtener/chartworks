# Rich semantics and dependency closure v1

Status: CW-04 implementation contract, 2026-09-22. This contract extends the
existing semantic foundation, topic lifecycle and NLQ context contracts. It does
not replace Pengui authority, immutable publication, the model gateway, or the
validator-issued read plan.

## Versioned semantic meaning

One topic pack now carries reviewed business aliases on columns, measures,
dimensions and KPIs; reviewed physical-column roles; measure/KPI units; typed
temporal calendars and supported grains; bounded governed value mappings; reviewed
mandatory filter concepts; confirmed join evidence; and candidate/rejected
relationship decisions. All references remain stable typed IDs. Display names,
aliases and stored values never become reference coordinates.

Governed values are a privacy-preserving replacement for unrestricted sample rows.
They are accepted only for a column explicitly reviewed as non-sensitive and carry
bounded evidence and policy identifiers. Sensitive or unknown-sensitivity values
cannot enter a compiled topic, facet generation or model context. The enclosing
dataset keeps the exact source, execution context, source revision and private
profile provenance; erased source/profile evidence makes private historical reads
inaccessible through the existing lifecycle fence. Logical erasure does not claim
physical backup or WAL deletion.

Temporal policy is valid only on temporal dimensions and uses the closed
minute/hour/day/week/month/quarter/year vocabulary with a reviewed calendar and
optional timezone. Relationship candidates and rejections remain review evidence;
only a `Join` is executable/retrievable relationship meaning. Both confirmed and
non-confirmed evidence are same-source, same-context and same-revision only.

## Lifecycle and portability

The compiler bounds, canonicalizes, hashes and deep-clones all rich fields.
Draft save, entity mutation, dataset rebind, reviewed publication, exact reads,
facets, SDK aliases and MCP topic description use the same definition. Rebind and
neutral import/export rewrite every reference-bearing filter and relationship
decision together with measures, dimensions, KPIs, joins and canonical keys.
Published definitions remain immutable; legacy definitions with absent optional
rich fields preserve their meaning and do not gain inferred defaults.

The bounded `enhance` gateway role can propose descriptions, aliases, units,
semantic roles and temporal policy for the exact supplied stable columns. It cannot
send or return sample rows, sensitive values, SQL, credentials, permissions,
canonical meaning or executable joins. It may propose bounded KPI formulas and
candidate/rejected relationship evidence over exact supplied identifiers. Governed
values remain explicit reviewed draft authoring data. Generated results remain
private drafts until the existing human review/publication transition.

## Generation context

A selected measure or KPI resolves to one deterministic transitive closure from the
already-authorized retained publication. The closure includes the selected KPI and
its formula, nested KPI inputs, measures, exact columns, related dimension metadata
(aliases, governed values and temporal policy), required filters, units, and
confirmed joins needed by the selected datasets. The sealed route context stores
that typed closure with the metric.

The existing `cl100k_base` assembler treats each selected metric and its complete
closure as mandatory input. It either retains the whole closure under the selected
1500/3000/6500-token tier or returns typed `nlq.ErrInsufficient` before model work.
Optional retrieval evidence cannot substitute for, or independently prune, a metric
dependency. The generation consumer accepts only the assembler-sealed context and
still sends SQL through the existing validator and read executor.

## Evidence and remaining gates

Focused pure tests cover rich-field canonicalization and caller-memory isolation,
sensitive/unknown value rejection, neutral remapping, transitive KPI closure,
English/Spanish tokenizer behavior and typed budget insufficiency. Native-parser,
real PostgreSQL publication/import, race, broad fuzz, coverage, all-phase and release
validation belong to the manual final-gap workflow. Recorded model fixtures do not
prove live provider quality, and this contract does not claim that phase 33's full
guided onboarding workflow or phase 34's cohort migration is complete.
