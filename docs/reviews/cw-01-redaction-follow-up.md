# CW-01 sensitive-spelling follow-up

## Finding and correction

The reviewed text resolver accepts trimmed, case-insensitive, Unicode-whitespace-normalized aliases. The question/instruction redactor previously matched the submitted spelling literally. A padded `  north coast  ` answer could resolve to a protected canonical value while `north coast` in the question escaped redaction.

The redactor now normalizes known sensitive spellings before deduplication and longest-first ordering. Every word remains regexp-quoted; only the service-owned Unicode whitespace separator is flexible. Ordinary punctuation and non-whitespace separators do not become aliases. Nonsensitive values and protected answer/resolution evidence remain unchanged. This is not arbitrary PII discovery or a new executable authoring matcher.

Five whitespace-equivalence regressions failed against the unmodified implementation. They pass after the fix. Additional regressions cover longest normalized phrase selection, regexp metacharacters, all 25 Unicode whitespace code points recognized by the normalizer, four non-whitespace negatives, and detached evidence. A bounded fuzz target exercises independently spaced answer/question pairs. The existing read-only clarification workflow runs that campaign alongside the decimal campaign.

The existing `TestCW01/AC09` consumer test now submits ASCII/Unicode-padded sensitive input with its unpadded alias in the question. Its original provider-body, actual execution, diagnostics, canonical persistence and budget assertions remain; it also checks that the retained question is redacted. No acceptance assertion, coverage threshold, authority boundary or toolchain pin was relaxed.

## Executed local evidence before push

The source was recovered from runtime run `35395823939`, artifact `10567488478`, at test merge `54e67139d5af65db7fc0ee54fbe5ac7e4a880813`. All 1,094 tracked blob hashes were verified against the archived Git tree. The change was developed on that tree with repository-pinned Go 1.26.4, offline modules and the race detector.

- The five new whitespace cases failed before changing the redactor.
- `go test -race -count=3 ./internal/semantics` passed with the final unit regressions and ordinary fuzz seeds.
- `go test -race ./internal/semantics -run '^$' -fuzz '^FuzzClarificationRedactionWhitespace$' -fuzztime=256x -parallel=2` passed with 258 actual executions.
- Formatting and `git diff --check` passed.

These results do not claim native-parser/PostgreSQL acceptance ran locally: those dependencies are absent in this environment. Final-head hosted execution, including the strengthened AC09, is reported on the PR. A timed-out earlier fuzz warm-up is not counted as a passing campaign.

## Historical CI discrepancy

Earlier reported Phase 05 persona-autofill and Phase 08 stored-rule-bind failure strings do not occur in this verified snapshot. Its Phase 05 test is gateway acceptance; its Phase 08 file is source acceptance. No production correction for those unreconciled logs is claimed. The read-only runtime workflow now retains committed source, its tree and checksums before tests; fresh final-head results must be evaluated separately from older logs.

## Canonical replay marker collision

A second adversarial regression found that a sensitive canonical value such as `red` is a substring of the public `[redacted answer]` marker. Redacting a persisted question a second time could mutate that marker, changing the question digest during `ReplayClarifications`. Four unit cases (`red`, `answer`, `[red`, `[`) failed before the correction.

The service-owned marker is now a reserved literal alternative. Leftmost-longest matching protects an existing marker while still consuming a longer known sensitive phrase that starts with it; a dedicated suffix-negative regression prevents marker reservation from exposing the remainder. No caller can supply executable matchers. The fuzz target also checks repeated-redaction stability.

The existing AC09 includes a separate reviewed-policy fixture whose canonical customer is `red` and whose submitted alias is in the question. A real execution must replay the protected evidence unchanged and return the expected empty result (the source has no such customer), not reject a changed digest or drop the predicate. Its hosted result must be inspected before being claimed as passing.

After this correction, `go test -race -count=3 ./internal/semantics` passed and the extended repeated-redaction fuzz campaign passed with 2,079 actual executions (`-fuzztime=2048x -parallel=2`). These are local semantic results, not substituted native/PostgreSQL acceptance.
