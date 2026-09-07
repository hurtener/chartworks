# Phase 13 current acceptance evidence

Status: implementation in progress; exact-head verification pending.

| Gate | Required evidence | Current record |
|---|---|---|
| Phase acceptance | `TestPhase13/AC01`–`AC06`, no skips | pending |
| Definition lifecycle | real PostgreSQL CAS, immutable publication and signed cross-actor reach | pending |
| Managed execution | pinned real Bruin v0.11.749 plus real PostgreSQL managed workspace | pending |
| Safety negatives | tenant/context/dependency denial before runner; baseline/unowned destination rejection; malformed/multiple/critical/oversized validator output denial | pending |
| Effects | quality failure, staging/activation, timeout/cancel/crash, exact-OID fresh resume and irreconcilable first-create uncertainty | pending |
| Strategy matrix | only actually supported strategies claimed and tested | pending |
| Surface parity | executable manifest, HTTP registry and public Go SDK | pending |
| Repository gates | build, vet, race, coverage, planning and cumulative preflight | pending |

Do not change the phase registry to shipped until current committed source supplies every required result. Local runner evidence must record the exact executable SHA, embedded parser/runtime dependency and private bounded cache/scratch environment; it is not cloud-engine evidence or phase 14 completion.
