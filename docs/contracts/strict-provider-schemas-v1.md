# Strict provider schema transport v1

Status: implementation with recorded-provider and local validation tests. This is
not a live-provider quality or capacity claim. The original domain schemas remain
authoritative for every structured role, including enhancement, whole-topic review,
SQL generation/repair/clarification, profile/pipeline authoring, feedback, narrative,
concept selection and visual ranking. Embedding/rerank contracts are unchanged.

## Admission and lossless optional fields

`gateway.NewStrictSchema` prepares a detached, deterministic provider schema before
budget reservation or dispatch. `Schema.Document` and its compiled domain validator
are unchanged. The adapter sends `strict: true`, closed objects and every declared
property in `required`. Optional domain fields do not become required business
choices:

- An optional field that cannot contain null gains a nullable transport branch.
  Its introduced null means absence and is removed before domain validation
- An optional field whose schema permits null uses an outer nullable wrapper.
  Outer null means absence; `{"value":null}` preserves an explicitly present null;
  `{"value":...}` preserves a present non-null value
- Required fields never gain an absence marker. Existing required nullable values
  remain present. Missing provider-required properties fail transport validation
- Nested object/array normalization follows the exact admitted branch. The complete
  wire shape is checked before normalization; the complete original domain schema
  is checked afterward, before any service sees the generated JSON

Untyped scalar enums and constants receive equivalent explicit types/enum forms.
`oneOf` becomes `anyOf` only when projected branches are provably disjoint, using
non-overlapping JSON types, nonnumeric finite enums, or object-only shared types
with disjoint required discriminator enums. A shared null/string type cannot be
distinguished by an object discriminator. Ambiguous unions, constraint siblings
on unions, references, open objects and unrecognized keywords fail closed. Numeric
enum comparisons do not assume different decimal spellings mean different values.
Numbers retain their exact JSON representation through actual SDK dispatch.

## Explicit local assertions and bounds

The one explicitly local-only assertion is `uniqueItems`. It is omitted from the
provider subset, recorded as `local_assertions: "uniqueItems"`, and always enforced
by the unchanged original validator after absence normalization. This also rejects
items that become equal only after normalization. No other unknown assertion is
silently dropped, and providers are never trusted to replace domain validation.

Identical repeated scalar or closed-object schemas may be factored into generated
local `$defs` and `$ref` only when doing so shrinks the SDK-style envelope. IDs are
deterministic,
references are acyclic and fully inlined, and no standalone array/union or
caller-supplied reference is factored. Unused definitions are omitted; parent
identities are computed before replacing child schemas with references. The full
final schema is compiled independently; normalization remains on
the unfactored projected structure. Domain descriptions and scalar constraints are
preserved exactly.

Schemas are bounded by the existing 64 KiB document limit, the admitted provider
property/enum limits and a conservative ten-level projected nesting limit (including
introduced wrappers). Individual string enums with more than 250 entries must fit
15,000 characters. Supported type constraints remain in the wire schema; unknown
formats and other unsupported constructs fail before any model attempt. This is a
conservative admitted subset, not an implementation of every JSON Schema feature
accepted by a particular provider. Fine-tuned model subset compatibility and future
provider changes still require separate qualification.

## Effective envelope and routing

Both local `GenerationEnvelope` preparation and actual dispatch use the same
strict projection. `prompt-envelope-v2` / `utf8-json-wire-byte-bound-v2` count the
SDK-style indented effective JSON envelope rather than assuming a fixed reserve
covers arbitrarily large pretty-printed schemas. The protocol reserve still covers
provider-specific framing differences; reported provider tokens/cost remain
separate nullable observations.

The redacted receipt includes the projection policy `strict-optional-v1`, original
and wire schema SHA-256 digests and any explicit local assertion. Even two schemas
with identical provider projections have distinct identities when their original
domain constraints differ. Optional expansion, escaping, output reserve, runtime
model/system configuration and capability routing participate in admission.
Original domain validation cannot be bypassed by a narrower transport digest.

OpenRouter chat calls send `provider.require_parameters: true` to exclude endpoints
that do not support the supplied structured-output parameter. The pinned SDK needs
its request-local extra-parameter passthrough flag enabled for this fixed parameter;
the isolated context disables inherited raw-body/large-payload substitution and
extra-parameter passthrough, and no request-supplied extra
parameters enter this path. Direct OpenAI calls do not receive OpenRouter settings.
The pinned chat codec also silently raises output caps below 16 tokens. Those caps
are rejected locally before reservation; no caller allowance is silently increased.
This minimum applies only to the actual pinned chat codec, not embedding/reranking
or independent fake engines. There is no host, TLS, credential, model or inference
transport substitution.

## Evidence and boundaries

Unit regressions use the actual enhancement/review/SQL, feedback, concept,
engineering and narrative schemas. They cover required values, optional omission,
explicit nullable values, nested/composite normalization, duplicate rejection,
unsupported schemas, immutability/concurrency and provider limits. SDK TLS fixtures
inspect OpenAI/OpenRouter strict wire schemas, exact large numeric constants,
capability parameters, envelope byte/digest identity, no-spend admission and
cross-role rejection before consumers. Existing recorded chat fixtures encode only
the transport markers; their domain examples and independent result oracles remain
unchanged. These are synthetic/local recorded tests, never paid live evidence.

The initial live enhancement request returned HTTP 400 before output. Source
inspection establishes schema incompatibility; the exact upstream error body was
not retained, so this change does not assert a fully proven causal diagnosis or a
successful live retest.

Primary provider references checked for this change:

- [OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs)
- [OpenRouter Structured Outputs](https://openrouter.ai/docs/guides/features/structured-outputs)

The SQL context recovery CI lane explicitly selects every strict-schema SDK-wire
regression and verifies each required terminal test name. The recorded generated
cohort uses the reviewed nonsecret live caps: 4,096 output tokens for enhancement,
review, generation and repair; 1,024 for clarification; 65,536 request bytes and
unchanged 65,536-unit authoring operation budgets. This remains recorded evidence.
