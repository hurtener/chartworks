# Canonical registry implementation decision

### D-068 — Reviewed publication approves exact canonical meaning · accepted

Canonical entity meaning is tenant-wide and immutable by `(entity ID, revision)`.
It consists of the stable ID, exact revision, display name and normalized aliases.
Physical key references remain topic-local: neutral import may remap them without
changing global meaning, while the resulting topic definition still receives a new
digest and review.

Private drafts and mapped imports may propose either exact existing meaning, revision
one for a new ID, or exactly the next revision of an existing ID. They do not approve
meaning. The existing explicit reviewed `topics.publish` transition approves any new
revision in the same transaction that stores the immutable topic version and switches
its complete facet generations. A registry-changing publication additionally requires
the existing tenant-write resource under `topics.publish`; exact revision reuse does
not. No action, grant, issuer or alternate publication route is added.

Normalized names and aliases are reserved append-only for their original entity ID.
This reservation begins with this implementation decision; it is not claimed as a
requirement of earlier phases. Permanent reservation prevents an old portable topic,
review record or retained publication from resolving one historical business term to
a different stable ID after later revisions. Same-revision/different-meaning, skipped
revisions and cross-ID normalized-term collisions fail with a typed conflict, including
a final check under the publication transaction lock.

Canonical facets contain only key references from their own source and execution
context. A canonical entity spanning contexts produces separate local facets, while
retained published definitions preserve the exact global revision and all reviewed
topic-local keys. Entity CRUD/move, onboarding and source-rewrite APIs remain later
phase 15 work.
