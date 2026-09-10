# Reviewed engineering and frozen reporting

These synthetic coordinates assume an operator-configured source, its current
profile and a validated published block already exist. Every request uses a
current Pengui bearer with the operation's signed action and complete resource
reach. The [runtime contract](../docs/contracts/reviewed-engineering-and-frozen-runs.md)
describes configuration and authority requirements.

## Prepare, review and apply

Send this body to `POST /v1/engineering-proposals`:

```json
{
  "id": "sales-projection-review",
  "pipeline": "sales-projection",
  "name": "Reviewed sales projection",
  "connection": "managed-workspace",
  "source": "sales-source",
  "context": "sales-source:v1",
  "goal": "Prepare a managed sales identifier projection for review.",
  "expected_pipeline_version": 0,
  "max_staleness_seconds": 3600,
  "topic": {
    "topic": "sales-topic",
    "profile": "sales-profile-v1",
    "version": "v1",
    "name": "Sales",
    "description": "Profile-backed sales authoring material",
    "expected_revision": 0
  }
}
```

The response is review material, with exact SQL, evidence, alternatives, usage
and optional private topic material. No pipeline or topic is published by this
request. The `topic` member is optional. New topic material is a scaffold;
explicit edits can add reviewed meaning through `PUT /v1/engineering-proposals/{id}`.

An independent reviewer sends `expected_version`, `revision`, `digest`,
`decision:"approve"` and a bounded `reason` to `/review`. Use values from the
current response. Then the applying caller sends the returned `expected_version`,
`revision`, `digest` and `resume:false` to `/apply`. The applying caller also needs
the normal managed-write and private-topic authoring authority. A failed stage
leaves its actual completed effects inspectable through `GET`; an explicit retry
uses the same proposal and current bearer. Topic publication remains a separate
ordinary topic review/publication operation.

The Go SDK exposes `EngineeringGoal`, `EngineeringTopicGoal`,
`ProposeEngineering`, `ReviewEngineeringProposal`, `ApplyEngineeringProposal`,
and `ReadEngineeringProposal`. It obtains a token from its provider for each call.

## Admit and execute a frozen block

Send this body to `POST /v1/blocks/sales-summary/runs`:

```json
{
  "key": "sales-summary-manual-001",
  "reference": {"revision": 0, "draft": false},
  "arguments": [],
  "resolution": {"at": "2026-09-10T12:00:00Z", "timezone": "UTC"},
  "outputs": [],
  "policy": "published",
  "locale": "en",
  "narrative": false,
  "partial_policy": "fail",
  "reuse_max_age_seconds": 0
}
```

Revision zero resolves the current published revision once. Empty `outputs`
selects enabled outputs in definition order; explicit output IDs preserve request
order. For this example the block must have deterministic outputs only. The
response contains the run ID. Send `{"resume":false}` to
`POST /v1/reporting-runs/{id}/execute`, then read its summary with
`GET /v1/reporting-runs/{id}` and values with `/rows?offset=0&limit=100`.
Retained reads perform no source or model work. Reuse the same admission key only
for the same intent; changed intent conflicts instead of silently changing a run.

The corresponding Go methods are `AdmitReportingRun`, `ExecuteReportingRun`,
`ReadReportingRun`, `InspectReportingRun`, and `ReportingRunRows`. Cancellation is
`CancelReportingRun`; disconnecting an HTTP request is not durable cancellation.
