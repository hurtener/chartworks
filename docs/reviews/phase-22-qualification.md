# Phase 22 qualification ledger

This ledger complements the [adversarial review](phase-22-adversarial.md) and
[MCP v1 contract](../contracts/mcp-v1.md). The phase stays `in_progress` until
required exact-source verification closes; neither this document nor an earlier
failed workflow constitutes acceptance.

## Reviewed source corrections

`96c4bbafcb9b069c18980fd0fb6c2b315259e37d` repairs resource-template matching
through the established SDK. Canonical identifiers containing colons are valid
Chartworks identifiers. RFC 6570 reserved expansion preserves them in topic and
dataset templates while the shared resolver continues to reject percent aliases,
extra path segments, queries, fragments, external schemes and traversal.
`TestReservedResourceIdentifiersThroughSDK` verifies the actual network resource
dispatch and exact in-process result parity, not merely the local URI parser.

The same change corrects two stale acceptance assumptions without weakening the
server: the registered capability phase is `01-22-mcp`, and malformed JSON-RPC
requests are sent directly to the server when testing its HTTP rejection. A
client-side rejection is not misreported as a server's HTTP status.

`7467de35921bef715194cf38d6982daeb7d79509` adopts Go's discard log handler and
makes the intentional absent-context negative fixtures explicit. It does not
suppress a lint rule, default a missing context to a valid one, or alter the
asserted rejection. The SDK cannot emit native diagnostics through that logger.

Both source corrections were produced from the recovered exact source, reviewed
locally, transferred with content hashes, and committed on the feature branch.
Their temporary transfer workflows remove themselves. They are authoring steps,
not verification evidence, and must not exist in the delivered tree.

## Local evidence and its limit

The pinned Go 1.26.4 Linux compiler ran the race-enabled MCP, shared API and
configuration tests after the resource fix. All passed. Isolated MCP statement
coverage was 86.9%; the configured package band remains 80%. The MCP race suite
passed again after the lint correction. These are focused package results, not
claims about the full-suite coverage gates or real database acceptance.

## Required hosted evidence

The dedicated read-only `MCP` workflow must pass all six `TestPhase22/ACxx`
children and all six cumulative `TestPhase21/ACxx` children against the actual
PostgreSQL/pgvector and pinned native validation/read fixtures. Its protocol
fuzzing, race-enabled package tests and clean-tree checks are also mandatory.

The ordinary `CI` workflow independently retains the complete build, native
Linux/macOS outputs, reference-container execution, vet, lint, race-enabled
coverage bands, implemented-phase acceptance, executable smoke and cumulative
preflight. No required check may be skipped, replaced by a transfer workflow,
or reported as passing from a previous source revision.

Each MCP workflow publishes the exact Git checkout SHA and a `git archive`
source bundle. For pull requests that SHA is GitHub's merge-test commit; the run
also identifies the feature head. The PR must record the qualifying run and head,
then recheck current head and all required conclusions before merge. A later
source or documentation commit requires its own required checks.

The remaining twelve workstreams, final release qualification, live-provider
quality, production deployment and cutover remain outside phase 22.
