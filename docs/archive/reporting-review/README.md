# Reporting foundation — development entry point

Status: proposed design package, 2026-09-04. No runtime implementation or full migration certification is claimed.

Read in this order:

1. [RFC-002](../../RFC-002-Governed-Reporting.md): product boundaries and targeted amendments to RFC-001.
2. [Evidence and parity audit](../research/14-reporting-parity-audit.md): what was observed, what is source debt, and what remains unverified.
3. [Domain and API contracts](contracts.md): revision, parameter, output, security, execution, and scheduling contracts.
4. [Delivery](delivery.md): MCP Apps, iframe authentication, genuine SSR, and host compatibility gates.
5. [Implementation and acceptance](implementation-plan.md): vertical slices, phase crosswalk, tests, migration, and coding-agent handoff.
6. [Planning review](planning-review.md): findings against the accessible planning baseline and unresolved evidence.

This package supplements the existing semantic/NLQ/engineering plans. It is not permission to drop their features. Acceptance of RFC-002 must be accompanied by the explicit reconciliation checklist in the implementation plan, so an implementation agent cannot accidentally follow the old no-rendering scope.

## Evidence discipline

`CODE` means the listed source or selected source range was inspected. `DOC` means behavior was described in a source document. `TEST` means a test definition was inspected, not executed. `INVENTORY` means the file/surface exists but its behavior was not fully audited. `STUB` means the inspected implementation is explicitly incomplete. `NEW` is a proposed product capability.

No label means production validation. This review did not clone or run the source systems, perform a live warehouse comparison, or exercise the deployed MCP host. Those are release gates, not hidden assumptions.

## Repository hygiene

The evidence identifiers intentionally avoid source repository URLs, product/client identifiers, copied code, prompts, schemas, and data. Authorized reviewers resolve the primary and secondary source checkouts privately. Test fixtures committed here must be newly authored synthetic examples.

## Definition of the product

Explore when the question is new. Govern the answer when it becomes reusable. Deliver the same retained result through the API, an agent, or an embedded view without silently changing its meaning or its audience.
