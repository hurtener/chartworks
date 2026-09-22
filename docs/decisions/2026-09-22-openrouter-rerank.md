# OpenRouter rerank route — 2026-09-22

### D-090 — OpenRouter rerank uses a bounded Bifrost custom provider

D-062 remains correct about the native OpenRouter rerank method in Bifrost core
v1.6.2: it returns unsupported. Its native-Cohere-only routing choice is
superseded for the reference configuration. The selected OpenRouter endpoint
accepts the Cohere rerank request/response shape and Bearer authentication, so
Chartworks configures the pinned Bifrost custom-provider mechanism with the
Cohere codec, a rerank-only allowed operation, and fixed `/api/v1/rerank` path.
This is still the one production model gateway; Chartworks adds no direct model
HTTP client. The trusted HTTPS origin, environment credential, exact
`cohere/rerank-4-fast` model, time/candidate/input/output/attempt bounds and
strict complete-index validation are required. Native Cohere stays available.
The request model remains the configured slug even when OpenRouter reports its
canonical `rerank-v4.0-fast`; both names are retained separately in the receipt.

Recorded-wire tests verify the actual SDK path, header, body, response mapping,
fail-closed malformed results and unknown billing semantics. A separate opt-in
paid smoke uses the same Chartworks gateway route with synthetic authorized
candidates; it is not an end-to-end release acceptance claim.
