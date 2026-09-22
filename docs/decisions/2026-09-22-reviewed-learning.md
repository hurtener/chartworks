# Reviewed learning continuation

### D-077 — Version-pinned reviewed examples and evidence-based feedback · accepted

Learned SQL examples are versioned protected metadata bound to exact topic,
source/context, rule, template and locale origins. New generation selects only
currently applicable active examples, persists selection and exclusion evidence,
and optionally reranks authorized question text through the single Bifrost
gateway. The existing generation-lane precedence remains unchanged.

Feedback uses immutable positive and negative outcomes, deterministic idempotency,
atomic aggregation, a bounded beta-posterior score and explicit uncertainty.
Fixed increments are retired. Feedback cannot activate an example: activation
requires a separate reviewed transition and threshold. Stale or invalid examples
cannot silently affect generation; frozen queries never reselect. Neutral import
revalidates exact origin and SQL and always returns to candidate state.
