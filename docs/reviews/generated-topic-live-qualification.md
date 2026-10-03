# Generated-topic live qualification — 2026-10-01

## Result and exact source

The fixed synthetic baseline passed on production candidate
[`a2567f816dce7c52981e2ef9e17af544f7bb6b50`](https://github.com/hurtener/chartworks/pull/68),
tree `59d615a997371e545d722bd04a6d325b43474a85`. The checkout stayed unchanged.
The qualification harness added bounded cost/capture instrumentation and an
explicit synthetic-operator adjudication path; it did not replace the production
quality-review implementation or weaken source/semantic validation.

The run completed in 50.71 seconds. It used real PostgreSQL profiling, two live
semantic enhancement pages, whole-candidate review, explicit operator review and
publication, live retrieval/reranking and two live SQL generations followed by
native validation and execution. No semantic entities or SQL answers were patched
into the generated candidate. No SQL correction call was needed.

## Independent results

The fixture has six orders, five paid and one cancelled, with non-NULL USD amounts
and an enforced `order_id` primary key. Its topic declares all-order gross and
Gregorian UTC calendar dates. Independent arithmetic over all source rows gave:

- All-order gross, including cancellation: **USD 700.00**
- January 2026: **USD 320.00**
- February 2026: **USD 240.00**
- March 2026: **USD 140.00**

Both generated queries matched those independent oracles. The scalar result had
one row and the month result had three. Both source executions succeeded. The
published candidate digest was
`64e0c1959fe9cba24ecf406f147e277d2fab99af3adf44f73a4f65eeec7f4617`.

## Advisory and operator boundary

The model returned one valid `needs_review` finding: sample distinct counts did
not establish table-wide uniqueness of `order_id`. This was a legitimate evidence
distinction, not a failed provider response. The synthetic operator independently
checked the actual primary-key constraint, all six rows, non-NULL amounts, admitted
statuses, declared calendar semantics and the result arithmetic. It then used the
existing exact-digest review/publication operations.

The entire original advisory, including that finding and `needs_review` status,
remained unchanged after publication. No model output was promoted to automatic
approval. This adjudication is valid for this fixed synthetic fixture only; it is
not a general automatic-review rule for customer topics.

The request-local handle schema and canonical-reference decoding completed
without HTTP400. The canonical domain and original advisory checks accepted the
restored references. Earlier failed attempts remain failures: strict optional
schema incompatibility, noncanonical calendar spelling, unqualified review
references and quoted enum literals were separately reproduced and corrected.

## Models, calls and cost

Reported model IDs were `openai/gpt-6-luna` for enhancement/review/SQL,
`pplx-embed-v1-0.6b` for embedding, and `rerank-v4.0-fast` for reranking. Requested
embedding/rerank routes were `perplexity/pplx-embed-v1-0.6b` and
`cohere/rerank-4-fast`; aliases are not a claim of immutable provider versions.

There were ten provider calls: two enhancements, one topic review, three
embeddings, two reranks and two SQL generations. **USD 0.007315153** was settled
for this run, with no new outstanding reservation. This is the cost of this run,
not a reset or reconciliation of earlier attempts or the shared project budget.

## What this does and does not establish

This establishes one real generated-topic → reviewed publication → correct SQL
baseline over a fixed six-row fixture. It does not establish broad live semantic
quality, held-out English/Spanish performance, safe behavior for absent net/refund
meaning, unknown amounts, fiscal/DST semantics, all source engines or migration
parity. Further predeclared cohorts retain every result, clarification and refusal.

Recorded regression suites and exact-head hosted checks are separate evidence.
On predecessor PR #67, both Go 1.26.4/1.27.1 passed 3,444 unit/subtest and 460
acceptance events each, zero failures/missing required/acceptance skips, with only
the opt-in reranker skip. Its tested merge
`54e834fa69fdc94d8c4c70bb182decd0d83a02cc` matches head tree
`1082b84a513e7a22db7b07ed5f5828010d844d14`.
[SQL evidence](https://github.com/hurtener/chartworks/actions/runs/36934479183).
PR #68's local gates passed 618 unit/subtest race events and 66 PostgreSQL/SDK
acceptance events, plus build, vet and planning; its longer hosted checks were
still running when this note was prepared.

Renderer charged-memory kernel qualification remains blocked on an admitted
cgroup hierarchy. Broader capability/engine gaps and final phase-34/25 release
criteria remain open. No merge, deployment or complete-recovery claim follows
from this baseline.

## Held-out English/Spanish cohort and grouping gap

A subsequent predeclared six-question cohort on the same frozen PR #68 production
source passed its explicitly labeled lanes in 65.87 seconds, with 15 real calls,
zero SQL correction calls and USD 0.011820585 settled. The Spanish scalar used
actual grounded concept selection with no metric IDs and returned USD 700.00.
The three grouped lanes supplied explicit reviewed metric/grouping selections:
month gave January/February/March 320/240/140, status gave paid/cancelled 640/60,
and calendar quarter gave Q1 700. Net-after-refunds and hourly New York requests
refused before model or source execution. The Spanish hourly refusal was safe
but still worded in English.

The original raw questions were preserved. Without explicit grouping selection,
three recorded grouped requests failed admission (month/status grain and quarter
expression). These are genuine automatic-grouping gaps, not passing language
results. The new opt-in grouping-intent producer addresses them separately; this
live cohort cannot be cited as qualification of that later producer.

The fresh six-row fixture changed only its recorded profile provenance digest.
The two successful semantic enhancement outputs were replayed unchanged, then a
fresh live review was obtained for the new exact candidate; old advisory digests
were not rewritten. A preserved `needs_review` uniqueness concern was resolved
through the same bounded synthetic operator check of the physical primary key.

PR #68 hosted SQL checks subsequently completed on both Go 1.26.4/1.27.1:
3,450 unit/subtest events across 33 packages and 461 acceptance events per
configuration, zero failures/missing required/acceptance skips, with only the
opt-in reranker skip. Tested merge `d3eba2e27e75996532f1b025ea7b480da0eb4da7`
has the exact production head tree.
[SQL run](https://github.com/hurtener/chartworks/actions/runs/36936579855).
Renderer kernel memory/stability and Phase32 AC03/04/05 remain blocked by the
unavailable admitted charged-memory hierarchy.
