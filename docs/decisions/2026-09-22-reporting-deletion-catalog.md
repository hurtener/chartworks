### D-081 — Fenced document erasure and descriptive catalog identity

Status: implemented for review, 2026-09-22. Owns REP-02 and REP-03 across
phases 23, 29, 30 and 31; phase 34 still owns imported deletion replay and final
cutover evidence.

Report and dashboard archive remains a reversible discovery lifecycle state.
Deletion is a separate irreversible live-data operation with an exact head CAS,
caller replay key, reason, bounded impact preview, retained tombstone and audit.
The transaction scrubs authored definition payloads and external import mappings,
erases document-owned composition payloads, expires their receipts, and retires only schedules whose
closed reporting target addresses the deleted report. Accepted work is fenced by
the tombstone and expired run state. Schedule history, non-secret dependency
evidence and the deletion tombstone remain. This makes no claim about erasing
database backups, replicas or WAL.

Deleting a dashboard never cascades into reports because dashboard composition
does not establish ownership. Deleting a report preserves dashboard history;
ordinary dashboard projections omit the now-deleted report under the existing
page eligibility filter. Shared blocks, topics and their retained artifacts are
not document-owned and are preserved.

Catalog creator/editor labels are resolved through an optional Pengui-owned
descriptive projection seam. Stable actor identifiers remain protected storage
and audit coordinates and are omitted from public summaries. Missing/deleted
people and service actors receive bounded non-identifying fallbacks. Labels and
relationship rows are never read by an authority check. Schedule identifiers are
projected only when the caller has current schedule-read action and resource reach;
topic and block identifiers come only from an already eligible document revision.
