# Native requirements for manual report writes

### D-099 — Proposed definitions and save baselines determine write dependencies

Accepted integration scope, 2026-10-05; source under local qualification. Extends
D-098's metadata boundary to native manual create/save intent. Pengui remains the
policy owner and verifies its own target-allocation receipt before creation.

The new HTTP BFF operation requires `reporting.discover` and exact report write;
create additionally requires tenant write. It structurally validates the same
manual definition and resolves native block revisions, publication pointers,
private digest/actor custody and dependency indexes. A save includes requirements
of the immutable baseline it will read, even for widgets removed by the proposal.
Root write may inspect a private baseline without root preview, matching native
edit semantics. Private block requirements still include independent preview.

Only identifiers, revision/digest coordinates and closed metadata-read actions
are returned. No definition, SQL, schema, result, identity or credential is
returned; no source/model work occurs. The proposal is edit intent, never an
authorization list. This does not weaken ordinary reads or writes, construct an
envelope, commit a draft, or make discovery a reusable validation proof.

Pengui checks every requirement centrally and mints bounded operation authority.
Native Create/Save repeat full checks, including CAS and current dependencies.
See [the contract](../contracts/report-dependencies-v1.md). Full real-service
Builder/Consumer and restricted MCP acceptance remain outstanding.
