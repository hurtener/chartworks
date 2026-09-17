# CW-01 delivery review

Status: in progress; PR #23 remains a draft. Scope is CLR-01, CLR-02 and
CLAR-AC01 through CLAR-AC10 in the clarification requirements. CLAR-AC11 is
separate representative-user research and has not been performed.

## Scope and invariants

Review the reviewed-policy evaluator, exact typed parsing, mandatory tokenizer
budget, live topic/source pins, sealed route, service-owned parameter binding,
validator-issued read proof, protected query persistence, correction/removal and
current-authority replay. Include actual HTTP, MCP and SDK consumers, draft
preview, retained replay/shadow, explicit import dispositions and forward-only
migration. No identity/issuer service, chart selection or reporting-definition
expansion is included.

The implementation contract is [conditional clarification v1](../contracts/conditional-clarification-v1.md).
Existing publication, source partition and execution gates remain mandatory.

## Concrete remediation

1. At `a6077d4d2b94f1b02bd46eac6afe125dc62714ad`, the new logging helper file
   redeclared `ClarificationAnswer.LogValue` and `ClarificationResolution.LogValue`,
   already present in `clarification_problem.go`. This prevented compilation.
   Commit `aca39a5cb4a86a038c538b3f2b7c6e8c7a3f3687` removes the duplicate methods,
   preserves the existing answer/resolution redaction and documents value redaction.
2. The prior exact-source lint log reported import grouping, missing exported
   documentation, five unused routing helpers, two simplification findings and
   a false positive on Spanish wording. The checked remediation authored in
   `dafbf36627dcb59551e62c1d97107fea9817606b` was applied by the existing scoped
   branch workflow and committed as `243766de9e0cd535e59df2859525558d3f3c3d45`.
   These edits do not relax linter configuration, runtime validation or tests.
3. Native CW-01 AC10 exposed an integration defect: the clarification export
   consumer requested `drafts.Export`, but `ReadPublishedTopic` rejected that
   existing access mode before reaching the signed export guard. Commit
   `904a0705e2f5b1ae16c6d5b8d068ff396220566b` admits the export mode through the
   same `publishedArgs`/`topicArgs` enforcement as other retained reads. It does
   not substitute ordinary read reach for export reach. New direct and HTTP/SDK
   negatives remove the signed export action and export resource separately.
   The same acceptance criterion now independently reports migration, authoring
   and consumer subtests so one failure cannot hide which journeys ran.

## Executed native verification before the export fix

Head: `aca39a5cb4a86a038c538b3f2b7c6e8c7a3f3687`.
Base: `2219fa29093253e0c51b94c4b9de3a4e52f19ee1`.
Actual test merge: `9ca4cf232b94aff0d7bd46b2b46b8800f83cc4ae`.
[Runtime job and complete logs](https://github.com/hurtener/chartworks/actions/runs/35263241273/job/105343922487).

Environment: hosted Ubuntu 24.04, Go 1.26.4, real PostgreSQL 17 with pgvector,
pinned native parser, race instrumentation and recorded provider responses.
`go mod verify` passed. Native build and compile-only consumer checks passed;
they are not counted as executed acceptance.

Executed commands and outcomes:

- `go test -race -count=1 -timeout=20m ./test/acceptance -run '^(TestPhase(16|17|18)|TestCW01)$' -v`:
  CW-01 AC01 through AC09 passed, including bilingual named periods, typed-invalid
  negatives, actual result filtering, correction/removal, source changes and
  whole-group token insufficiency. AC10 failed at the exact-version authoring
  export. Its later HTTP/MCP roundtrip was not reached. All six criteria in each
  of phases 16, 17 and 18 passed, including their nested execution/replay tests.
- `go test -race -count=1 -timeout=20m ./internal/semantics/... ./internal/exec ./internal/nlq... ./internal/api ./internal/topicapi ./internal/mcpserver ./internal/store/postgres ./sdk/...`:
  all test-bearing packages passed. The draft package reports no test files;
  this is not a separate acceptance result.
- `git diff --exit-code` and the no-untracked-source check passed.

The complete runtime job failed because AC10 failed. These results do not prove
the subsequently changed export path or later integrated heads. Fresh exact-source
acceptance must verify those changes before readiness.

## Review and final verification boundary

The scoped editing workflow establishes only that explicitly authored changes
were applied. It is not runtime acceptance. Final verification uses committed
source with read-only workflow permissions and must not prepare or repair source.
Temporary editing/toolchain workflows are removed before delivery.

The bounded source review checks whole-query predicate placement, typed parameter
and source pins, replay binding reconstruction, current signed reach, protected
storage, mandatory context and legacy migration. Remaining concrete CI findings
must be fixed and affected checks rerun before recording a final disposition.
The owned map is updated only for delivered behavior. Recorded provider fixtures
are not live model quality or production qualification.
