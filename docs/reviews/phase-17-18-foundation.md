# Phase 17/18 bounded context foundation evidence

Status: bounded foundation verified at `3b008ca` on 2026-09-08. Phases 17 and
18 remain `in_progress`; this slice does not claim the cumulative
`TestPhase17/AC01`–`AC06` or `TestPhase18/AC01`–`AC06` acceptance parents,
release readiness, or a hosted/cloud qualification.

This document records the earlier context-only foundation. The subsequent
durable Phase 18 core and its Phase 16 invalidation-ledger consumer are recorded
separately in [phase 18 NLQ runtime evidence](phase-18-nlq-runtime.md); the
historical boundary and checks below remain unchanged.

## Implemented boundary

The phase 17 slice now has one deterministic `ContextAssembler` using the
pinned `cl100k_base` tokenizer and 1500/3000/6500 token tiers. It preserves
evaluated mandatory constraints and pinned metrics or returns typed
insufficiency, caps examples at seven, records at most seven bounded omission
descriptors, and accepts detached English and Spanish inputs. The omission
audit contains counts and lane usage only; omitted source text is not retained
on the model-facing JSON wire. An unexported seal, canonical prompt rebuild,
and tokenizer revalidation reject denied constraints, clarify/no-route misuse,
stale budgets, mismatched token counts, and caller mutation before a consumer
uses the context.

The first phase 18 consumer applies the deterministic
`edit_base > hints > examples > default` precedence. It serializes the chosen
instructions into the final prompt and counts that exact payload with the same
assembler tokenizer, so selected instructions cannot bypass the tier budget.
Clarify and no-route strategies stop before generation-context construction.
This package does not retrieve or rerank candidates, call Bifrost, generate or
correct SQL, validate executable plans, execute sources, or persist examples,
feedback, refinement, or learning state.

## Verification evidence

The implementation commit adds meaningful malformed-input, seal/revalidation,
wire-omission, final-payload-budget, tokenizer-error, precedence, cancellation,
and caller-detachment regressions. Independent `cl100k_base` vectors include
`revenue by month` → 4 tokens, `ingresos por mes` → 5, `Hello, world!` → 4,
and `Use the approved period.` → 5.

| Check | Result |
| --- | --- |
| Focused race suite | `go test -race -count=1 ./internal/nlq` passed |
| Focused coverage | `go test -race -count=1 -coverprofile=/tmp/chartworks-phase17-nlq-fixed.out ./internal/nlq` passed at 86.0% statements; the configured `internal/nlq` band is 80% |
| Static check | `go vet ./internal/nlq` passed |
| Planning and mirrors | `make planning-check` and `make check-mirror` passed |
| Diff hygiene | `git diff --check` passed before commit |
| Full phase acceptance | Not claimed; all named phase 17/18 criteria remain required |
| Cumulative Linux/native/source acceptance | Not claimed; root must verify the exact committed head in the reference environment |

The remaining work includes signed reach and real facet retrieval, remote
reranking and calibrated outcomes, confirmed same-source multi-topic joins,
gateway-backed constrained generation, executable-plan validation and source
execution, correction/refinement/template/example lifecycle, feedback and
learning, HTTP/SDK consumers, and the full named phase acceptance fixtures.
No model call, live cloud provider, Docker restart, migration, or speculative
query/execution seam was added for this bounded foundation.
